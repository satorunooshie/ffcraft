package flagd

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/capability"
	"github.com/satorunooshie/ffcraft/internal/ir"
)

func compileIRDocument(doc *irv1.Document, environment string, opts CompileOptions) ([]byte, []string, error) {
	if err := ir.Validate(doc); err != nil {
		return nil, nil, err
	}
	out := document{Schema: schemaURL, Flags: make(map[string]*flag, len(doc.Flags))}
	warnings := make([]string, 0)
	for key, source := range doc.Flags {
		env, ok := source.Environments[environment]
		if !ok {
			if opts.AllowMissingEnvironment {
				warnings = append(warnings, fmt.Sprintf("warning: skipping flag %q because environment %q is not defined", key, environment))
				continue
			}
			return nil, nil, fmt.Errorf("flag %q: environment %q not found", key, environment)
		}
		compiled := &flag{State: "ENABLED", Variants: compileIRVariants(source.Variants)}
		baseVariant, err := compileIRServeVariant(env.Base.DefaultAction)
		if err == nil && len(env.Base.Rules) == 0 && len(env.Schedule) == 0 {
			compiled.DefaultVariant = baseVariant
		} else if err == nil {
			compiled.DefaultVariant = baseVariant
		} else {
			return nil, nil, fmt.Errorf("flag %q: %w", key, err)
		}
		if len(env.Base.Rules) > 0 || len(env.Schedule) > 0 {
			compiled.Targeting, err = compileIREnvironment(env)
		} else {
			compiled.Targeting, err = compileIRAction(env.Base.DefaultAction)
			if _, serve := env.Base.DefaultAction.GetKind().(*irv1.Action_Serve); serve {
				compiled.Targeting = nil
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("flag %q: %w", key, err)
		}
		out.Flags[key] = compiled
	}
	output, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return output, warnings, nil
}

func compileIREnvironment(env *irv1.Environment) (any, error) {
	base, err := compileIREvaluation(env.Base)
	if err != nil {
		return nil, err
	}
	active := base
	for index := len(env.Schedule) - 1; index >= 0; index-- {
		scheduled := env.Schedule[index]
		condition := map[string]any{">=": []any{
			map[string]any{"var": "$flagd.timestamp"},
			scheduled.EffectiveAt.AsTime().Unix(),
		}}
		snapshot, err := compileIREvaluation(scheduled.Evaluation)
		if err != nil {
			return nil, fmt.Errorf("schedule[%d]: %w", index, err)
		}
		active = map[string]any{"if": []any{condition, snapshot, active}}
	}
	return active, nil
}

func compileIREvaluation(eval *irv1.Evaluation) (any, error) {
	if eval == nil || eval.DefaultAction == nil {
		return nil, fmt.Errorf("default_action is required")
	}
	defaultResult, err := compileIRAction(eval.DefaultAction)
	if err != nil {
		return nil, err
	}
	next := defaultResult
	for _, rule := range slices.Backward(eval.Rules) {
		if rule == nil || rule.Condition == nil || rule.Action == nil {
			return nil, fmt.Errorf("rule is incomplete")
		}
		condition, err := compileIRCondition(rule.Condition)
		if err != nil {
			return nil, err
		}
		action, err := compileIRAction(rule.Action)
		if err != nil {
			return nil, err
		}
		next = map[string]any{"if": []any{condition, action, next}}
	}
	return next, nil
}

func compileIRServeVariant(action *irv1.Action) (string, error) {
	if action == nil {
		return "", fmt.Errorf("default_action is required")
	}
	serve, ok := action.GetKind().(*irv1.Action_Serve)
	if !ok {
		return "", fmt.Errorf("flagd compiler requires default_action.serve")
	}
	return serve.Serve, nil
}

func compileIRAction(action *irv1.Action) (any, error) {
	if action == nil {
		return nil, fmt.Errorf("action is required")
	}
	switch kind := action.GetKind().(type) {
	case *irv1.Action_Serve:
		return kind.Serve, nil
	case *irv1.Action_Distribute:
		if kind.Distribute == nil || kind.Distribute.AllocationKey == nil {
			return nil, fmt.Errorf("distribution allocation_key is required")
		}
		keys := make([]string, 0, len(kind.Distribute.Weights))
		var total uint64
		for key, weight := range kind.Distribute.Weights {
			keys = append(keys, key)
			if weight == 0 {
				return nil, fmt.Errorf("distribution weight for %q must be positive", key)
			}
			total += uint64(weight)
		}
		if total > uint64(math.MaxInt32) {
			return nil, fmt.Errorf("flagd distribution weight total %d exceeds maximum %d", total, math.MaxInt32)
		}
		sort.Strings(keys)
		fractional := make([]any, 0, len(keys)+1)
		fractional = append(fractional, map[string]any{"cat": []any{
			map[string]any{"var": "$flagd.flagKey"},
			map[string]any{"var": strings.Join(kind.Distribute.AllocationKey.Segments, ".")},
		}})
		for _, key := range keys {
			fractional = append(fractional, []any{key, kind.Distribute.Weights[key]})
		}
		return map[string]any{"fractional": fractional}, nil
	default:
		return nil, fmt.Errorf("unsupported IR action %T", action.GetKind())
	}
}

func compileIRCondition(condition *irv1.Condition) (any, error) {
	if condition == nil {
		return nil, fmt.Errorf("condition is required")
	}
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_Constant:
		return kind.Constant, nil
	case *irv1.Condition_Equality:
		return compileIRBinary(kind.Equality.Attribute, kind.Equality.Literal, equalityOperator(kind.Equality.Operator))
	case *irv1.Condition_NumericComparison:
		return compileIRBinaryNumeric(kind.NumericComparison)
	case *irv1.Condition_Membership:
		values := make([]any, 0, len(kind.Membership.Literals.Values))
		for _, value := range kind.Membership.Literals.Values {
			values = append(values, compileIRScalar(value))
		}
		return map[string]any{"in": []any{compileIRVar(kind.Membership.Attribute), values}}, nil
	case *irv1.Condition_StringMatch:
		operator, ok := map[irv1.StringMatchOperator]string{
			irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS:    "in",
			irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH: "starts_with",
			irv1.StringMatchOperator_STRING_MATCH_OPERATOR_ENDS_WITH:   "ends_with",
		}[kind.StringMatch.Operator]
		if !ok {
			return nil, fmt.Errorf("unsupported string match operator")
		}
		if operator == "in" {
			return map[string]any{"in": []any{kind.StringMatch.Literal, compileIRVar(kind.StringMatch.Attribute)}}, nil
		}
		return map[string]any{operator: []any{compileIRVar(kind.StringMatch.Attribute), kind.StringMatch.Literal}}, nil
	case *irv1.Condition_SemverComparison:
		operator, ok := map[irv1.SemVerComparisonOperator]string{
			irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT:  ">",
			irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE: ">=",
			irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LT:  "<",
			irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LTE: "<=",
		}[kind.SemverComparison.Operator]
		if !ok {
			return nil, fmt.Errorf("unsupported semver comparison operator")
		}
		return map[string]any{"sem_ver": []any{compileIRVar(kind.SemverComparison.Attribute), operator, kind.SemverComparison.Semver}}, nil
	case *irv1.Condition_Presence:
		return nil, &capability.UnsupportedConditionError{Target: capability.TargetFlagd, Condition: capability.ConditionPresence}
	case *irv1.Condition_Logical:
		operator, ok := map[irv1.LogicalOperator]string{
			irv1.LogicalOperator_LOGICAL_OPERATOR_ALL: "and",
			irv1.LogicalOperator_LOGICAL_OPERATOR_ANY: "or",
		}[kind.Logical.Operator]
		if kind.Logical.Operator == irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE {
			return compileIRExactlyOne(kind.Logical.Conditions)
		}
		if !ok {
			return nil, fmt.Errorf("unsupported logical operator")
		}
		values := make([]any, 0, len(kind.Logical.Conditions))
		for _, child := range kind.Logical.Conditions {
			compiled, err := compileIRCondition(child)
			if err != nil {
				return nil, err
			}
			values = append(values, compiled)
		}
		return map[string]any{operator: values}, nil
	case *irv1.Condition_Negation:
		child, err := compileIRCondition(kind.Negation)
		if err != nil {
			return nil, err
		}
		return map[string]any{"!": []any{child}}, nil
	default:
		return nil, fmt.Errorf("unsupported IR condition %T", condition.GetKind())
	}
}

