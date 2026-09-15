package runtimeeval

import (
	"math"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

func TestEvaluate(t *testing.T) {
	path := func(segments ...string) *irv1.AttributePath { return &irv1.AttributePath{Segments: segments} }
	integer := func(value int64) *irv1.ScalarValue {
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_IntValue{IntValue: value}}
	}
	float := func(value float64) *irv1.ScalarValue {
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_DoubleValue{DoubleValue: value}}
	}
	f64 := func(value float64) float64 { return value }
	stringValue := func(value string) *irv1.ScalarValue {
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: value}}
	}
	condition := func(kind any) *irv1.Condition {
		result := &irv1.Condition{}
		switch kind := kind.(type) {
		case *irv1.Condition_Constant:
			result.Kind = kind
		case *irv1.Condition_Equality:
			result.Kind = kind
		case *irv1.Condition_NumericComparison:
			result.Kind = kind
		case *irv1.Condition_Membership:
			result.Kind = kind
		case *irv1.Condition_StringMatch:
			result.Kind = kind
		case *irv1.Condition_SemverComparison:
			result.Kind = kind
		case *irv1.Condition_Presence:
			result.Kind = kind
		case *irv1.Condition_Logical:
			result.Kind = kind
		case *irv1.Condition_Negation:
			result.Kind = kind
		default:
			t.Fatal("unsupported test condition kind")
		}
		return result
	}

	tests := []struct {
		name    string
		context Context
		input   *irv1.Condition
		want    bool
	}{
		{
			name:    "lossless int64 and double equality",
			context: Context{"n": f64(9007199254740992)},
			input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{
				Operator:  irv1.EqualityOperator_EQUALITY_OPERATOR_EQ,
				Attribute: path("n"),
				Literal:   integer(9007199254740993),
			}}),
			want: false,
		},
		{
			name:    "lossless numeric equality when mathematically equal",
			context: Context{"n": f64(9007199254740992)},
			input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{
				Operator:  irv1.EqualityOperator_EQUALITY_OPERATOR_EQ,
				Attribute: path("n"),
				Literal:   integer(9007199254740992),
			}}),
			want: true,
		},
		{
			name:    "present distinguishes missing from null",
			context: Context{"null": nil, "value": "x"},
			input: condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{
				Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_ALL,
				Conditions: []*irv1.Condition{
					condition(&irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: path("null")}}),
					condition(&irv1.Condition_Negation{Negation: condition(&irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: path("missing")}})}),
				},
			}}),
			want: true,
		},
		{
			name:    "null equality does not match missing",
			context: Context{},
			input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{
				Operator:  irv1.EqualityOperator_EQUALITY_OPERATOR_EQ,
				Attribute: path("missing"),
				Literal:   &irv1.ScalarValue{Kind: &irv1.ScalarValue_NullValue{NullValue: &irv1.ScalarNull{}}},
			}}),
			want: false,
		},
		{
			name:    "semver prerelease ordering",
			context: Context{"version": "1.0.0-rc.1"},
			input: condition(&irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{
				Operator:  irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LT,
				Attribute: path("version"),
				Semver:    "1.0.0",
			}}),
			want: true,
		},
		{
			name:    "invalid runtime semver is false",
			context: Context{"version": "1.0.0-01"},
			input: condition(&irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{
				Operator:  irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE,
				Attribute: path("version"),
				Semver:    "1.0.0",
			}}),
			want: false,
		},
		{
			name:    "string contains rejects non-string",
			context: Context{"value": int64(1)},
			input: condition(&irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{
				Operator:  irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS,
				Attribute: path("value"),
				Literal:   "1",
			}}),
			want: false,
		},
		{
			name:    "numeric ordering",
			context: Context{"n": int64(9007199254740993)},
			input: condition(&irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{
				Operator:  irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT,
				Attribute: path("n"),
				Literal:   &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 9007199254740992}},
			}}),
			want: true,
		},
		{
			name:    "membership numeric domain",
			context: Context{"n": int64(2)},
			input: condition(&irv1.Condition_Membership{Membership: &irv1.MembershipCondition{
				Attribute: path("n"),
				Literals:  &irv1.ScalarList{Values: []*irv1.ScalarValue{stringValue("1"), float(2)}},
			}}),
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Evaluate(test.input, test.context); got != test.want {
				t.Fatalf("Evaluate() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestEvaluateOperatorTruthTable(t *testing.T) {
	path := func(name string) *irv1.AttributePath { return &irv1.AttributePath{Segments: []string{name}} }
	stringLiteral := func(value string) *irv1.ScalarValue {
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: value}}
	}
	condition := func(kind any) *irv1.Condition {
		result := &irv1.Condition{}
		switch kind := kind.(type) {
		case *irv1.Condition_Constant:
			result.Kind = kind
		case *irv1.Condition_Equality:
			result.Kind = kind
		case *irv1.Condition_NumericComparison:
			result.Kind = kind
		case *irv1.Condition_Membership:
			result.Kind = kind
		case *irv1.Condition_StringMatch:
			result.Kind = kind
		case *irv1.Condition_SemverComparison:
			result.Kind = kind
		case *irv1.Condition_Presence:
			result.Kind = kind
		case *irv1.Condition_Logical:
			result.Kind = kind
		case *irv1.Condition_Negation:
			result.Kind = kind
		default:
			t.Fatal("unsupported condition kind")
		}
		return result
	}
	tests := []struct {
		name    string
		input   *irv1.Condition
		context Context
		want    bool
	}{
		{name: "nil condition", input: nil, want: false},
		{name: "constant true", input: condition(&irv1.Condition_Constant{Constant: true}), want: true},
		{name: "constant false", input: condition(&irv1.Condition_Constant{Constant: false}), want: false},
		{name: "equality string", input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: path("value"), Literal: stringLiteral("on")}}), context: Context{"value": "on"}, want: true},
		{name: "inequality string", input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_NE, Attribute: path("value"), Literal: stringLiteral("on")}}), context: Context{"value": "off"}, want: true},
		{name: "equality bool", input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: path("value"), Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}}}), context: Context{"value": true}, want: true},
		{name: "equality type mismatch", input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: path("value"), Literal: stringLiteral("1")}}), context: Context{"value": 1}, want: false},
		{name: "inequality type mismatch is false", input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_NE, Attribute: path("value"), Literal: stringLiteral("1")}}), context: Context{"value": 1}, want: false},
		{name: "inequality non-finite number is false", input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_NE, Attribute: path("value"), Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_IntValue{IntValue: 1}}}}), context: Context{"value": math.NaN()}, want: false},
		{name: "equality explicit null", input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: path("value"), Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_NullValue{NullValue: &irv1.ScalarNull{}}}}}), context: Context{"value": nil}, want: true},
		{name: "inequality explicit null", input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_NE, Attribute: path("value"), Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_NullValue{NullValue: &irv1.ScalarNull{}}}}}), context: Context{"value": nil}, want: false},
		{name: "inequality missing is false", input: condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_NE, Attribute: path("value"), Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_NullValue{NullValue: &irv1.ScalarNull{}}}}}), context: Context{}, want: false},
		{name: "numeric gt", input: condition(&irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT, Attribute: path("value"), Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}}}), context: Context{"value": int64(2)}, want: true},
		{name: "numeric gte equal", input: condition(&irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE, Attribute: path("value"), Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 2}}}}), context: Context{"value": int64(2)}, want: true},
		{name: "numeric lt", input: condition(&irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LT, Attribute: path("value"), Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 3}}}}), context: Context{"value": int64(2)}, want: true},
		{name: "numeric lte", input: condition(&irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LTE, Attribute: path("value"), Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 2}}}}), context: Context{"value": float32(2)}, want: true},
		{name: "numeric rejects string", input: condition(&irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT, Attribute: path("value"), Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}}}), context: Context{"value": "2"}, want: false},
		{name: "numeric unsigned", input: condition(&irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE, Attribute: path("value"), Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 2}}}}), context: Context{"value": uint64(2)}, want: true},
		{name: "numeric rejects nan", input: condition(&irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT, Attribute: path("value"), Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}}}), context: Context{"value": math.NaN()}, want: false},
		{name: "membership string", input: condition(&irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: path("value"), Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{stringLiteral("on"), stringLiteral("off")}}}}), context: Context{"value": "off"}, want: true},
		{name: "membership missing", input: condition(&irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: path("value"), Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{stringLiteral("on")}}}}), context: Context{}, want: false},
		{name: "membership bool", input: condition(&irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: path("value"), Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{{Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}}}}}), context: Context{"value": true}, want: true},
		{name: "contains", input: condition(&irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS, Attribute: path("value"), Literal: "bc"}}), context: Context{"value": "abcd"}, want: true},
		{name: "starts with", input: condition(&irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH, Attribute: path("value"), Literal: "ab"}}), context: Context{"value": "abcd"}, want: true},
		{name: "ends with", input: condition(&irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_ENDS_WITH, Attribute: path("value"), Literal: "cd"}}), context: Context{"value": "abcd"}, want: true},
		{name: "string unknown operator", input: condition(&irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Attribute: path("value"), Literal: "abcd"}}), context: Context{"value": "abcd"}, want: false},
		{name: "semver gt", input: condition(&irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT, Attribute: path("value"), Semver: "1.0.0"}}), context: Context{"value": "2.0.0"}, want: true},
		{name: "semver gte", input: condition(&irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE, Attribute: path("value"), Semver: "2.0.0"}}), context: Context{"value": "2.0.0"}, want: true},
		{name: "semver lt", input: condition(&irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LT, Attribute: path("value"), Semver: "2.0.0"}}), context: Context{"value": "1.0.0"}, want: true},
		{name: "semver lte", input: condition(&irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LTE, Attribute: path("value"), Semver: "2.0.0"}}), context: Context{"value": "2.0.0"}, want: true},
		{name: "semver invalid runtime", input: condition(&irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE, Attribute: path("value"), Semver: "1.0.0"}}), context: Context{"value": "invalid"}, want: false},
		{name: "presence null", input: condition(&irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: path("value")}}), context: Context{"value": nil}, want: true},
		{name: "presence missing", input: condition(&irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: path("value")}}), context: Context{}, want: false},
		{name: "logical any", input: condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_ANY, Conditions: []*irv1.Condition{condition(&irv1.Condition_Constant{Constant: false}), condition(&irv1.Condition_Constant{Constant: true})}}}), want: true},
		{name: "logical exactly one", input: condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE, Conditions: []*irv1.Condition{condition(&irv1.Condition_Constant{Constant: true}), condition(&irv1.Condition_Constant{Constant: false})}}}), want: true},
		{name: "logical all false", input: condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_ALL, Conditions: []*irv1.Condition{condition(&irv1.Condition_Constant{Constant: true}), condition(&irv1.Condition_Constant{Constant: false})}}}), want: false},
		{name: "logical any false", input: condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_ANY, Conditions: []*irv1.Condition{condition(&irv1.Condition_Constant{Constant: false}), condition(&irv1.Condition_Constant{Constant: false})}}}), want: false},
		{name: "logical exactly zero", input: condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE, Conditions: []*irv1.Condition{condition(&irv1.Condition_Constant{Constant: false}), condition(&irv1.Condition_Constant{Constant: false})}}}), want: false},
		{name: "logical exactly one of three", input: condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE, Conditions: []*irv1.Condition{condition(&irv1.Condition_Constant{Constant: false}), condition(&irv1.Condition_Constant{Constant: true}), condition(&irv1.Condition_Constant{Constant: false})}}}), want: true},
		{name: "logical exactly two of three", input: condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE, Conditions: []*irv1.Condition{condition(&irv1.Condition_Constant{Constant: true}), condition(&irv1.Condition_Constant{Constant: true}), condition(&irv1.Condition_Constant{Constant: false})}}}), want: false},
		{name: "negation", input: condition(&irv1.Condition_Negation{Negation: condition(&irv1.Condition_Constant{Constant: false})}), want: true},
		{name: "invalid nil equality", input: condition(&irv1.Condition_Equality{}), context: Context{"value": "on"}, want: false},
		{name: "invalid nil logical", input: condition(&irv1.Condition_Logical{}), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Evaluate(test.input, test.context); got != test.want {
				t.Fatalf("Evaluate() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestExactNumberConversionTable(t *testing.T) {
	tests := []struct {
		name  string
		value any
		valid bool
	}{
		{name: "int", value: int(1), valid: true},
		{name: "int8", value: int8(1), valid: true},
		{name: "int16", value: int16(1), valid: true},
		{name: "int32", value: int32(1), valid: true},
		{name: "int64", value: int64(1), valid: true},
		{name: "uint", value: uint(1), valid: true},
		{name: "uint8", value: uint8(1), valid: true},
		{name: "uint16", value: uint16(1), valid: true},
		{name: "uint32", value: uint32(1), valid: true},
		{name: "uint64", value: uint64(1), valid: true},
		{name: "float32", value: float32(1), valid: true},
		{name: "float64", value: float64(1), valid: true},
		{name: "nan", value: math.NaN(), valid: false},
		{name: "positive infinity", value: math.Inf(1), valid: false},
		{name: "negative infinity", value: math.Inf(-1), valid: false},
		{name: "unsupported", value: struct{}{}, valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, valid := number(test.value)
			if valid != test.valid {
				t.Fatalf("number(%T) valid = %v, want %v", test.value, valid, test.valid)
			}
		})
	}
}

func TestScalarAndSemverHelperContracts(t *testing.T) {
	stringValue := func(value string) *irv1.ScalarValue {
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: value}}
	}
	intValue := func(value int64) *irv1.ScalarValue {
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_IntValue{IntValue: value}}
	}
	for _, test := range []struct {
		name    string
		actual  any
		literal *irv1.ScalarValue
		want    bool
	}{
		{name: "string equal", actual: "x", literal: stringValue("x"), want: true},
		{name: "int equal", actual: int64(2), literal: intValue(2), want: true},
		{name: "double equal", actual: float64(2), literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_DoubleValue{DoubleValue: 2}}, want: true},
		{name: "bool mismatch", actual: true, literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_BoolValue{BoolValue: false}}, want: false},
		{name: "nil literal", actual: nil, literal: nil, want: false},
		{name: "unsupported actual", actual: []string{"x"}, literal: stringValue("x"), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := equal(test.actual, test.literal); got != test.want {
				t.Fatalf("equal() = %v, want %v", got, test.want)
			}
		})
	}
	for _, test := range []struct {
		literal string
		valid   bool
	}{
		{literal: "0.0.0", valid: true},
		{literal: "1.2.3-alpha.1+build.5", valid: true},
		{literal: "01.2.3", valid: false},
		{literal: "1.2", valid: false},
		{literal: "", valid: false},
	} {
		t.Run("semver/"+test.literal, func(t *testing.T) {
			_, valid := parseSemver(test.literal)
			if valid != test.valid {
				t.Fatalf("parseSemver(%q) valid = %v, want %v", test.literal, valid, test.valid)
			}
		})
	}
}

