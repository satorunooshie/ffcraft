package ir

import (
	"fmt"
	"strings"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ast"
)

// ToAST is a target adapter for the existing provider serializers. It is not
// a semantic model: all decisions have already been made in the protobuf IR.
func ToAST(doc *irv1.Document) (*ast.Document, error) {
	if err := Validate(doc); err != nil {
		return nil, err
	}
	out := &ast.Document{Flags: make([]*ast.Flag, 0, len(doc.Flags)), Extensions: cloneExtensions(doc.Extensions)}
	for key, source := range doc.Flags {
		flag := &ast.Flag{Key: key, Variants: make(map[string]ast.VariantValue, len(source.Variants)), Environments: make(map[string]*ast.Environment, len(source.Environments)), Extensions: cloneExtensions(source.Extensions)}
		for name, value := range source.Variants {
			flag.Variants[name] = astVariant(value)
		}
		for name, env := range source.Environments {
			converted, err := astEnvironment(env, flag.Variants)
			if err != nil {
				return nil, fmt.Errorf("flag %q environment %q: %w", key, name, err)
			}
			flag.Environments[name] = converted
			if flag.DefaultVariant == "" {
				if serve, ok := env.Base.DefaultAction.GetKind().(*irv1.Action_Serve); ok {
					flag.DefaultVariant = serve.Serve
				}
			}
		}
		out.Flags = append(out.Flags, flag)
	}
	return out, nil
}

func astEnvironment(source *irv1.Environment, variants map[string]ast.VariantValue) (*ast.Environment, error) {
	base, err := astEvaluation(source.Base, variants)
	if err != nil {
		return nil, err
	}
	out := &ast.Environment{Rules: base.Rules, DefaultAction: base.DefaultAction, Extensions: cloneExtensions(source.Extensions)}
	if serve, ok := base.DefaultAction.(*ast.ServeAction); ok && len(base.Rules) == 0 && len(source.Schedule) == 0 {
		out.StaticVariant = serve.Variant
	}
	for _, scheduled := range source.Schedule {
		evaluation, err := astEvaluation(scheduled.Evaluation, variants)
		if err != nil {
			return nil, err
		}
		at := scheduled.EffectiveAt.AsTime().UTC().Format(time.RFC3339Nano)
		out.ScheduledRollouts = append(out.ScheduledRollouts, &ast.ScheduledStep{Date: at, Rules: evaluation.Rules, DefaultAction: evaluation.DefaultAction})
	}
	return out, nil
}

type astEvaluationResult struct {
	Rules         []*ast.Rule
	DefaultAction ast.Action
}

