package gofeatureflag

import (
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

func TestCompileIRConditionExpressionTable(t *testing.T) {
	attribute := &irv1.AttributePath{Segments: []string{"user", "id"}}
	stringValue := &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "beta"}}
	tests := []struct {
		name      string
		condition *irv1.Condition
		want      string
	}{
		{"constant true", &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}, "true"},
		{"constant false", &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: false}}, "false"},
		{"equality", &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: attribute, Literal: stringValue}}}, `user.id eq "beta"`},
		{"numeric", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT, Attribute: attribute, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 1.5}}}}}, "user.id gt 1.5"},
		{"membership", &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: attribute, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{stringValue}}}}}, `user.id in ["beta"]`},
		{"contains", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS, Attribute: attribute, Literal: "be"}}}, `user.id co "be"`},
		{"semver", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE, Attribute: attribute, Semver: "1.2.3"}}}, "user.id ge 1.2.3"},
		{"logical", logicalCondition(irv1.LogicalOperator_LOGICAL_OPERATOR_ALL), "(true) AND (false)"},
		{"exactly one", logicalCondition(irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE), "((true) AND not (false)) OR ((false) AND not (true))"},
		{"negation", &irv1.Condition{Kind: &irv1.Condition_Negation{Negation: &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}}}, "not (true)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := compileIRCondition(test.condition)
			if err != nil || got != test.want {
				t.Fatalf("compileIRCondition() = %q, %v, want %q", got, err, test.want)
			}
		})
	}
}

func logicalCondition(operator irv1.LogicalOperator) *irv1.Condition {
	return &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{
		Operator: operator,
		Conditions: []*irv1.Condition{
			{Kind: &irv1.Condition_Constant{Constant: true}},
			{Kind: &irv1.Condition_Constant{Constant: false}},
		},
	}}}
}

func TestCompileIRVariantAndTransportContracts(t *testing.T) {
	variants := map[string]*irv1.VariantValue{
		"bool":   {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
		"string": {Kind: &irv1.VariantValue_StringValue{StringValue: "x"}},
		"int":    {Kind: &irv1.VariantValue_IntValue{IntValue: 7}},
		"double": {Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 1.5}},
		"null":   {Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}},
		"object": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"nested": {Kind: &irv1.VariantValue_StringValue{StringValue: "y"}}}}}},
		"list":   {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}}}}},
	}
	if got := compileIRVariants(variants); len(got) != len(variants) || got["int"] != int64(7) {
		t.Fatalf("compileIRVariants() = %#v", got)
	}
	tests := []struct {
		name     string
		variants map[string]*irv1.VariantValue
		want     string
	}{
		{"unsafe root", map[string]*irv1.VariantValue{"x": {Kind: &irv1.VariantValue_IntValue{IntValue: 1 << 53}}}, "outside the safe JSON"},
		{"unsafe nested object", map[string]*irv1.VariantValue{"x": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"id": {Kind: &irv1.VariantValue_IntValue{IntValue: 1 << 53}}}}}}}, "object field"},
		{"unsafe nested list", map[string]*irv1.VariantValue{"x": {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_IntValue{IntValue: 1 << 53}}}}}}}, "list[0]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateIRNumericTransport(test.variants); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateIRNumericTransport() error = %v, want %q", err, test.want)
			}
		})
	}
}
