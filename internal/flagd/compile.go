package flagd

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/satorunooshie/ffcraft/internal/ast"
)

const schemaURL = "https://flagd.dev/schema/v0/flags.json"

type document struct {
	Schema string           `json:"$schema"`
	Flags  map[string]*flag `json:"flags"`
}

type flag struct {
	State          string         `json:"state"`
	Variants       map[string]any `json:"variants"`
	DefaultVariant string         `json:"defaultVariant"`
	Targeting      any            `json:"targeting,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type CompileOptions struct {
	AllowMissingEnvironment bool
}

func CompileJSON(doc *ast.Document, environment string) ([]byte, error) {
	output, _, err := CompileJSONWithOptions(doc, environment, CompileOptions{})
	return output, err
}

func CompileJSONWithOptions(doc *ast.Document, environment string, opts CompileOptions) ([]byte, []string, error) {
	out := document{
		Schema: schemaURL,
		Flags:  map[string]*flag{},
	}
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

		compiled := &flag{
			State:          "ENABLED",
			Variants:       compileVariants(src.Variants),
			Metadata:       compileMetadata(src.Metadata),
			DefaultVariant: src.DefaultVariant,
		}

		if env.StaticVariant != "" {
			compiled.DefaultVariant = env.StaticVariant
		} else if defaultVariant, err := defaultServeVariant(env.DefaultAction); err == nil {
			compiled.DefaultVariant = defaultVariant
		} else if env.DefaultAction == nil {
			return nil, nil, fmt.Errorf("flag %q: default_action is required", src.Key)
		}

		if env.Experimentation != nil {
			return nil, nil, fmt.Errorf("flag %q: flagd compiler does not support experimentation", src.Key)
		}
		if len(env.Rules) > 0 || len(env.ScheduledRollouts) > 0 {
			targeting, err := compileEnvironment(env, src.DefaultVariant)
			if err != nil {
				return nil, nil, fmt.Errorf("flag %q: %w", src.Key, err)
			}
			compiled.Targeting = targeting
		} else if _, ok := env.DefaultAction.(*ast.DistributeAction); ok {
			targeting, err := compileAction(env.DefaultAction)
			if err != nil {
				return nil, nil, fmt.Errorf("flag %q: %w", src.Key, err)
			}
			compiled.Targeting = targeting
		}

		out.Flags[src.Key] = compiled
	}

	output, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return output, warnings, nil
}

func defaultServeVariant(action ast.Action) (string, error) {
	switch action := action.(type) {
	case *ast.ServeAction:
		return action.Variant, nil
	default:
		return "", fmt.Errorf("flagd compiler requires default_action.serve")
	}
}

func compileEnvironment(env *ast.Environment, defaultVariant string) (any, error) {
	baseAction := env.DefaultAction
	progressiveSteps, err := expandProgressiveRollout(defaultVariant, env.Rules, env.DefaultAction)
	if err != nil {
		return nil, err
	}
	if len(progressiveSteps) > 0 {
		baseAction = &ast.ServeAction{Variant: defaultVariant}
	}

	base, err := compileRuleChain(env.Rules, baseAction)
	if err != nil {
		return nil, err
	}
	allSteps := make([]*ast.ScheduledStep, 0, len(progressiveSteps)+len(env.ScheduledRollouts))
	allSteps = append(allSteps, progressiveSteps...)
	allSteps = append(allSteps, env.ScheduledRollouts...)
	if len(allSteps) == 0 {
		return base, nil
	}

	active := base
	for i := len(allSteps) - 1; i >= 0; i-- {
		step := allSteps[i]
		if step.Disabled {
			continue
		}
		timestamp, err := parseRFC3339Unix(step.Date)
		if err != nil {
			return nil, fmt.Errorf("scheduled_rollouts[%d].date: %w", i, err)
		}
		snapshot, err := compileRuleChain(step.Rules, step.DefaultAction)
		if err != nil {
			return nil, fmt.Errorf("scheduled_rollouts[%d]: %w", i, err)
		}
		condition := any(map[string]any{
			">=": []any{
				map[string]any{"var": "$flagd.timestamp"},
				timestamp,
			},
		})
		if step.Experimentation != nil {
			condition, err = compileExperimentCondition(condition, step.Experimentation)
			if err != nil {
				return nil, fmt.Errorf("scheduled_rollouts[%d].experimentation: %w", i, err)
			}
		}
		active = map[string]any{
			"if": []any{
				condition,
				snapshot,
				active,
			},
		}
	}
	return active, nil
}

func expandProgressiveRollout(defaultVariant string, rules []*ast.Rule, action ast.Action) ([]*ast.ScheduledStep, error) {
	rollout, ok := action.(*ast.ProgressiveRolloutAction)
	if !ok {
		return nil, nil
	}
	steps, err := progressiveSteps(defaultVariant, rules, rollout)
	if err != nil {
		return nil, fmt.Errorf("expand progressive_rollout: %w", err)
	}
	return steps, nil
}

func compileRuleChain(rules []*ast.Rule, defaultAction ast.Action) (any, error) {
	if defaultAction == nil {
		return nil, fmt.Errorf("default_action is required")
	}
	defaultResult, err := compileAction(defaultAction)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return defaultResult, nil
	}

	next := defaultResult
	for i := len(rules) - 1; i >= 0; i-- {
		cond, err := compileCondition(rules[i].Condition)
		if err != nil {
			return nil, err
		}
		action, err := compileAction(rules[i].Action)
		if err != nil {
			return nil, err
		}
		next = map[string]any{
			"if": []any{cond, action, next},
		}
	}
	return next, nil
}

func progressiveSteps(defaultVariant string, rules []*ast.Rule, rollout *ast.ProgressiveRolloutAction) ([]*ast.ScheduledStep, error) {
	start, err := time.Parse(time.RFC3339Nano, rollout.Start)
	if err != nil {
		return nil, fmt.Errorf("start: must be RFC3339 timestamp")
	}
	end, err := time.Parse(time.RFC3339Nano, rollout.End)
	if err != nil {
		return nil, fmt.Errorf("end: must be RFC3339 timestamp")
	}
	if !start.Before(end) {
		return nil, fmt.Errorf("start must be before end")
	}

	steps := make([]*ast.ScheduledStep, 0, rollout.Steps)
	totalSteps := int(rollout.Steps)
	for i := 1; i <= totalSteps; i++ {
		progress := 1.0
		if totalSteps > 1 {
			progress = float64(i-1) / float64(totalSteps-1)
		}
		at := start.Add(time.Duration(float64(end.Sub(start)) * progress)).UTC()
		steps = append(steps, &ast.ScheduledStep{
			Date:          at.Format(time.RFC3339),
			Rules:         cloneRules(rules),
			DefaultAction: progressiveStepAction(defaultVariant, rollout, i),
		})
	}
	return steps, nil
}

func progressiveStepAction(defaultVariant string, rollout *ast.ProgressiveRolloutAction, step int) ast.Action {
	if step >= int(rollout.Steps) {
		return &ast.ServeAction{Variant: rollout.Variant}
	}

	percentage := float64(step) * 100 / float64(rollout.Steps)
	return &ast.DistributeAction{
		Stickiness: rollout.Stickiness,
		Allocations: map[string]float64{
			rollout.Variant: percentage,
			defaultVariant:  100 - percentage,
		},
	}
}

func cloneRules(in []*ast.Rule) []*ast.Rule {
	out := make([]*ast.Rule, 0, len(in))
	for _, rule := range in {
		out = append(out, &ast.Rule{
			Condition: rule.Condition,
			Action:    rule.Action,
		})
	}
	return out
}

func parseRFC3339Unix(value string) (int64, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0, fmt.Errorf("must be RFC3339 timestamp")
	}
	return t.Unix(), nil
}

func compileExperimentCondition(base any, exp *ast.Experimentation) (any, error) {
	start, err := parseRFC3339Unix(exp.Start)
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	end, err := parseRFC3339Unix(exp.End)
	if err != nil {
		return nil, fmt.Errorf("end: %w", err)
	}
	return map[string]any{
		"and": []any{
			base,
			map[string]any{
				">=": []any{
					map[string]any{"var": "$flagd.timestamp"},
					start,
				},
			},
			map[string]any{
				"<": []any{
					map[string]any{"var": "$flagd.timestamp"},
					end,
				},
			},
		},
	}, nil
}

func compileAction(action ast.Action) (any, error) {
	switch value := action.(type) {
	case *ast.ServeAction:
		return value.Variant, nil
	case *ast.DistributeAction:
		keys := make([]string, 0, len(value.Allocations))
		for key := range value.Allocations {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		fractional := make([]any, 0, len(keys)+1)
		fractional = append(fractional, map[string]any{
			"cat": []any{
				map[string]any{"var": "$flagd.flagKey"},
				map[string]any{"var": value.Stickiness},
			},
		})
		for _, key := range keys {
			fractional = append(fractional, []any{key, value.Allocations[key]})
		}
		return map[string]any{"fractional": fractional}, nil
	case *ast.ProgressiveRolloutAction:
		return nil, fmt.Errorf("progressive_rollout is not supported by the flagd compiler")
	default:
		return nil, fmt.Errorf("unsupported action type %T", action)
	}
}

func compileCondition(cond ast.Condition) (any, error) {
	switch value := cond.(type) {
	case *ast.LiteralBool:
		return value.Value, nil
	case *ast.Eq:
		return binaryOp("==", value.Left, value.Right)
	case *ast.Ne:
		return binaryOp("!=", value.Left, value.Right)
	case *ast.Gt:
		return binaryOp(">", value.Left, value.Right)
	case *ast.Gte:
		return binaryOp(">=", value.Left, value.Right)
	case *ast.Lt:
		return binaryOp("<", value.Left, value.Right)
	case *ast.Lte:
		return binaryOp("<=", value.Left, value.Right)
	case *ast.In:
		return binaryOp("in", value.Target, value.Candidate)
	case *ast.Contains:
		return binaryOp("in", value.Value, value.Container)
	case *ast.StartsWith:
		target, err := compileValue(value.Target)
		if err != nil {
			return nil, err
		}
		return map[string]any{"starts_with": []any{target, value.Prefix}}, nil
	case *ast.EndsWith:
		target, err := compileValue(value.Target)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ends_with": []any{target, value.Suffix}}, nil
	case *ast.Matches:
		return nil, fmt.Errorf("matches is not compiled because current flagd docs do not document a regex operator")
	case *ast.SemverGt:
		return semverOp(">", value.Left, value.Right)
	case *ast.SemverGte:
		return semverOp(">=", value.Left, value.Right)
	case *ast.SemverLt:
		return semverOp("<", value.Left, value.Right)
	case *ast.SemverLte:
		return semverOp("<=", value.Left, value.Right)
	case *ast.AllOf:
		return variadicCondition("and", value.Conditions)
	case *ast.AnyOf:
		return variadicCondition("or", value.Conditions)
	case *ast.OneOf:
		return compileOneOf(value.Conditions)
	case *ast.Not:
		child, err := compileCondition(value.Condition)
		if err != nil {
			return nil, err
		}
		return map[string]any{"!": []any{child}}, nil
	default:
		return nil, fmt.Errorf("unsupported condition type %T", cond)
	}
}

func binaryOp(op string, left, right ast.Value) (any, error) {
	l, err := compileValue(left)
	if err != nil {
		return nil, err
	}
	r, err := compileValue(right)
	if err != nil {
		return nil, err
	}
	return map[string]any{op: []any{l, r}}, nil
}

func semverOp(op string, left ast.Value, right string) (any, error) {
	l, err := compileValue(left)
	if err != nil {
		return nil, err
	}
	return map[string]any{"sem_ver": []any{l, op, right}}, nil
}

func variadicCondition(op string, conditions []ast.Condition) (any, error) {
	values := make([]any, 0, len(conditions))
	for _, cond := range conditions {
		value, err := compileCondition(cond)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return map[string]any{op: values}, nil
}

func compileOneOf(conditions []ast.Condition) (any, error) {
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

func compileValue(value ast.Value) (any, error) {
	switch v := value.(type) {
	case *ast.Var:
		return map[string]any{"var": v.Path}, nil
	case *ast.Scalar:
		switch v.Kind {
		case ast.ScalarKindString:
			return v.String, nil
		case ast.ScalarKindBool:
			return v.Bool, nil
		case ast.ScalarKindInt:
			return v.Int, nil
		case ast.ScalarKindDouble:
			return v.Double, nil
		case ast.ScalarKindNull:
			return nil, nil
		default:
			return nil, fmt.Errorf("unsupported scalar kind %v", v.Kind)
		}
	case *ast.List:
		values := make([]any, 0, len(v.Values))
		for _, item := range v.Values {
			value, err := compileValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported value type %T", value)
	}
}

func compileVariants(variants map[string]ast.VariantValue) map[string]any {
	out := make(map[string]any, len(variants))
	for key, value := range variants {
		out[key] = compileVariantValue(value)
	}
	return out
}

func compileVariantValue(value ast.VariantValue) any {
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
			items = append(items, compileVariantValue(item))
		}
		return items
	case ast.VariantValueKindNull:
		return nil
	default:
		return nil
	}
}

func compileMetadata(metadata *ast.Metadata) map[string]any {
	if metadata == nil {
		return nil
	}
	out := map[string]any{}
	if metadata.Owner != "" {
		out["owner"] = metadata.Owner
	}
	if metadata.Description != "" {
		out["description"] = metadata.Description
	}
	if metadata.Expiry != "" {
		out["expiry"] = metadata.Expiry
	}
	if len(metadata.Tags) > 0 {
		out["tags"] = append([]string(nil), metadata.Tags...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