func compileIRExactlyOne(conditions []*irv1.Condition) (any, error) {
	clauses := make([]any, 0, len(conditions))
	for index, current := range conditions {
		currentValue, err := compileIRCondition(current)
		if err != nil {
			return nil, err
		}
		terms := []any{currentValue}
		for otherIndex, other := range conditions {
			if index == otherIndex {
				continue
			}
			otherValue, err := compileIRCondition(other)
			if err != nil {
				return nil, err
			}
			terms = append(terms, map[string]any{"!": []any{otherValue}})
		}
		clauses = append(clauses, map[string]any{"and": terms})
	}
	return map[string]any{"or": clauses}, nil
}

func compileIRBinary(attribute *irv1.AttributePath, literal *irv1.ScalarValue, operator string) (any, error) {
	if operator == "" {
		return nil, fmt.Errorf("unsupported equality operator")
	}
	comparison := map[string]any{"===": []any{compileIRVar(attribute), compileIRScalar(literal)}}
	if operator == "==" {
		return map[string]any{"if": []any{map[string]any{"missing": []any{strings.Join(attribute.Segments, ".")}}, false, comparison["==="]}}, nil
	}
	return map[string]any{"if": []any{map[string]any{"missing": []any{strings.Join(attribute.Segments, ".")}}, false, map[string]any{"!==": []any{compileIRVar(attribute), compileIRScalar(literal)}}}}, nil
}

