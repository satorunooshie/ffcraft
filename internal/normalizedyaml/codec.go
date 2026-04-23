package normalizedyaml

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/satorunooshie/ffcraft/internal/ast"
)

const version = "normalized/v1"

type documentFile struct {
	Version string     `yaml:"version"`
	Flags   []flagFile `yaml:"flags"`
}

type flagFile struct {
	Key            string                      `yaml:"key"`
	Variants       map[string]variantValueYAML `yaml:"variants"`
	DefaultVariant string                      `yaml:"default_variant"`
	Environments   map[string]environmentFile  `yaml:"environments"`
	Metadata       *metadataFile               `yaml:"metadata,omitempty"`
}

type metadataFile struct {
	Owner       string   `yaml:"owner,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Expiry      string   `yaml:"expiry,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
}

type environmentFile struct {
	StaticVariant     string               `yaml:"static_variant,omitempty"`
	DefaultAction     *actionYAML          `yaml:"default_action,omitempty"`
	Experimentation   *experimentationFile `yaml:"experimentation,omitempty"`
	ScheduledRollouts []scheduledStepFile  `yaml:"scheduled_rollouts,omitempty"`
	Rules             []ruleFile           `yaml:"rules,omitempty"`
}

type ruleFile struct {
	If     conditionYAML `yaml:"if"`
	Action actionYAML    `yaml:"action"`
}

type actionYAML struct {
	Value ast.Action
}

type distributionActionFile struct {
	Stickiness  string             `yaml:"stickiness"`
	Allocations map[string]float64 `yaml:"allocations"`
}

type progressiveRolloutFile struct {
	Variant    string `yaml:"variant"`
	Stickiness string `yaml:"stickiness"`
	Start      string `yaml:"start"`
	End        string `yaml:"end"`
	Steps      uint32 `yaml:"steps"`
}

type experimentationFile struct {
	Start string `yaml:"start"`
	End   string `yaml:"end"`
}

type scheduledStepFile struct {
	Name            string               `yaml:"name,omitempty"`
	Description     string               `yaml:"description,omitempty"`
	Disabled        bool                 `yaml:"disabled,omitempty"`
	Date            string               `yaml:"date"`
	DefaultAction   *actionYAML          `yaml:"default_action,omitempty"`
	Experimentation *experimentationFile `yaml:"experimentation,omitempty"`
	Rules           []ruleFile           `yaml:"rules,omitempty"`
}

type conditionYAML struct {
	Value ast.Condition
}

type valueYAML struct {
	Value ast.Value
}

type variantValueYAML struct {
	Value ast.VariantValue
}

func Marshal(doc *ast.Document) ([]byte, error) {
	file := documentFile{
		Version: version,
		Flags:   make([]flagFile, 0, len(doc.Flags)),
	}
	for _, flag := range doc.Flags {
		file.Flags = append(file.Flags, flagFile{
			Key:            flag.Key,
			Variants:       wrapVariantValues(flag.Variants),
			DefaultVariant: flag.DefaultVariant,
			Environments:   wrapEnvironments(flag.Environments),
			Metadata:       wrapMetadata(flag.Metadata),
		})
	}
	return yaml.Marshal(file)
}

func Unmarshal(data []byte) (*ast.Document, error) {
	var file documentFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if file.Version != "" && file.Version != version {
		return nil, fmt.Errorf("unsupported normalized yaml version %q", file.Version)
	}

	out := &ast.Document{
		Flags: make([]*ast.Flag, 0, len(file.Flags)),
	}
	for _, flag := range file.Flags {
		out.Flags = append(out.Flags, &ast.Flag{
			Key:            flag.Key,
			Variants:       unwrapVariantValues(flag.Variants),
			DefaultVariant: flag.DefaultVariant,
			Environments:   unwrapEnvironments(flag.Environments),
			Metadata:       unwrapMetadata(flag.Metadata),
		})
	}
	return out, nil
}

func (a actionYAML) MarshalYAML() (any, error) {
	return marshalAction(a.Value)
}

func (a *actionYAML) UnmarshalYAML(node *yaml.Node) error {
	value, err := parseActionNode(node)
	if err != nil {
		return err
	}
	a.Value = value
	return nil
}

