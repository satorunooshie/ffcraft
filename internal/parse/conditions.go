package parse

import (
	"fmt"

	"gopkg.in/yaml.v3"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

func parseCondition(node *yaml.Node, path string) (*ffv1.Condition, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	if len(fields) != 1 {
		return nil, fmt.Errorf("%s: condition must contain exactly one operator", path)
	}
	for operator, value := range fields {
		return parseConditionOperator(operator, value, path)
	}
	return nil, fmt.Errorf("%s: empty condition", path)
}

func parseConditionOperator(operator string, node *yaml.Node, path string) (*ffv1.Condition, error) {
	switch operator {
	case "rule":
		name, err := scalarString(node, path+".rule")
		if err != nil {
			return nil, err
		}
		return &ffv1.Condition{Kind: &ffv1.Condition_Rule{Rule: &ffv1.RuleRef{Name: name}}}, nil
	case "literal_bool":
		value, err := scalarBool(node, path+".literal_bool")
		if err != nil {
			return nil, err
		}
		return &ffv1.Condition{Kind: &ffv1.Condition_LiteralBool{LiteralBool: &ffv1.LiteralBool{Value: value}}}, nil
	case "all_of":
		conditions, err := parseConditionList(node, path+".all_of")
		if err != nil {
			return nil, err
		}
		return &ffv1.Condition{Kind: &ffv1.Condition_AllOf{AllOf: &ffv1.AllOf{Conditions: conditions}}}, nil
	case "any_of":
		conditions, err := parseConditionList(node, path+".any_of")
		if err != nil {
			return nil, err
		}
		return &ffv1.Condition{Kind: &ffv1.Condition_AnyOf{AnyOf: &ffv1.AnyOf{Conditions: conditions}}}, nil
	case "one_of":
		conditions, err := parseConditionList(node, path+".one_of")
		if err != nil {
			return nil, err
		}
		return &ffv1.Condition{Kind: &ffv1.Condition_OneOf{OneOf: &ffv1.OneOf{Conditions: conditions}}}, nil
	case "not":
		condition, err := parseCondition(node, path+".not")
		if err != nil {
			return nil, err
		}
		return &ffv1.Condition{Kind: &ffv1.Condition_Not{Not: &ffv1.Not{Condition: condition}}}, nil
	case "eq":
		return parseBinaryCondition(node, path+".eq", func(left, right *ffv1.Value) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_Eq{Eq: &ffv1.Eq{Left: left, Right: right}}}
		})
	case "ne":
		return parseBinaryCondition(node, path+".ne", func(left, right *ffv1.Value) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_Ne{Ne: &ffv1.Ne{Left: left, Right: right}}}
		})
	case "gt":
		return parseBinaryCondition(node, path+".gt", func(left, right *ffv1.Value) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_Gt{Gt: &ffv1.Gt{Left: left, Right: right}}}
		})
	case "gte":
		return parseBinaryCondition(node, path+".gte", func(left, right *ffv1.Value) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_Gte{Gte: &ffv1.Gte{Left: left, Right: right}}}
		})
	case "lt":
		return parseBinaryCondition(node, path+".lt", func(left, right *ffv1.Value) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_Lt{Lt: &ffv1.Lt{Left: left, Right: right}}}
		})
	case "lte":
		return parseBinaryCondition(node, path+".lte", func(left, right *ffv1.Value) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_Lte{Lte: &ffv1.Lte{Left: left, Right: right}}}
		})
	case "in":
		return parseBinaryCondition(node, path+".in", func(left, right *ffv1.Value) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_In{In: &ffv1.In{Target: left, Candidate: right}}}
		})
	case "contains":
		return parseBinaryCondition(node, path+".contains", func(left, right *ffv1.Value) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_Contains{Contains: &ffv1.Contains{Container: left, Value: right}}}
		})
	case "starts_with":
		return parseValueStringCondition(node, path+".starts_with", func(target *ffv1.Value, literal string) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_StartsWith{StartsWith: &ffv1.StartsWith{Target: target, Prefix: literal}}}
		})
	case "ends_with":
		return parseValueStringCondition(node, path+".ends_with", func(target *ffv1.Value, literal string) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_EndsWith{EndsWith: &ffv1.EndsWith{Target: target, Suffix: literal}}}
		})
	case "matches":
		return parseValueStringCondition(node, path+".matches", func(target *ffv1.Value, literal string) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_Matches{Matches: &ffv1.Matches{Target: target, Pattern: literal}}}
		})
	case "semver_gt":
		return parseValueStringCondition(node, path+".semver_gt", func(target *ffv1.Value, literal string) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_SemverGt{SemverGt: &ffv1.SemverGt{Left: target, Right: literal}}}
		})
	case "semver_gte":
		return parseValueStringCondition(node, path+".semver_gte", func(target *ffv1.Value, literal string) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_SemverGte{SemverGte: &ffv1.SemverGte{Left: target, Right: literal}}}
		})
	case "semver_lt":
		return parseValueStringCondition(node, path+".semver_lt", func(target *ffv1.Value, literal string) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_SemverLt{SemverLt: &ffv1.SemverLt{Left: target, Right: literal}}}
		})
	case "semver_lte":
		return parseValueStringCondition(node, path+".semver_lte", func(target *ffv1.Value, literal string) *ffv1.Condition {
			return &ffv1.Condition{Kind: &ffv1.Condition_SemverLte{SemverLte: &ffv1.SemverLte{Left: target, Right: literal}}}
		})
	default:
		return nil, fmt.Errorf("%s: unsupported condition operator %q", path, operator)
	}
}

func parseConditionList(node *yaml.Node, path string) ([]*ffv1.Condition, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected sequence", path)
	}
	out := make([]*ffv1.Condition, 0, len(node.Content))
	for i, item := range node.Content {
		condition, err := parseCondition(item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		out = append(out, condition)
	}
	return out, nil
}

func parseBinaryCondition(node *yaml.Node, path string, build func(left, right *ffv1.Value) *ffv1.Condition) (*ffv1.Condition, error) {
	left, right, err := parseBinaryValueOperands(node, path)
	if err != nil {
		return nil, err
	}
	return build(left, right), nil
}

func parseValueStringCondition(node *yaml.Node, path string, build func(target *ffv1.Value, literal string) *ffv1.Condition) (*ffv1.Condition, error) {
	target, literal, err := parseValueAndStringOperands(node, path)
	if err != nil {
		return nil, err
	}
	return build(target, literal), nil
}
