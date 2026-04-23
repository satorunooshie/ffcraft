package gofeatureflag

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/satorunooshie/ffcraft/internal/ast"
)

type CompileOptions struct {
	AllowMissingEnvironment bool
}

type flagFile struct {
	Variations       map[string]any     `yaml:"variations"`
	DefaultRule      ruleResult         `yaml:"defaultRule"`
	Targeting        []targetRule       `yaml:"targeting,omitempty"`
	BucketingKey     string             `yaml:"bucketingKey,omitempty"`
	Experimentation  *experimentation   `yaml:"experimentation,omitempty"`
	ScheduledRollout []scheduledStepOut `yaml:"scheduledRollout,omitempty"`
}

type ruleResult struct {
	Variation          string              `yaml:"variation,omitempty"`
	Percentage         map[string]float64  `yaml:"percentage,omitempty"`
	ProgressiveRollout *progressiveRollout `yaml:"progressiveRollout,omitempty"`
}

type targetRule struct {
	Query              string              `yaml:"query"`
	Variation          string              `yaml:"variation,omitempty"`
	Percentage         map[string]float64  `yaml:"percentage,omitempty"`
	ProgressiveRollout *progressiveRollout `yaml:"progressiveRollout,omitempty"`
}

type experimentation struct {
	Start string `yaml:"start"`
	End   string `yaml:"end"`
}

type progressiveRollout struct {
	Initial progressiveState `yaml:"initial"`
	End     progressiveState `yaml:"end"`
}

type progressiveState struct {
	Variation  string   `yaml:"variation"`
	Date       string   `yaml:"date"`
	Percentage *float64 `yaml:"percentage,omitempty"`
}

type scheduledStepOut struct {
	Date            string           `yaml:"date"`
	Targeting       []targetRule     `yaml:"targeting,omitempty"`
	DefaultRule     *ruleResult      `yaml:"defaultRule,omitempty"`
	Experimentation *experimentation `yaml:"experimentation,omitempty"`
}

func CompileYAML(doc *ast.Document, environment string) ([]byte, error) {
	output, _, err := CompileYAMLWithOptions(doc, environment, CompileOptions{})
	return output, err
}

func CompileYAMLWithOptions(doc *ast.Document, environment string, opts CompileOptions) ([]byte, []string, error) {
	flags := make(map[string]flagFile, len(doc.Flags))
	warnings := make([]string, 0)

	for _, src := range doc.Flags {
		env, ok := src.Environments[environment]
		if !ok {
			if opts.AllowMissingEnvironment {
				warnings = append(warnings, fmt.Sprintf("warning: skipping flag %q because environment %q is not defined", src.Key, environment))
				continue
			}
			return nil, nil, fmt.Errorf("flag %q: environment %q not found", src.Key, environment)
		}

		compiled := flagFile{
			Variations: compileVariants(src.Variants),
		}

		if env.StaticVariant != "" {
			compiled.DefaultRule = ruleResult{Variation: env.StaticVariant}
		} else {
			defaultRule, defaultBucketingKey, err := compileDefaultRule(src.DefaultVariant, env.DefaultAction)
			if err != nil {
				return nil, nil, fmt.Errorf("flag %q: %w", src.Key, err)
			}
			compiled.DefaultRule = defaultRule

			targeting, ruleBucketingKey, err := compileRules(env.Rules)
			if err != nil {
				return nil, nil, fmt.Errorf("flag %q: %w", src.Key, err)
			}
			compiled.Targeting = targeting
			if compiled.BucketingKey, err = mergeBucketingKeys(defaultBucketingKey, ruleBucketingKey); err != nil {
				return nil, nil, fmt.Errorf("flag %q: %w", src.Key, err)
			}
			compiled.Experimentation = compileExperimentation(env.Experimentation)
			scheduled, err := compileScheduledRollout(src.DefaultVariant, env.ScheduledRollouts)
			if err != nil {
				return nil, nil, fmt.Errorf("flag %q: %w", src.Key, err)
			}
			compiled.ScheduledRollout = scheduled
		}

		flags[src.Key] = compiled
	}

	return marshalDocument(flags), warnings, nil
}