func (c conditionYAML) MarshalYAML() (any, error) {
	return marshalCondition(c.Value)
}

func (c *conditionYAML) UnmarshalYAML(node *yaml.Node) error {
	value, err := parseConditionNode(node)
	if err != nil {
		return err
	}
	c.Value = value
	return nil
}

func (v valueYAML) MarshalYAML() (any, error) {
	return marshalValue(v.Value)
}

func (v *valueYAML) UnmarshalYAML(node *yaml.Node) error {
	value, err := parseValueNode(node)
	if err != nil {
		return err
	}
	v.Value = value
	return nil
}

func (v variantValueYAML) MarshalYAML() (any, error) {
	return marshalVariantValue(v.Value), nil
}

func (v *variantValueYAML) UnmarshalYAML(node *yaml.Node) error {
	value, err := parseVariantValueNode(node)
	if err != nil {
		return err
	}
	v.Value = value
	return nil
}

func wrapVariantValues(values map[string]ast.VariantValue) map[string]variantValueYAML {
	out := make(map[string]variantValueYAML, len(values))
	for key, value := range values {
		out[key] = variantValueYAML{Value: value}
	}
	return out
}

func unwrapVariantValues(values map[string]variantValueYAML) map[string]ast.VariantValue {
	out := make(map[string]ast.VariantValue, len(values))
	for key, value := range values {
		out[key] = value.Value
	}
	return out
}

func wrapEnvironments(values map[string]*ast.Environment) map[string]environmentFile {
	out := make(map[string]environmentFile, len(values))
	for key, value := range values {
		out[key] = environmentFile{
			StaticVariant:     value.StaticVariant,
			DefaultAction:     wrapOptionalAction(value.DefaultAction),
			Experimentation:   wrapExperimentation(value.Experimentation),
			ScheduledRollouts: wrapScheduledSteps(value.ScheduledRollouts),
			Rules:             wrapRules(value.Rules),
		}
	}
	return out
}

func unwrapEnvironments(values map[string]environmentFile) map[string]*ast.Environment {
	out := make(map[string]*ast.Environment, len(values))
	for key, value := range values {
		out[key] = &ast.Environment{
			StaticVariant:     value.StaticVariant,
			DefaultAction:     unwrapOptionalAction(value.DefaultAction),
			Experimentation:   unwrapExperimentation(value.Experimentation),
			ScheduledRollouts: unwrapScheduledSteps(value.ScheduledRollouts),
			Rules:             unwrapRules(value.Rules),
		}
	}
	return out
}

func wrapRules(rules []*ast.Rule) []ruleFile {
	out := make([]ruleFile, 0, len(rules))
	for _, rule := range rules {
		out = append(out, ruleFile{
			If:     conditionYAML{Value: rule.Condition},
			Action: actionYAML{Value: rule.Action},
		})
	}
	return out
}

func unwrapRules(rules []ruleFile) []*ast.Rule {
	out := make([]*ast.Rule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, &ast.Rule{
			Condition: rule.If.Value,
			Action:    rule.Action.Value,
		})
	}
	return out
}

func wrapOptionalAction(action ast.Action) *actionYAML {
	if action == nil {
		return nil
	}
	return &actionYAML{Value: action}
}

func unwrapOptionalAction(action *actionYAML) ast.Action {
	if action == nil {
		return nil
	}
	return action.Value
}

func wrapExperimentation(exp *ast.Experimentation) *experimentationFile {
	if exp == nil {
		return nil
	}
	return &experimentationFile{Start: exp.Start, End: exp.End}
}

func unwrapExperimentation(exp *experimentationFile) *ast.Experimentation {
	if exp == nil {
		return nil
	}
	return &ast.Experimentation{Start: exp.Start, End: exp.End}
}

func wrapScheduledSteps(steps []*ast.ScheduledStep) []scheduledStepFile {
	out := make([]scheduledStepFile, 0, len(steps))
	for _, step := range steps {
		out = append(out, scheduledStepFile{
			Name:            step.Name,
			Description:     step.Description,
			Disabled:        step.Disabled,
			Date:            step.Date,
			DefaultAction:   wrapOptionalAction(step.DefaultAction),
			Experimentation: wrapExperimentation(step.Experimentation),
			Rules:           wrapRules(step.Rules),
		})
	}
	return out
}

