package normalize

import (
	"fmt"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
	"github.com/satorunooshie/ffcraft/internal/ast"
)

func normalizeCondition(doc *ffv1.FeatureFlagDocument, cond *ffv1.Condition) (ast.Condition, error) {
	switch kind := cond.Kind.(type) {
	case *ffv1.Condition_Rule:
		return normalizeCondition(doc, doc.Rules[kind.Rule.Name])
	case *ffv1.Condition_LiteralBool:
		return &ast.LiteralBool{Value: kind.LiteralBool.Value}, nil
	case *ffv1.Condition_Eq:
		return normalizeBinaryCondition(kind.Eq.Left, kind.Eq.Right, func(left, right ast.Value) ast.Condition {
			return &ast.Eq{Left: left, Right: right}
		})
	case *ffv1.Condition_Ne:
		return normalizeBinaryCondition(kind.Ne.Left, kind.Ne.Right, func(left, right ast.Value) ast.Condition {
			return &ast.Ne{Left: left, Right: right}
		})
	case *ffv1.Condition_Gt:
		return normalizeBinaryCondition(kind.Gt.Left, kind.Gt.Right, func(left, right ast.Value) ast.Condition {
			return &ast.Gt{Left: left, Right: right}
		})
	case *ffv1.Condition_Gte:
		return normalizeBinaryCondition(kind.Gte.Left, kind.Gte.Right, func(left, right ast.Value) ast.Condition {
			return &ast.Gte{Left: left, Right: right}
		})
	case *ffv1.Condition_Lt:
		return normalizeBinaryCondition(kind.Lt.Left, kind.Lt.Right, func(left, right ast.Value) ast.Condition {
			return &ast.Lt{Left: left, Right: right}
		})
	case *ffv1.Condition_Lte:
		return normalizeBinaryCondition(kind.Lte.Left, kind.Lte.Right, func(left, right ast.Value) ast.Condition {
			return &ast.Lte{Left: left, Right: right}
		})
	case *ffv1.Condition_In:
		return normalizeBinaryCondition(kind.In.Target, kind.In.Candidate, func(left, right ast.Value) ast.Condition {
			return &ast.In{Target: left, Candidate: right}
		})
	case *ffv1.Condition_Contains:
		return normalizeBinaryCondition(kind.Contains.Container, kind.Contains.Value, func(left, right ast.Value) ast.Condition {
			return &ast.Contains{Container: left, Value: right}
		})
	case *ffv1.Condition_StartsWith:
		target, err := normalizeValue(kind.StartsWith.Target)
		if err != nil {
			return nil, err
		}
		return &ast.StartsWith{Target: target, Prefix: kind.StartsWith.Prefix}, nil
	case *ffv1.Condition_EndsWith:
		target, err := normalizeValue(kind.EndsWith.Target)
		if err != nil {
			return nil, err
		}
		return &ast.EndsWith{Target: target, Suffix: kind.EndsWith.Suffix}, nil
	case *ffv1.Condition_Matches:
		target, err := normalizeValue(kind.Matches.Target)
		if err != nil {
			return nil, err
		}
		return &ast.Matches{Target: target, Pattern: kind.Matches.Pattern}, nil
	case *ffv1.Condition_SemverGt:
		return normalizeSemverCondition(kind.SemverGt.Left, kind.SemverGt.Right, func(left ast.Value, right string) ast.Condition {
			return &ast.SemverGt{Left: left, Right: right}
		})
	case *ffv1.Condition_SemverGte:
		return normalizeSemverCondition(kind.SemverGte.Left, kind.SemverGte.Right, func(left ast.Value, right string) ast.Condition {
			return &ast.SemverGte{Left: left, Right: right}
		})
	case *ffv1.Condition_SemverLt:
		return normalizeSemverCondition(kind.SemverLt.Left, kind.SemverLt.Right, func(left ast.Value, right string) ast.Condition {
			return &ast.SemverLt{Left: left, Right: right}
		})
	case *ffv1.Condition_SemverLte:
		return normalizeSemverCondition(kind.SemverLte.Left, kind.SemverLte.Right, func(left ast.Value, right string) ast.Condition {
			return &ast.SemverLte{Left: left, Right: right}
		})
	case *ffv1.Condition_AllOf:
		return normalizeConditionGroup(doc, kind.AllOf.Conditions, func(items []ast.Condition) ast.Condition {
			return &ast.AllOf{Conditions: items}
		})
	case *ffv1.Condition_AnyOf:
		return normalizeConditionGroup(doc, kind.AnyOf.Conditions, func(items []ast.Condition) ast.Condition {
			return &ast.AnyOf{Conditions: items}
		})
	case *ffv1.Condition_OneOf:
		return normalizeConditionGroup(doc, kind.OneOf.Conditions, func(items []ast.Condition) ast.Condition {
			return &ast.OneOf{Conditions: items}
		})
	case *ffv1.Condition_Not:
		child, err := normalizeCondition(doc, kind.Not.Condition)
		if err != nil {
			return nil, err
		}
		return &ast.Not{Condition: child}, nil
	default:
		return nil, fmt.Errorf("condition must define exactly one branch")
	}
}

func normalizeConditionGroup(doc *ffv1.FeatureFlagDocument, conditions []*ffv1.Condition, build func([]ast.Condition) ast.Condition) (ast.Condition, error) {
	items, err := normalizeConditionList(doc, conditions)
	if err != nil {
		return nil, err
	}
	return build(items), nil
}

func normalizeConditionList(doc *ffv1.FeatureFlagDocument, conditions []*ffv1.Condition) ([]ast.Condition, error) {
	out := make([]ast.Condition, 0, len(conditions))
	for _, cond := range conditions {
		value, err := normalizeCondition(doc, cond)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func normalizeBinaryCondition(left, right *ffv1.Value, build func(left, right ast.Value) ast.Condition) (ast.Condition, error) {
	nleft, nright, err := normalizeBinaryValues(left, right)
	if err != nil {
		return nil, err
	}
	return build(nleft, nright), nil
}

func normalizeSemverCondition(left *ffv1.Value, right string, build func(left ast.Value, right string) ast.Condition) (ast.Condition, error) {
	value, err := normalizeValue(left)
	if err != nil {
		return nil, err
	}
	return build(value, right), nil
}
