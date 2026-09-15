package gofeatureflag

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/capability"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/numeric"
)

func compileIRDocument(doc *irv1.Document, environment string, opts CompileOptions) ([]byte, []string, error) {
	if err := ir.Validate(doc); err != nil {
		return nil, nil, err
	}
	flags := make(map[string]flagFile, len(doc.Flags))
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
		if err := validateIRNumericTransport(source.Variants); err != nil {
			return nil, nil, fmt.Errorf("flag %q: %w", key, err)
		}
		compiled := flagFile{Variations: compileIRVariants(source.Variants)}
		defaultRule, defaultKey, err := compileIRDefaultRule(env.Base.DefaultAction)
		if err != nil {
			return nil, nil, fmt.Errorf("flag %q: %w", key, err)
		}
		compiled.DefaultRule = defaultRule
		compiled.BucketingKey = defaultKey
		compiled.Targeting, defaultKey, err = compileIRRules(env.Base.Rules)
		if err != nil {
			return nil, nil, fmt.Errorf("flag %q: %w", key, err)
		}
		if compiled.BucketingKey, err = mergeBucketingKeys(compiled.BucketingKey, defaultKey); err != nil {
			return nil, nil, fmt.Errorf("flag %q: %w", key, err)
		}
		compiled.ScheduledRollout, defaultKey, err = compileIRSchedule(env.Schedule)
		if err != nil {
			return nil, nil, fmt.Errorf("flag %q: %w", key, err)
		}
		if compiled.BucketingKey, err = mergeBucketingKeys(compiled.BucketingKey, defaultKey); err != nil {
			return nil, nil, fmt.Errorf("flag %q: %w", key, err)
		}
		flags[key] = compiled
	}
	output, err := marshalDocument(flags)
	if err != nil {
		return nil, nil, err
	}
	return output, warnings, nil
}

func compileIRDefaultRule(action *irv1.Action) (ruleResult, string, error) {
	if action == nil {
		return ruleResult{}, "", fmt.Errorf("default_action is required")
	}
	switch kind := action.GetKind().(type) {
	case *irv1.Action_Serve:
		return ruleResult{Variation: kind.Serve}, "", nil
	case *irv1.Action_Distribute:
		return compileIRDistribution(kind.Distribute)
	default:
		return ruleResult{}, "", fmt.Errorf("unsupported IR action %T", action.GetKind())
	}
}

func compileIRRules(rules []*irv1.Rule) ([]targetRule, string, error) {
	out := make([]targetRule, 0, len(rules))
	bucketingKey := ""
	for _, rule := range rules {
		if rule == nil || rule.Condition == nil || rule.Action == nil {
			return nil, "", fmt.Errorf("rule is incomplete")
		}
		query, err := compileIRCondition(rule.Condition)
		if err != nil {
			return nil, "", err
		}
		compiled := targetRule{Query: query}
		key, err := compileIRActionResult(rule.Action, &compiled)
		if err != nil {
			return nil, "", err
		}
		if bucketingKey, err = mergeBucketingKeys(bucketingKey, key); err != nil {
			return nil, "", err
		}
		out = append(out, compiled)
	}
	return out, bucketingKey, nil
}

func compileIRActionResult(action *irv1.Action, target *targetRule) (string, error) {
	switch kind := action.GetKind().(type) {
	case *irv1.Action_Serve:
		target.Variation = kind.Serve
		return "", nil
	case *irv1.Action_Distribute:
		result, key, err := compileIRDistribution(kind.Distribute)
		if err != nil {
			return "", err
		}
		target.Percentage = result.Percentage
		return key, nil
	default:
		return "", fmt.Errorf("unsupported IR action %T", action.GetKind())
	}
}

func compileIRDistribution(distribution *irv1.Distribution) (ruleResult, string, error) {
	if distribution == nil || distribution.AllocationKey == nil {
		return ruleResult{}, "", fmt.Errorf("distribution allocation_key is required")
	}
	return ruleResult{Percentage: compileIRPercentages(distribution.Weights)}, strings.Join(distribution.AllocationKey.Segments, "."), nil
}