func compileDefaultRule(defaultVariant string, action ast.Action) (ruleResult, string, error) {
	switch value := action.(type) {
	case *ast.ServeAction:
		return ruleResult{Variation: value.Variant}, "", nil
	case *ast.DistributeAction:
		return ruleResult{Percentage: sortedAllocations(value.Allocations)}, value.Stickiness, nil
	case *ast.ProgressiveRolloutAction:
		return ruleResult{
			ProgressiveRollout: &progressiveRollout{
				Initial: progressiveState{
					Variation:  defaultVariant,
					Date:       value.Start,
					Percentage: new(float64(0)),
				},
				End: progressiveState{
					Variation:  value.Variant,
					Date:       value.End,
					Percentage: new(float64(100)),
				},
			},
		}, value.Stickiness, nil
	case nil:
		return ruleResult{}, "", fmt.Errorf("default_action is required")
	default:
		return ruleResult{}, "", fmt.Errorf("unsupported default_action type %T", action)
	}
}

func compileRules(rules []*ast.Rule) ([]targetRule, string, error) {
	out := make([]targetRule, 0, len(rules))
	bucketingKey := ""

	for _, rule := range rules {
		query, err := compileCondition(rule.Condition)
		if err != nil {
			return nil, "", err
		}

		compiled := targetRule{Query: query}
		switch action := rule.Action.(type) {
		case *ast.ServeAction:
			compiled.Variation = action.Variant
		case *ast.DistributeAction:
			if bucketingKey == "" {
				bucketingKey = action.Stickiness
			} else if bucketingKey != action.Stickiness {
				return nil, "", fmt.Errorf("multiple distribute stickiness values are not supported by GO Feature Flag: %q and %q", bucketingKey, action.Stickiness)
			}
			compiled.Percentage = sortedAllocations(action.Allocations)
		case *ast.ProgressiveRolloutAction:
			compiled.ProgressiveRollout = &progressiveRollout{
				Initial: progressiveState{
					Variation:  "",
					Date:       action.Start,
					Percentage: new(float64(0)),
				},
				End: progressiveState{
					Variation:  action.Variant,
					Date:       action.End,
					Percentage: new(float64(100)),
				},
			}
			bucketingKey = action.Stickiness
		default:
			return nil, "", fmt.Errorf("unsupported action type %T", rule.Action)
		}

		out = append(out, compiled)
	}

	return out, bucketingKey, nil
}

func compileExperimentation(exp *ast.Experimentation) *experimentation {
	if exp == nil {
		return nil
	}
	return &experimentation{Start: exp.Start, End: exp.End}
}

func compileScheduledRollout(defaultVariant string, steps []*ast.ScheduledStep) ([]scheduledStepOut, error) {
	out := make([]scheduledStepOut, 0, len(steps))
	for _, step := range steps {
		if step.Disabled {
			continue
		}
		compiled := scheduledStepOut{
			Date:            step.Date,
			Experimentation: compileExperimentation(step.Experimentation),
		}
		if len(step.Rules) > 0 {
			targeting, _, err := compileRules(step.Rules)
			if err != nil {
				return nil, err
			}
			compiled.Targeting = targeting
		}
		if step.DefaultAction != nil {
			defaultRule, _, err := compileDefaultRule(defaultVariant, step.DefaultAction)
			if err != nil {
				return nil, err
			}
			compiled.DefaultRule = &defaultRule
		}
		out = append(out, compiled)
	}
	return out, nil
}

func mergeBucketingKeys(left, right string) (string, error) {
	if left == "" {
		return right, nil
	}
	if right == "" {
		return left, nil
	}
	if left != right {
		return "", fmt.Errorf("multiple distribute stickiness values are not supported by GO Feature Flag: %q and %q", left, right)
	}
	return left, nil
}

