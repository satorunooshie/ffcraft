// Package runtimeeval evaluates the target-independent condition semantics of
// the normalized IR. It deliberately operates on protobuf IR, not on an
// authoring or target-specific representation.
package runtimeeval

import (
	"math"
	"math/big"
	"regexp"
	"strings"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

// Context is a structural runtime attribute tree. A nil value is an explicit
// null; a missing map entry is absence.
type Context map[string]any

// Evaluate returns the IR condition result. Invalid runtime values and type
// mismatches are false by contract.
func Evaluate(condition *irv1.Condition, context Context) bool {
	if condition == nil {
		return false
	}
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_Constant:
		return kind.Constant
	case *irv1.Condition_Equality:
		if kind.Equality == nil {
			return false
		}
		value, ok := lookup(context, kind.Equality.Attribute)
		if !ok || !equalityCompatible(value, kind.Equality.Literal) {
			return false
		}
		matched := equal(value, kind.Equality.Literal)
		switch kind.Equality.Operator {
		case irv1.EqualityOperator_EQUALITY_OPERATOR_EQ:
			return matched
		case irv1.EqualityOperator_EQUALITY_OPERATOR_NE:
			return !matched
		default:
			return false
		}
	case *irv1.Condition_NumericComparison:
		if kind.NumericComparison == nil {
			return false
		}
		value, ok := lookup(context, kind.NumericComparison.Attribute)
		left, leftOK := number(value)
		right, rightOK := numericValue(kind.NumericComparison.Literal)
		if !ok || !leftOK || !rightOK {
			return false
		}
		return compareNumbers(left, right, kind.NumericComparison.Operator)
	case *irv1.Condition_Membership:
		if kind.Membership == nil || kind.Membership.Literals == nil {
			return false
		}
		value, ok := lookup(context, kind.Membership.Attribute)
		if !ok {
			return false
		}
		for _, literal := range kind.Membership.Literals.Values {
			if equal(value, literal) {
				return true
			}
		}
		return false
	case *irv1.Condition_StringMatch:
		if kind.StringMatch == nil {
			return false
		}
		value, ok := lookup(context, kind.StringMatch.Attribute)
		text, textOK := value.(string)
		if !ok || !textOK {
			return false
		}
		switch kind.StringMatch.Operator {
		case irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS:
			return strings.Contains(text, kind.StringMatch.Literal)
		case irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH:
			return strings.HasPrefix(text, kind.StringMatch.Literal)
		case irv1.StringMatchOperator_STRING_MATCH_OPERATOR_ENDS_WITH:
			return strings.HasSuffix(text, kind.StringMatch.Literal)
		default:
			return false
		}
	case *irv1.Condition_SemverComparison:
		if kind.SemverComparison == nil {
			return false
		}
		value, ok := lookup(context, kind.SemverComparison.Attribute)
		left, leftOK := value.(string)
		right, rightOK := parseSemver(kind.SemverComparison.Semver)
		if !ok || !leftOK || !rightOK {
			return false
		}
		actual, valid := parseSemver(left)
		if !valid {
			return false
		}
		return compareSemver(actual, right, kind.SemverComparison.Operator)
	case *irv1.Condition_Presence:
		if kind.Presence == nil {
			return false
		}
		_, ok := lookup(context, kind.Presence.Attribute)
		return ok
	case *irv1.Condition_Logical:
		return evaluateLogical(kind.Logical, context)
	case *irv1.Condition_Negation:
		return !Evaluate(kind.Negation, context)
	default:
		return false
	}
}

func evaluateLogical(condition *irv1.LogicalCondition, context Context) bool {
	if condition == nil {
		return false
	}
	matched := 0
	for _, child := range condition.Conditions {
		if Evaluate(child, context) {
			matched++
		}
	}
	switch condition.Operator {
	case irv1.LogicalOperator_LOGICAL_OPERATOR_ALL:
		return matched == len(condition.Conditions)
	case irv1.LogicalOperator_LOGICAL_OPERATOR_ANY:
		return matched > 0
	case irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE:
		return matched == 1
	default:
		return false
	}
}