func unwrapScheduledSteps(steps []scheduledStepFile) []*ast.ScheduledStep {
	out := make([]*ast.ScheduledStep, 0, len(steps))
	for _, step := range steps {
		out = append(out, &ast.ScheduledStep{
			Name:            step.Name,
			Description:     step.Description,
			Disabled:        step.Disabled,
			Date:            step.Date,
			DefaultAction:   unwrapOptionalAction(step.DefaultAction),
			Experimentation: unwrapExperimentation(step.Experimentation),
			Rules:           unwrapRules(step.Rules),
		})
	}
	return out
}

func wrapMetadata(meta *ast.Metadata) *metadataFile {
	if meta == nil {
		return nil
	}
	return &metadataFile{
		Owner:       meta.Owner,
		Description: meta.Description,
		Expiry:      meta.Expiry,
		Tags:        append([]string(nil), meta.Tags...),
	}
}

func unwrapMetadata(meta *metadataFile) *ast.Metadata {
	if meta == nil {
		return nil
	}
	return &ast.Metadata{
		Owner:       meta.Owner,
		Description: meta.Description,
		Expiry:      meta.Expiry,
		Tags:        append([]string(nil), meta.Tags...),
	}
}

func marshalAction(action ast.Action) (any, error) {
	switch value := action.(type) {
	case *ast.ServeAction:
		return map[string]any{"serve": value.Variant}, nil
	case *ast.DistributeAction:
		return map[string]any{"distribute": distributionActionFile{
			Stickiness:  value.Stickiness,
			Allocations: value.Allocations,
		}}, nil
	case *ast.ProgressiveRolloutAction:
		return map[string]any{"progressive_rollout": progressiveRolloutFile{
			Variant:    value.Variant,
			Stickiness: value.Stickiness,
			Start:      value.Start,
			End:        value.End,
			Steps:      value.Steps,
		}}, nil
	default:
		return nil, fmt.Errorf("unsupported action type %T", action)
	}
}

func parseActionNode(node *yaml.Node) (ast.Action, error) {
	fields, err := mapping(node, "$action")
	if err != nil {
		return nil, err
	}
	if len(fields) != 1 {
		return nil, fmt.Errorf("$action: expected exactly one action")
	}
	if value, ok := fields["serve"]; ok {
		variant, err := scalarString(value, "$action.serve")
		if err != nil {
			return nil, err
		}
		return &ast.ServeAction{Variant: variant}, nil
	}
	if value, ok := fields["distribute"]; ok {
		distFields, err := mapping(value, "$action.distribute")
		if err != nil {
			return nil, err
		}
		stickiness, err := scalarString(distFields["stickiness"], "$action.distribute.stickiness")
		if err != nil {
			return nil, err
		}
		allocations := map[string]float64{}
		allocNode := distFields["allocations"]
		allocFields, err := mapping(allocNode, "$action.distribute.allocations")
		if err != nil {
			return nil, err
		}
		for key, valueNode := range allocFields {
			value, err := scalarFloat(valueNode, "$action.distribute.allocations."+key)
			if err != nil {
				return nil, err
			}
			allocations[key] = value
		}
		return &ast.DistributeAction{Stickiness: stickiness, Allocations: allocations}, nil
	}
	if value, ok := fields["progressive_rollout"]; ok {
		rolloutFields, err := mapping(value, "$action.progressive_rollout")
		if err != nil {
			return nil, err
		}
		steps, err := scalarInt(rolloutFields["steps"], "$action.progressive_rollout.steps")
		if err != nil {
			return nil, err
		}
		variant, err := scalarString(rolloutFields["variant"], "$action.progressive_rollout.variant")
		if err != nil {
			return nil, err
		}
		stickiness, err := scalarString(rolloutFields["stickiness"], "$action.progressive_rollout.stickiness")
		if err != nil {
			return nil, err
		}
		start, err := scalarString(rolloutFields["start"], "$action.progressive_rollout.start")
		if err != nil {
			return nil, err
		}
		end, err := scalarString(rolloutFields["end"], "$action.progressive_rollout.end")
		if err != nil {
			return nil, err
		}
		return &ast.ProgressiveRolloutAction{
			Variant:    variant,
			Stickiness: stickiness,
			Start:      start,
			End:        end,
			Steps:      uint32(steps),
		}, nil
	}
	return nil, fmt.Errorf("$action: unsupported action")
}

