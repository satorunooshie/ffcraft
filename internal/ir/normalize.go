// Package ir owns the normalized ffcraft.ir.v1 semantic model.
package ir

import (
	"fmt"
	"math"
	"sort"
	"strings"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/normalize"
)

// Normalize parses authoring semantics through the authoring adapter and
// returns the normative protobuf IR consumed by downstream adapters.
func Normalize(doc *ffv1.FeatureFlagDocument) (*irv1.Document, error) {
	normalized, err := normalize.Normalize(doc)
	if err != nil {
		return nil, err
	}
	return FromAST(normalized)
}

// FromAST is the compatibility boundary for the pre-IR internal model. No
// compiler should call it; new code should receive *irv1.Document directly.
func FromAST(doc *ast.Document) (*irv1.Document, error) {
	if doc == nil {
		return nil, fmt.Errorf("document is nil")
	}
	out := &irv1.Document{Flags: make(map[string]*irv1.Flag, len(doc.Flags)), Extensions: cloneExtensions(doc.Extensions)}
	for _, flag := range doc.Flags {
		if _, exists := out.Flags[flag.Key]; exists {
			return nil, fmt.Errorf("duplicate flag %q", flag.Key)
		}
		f := &irv1.Flag{Variants: make(map[string]*irv1.VariantValue, len(flag.Variants)), Environments: make(map[string]*irv1.Environment, len(flag.Environments)), Extensions: cloneExtensions(flag.Extensions)}
		for name, value := range flag.Variants {
			f.Variants[name] = variant(value)
		}
		for name, env := range flag.Environments {
			converted, err := environment(env, f.Variants)
			if err != nil {
				return nil, fmt.Errorf("flag %q environment %q: %w", flag.Key, name, err)
			}
			f.Environments[name] = converted
		}
		out.Flags[flag.Key] = f
	}
	if err := Validate(out); err != nil {
		return nil, err
	}
	return out, nil
}

func environment(source *ast.Environment, variants map[string]*irv1.VariantValue) (*irv1.Environment, error) {
	base, err := evaluation(source.Rules, source.DefaultAction)
	if err != nil {
		return nil, err
	}
	if source.StaticVariant != "" {
		base = &irv1.Evaluation{DefaultAction: serve(source.StaticVariant)}
	}
	if source.Experimentation != nil || len(source.ScheduledRollouts) != 0 {
		return nil, fmt.Errorf("authoring rollout metadata requires snapshot lowering")
	}
	return &irv1.Environment{Base: base, Extensions: cloneExtensions(source.Extensions)}, nil
}

func evaluation(rules []*ast.Rule, fallback ast.Action) (*irv1.Evaluation, error) {
	out := &irv1.Evaluation{Rules: make([]*irv1.Rule, 0, len(rules))}
	for _, rule := range rules {
		condition, err := condition(rule.Condition)
		if err != nil {
			return nil, err
		}
		action, err := action(rule.Action)
		if err != nil {
			return nil, err
		}
		out.Rules = append(out.Rules, &irv1.Rule{Condition: condition, Action: action})
	}
	var err error
	out.DefaultAction, err = action(fallback)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func action(value ast.Action) (*irv1.Action, error) {
	switch value := value.(type) {
	case *ast.ServeAction:
		return &irv1.Action{Kind: &irv1.Action_Serve{Serve: value.Variant}}, nil
	case *ast.DistributeAction:
		weights := make(map[string]uint32, len(value.Allocations))
		for name, percentage := range value.Allocations {
			if percentage <= 0 || math.IsNaN(percentage) || math.IsInf(percentage, 0) || percentage != math.Trunc(percentage) {
				return nil, fmt.Errorf("distribution weight %q is not a positive integer", name)
			}
			weights[name] = uint32(percentage)
		}
		return &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: path(value.Stickiness), Weights: canonicalWeights(weights)}}}, nil
	default:
		return nil, fmt.Errorf("unsupported action %T in normalized IR", value)
	}
}

