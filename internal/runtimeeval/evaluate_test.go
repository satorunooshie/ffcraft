package runtimeeval

import (
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