func marshalCondition(cond ast.Condition) (any, error) {
	switch value := cond.(type) {
	case *ast.LiteralBool:
		return map[string]any{"literal_bool": value.Value}, nil
	case *ast.Eq:
		return marshalBinaryCondition("eq", value.Left, value.Right)
	case *ast.Ne:
		return marshalBinaryCondition("ne", value.Left, value.Right)
	case *ast.Gt:
		return marshalBinaryCondition("gt", value.Left, value.Right)
	case *ast.Gte:
		return marshalBinaryCondition("gte", value.Left, value.Right)
	case *ast.Lt:
		return marshalBinaryCondition("lt", value.Left, value.Right)
	case *ast.Lte:
		return marshalBinaryCondition("lte", value.Left, value.Right)
	case *ast.In:
		return marshalBinaryCondition("in", value.Target, value.Candidate)
	case *ast.Contains:
		return marshalBinaryCondition("contains", value.Container, value.Value)
	case *ast.StartsWith:
		return marshalValueStringCondition("starts_with", value.Target, value.Prefix)
	case *ast.EndsWith:
		return marshalValueStringCondition("ends_with", value.Target, value.Suffix)
	case *ast.Matches:
		return marshalValueStringCondition("matches", value.Target, value.Pattern)
	case *ast.SemverGt:
		return marshalValueStringCondition("semver_gt", value.Left, value.Right)
	case *ast.SemverGte:
		return marshalValueStringCondition("semver_gte", value.Left, value.Right)
	case *ast.SemverLt:
		return marshalValueStringCondition("semver_lt", value.Left, value.Right)
	case *ast.SemverLte:
		return marshalValueStringCondition("semver_lte", value.Left, value.Right)
	case *ast.AllOf:
		return marshalConditionList("all_of", value.Conditions)
	case *ast.AnyOf:
		return marshalConditionList("any_of", value.Conditions)
	case *ast.OneOf:
		return marshalConditionList("one_of", value.Conditions)
	case *ast.Not:
		child, err := marshalCondition(value.Condition)
		if err != nil {
			return nil, err
		}
		return map[string]any{"not": child}, nil
	default:
		return nil, fmt.Errorf("unsupported condition type %T", cond)
	}
}