func condition(value ast.Condition) (*irv1.Condition, error) {
	switch value := value.(type) {
	case *ast.LiteralBool:
		return &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: value.Value}}, nil
	case *ast.Eq, *ast.Ne:
		left, right := valuePair(value)
		attribute, literal, err := attributeLiteral(left, right)
		if err != nil {
			return nil, err
		}
		op := irv1.EqualityOperator_EQUALITY_OPERATOR_EQ
		if _, ok := value.(*ast.Ne); ok {
			op = irv1.EqualityOperator_EQUALITY_OPERATOR_NE
		}
		return &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: op, Attribute: path(attribute.Path), Literal: scalar(literal)}}}, nil
	case *ast.Gt, *ast.Gte, *ast.Lt, *ast.Lte:
		left, right := valuePair(value)
		variable, literal, err := numericPair(left, right)
		if err != nil {
			return nil, err
		}
		op := numericOperator(value)
		return &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: op, Attribute: path(variable.Path), Literal: numericValue(literal)}}}, nil
	case *ast.In:
		variable, ok := value.Target.(*ast.Var)
		if !ok {
			return nil, fmt.Errorf("membership target must be a variable")
		}
		list, ok := value.Candidate.(*ast.List)
		if !ok {
			return nil, fmt.Errorf("membership literals must be a list")
		}
		literals := make([]*irv1.ScalarValue, 0, len(list.Values))
		for _, item := range list.Values {
			s, ok := item.(*ast.Scalar)
			if !ok {
				return nil, fmt.Errorf("membership literals must be scalar")
			}
			literals = append(literals, scalar(s))
		}
		return &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: path(variable.Path), Literals: &irv1.ScalarList{Values: literals}}}}, nil
	case *ast.Contains, *ast.StartsWith, *ast.EndsWith:
		var target *ast.Var
		var literal string
		var op irv1.StringMatchOperator
		switch item := value.(type) {
		case *ast.Contains:
			var ok bool
			target, ok = item.Container.(*ast.Var)
			if !ok {
				return nil, fmt.Errorf("string match target must be a variable")
			}
			s, ok := item.Value.(*ast.Scalar)
			if !ok || s.Kind != ast.ScalarKindString {
				return nil, fmt.Errorf("string match literal must be a string")
			}
			literal = s.String
			op = irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS
		case *ast.StartsWith:
			target, _ = item.Target.(*ast.Var)
			literal, op = item.Prefix, irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH
		case *ast.EndsWith:
			target, _ = item.Target.(*ast.Var)
			literal, op = item.Suffix, irv1.StringMatchOperator_STRING_MATCH_OPERATOR_ENDS_WITH
		}
		if target == nil {
			return nil, fmt.Errorf("string match target must be a variable")
		}
		return &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: op, Attribute: path(target.Path), Literal: literal}}}, nil
	case *ast.AllOf, *ast.AnyOf, *ast.OneOf:
		children, op := logicalChildren(value)
		conditions := make([]*irv1.Condition, 0, len(children))
		for _, child := range children {
			c, err := condition(child)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, c)
		}
		return &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: op, Conditions: conditions}}}, nil
	case *ast.Not:
		child, err := condition(value.Condition)
		if err != nil {
			return nil, err
		}
		return &irv1.Condition{Kind: &irv1.Condition_Negation{Negation: child}}, nil
	default:
		return nil, fmt.Errorf("unsupported condition %T in normalized IR", value)
	}
}