func TestComparisonHelperTables(t *testing.T) {
	left := integerNumber(2)
	right := integerNumber(2)
	for _, test := range []struct {
		operator irv1.NumericComparisonOperator
		want     bool
	}{
		{operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT, want: false},
		{operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE, want: true},
		{operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LT, want: false},
		{operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LTE, want: true},
		{operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_UNSPECIFIED, want: false},
	} {
		if got := compareNumbers(left, right, test.operator); got != test.want {
			t.Fatalf("compareNumbers(%v) = %v, want %v", test.operator, got, test.want)
		}
	}
	semverValue := func(value string) semver {
		parsed, valid := parseSemver(value)
		if !valid {
			t.Fatalf("parseSemver(%q) failed", value)
		}
		return parsed
	}
	for _, test := range []struct {
		name        string
		left, right string
		want        int
	}{
		{name: "major", left: "2.0.0", right: "1.0.0", want: 1},
		{name: "minor", left: "1.2.0", right: "1.1.0", want: 1},
		{name: "patch", left: "1.0.2", right: "1.0.1", want: 1},
		{name: "release beats prerelease", left: "1.0.0", right: "1.0.0-rc.1", want: 1},
		{name: "numeric prerelease", left: "1.0.0-2", right: "1.0.0-10", want: -1},
		{name: "numeric beats text", left: "1.0.0-1", right: "1.0.0-alpha", want: -1},
		{name: "text lexical", left: "1.0.0-beta", right: "1.0.0-alpha", want: 1},
		{name: "short prerelease", left: "1.0.0-alpha", right: "1.0.0-alpha.1", want: -1},
		{name: "equal build metadata", left: "1.0.0+one", right: "1.0.0+two", want: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := compareSemverValue(semverValue(test.left), semverValue(test.right))
			if got != test.want {
				t.Fatalf("compareSemverValue() = %d, want %d", got, test.want)
			}
		})
	}
	if compareSemver(semverValue("1.0.0"), semverValue("1.0.0"), irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_UNSPECIFIED) {
		t.Fatal("unspecified SemVer operator matched")
	}
}