func compileIRBinaryNumeric(condition *irv1.NumericComparisonCondition) (any, error) {
	operator := map[irv1.NumericComparisonOperator]string{
		irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT:  ">",
		irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE: ">=",
		irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LT:  "<",
		irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LTE: "<=",
	}[condition.Operator]
	if operator == "" {
		return nil, fmt.Errorf("unsupported numeric comparison operator")
	}
	return map[string]any{operator: []any{compileIRVar(condition.Attribute), compileIRNumeric(condition.Literal)}}, nil
}

func equalityOperator(operator irv1.EqualityOperator) string {
	if operator == irv1.EqualityOperator_EQUALITY_OPERATOR_NE {
		return "!="
	}
	if operator == irv1.EqualityOperator_EQUALITY_OPERATOR_EQ {
		return "=="
	}
	return ""
}

func compileIRVar(path *irv1.AttributePath) any {
	return map[string]any{"var": strings.Join(path.Segments, ".")}
}

func compileIRNumeric(value *irv1.NumericValue) any {
	switch kind := value.GetKind().(type) {
	case *irv1.NumericValue_IntValue:
		return kind.IntValue
	case *irv1.NumericValue_DoubleValue:
		return kind.DoubleValue
	default:
		return nil
	}
}

func compileIRScalar(value *irv1.ScalarValue) any {
	switch kind := value.GetKind().(type) {
	case *irv1.ScalarValue_StringValue:
		return kind.StringValue
	case *irv1.ScalarValue_BoolValue:
		return kind.BoolValue
	case *irv1.ScalarValue_IntValue:
		return kind.IntValue
	case *irv1.ScalarValue_DoubleValue:
		return kind.DoubleValue
	default:
		return nil
	}
}

func compileIRVariants(variants map[string]*irv1.VariantValue) map[string]any {
	out := make(map[string]any, len(variants))
	for name, value := range variants {
		out[name] = compileIRVariant(value)
	}
	return out
}

func compileIRVariant(value *irv1.VariantValue) any {
	switch kind := value.GetKind().(type) {
	case *irv1.VariantValue_BoolValue:
		return kind.BoolValue
	case *irv1.VariantValue_StringValue:
		return kind.StringValue
	case *irv1.VariantValue_IntValue:
		return kind.IntValue
	case *irv1.VariantValue_DoubleValue:
		return kind.DoubleValue
	case *irv1.VariantValue_NullValue:
		return nil
	case *irv1.VariantValue_ObjectValue:
		out := make(map[string]any, len(kind.ObjectValue.Fields))
		for name, child := range kind.ObjectValue.Fields {
			out[name] = compileIRVariant(child)
		}
		return out
	case *irv1.VariantValue_ListValue:
		out := make([]any, 0, len(kind.ListValue.Values))
		for _, child := range kind.ListValue.Values {
			out = append(out, compileIRVariant(child))
		}
		return out
	default:
		return nil
	}
}