func valuePair(value any) (ast.Value, ast.Value) {
	switch v := value.(type) {
	case *ast.Eq:
		return v.Left, v.Right
	case *ast.Ne:
		return v.Left, v.Right
	case *ast.Gt:
		return v.Left, v.Right
	case *ast.Gte:
		return v.Left, v.Right
	case *ast.Lt:
		return v.Left, v.Right
	case *ast.Lte:
		return v.Left, v.Right
	default:
		panic("unreachable")
	}
}
func attributeLiteral(left, right ast.Value) (*ast.Var, *ast.Scalar, error) {
	if v, ok := left.(*ast.Var); ok {
		if s, ok := right.(*ast.Scalar); ok {
			return v, s, nil
		}
	}
	if v, ok := right.(*ast.Var); ok {
		if s, ok := left.(*ast.Scalar); ok {
			return v, s, nil
		}
	}
	return nil, nil, fmt.Errorf("condition must compare an attribute with a scalar")
}
func numericPair(left, right ast.Value) (*ast.Var, *ast.Scalar, error) {
	variable, literal, err := attributeLiteral(left, right)
	if err != nil || (literal.Kind != ast.ScalarKindInt && literal.Kind != ast.ScalarKindDouble) {
		return nil, nil, fmt.Errorf("numeric comparison requires numeric literal")
	}
	return variable, literal, nil
}
func numericOperator(value any) irv1.NumericComparisonOperator {
	switch value.(type) {
	case *ast.Gt:
		return irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT
	case *ast.Gte:
		return irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE
	case *ast.Lt:
		return irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LT
	default:
		return irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LTE
	}
}
func logicalChildren(value ast.Condition) ([]ast.Condition, irv1.LogicalOperator) {
	switch v := value.(type) {
	case *ast.AllOf:
		return v.Conditions, irv1.LogicalOperator_LOGICAL_OPERATOR_ALL
	case *ast.AnyOf:
		return v.Conditions, irv1.LogicalOperator_LOGICAL_OPERATOR_ANY
	default:
		return v.(*ast.OneOf).Conditions, irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE
	}
}
func path(value string) *irv1.AttributePath {
	return &irv1.AttributePath{Segments: strings.Split(value, ".")}
}
func serve(name string) *irv1.Action { return &irv1.Action{Kind: &irv1.Action_Serve{Serve: name}} }
func scalar(value *ast.Scalar) *irv1.ScalarValue {
	switch value.Kind {
	case ast.ScalarKindString:
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: value.String}}
	case ast.ScalarKindBool:
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_BoolValue{BoolValue: value.Bool}}
	case ast.ScalarKindInt:
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_IntValue{IntValue: value.Int}}
	case ast.ScalarKindDouble:
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_DoubleValue{DoubleValue: value.Double}}
	default:
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_NullValue{NullValue: &irv1.ScalarNull{}}}
	}
}
func numericValue(value *ast.Scalar) *irv1.NumericValue {
	if value.Kind == ast.ScalarKindInt {
		return &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: value.Int}}
	}
	return &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: value.Double}}
}
func variant(value ast.VariantValue) *irv1.VariantValue {
	switch value.Kind {
	case ast.VariantValueKindBool:
		return &irv1.VariantValue{Kind: &irv1.VariantValue_BoolValue{BoolValue: value.Bool}}
	case ast.VariantValueKindString:
		return &irv1.VariantValue{Kind: &irv1.VariantValue_StringValue{StringValue: value.String}}
	case ast.VariantValueKindInt:
		return &irv1.VariantValue{Kind: &irv1.VariantValue_IntValue{IntValue: value.Int}}
	case ast.VariantValueKindDouble:
		return &irv1.VariantValue{Kind: &irv1.VariantValue_DoubleValue{DoubleValue: value.Double}}
	case ast.VariantValueKindNull:
		return &irv1.VariantValue{Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}}
	default:
		return &irv1.VariantValue{Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}}
	}
}
func canonicalWeights(weights map[string]uint32) map[string]uint32 {
	names := make([]string, 0, len(weights))
	for name := range weights {
		names = append(names, name)
	}
	sort.Strings(names)
	gcd := uint32(0)
	for _, name := range names {
		gcd = greatestCommonDivisor(gcd, weights[name])
	}
	out := make(map[string]uint32, len(weights))
	for _, name := range names {
		out[name] = weights[name] / gcd
	}
	return out
}
func greatestCommonDivisor(a, b uint32) uint32 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
func cloneExtensions(values map[string]*irv1.ExtensionValue) map[string]*irv1.ExtensionValue {
	out := make(map[string]*irv1.ExtensionValue, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