func astEvaluation(source *irv1.Evaluation, variants map[string]ast.VariantValue) (*astEvaluationResult, error) {
	out := &astEvaluationResult{Rules: make([]*ast.Rule, 0, len(source.Rules))}
	for _, rule := range source.Rules {
		condition, err := astCondition(rule.Condition)
		if err != nil {
			return nil, err
		}
		action, err := astAction(rule.Action)
		if err != nil {
			return nil, err
		}
		out.Rules = append(out.Rules, &ast.Rule{Condition: condition, Action: action})
	}
	action, err := astAction(source.DefaultAction)
	if err != nil {
		return nil, err
	}
	return &astEvaluationResult{Rules: out.Rules, DefaultAction: action}, nil
}
func astAction(source *irv1.Action) (ast.Action, error) {
	switch kind := source.GetKind().(type) {
	case *irv1.Action_Serve:
		return &ast.ServeAction{Variant: kind.Serve}, nil
	case *irv1.Action_Distribute:
		allocations := make(map[string]float64, len(kind.Distribute.Weights))
		for name, weight := range kind.Distribute.Weights {
			allocations[name] = float64(weight)
		}
		return &ast.DistributeAction{Stickiness: strings.Join(kind.Distribute.AllocationKey.Segments, "."), Allocations: allocations}, nil
	default:
		return nil, fmt.Errorf("unsupported IR action")
	}
}
func astCondition(source *irv1.Condition) (ast.Condition, error) {
	switch kind := source.GetKind().(type) {
	case *irv1.Condition_Constant:
		return &ast.LiteralBool{Value: kind.Constant}, nil
	case *irv1.Condition_Equality:
		return binaryCondition(kind.Equality.Operator == irv1.EqualityOperator_EQUALITY_OPERATOR_NE, kind.Equality.Attribute, kind.Equality.Literal)
	case *irv1.Condition_NumericComparison:
		literal := astScalarNumeric(kind.NumericComparison.Literal)
		variable := &ast.Var{Path: strings.Join(kind.NumericComparison.Attribute.Segments, ".")}
		switch kind.NumericComparison.Operator {
		case irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT:
			return &ast.Gt{Left: variable, Right: literal}, nil
		case irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE:
			return &ast.Gte{Left: variable, Right: literal}, nil
		case irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LT:
			return &ast.Lt{Left: variable, Right: literal}, nil
		default:
			return &ast.Lte{Left: variable, Right: literal}, nil
		}
	case *irv1.Condition_StringMatch:
		variable := &ast.Var{Path: strings.Join(kind.StringMatch.Attribute.Segments, ".")}
		switch kind.StringMatch.Operator {
		case irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS:
			return &ast.Contains{Container: variable, Value: &ast.Scalar{Kind: ast.ScalarKindString, String: kind.StringMatch.Literal}}, nil
		case irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH:
			return &ast.StartsWith{Target: variable, Prefix: kind.StringMatch.Literal}, nil
		default:
			return &ast.EndsWith{Target: variable, Suffix: kind.StringMatch.Literal}, nil
		}
	case *irv1.Condition_Logical:
		children := make([]ast.Condition, 0, len(kind.Logical.Conditions))
		for _, child := range kind.Logical.Conditions {
			converted, err := astCondition(child)
			if err != nil {
				return nil, err
			}
			children = append(children, converted)
		}
		switch kind.Logical.Operator {
		case irv1.LogicalOperator_LOGICAL_OPERATOR_ALL:
			return &ast.AllOf{Conditions: children}, nil
		case irv1.LogicalOperator_LOGICAL_OPERATOR_ANY:
			return &ast.AnyOf{Conditions: children}, nil
		default:
			return &ast.OneOf{Conditions: children}, nil
		}
	case *irv1.Condition_Negation:
		child, err := astCondition(kind.Negation)
		if err != nil {
			return nil, err
		}
		return &ast.Not{Condition: child}, nil
	default:
		return nil, fmt.Errorf("unsupported IR condition")
	}
}
func binaryCondition(negated bool, attribute *irv1.AttributePath, literal *irv1.ScalarValue) (ast.Condition, error) {
	left := &ast.Var{Path: strings.Join(attribute.Segments, ".")}
	right := astScalar(literal)
	if negated {
		return &ast.Ne{Left: left, Right: right}, nil
	}
	return &ast.Eq{Left: left, Right: right}, nil
}
func astScalar(value *irv1.ScalarValue) ast.Value {
	switch kind := value.GetKind().(type) {
	case *irv1.ScalarValue_StringValue:
		return &ast.Scalar{Kind: ast.ScalarKindString, String: kind.StringValue}
	case *irv1.ScalarValue_BoolValue:
		return &ast.Scalar{Kind: ast.ScalarKindBool, Bool: kind.BoolValue}
	case *irv1.ScalarValue_IntValue:
		return &ast.Scalar{Kind: ast.ScalarKindInt, Int: kind.IntValue}
	case *irv1.ScalarValue_DoubleValue:
		return &ast.Scalar{Kind: ast.ScalarKindDouble, Double: kind.DoubleValue}
	default:
		return &ast.Scalar{Kind: ast.ScalarKindNull}
	}
}
func astScalarNumeric(value *irv1.NumericValue) *ast.Scalar {
	switch kind := value.GetKind().(type) {
	case *irv1.NumericValue_IntValue:
		return &ast.Scalar{Kind: ast.ScalarKindInt, Int: kind.IntValue}
	default:
		return &ast.Scalar{Kind: ast.ScalarKindDouble, Double: value.GetDoubleValue()}
	}
}
func astVariant(value *irv1.VariantValue) ast.VariantValue {
	switch kind := value.GetKind().(type) {
	case *irv1.VariantValue_BoolValue:
		return ast.VariantValue{Kind: ast.VariantValueKindBool, Bool: kind.BoolValue}
	case *irv1.VariantValue_StringValue:
		return ast.VariantValue{Kind: ast.VariantValueKindString, String: kind.StringValue}
	case *irv1.VariantValue_IntValue:
		return ast.VariantValue{Kind: ast.VariantValueKindInt, Int: kind.IntValue}
	case *irv1.VariantValue_DoubleValue:
		return ast.VariantValue{Kind: ast.VariantValueKindDouble, Double: kind.DoubleValue}
	case *irv1.VariantValue_NullValue:
		return ast.VariantValue{Kind: ast.VariantValueKindNull}
	case *irv1.VariantValue_ObjectValue:
		fields := make(map[string]any, len(kind.ObjectValue.Fields))
		for name, child := range kind.ObjectValue.Fields {
			fields[name] = astVariantAny(child)
		}
		return ast.VariantValue{Kind: ast.VariantValueKindObject, Object: fields}
	case *irv1.VariantValue_ListValue:
		list := make([]ast.VariantValue, 0, len(kind.ListValue.Values))
		for _, child := range kind.ListValue.Values {
			list = append(list, astVariant(child))
		}
		return ast.VariantValue{Kind: ast.VariantValueKindList, List: list}
	default:
		return ast.VariantValue{}
	}
}
func astVariantAny(value *irv1.VariantValue) any { v := astVariant(value); return astValueAny(v) }
func astValueAny(value ast.VariantValue) any {
	switch value.Kind {
	case ast.VariantValueKindBool:
		return value.Bool
	case ast.VariantValueKindString:
		return value.String
	case ast.VariantValueKindInt:
		return value.Int
	case ast.VariantValueKindDouble:
		return value.Double
	case ast.VariantValueKindNull:
		return nil
	case ast.VariantValueKindObject:
		return value.Object
	case ast.VariantValueKindList:
		out := make([]any, 0, len(value.List))
		for _, child := range value.List {
			out = append(out, astValueAny(child))
		}
		return out
	default:
		return nil
	}
}