func TestRuntimeInternalBoundaryHelpers(t *testing.T) {
	if _, ok := numericValue(nil); ok {
		t.Fatal("numericValue(nil) unexpectedly succeeded")
	}
	if _, ok := numericValue(&irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}); !ok {
		t.Fatal("numericValue(int) failed")
	}
	if _, ok := numericValue(&irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 1.5}}); !ok {
		t.Fatal("numericValue(double) failed")
	}
	if _, ok := numericValue(&irv1.NumericValue{}); ok {
		t.Fatal("numericValue(empty) unexpectedly succeeded")
	}
	if _, ok := scalarNumber(&irv1.ScalarValue{Kind: &irv1.ScalarValue_NullValue{NullValue: &irv1.ScalarNull{}}}); ok {
		t.Fatal("scalarNumber(null) unexpectedly succeeded")
	}
	if _, ok := scalarNumber(nil); ok {
		t.Fatal("scalarNumber(nil) unexpectedly succeeded")
	}
	if value, ok := lookup(Context{"user": map[string]any{"id": "u1"}}, &irv1.AttributePath{Segments: []string{"user", "id"}}); !ok || value != "u1" {
		t.Fatalf("nested lookup = %#v, %v", value, ok)
	}
	if _, ok := lookup(Context{"user": "u1"}, &irv1.AttributePath{Segments: []string{"user", "id"}}); ok {
		t.Fatal("lookup traversed a non-object value")
	}
	if _, ok := lookup(Context{}, &irv1.AttributePath{}); ok {
		t.Fatal("lookup accepted an empty path")
	}
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "", want: false},
		{value: "123", want: true},
		{value: "12a", want: false},
	} {
		if got := allDigits(test.value); got != test.want {
			t.Fatalf("allDigits(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}