func compileCondition(cond ast.Condition) (string, error) {
	switch value := cond.(type) {
	case *ast.LiteralBool:
		if value.Value {
			return "true", nil
		}
		return "false", nil
	case *ast.Eq:
		return binaryOp("eq", value.Left, value.Right)
	case *ast.Ne:
		return binaryOp("ne", value.Left, value.Right)
	case *ast.Gt:
		return binaryOp("gt", value.Left, value.Right)
	case *ast.Gte:
		return binaryOp("ge", value.Left, value.Right)
	case *ast.Lt:
		return binaryOp("lt", value.Left, value.Right)
	case *ast.Lte:
		return binaryOp("le", value.Left, value.Right)
	case *ast.In:
		return binaryOp("in", value.Target, value.Candidate)
	case *ast.Contains:
		return binaryOp("co", value.Container, value.Value)
	case *ast.StartsWith:
		target, err := compileValue(value.Target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s sw %s", target, strconv.Quote(value.Prefix)), nil
	case *ast.EndsWith:
		target, err := compileValue(value.Target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s ew %s", target, strconv.Quote(value.Suffix)), nil
	case *ast.Matches:
		return "", fmt.Errorf("matches is not compiled because GO Feature Flag rules do not document a regex operator")
	case *ast.SemverGt:
		return semverOp("gt", value.Left, value.Right)
	case *ast.SemverGte:
		return semverOp("ge", value.Left, value.Right)
	case *ast.SemverLt:
		return semverOp("lt", value.Left, value.Right)
	case *ast.SemverLte:
		return semverOp("le", value.Left, value.Right)
	case *ast.AllOf:
		return variadicCondition("and", value.Conditions)
	case *ast.AnyOf:
		return variadicCondition("or", value.Conditions)
	case *ast.OneOf:
		return compileOneOf(value.Conditions)
	case *ast.Not:
		child, err := compileCondition(value.Condition)
		if err != nil {
			return "", err
		}
		return "not (" + child + ")", nil
	default:
		return "", fmt.Errorf("unsupported condition type %T", cond)
	}
}

func binaryOp(op string, left, right ast.Value) (string, error) {
	l, err := compileValue(left)
	if err != nil {
		return "", err
	}
	r, err := compileValue(right)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s %s", l, op, r), nil
}

func semverOp(op string, left ast.Value, right string) (string, error) {
	l, err := compileValue(left)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s %s", l, op, right), nil
}

func variadicCondition(op string, conditions []ast.Condition) (string, error) {
	parts := make([]string, 0, len(conditions))
	for _, cond := range conditions {
		value, err := compileCondition(cond)
		if err != nil {
			return "", err
		}
		parts = append(parts, "("+value+")")
	}
	return strings.Join(parts, " "+strings.ToUpper(op)+" "), nil
}

func compileOneOf(conditions []ast.Condition) (string, error) {
	if len(conditions) == 1 {
		return compileCondition(conditions[0])
	}

	clauses := make([]ast.Condition, 0, len(conditions))
	for i, current := range conditions {
		andTerms := make([]ast.Condition, 0, len(conditions))
		andTerms = append(andTerms, current)
		for j, other := range conditions {
			if i == j {
				continue
			}
			andTerms = append(andTerms, &ast.Not{Condition: other})
		}
		clauses = append(clauses, &ast.AllOf{Conditions: andTerms})
	}
	return variadicCondition("or", clauses)
}

func compileValue(value ast.Value) (string, error) {
	switch v := value.(type) {
	case *ast.Var:
		return v.Path, nil
	case *ast.Scalar:
		switch v.Kind {
		case ast.ScalarKindString:
			return strconv.Quote(v.String), nil
		case ast.ScalarKindBool:
			if v.Bool {
				return "true", nil
			}
			return "false", nil
		case ast.ScalarKindInt:
			return strconv.FormatInt(v.Int, 10), nil
		case ast.ScalarKindDouble:
			return strconv.FormatFloat(v.Double, 'f', -1, 64), nil
		case ast.ScalarKindNull:
			return "null", nil
		default:
			return "", fmt.Errorf("unsupported scalar kind %v", v.Kind)
		}
	case *ast.List:
		items := make([]string, 0, len(v.Values))
		for _, item := range v.Values {
			compiled, err := compileValue(item)
			if err != nil {
				return "", err
			}
			items = append(items, compiled)
		}
		return "[" + strings.Join(items, ", ") + "]", nil
	default:
		return "", fmt.Errorf("unsupported value type %T", value)
	}
}

func compileVariants(variants map[string]ast.VariantValue) map[string]any {
	out := make(map[string]any, len(variants))
	for key, value := range variants {
		out[key] = variantValueToAny(value)
	}
	return out
}

func variantValueToAny(value ast.VariantValue) any {
	switch value.Kind {
	case ast.VariantValueKindBool:
		return value.Bool
	case ast.VariantValueKindString:
		return value.String
	case ast.VariantValueKindInt:
		return value.Int
	case ast.VariantValueKindDouble:
		return value.Double
	case ast.VariantValueKindObject:
		return value.Object
	case ast.VariantValueKindList:
		items := make([]any, 0, len(value.List))
		for _, item := range value.List {
			items = append(items, variantValueToAny(item))
		}
		return items
	case ast.VariantValueKindNull:
		return nil
	default:
		return nil
	}
}

func sortedAllocations(in map[string]float64) map[string]float64 {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make(map[string]float64, len(in))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}

func marshalDocument(flags map[string]flagFile) []byte {
	keys := make([]string, 0, len(flags))
	for key := range flags {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	root := &yaml.Node{Kind: yaml.MappingNode}
	for _, key := range keys {
		value, err := yaml.Marshal(flags[key])
		if err != nil {
			panic(err)
		}

		var node yaml.Node
		if err := yaml.Unmarshal(value, &node); err != nil {
			panic(err)
		}

		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			node.Content[0],
		)
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		panic(err)
	}
	return out
}