func compileIRSchedule(schedule []*irv1.ScheduledEvaluation) ([]scheduledStepOut, string, error) {
	out := make([]scheduledStepOut, 0, len(schedule))
	bucketingKey := ""
	for index, scheduled := range schedule {
		if scheduled == nil || scheduled.Evaluation == nil || scheduled.EffectiveAt == nil {
			return nil, "", fmt.Errorf("schedule[%d] is incomplete", index)
		}
		targeting, ruleKey, err := compileIRRules(scheduled.Evaluation.Rules)
		if err != nil {
			return nil, "", fmt.Errorf("schedule[%d]: %w", index, err)
		}
		defaultRule, defaultKey, err := compileIRDefaultRule(scheduled.Evaluation.DefaultAction)
		if err != nil {
			return nil, "", fmt.Errorf("schedule[%d]: %w", index, err)
		}
		bucketingKey, err = mergeBucketingKeys(bucketingKey, ruleKey)
		if err != nil {
			return nil, "", err
		}
		bucketingKey, err = mergeBucketingKeys(bucketingKey, defaultKey)
		if err != nil {
			return nil, "", err
		}
		out = append(out, scheduledStepOut{
			Date:        scheduled.EffectiveAt.AsTime().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
			Targeting:   targeting,
			DefaultRule: &defaultRule,
		})
	}
	return out, bucketingKey, nil
}

func compileIRCondition(condition *irv1.Condition) (string, error) {
	if condition == nil {
		return "", fmt.Errorf("condition is required")
	}
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_Constant:
		if kind.Constant {
			return "true", nil
		}
		return "false", nil
	case *irv1.Condition_Equality:
		return compileIRBinary(kind.Equality.Attribute, kind.Equality.Literal, equalityOperator(kind.Equality.Operator))
	case *irv1.Condition_NumericComparison:
		operator, ok := map[irv1.NumericComparisonOperator]string{
			irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT:  "gt",
			irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE: "ge",
			irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LT:  "lt",
			irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LTE: "le",
		}[kind.NumericComparison.Operator]
		if !ok {
			return "", fmt.Errorf("unsupported numeric comparison operator")
		}
		return fmt.Sprintf("%s %s %s", compileIRVar(kind.NumericComparison.Attribute), operator, compileIRNumeric(kind.NumericComparison.Literal)), nil
	case *irv1.Condition_Membership:
		values := make([]string, 0, len(kind.Membership.Literals.Values))
		for _, literal := range kind.Membership.Literals.Values {
			values = append(values, compileIRScalar(literal))
		}
		return fmt.Sprintf("%s in [%s]", compileIRVar(kind.Membership.Attribute), strings.Join(values, ", ")), nil
	case *irv1.Condition_StringMatch:
		operator, ok := map[irv1.StringMatchOperator]string{
			irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS:    "co",
			irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH: "sw",
			irv1.StringMatchOperator_STRING_MATCH_OPERATOR_ENDS_WITH:   "ew",
		}[kind.StringMatch.Operator]
		if !ok {
			return "", fmt.Errorf("unsupported string match operator")
		}
		return fmt.Sprintf("%s %s %s", compileIRVar(kind.StringMatch.Attribute), operator, strconv.Quote(kind.StringMatch.Literal)), nil
	case *irv1.Condition_SemverComparison:
		operator, ok := map[irv1.SemVerComparisonOperator]string{
			irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT:  "gt",
			irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE: "ge",
			irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LT:  "lt",
			irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LTE: "le",
		}[kind.SemverComparison.Operator]
		if !ok {
			return "", fmt.Errorf("unsupported semver comparison operator")
		}
		return fmt.Sprintf("%s %s %s", compileIRVar(kind.SemverComparison.Attribute), operator, kind.SemverComparison.Semver), nil
	case *irv1.Condition_Presence:
		return "", &capability.UnsupportedConditionError{Target: capability.TargetGOFeatureFlag, Condition: capability.ConditionPresence}
	case *irv1.Condition_Logical:
		operator := "AND"
		if kind.Logical.Operator == irv1.LogicalOperator_LOGICAL_OPERATOR_ANY {
			operator = "OR"
		}
		if kind.Logical.Operator == irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE {
			return compileIRExactlyOne(kind.Logical.Conditions)
		}
		if kind.Logical.Operator != irv1.LogicalOperator_LOGICAL_OPERATOR_ALL && kind.Logical.Operator != irv1.LogicalOperator_LOGICAL_OPERATOR_ANY {
			return "", fmt.Errorf("unsupported logical operator")
		}
		parts := make([]string, 0, len(kind.Logical.Conditions))
		for _, child := range kind.Logical.Conditions {
			value, err := compileIRCondition(child)
			if err != nil {
				return "", err
			}
			parts = append(parts, "("+value+")")
		}
		return strings.Join(parts, " "+operator+" "), nil
	case *irv1.Condition_Negation:
		child, err := compileIRCondition(kind.Negation)
		if err != nil {
			return "", err
		}
		return "not (" + child + ")", nil
	default:
		return "", fmt.Errorf("unsupported IR condition %T", condition.GetKind())
	}
}