func parseConditionNode(node *yaml.Node) (ast.Condition, error) {
	fields, err := mapping(node, "$condition")
	if err != nil {
		return nil, err
	}
	if len(fields) != 1 {
		return nil, fmt.Errorf("$condition: expected exactly one operator")
	}
	for operator, value := range fields {
		switch operator {
		case "literal_bool":
			literal, err := scalarBool(value, "$condition.literal_bool")
			if err != nil {
				return nil, err
			}
			return &ast.LiteralBool{Value: literal}, nil
		case "eq":
			return parseBinaryConditionNode(value, "$condition.eq", func(left, right ast.Value) ast.Condition {
				return &ast.Eq{Left: left, Right: right}
			})
		case "ne":
			return parseBinaryConditionNode(value, "$condition.ne", func(left, right ast.Value) ast.Condition {
				return &ast.Ne{Left: left, Right: right}
			})
		case "gt":
			return parseBinaryConditionNode(value, "$condition.gt", func(left, right ast.Value) ast.Condition {
				return &ast.Gt{Left: left, Right: right}
			})
		case "gte":
			return parseBinaryConditionNode(value, "$condition.gte", func(left, right ast.Value) ast.Condition {
				return &ast.Gte{Left: left, Right: right}
			})
		case "lt":
			return parseBinaryConditionNode(value, "$condition.lt", func(left, right ast.Value) ast.Condition {
				return &ast.Lt{Left: left, Right: right}
			})
		case "lte":
			return parseBinaryConditionNode(value, "$condition.lte", func(left, right ast.Value) ast.Condition {
				return &ast.Lte{Left: left, Right: right}
			})
		case "in":
			return parseBinaryConditionNode(value, "$condition.in", func(left, right ast.Value) ast.Condition {
				return &ast.In{Target: left, Candidate: right}
			})
		case "contains":
			return parseBinaryConditionNode(value, "$condition.contains", func(left, right ast.Value) ast.Condition {
				return &ast.Contains{Container: left, Value: right}
			})
		case "starts_with":
			return parseValueStringConditionNode(value, "$condition.starts_with", func(target ast.Value, literal string) ast.Condition {
				return &ast.StartsWith{Target: target, Prefix: literal}
			})
		case "ends_with":
			return parseValueStringConditionNode(value, "$condition.ends_with", func(target ast.Value, literal string) ast.Condition {
				return &ast.EndsWith{Target: target, Suffix: literal}
			})
		case "matches":
			return parseValueStringConditionNode(value, "$condition.matches", func(target ast.Value, literal string) ast.Condition {
				return &ast.Matches{Target: target, Pattern: literal}
			})
		case "semver_gt":
			return parseValueStringConditionNode(value, "$condition.semver_gt", func(target ast.Value, literal string) ast.Condition {
				return &ast.SemverGt{Left: target, Right: literal}
			})
		case "semver_gte":
			return parseValueStringConditionNode(value, "$condition.semver_gte", func(target ast.Value, literal string) ast.Condition {
				return &ast.SemverGte{Left: target, Right: literal}
			})
		case "semver_lt":
			return parseValueStringConditionNode(value, "$condition.semver_lt", func(target ast.Value, literal string) ast.Condition {
				return &ast.SemverLt{Left: target, Right: literal}
			})
		case "semver_lte":
			return parseValueStringConditionNode(value, "$condition.semver_lte", func(target ast.Value, literal string) ast.Condition {
				return &ast.SemverLte{Left: target, Right: literal}
			})
		case "all_of":
			return parseConditionListNode(value, "$condition.all_of", func(items []ast.Condition) ast.Condition {
				return &ast.AllOf{Conditions: items}
			})
		case "any_of":
			return parseConditionListNode(value, "$condition.any_of", func(items []ast.Condition) ast.Condition {
				return &ast.AnyOf{Conditions: items}
			})
		case "one_of":
			return parseConditionListNode(value, "$condition.one_of", func(items []ast.Condition) ast.Condition {
				return &ast.OneOf{Conditions: items}
			})
		case "not":
			child, err := parseConditionNode(value)
			if err != nil {
				return nil, err
			}
			return &ast.Not{Condition: child}, nil
		default:
			return nil, fmt.Errorf("$condition: unsupported operator %q", operator)
		}
	}
	return nil, fmt.Errorf("$condition: empty condition")
}

func marshalBinaryCondition(operator string, left, right ast.Value) (any, error) {
	l, err := marshalValue(left)
	if err != nil {
		return nil, err
	}
	r, err := marshalValue(right)
	if err != nil {
		return nil, err
	}
	return map[string]any{operator: []any{l, r}}, nil
}

func marshalValueStringCondition(operator string, left ast.Value, right string) (any, error) {
	l, err := marshalValue(left)
	if err != nil {
		return nil, err
	}
	return map[string]any{operator: []any{l, right}}, nil
}