func lookup(context Context, path *irv1.AttributePath) (any, bool) {
	if path == nil || len(path.Segments) == 0 {
		return nil, false
	}
	var current any = map[string]any(context)
	for _, segment := range path.Segments {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func equal(actual any, literal *irv1.ScalarValue) bool {
	if literal == nil {
		return false
	}
	if left, ok := number(actual); ok {
		if right, ok := scalarNumber(literal); ok {
			return compareExact(left, right) == 0
		}
		return false
	}
	switch kind := literal.GetKind().(type) {
	case *irv1.ScalarValue_StringValue:
		value, ok := actual.(string)
		return ok && value == kind.StringValue
	case *irv1.ScalarValue_BoolValue:
		value, ok := actual.(bool)
		return ok && value == kind.BoolValue
	case *irv1.ScalarValue_NullValue:
		return actual == nil
	default:
		return false
	}
}

func equalityCompatible(actual any, literal *irv1.ScalarValue) bool {
	if literal == nil {
		return false
	}
	if _, ok := number(actual); ok {
		_, literalOK := scalarNumber(literal)
		return literalOK
	}
	switch literal.GetKind().(type) {
	case *irv1.ScalarValue_StringValue:
		_, ok := actual.(string)
		return ok
	case *irv1.ScalarValue_BoolValue:
		_, ok := actual.(bool)
		return ok
	case *irv1.ScalarValue_NullValue:
		return actual == nil
	default:
		return false
	}
}

type exactNumber struct {
	rational *big.Rat
}

func number(value any) (exactNumber, bool) {
	switch value := value.(type) {
	case int:
		return integerNumber(int64(value)), true
	case int8:
		return integerNumber(int64(value)), true
	case int16:
		return integerNumber(int64(value)), true
	case int32:
		return integerNumber(int64(value)), true
	case int64:
		return integerNumber(value), true
	case uint:
		return unsignedNumber(uint64(value))
	case uint8:
		return unsignedNumber(uint64(value))
	case uint16:
		return unsignedNumber(uint64(value))
	case uint32:
		return unsignedNumber(uint64(value))
	case uint64:
		return unsignedNumber(value)
	case float32:
		return floatNumber(float64(value))
	case float64:
		return floatNumber(value)
	default:
		return exactNumber{}, false
	}
}

func integerNumber(value int64) exactNumber {
	return exactNumber{rational: new(big.Rat).SetInt64(value)}
}

func unsignedNumber(value uint64) (exactNumber, bool) {
	return exactNumber{rational: new(big.Rat).SetUint64(value)}, true
}

func floatNumber(value float64) (exactNumber, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return exactNumber{}, false
	}
	rational := new(big.Rat)
	if rational.SetFloat64(value) == nil {
		return exactNumber{}, false
	}
	return exactNumber{rational: rational}, true
}

func scalarNumber(value *irv1.ScalarValue) (exactNumber, bool) {
	switch kind := value.GetKind().(type) {
	case *irv1.ScalarValue_IntValue:
		return integerNumber(kind.IntValue), true
	case *irv1.ScalarValue_DoubleValue:
		return floatNumber(kind.DoubleValue)
	default:
		return exactNumber{}, false
	}
}

func numericValue(value *irv1.NumericValue) (exactNumber, bool) {
	if value == nil {
		return exactNumber{}, false
	}
	switch kind := value.GetKind().(type) {
	case *irv1.NumericValue_IntValue:
		return integerNumber(kind.IntValue), true
	case *irv1.NumericValue_DoubleValue:
		return floatNumber(kind.DoubleValue)
	default:
		return exactNumber{}, false
	}
}

func compareExact(left, right exactNumber) int { return left.rational.Cmp(right.rational) }

func compareNumbers(left, right exactNumber, operator irv1.NumericComparisonOperator) bool {
	comparison := compareExact(left, right)
	switch operator {
	case irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT:
		return comparison > 0
	case irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE:
		return comparison >= 0
	case irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LT:
		return comparison < 0
	case irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LTE:
		return comparison <= 0
	default:
		return false
	}
}

type semver struct {
	major, minor, patch string
	pre                 []string
}

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func parseSemver(value string) (semver, bool) {
	matches := semverPattern.FindStringSubmatch(value)
	if matches == nil {
		return semver{}, false
	}
	var pre []string
	if matches[4] != "" {
		pre = strings.Split(matches[4], ".")
		for _, identifier := range pre {
			if len(identifier) > 1 && identifier[0] == '0' && allDigits(identifier) {
				return semver{}, false
			}
		}
	}
	return semver{major: matches[1], minor: matches[2], patch: matches[3], pre: pre}, true
}

func allDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value != ""
}

func compareSemver(left, right semver, operator irv1.SemVerComparisonOperator) bool {
	comparison := compareSemverValue(left, right)
	switch operator {
	case irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT:
		return comparison > 0
	case irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE:
		return comparison >= 0
	case irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LT:
		return comparison < 0
	case irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LTE:
		return comparison <= 0
	default:
		return false
	}
}

func compareSemverValue(left, right semver) int {
	for _, pair := range [][2]string{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		comparison := compareUnsignedDecimal(pair[0], pair[1])
		if comparison < 0 {
			return -1
		}
		if comparison > 0 {
			return 1
		}
	}
	if len(left.pre) == 0 || len(right.pre) == 0 {
		if len(left.pre) == len(right.pre) {
			return 0
		}
		if len(left.pre) == 0 {
			return 1
		}
		return -1
	}
	for index := 0; index < len(left.pre) && index < len(right.pre); index++ {
		leftID, rightID := left.pre[index], right.pre[index]
		leftNumeric, rightNumeric := allDigits(leftID), allDigits(rightID)
		if leftNumeric && rightNumeric {
			comparison := compareUnsignedDecimal(leftID, rightID)
			if comparison < 0 {
				return -1
			}
			if comparison > 0 {
				return 1
			}
		} else if leftNumeric != rightNumeric {
			if leftNumeric {
				return -1
			}
			return 1
		} else if leftID < rightID {
			return -1
		} else if leftID > rightID {
			return 1
		}
	}
	if len(left.pre) < len(right.pre) {
		return -1
	}
	if len(left.pre) > len(right.pre) {
		return 1
	}
	return 0
}

func compareUnsignedDecimal(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