func compileIRExactlyOne(conditions []*irv1.Condition) (string, error) {
	clauses := make([]string, 0, len(conditions))
	for index, current := range conditions {
		value, err := compileIRCondition(current)
		if err != nil {
			return "", err
		}
		terms := []string{"(" + value + ")"}
		for otherIndex, other := range conditions {
			if index == otherIndex {
				continue
			}
			value, err := compileIRCondition(other)
			if err != nil {
				return "", err
			}
			terms = append(terms, "not ("+value+")")
		}
		clauses = append(clauses, "("+strings.Join(terms, " AND ")+")")
	}
	return strings.Join(clauses, " OR "), nil
}

func compileIRBinary(attribute *irv1.AttributePath, literal *irv1.ScalarValue, operator string) (string, error) {
	if operator == "" {
		return "", fmt.Errorf("unsupported equality operator")
	}
	return fmt.Sprintf("%s %s %s", compileIRVar(attribute), operator, compileIRScalar(literal)), nil
}

func equalityOperator(operator irv1.EqualityOperator) string {
	if operator == irv1.EqualityOperator_EQUALITY_OPERATOR_EQ {
		return "eq"
	}
	if operator == irv1.EqualityOperator_EQUALITY_OPERATOR_NE {
		return "ne"
	}
	return ""
}

func compileIRVar(path *irv1.AttributePath) string {
	return strings.Join(path.Segments, ".")
}

func compileIRNumeric(value *irv1.NumericValue) string {
	switch kind := value.GetKind().(type) {
	case *irv1.NumericValue_IntValue:
		return strconv.FormatInt(kind.IntValue, 10)
	case *irv1.NumericValue_DoubleValue:
		return strconv.FormatFloat(kind.DoubleValue, 'f', -1, 64)
	default:
		return "null"
	}
}

func compileIRScalar(value *irv1.ScalarValue) string {
	switch kind := value.GetKind().(type) {
	case *irv1.ScalarValue_StringValue:
		return strconv.Quote(kind.StringValue)
	case *irv1.ScalarValue_BoolValue:
		return strconv.FormatBool(kind.BoolValue)
	case *irv1.ScalarValue_IntValue:
		return strconv.FormatInt(kind.IntValue, 10)
	case *irv1.ScalarValue_DoubleValue:
		return strconv.FormatFloat(kind.DoubleValue, 'f', -1, 64)
	default:
		return "null"
	}
}

func compileIRPercentages(weights map[string]uint32) map[string]float64 {
	var total uint64
	for _, weight := range weights {
		total += uint64(weight)
	}
	out := make(map[string]float64, len(weights))
	for variant, weight := range weights {
		out[variant] = math.Round(float64(weight) * 100 / float64(total))
	}
	return sortedAllocations(out)
}

func validateIRNumericTransport(variants map[string]*irv1.VariantValue) error {
	for name, value := range variants {
		if err := validateIRNumericVariant(value); err != nil {
			return fmt.Errorf("variant %q: %w", name, err)
		}
	}
	return nil
}

func validateIRNumericVariant(value *irv1.VariantValue) error {
	switch kind := value.GetKind().(type) {
	case *irv1.VariantValue_IntValue:
		if !numeric.IsSafeJSONInteger(kind.IntValue) {
			return fmt.Errorf("GO Feature Flag transport cannot preserve int64 value %d; value is outside the safe JSON integer range", kind.IntValue)
		}
	case *irv1.VariantValue_ObjectValue:
		for key, child := range kind.ObjectValue.Fields {
			if err := validateIRNumericVariant(child); err != nil {
				return fmt.Errorf("object field %q: %w", key, err)
			}
		}
	case *irv1.VariantValue_ListValue:
		for index, child := range kind.ListValue.Values {
			if err := validateIRNumericVariant(child); err != nil {
				return fmt.Errorf("list[%d]: %w", index, err)
			}
		}
	}
	return nil
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