func marshalConditionList(operator string, conditions []ast.Condition) (any, error) {
	out := make([]any, 0, len(conditions))
	for _, condition := range conditions {
		value, err := marshalCondition(condition)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return map[string]any{operator: out}, nil
}

func marshalValue(value ast.Value) (any, error) {
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
		out := make([]any, 0, len(v.Values))
		for _, item := range v.Values {
			value, err := marshalValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported value type %T", value)
	}
}

func parseValueNode(node *yaml.Node) (ast.Value, error) {
	switch node.Kind {
	case yaml.MappingNode:
		fields, err := mapping(node, "$value")
		if err != nil {
			return nil, err
		}
		if len(fields) == 1 {
			if value, ok := fields["var"]; ok {
				path, err := scalarString(value, "$value.var")
				if err != nil {
					return nil, err
				}
				return &ast.Var{Path: path}, nil
			}
		}
		return nil, fmt.Errorf("$value: object values only support {var: ...}")
	case yaml.SequenceNode:
		out := make([]ast.Value, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := parseValueNode(child)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return &ast.List{Values: out}, nil
	case yaml.ScalarNode:
		return parseScalarNode(node), nil
	case yaml.AliasNode:
		return nil, fmt.Errorf("$value: yaml aliases are not supported")
	default:
		return nil, fmt.Errorf("$value: unsupported yaml node kind %v", node.Kind)
	}
}

func parseScalarNode(node *yaml.Node) *ast.Scalar {
	if node.Tag == "!!null" || node.Value == "null" {
		return &ast.Scalar{Kind: ast.ScalarKindNull}
	}
	if value, ok := parseBoolScalar(node); ok {
		return &ast.Scalar{Kind: ast.ScalarKindBool, Bool: value}
	}
	if value, ok := parseIntScalar(node); ok {
		return &ast.Scalar{Kind: ast.ScalarKindInt, Int: value}
	}
	if value, ok := parseFloatScalar(node); ok {
		return &ast.Scalar{Kind: ast.ScalarKindDouble, Double: value}
	}
	return &ast.Scalar{Kind: ast.ScalarKindString, String: node.Value}
}

func marshalVariantValue(value ast.VariantValue) any {
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
		out := make([]any, 0, len(value.List))
		for _, item := range value.List {
			out = append(out, marshalVariantValue(item))
		}
		return out
	case ast.VariantValueKindNull:
		return nil
	default:
		return nil
	}
}

func parseVariantValueNode(node *yaml.Node) (ast.VariantValue, error) {
	switch node.Kind {
	case yaml.MappingNode:
		value, err := nodeToAny(node, "$variant")
		if err != nil {
			return ast.VariantValue{}, err
		}
		object, _ := value.(map[string]any)
		return ast.VariantValue{Kind: ast.VariantValueKindObject, Object: object}, nil
	case yaml.SequenceNode:
		out := make([]ast.VariantValue, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := parseVariantValueNode(child)
			if err != nil {
				return ast.VariantValue{}, err
			}
			out = append(out, value)
		}
		return ast.VariantValue{Kind: ast.VariantValueKindList, List: out}, nil
	case yaml.ScalarNode:
		if node.Tag == "!!null" || node.Value == "null" {
			return ast.VariantValue{Kind: ast.VariantValueKindNull}, nil
		}
		if value, ok := parseBoolScalar(node); ok {
			return ast.VariantValue{Kind: ast.VariantValueKindBool, Bool: value}, nil
		}
		if value, ok := parseIntScalar(node); ok {
			return ast.VariantValue{Kind: ast.VariantValueKindInt, Int: value}, nil
		}
		if value, ok := parseFloatScalar(node); ok {
			return ast.VariantValue{Kind: ast.VariantValueKindDouble, Double: value}, nil
		}
		return ast.VariantValue{Kind: ast.VariantValueKindString, String: node.Value}, nil
	case yaml.AliasNode:
		return ast.VariantValue{}, fmt.Errorf("$variant: yaml aliases are not supported")
	default:
		return ast.VariantValue{}, fmt.Errorf("$variant: unsupported yaml node kind %v", node.Kind)
	}
}

func parseBinaryConditionNode(node *yaml.Node, path string, build func(left, right ast.Value) ast.Condition) (ast.Condition, error) {
	if node.Kind != yaml.SequenceNode || len(node.Content) != 2 {
		return nil, fmt.Errorf("%s: expected a two-item sequence", path)
	}
	left, err := parseValueNode(node.Content[0])
	if err != nil {
		return nil, err
	}
	right, err := parseValueNode(node.Content[1])
	if err != nil {
		return nil, err
	}
	return build(left, right), nil
}

func parseValueStringConditionNode(node *yaml.Node, path string, build func(left ast.Value, right string) ast.Condition) (ast.Condition, error) {
	if node.Kind != yaml.SequenceNode || len(node.Content) != 2 {
		return nil, fmt.Errorf("%s: expected a two-item sequence", path)
	}
	left, err := parseValueNode(node.Content[0])
	if err != nil {
		return nil, err
	}
	right, err := scalarString(node.Content[1], path+"[1]")
	if err != nil {
		return nil, err
	}
	return build(left, right), nil
}

func parseConditionListNode(node *yaml.Node, path string, build func(items []ast.Condition) ast.Condition) (ast.Condition, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected sequence", path)
	}
	out := make([]ast.Condition, 0, len(node.Content))
	for _, child := range node.Content {
		value, err := parseConditionNode(child)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return build(out), nil
}
