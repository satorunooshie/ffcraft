package flagd

import (
	"encoding/json"
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

func TestCompileIRConditionOperatorTable(t *testing.T) {
	attribute := &irv1.AttributePath{Segments: []string{"user", "id"}}
	stringValue := &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "beta"}}
	numeric := &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 3}}
	tests := []struct {
		name      string
		condition *irv1.Condition
		want      string
	}{
		{"constant", &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}, "true"},
		{"equality", &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_NE, Attribute: attribute, Literal: stringValue}}}, "!="},
		{"numeric", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE, Attribute: attribute, Literal: numeric}}}, `\u003e=`},
		{"membership", &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: attribute, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{stringValue}}}}}, "in"},
		{"contains", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS, Attribute: attribute, Literal: "be"}}}, "in"},
		{"starts with", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH, Attribute: attribute, Literal: "be"}}}, "starts_with"},
		{"ends with", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_ENDS_WITH, Attribute: attribute, Literal: "ta"}}}, "ends_with"},
		{"semver", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LT, Attribute: attribute, Semver: "2.0.0"}}}, "sem_ver"},
		{"all", logical(irv1.LogicalOperator_LOGICAL_OPERATOR_ALL), "and"},
		{"any", logical(irv1.LogicalOperator_LOGICAL_OPERATOR_ANY), "or"},
		{"exactly one", logical(irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE), "or"},
		{"negation", &irv1.Condition{Kind: &irv1.Condition_Negation{Negation: &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}}}, "!"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := compileIRCondition(test.condition)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(value)
			if err != nil || !strings.Contains(string(encoded), test.want) {
				t.Fatalf("compiled condition = %s, want %q", encoded, test.want)
			}
		})
	}
}

func logical(operator irv1.LogicalOperator) *irv1.Condition {
	return &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{
		Operator: operator,
		Conditions: []*irv1.Condition{
			{Kind: &irv1.Condition_Constant{Constant: true}},
			{Kind: &irv1.Condition_Constant{Constant: false}},
		},
	}}}
}

func TestCompileIRActionAndCapabilityErrors(t *testing.T) {
	tests := []struct {
		name   string
		action *irv1.Action
		want   string
	}{
		{"nil action", nil, "action is required"},
		{"missing distribution key", &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{}}}, "allocation_key"},
		{"unsupported action", &irv1.Action{}, "unsupported IR action"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileIRAction(test.action)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compileIRAction() error = %v, want %q", err, test.want)
			}
		})
	}
	if _, err := compileIRCondition(&irv1.Condition{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: &irv1.AttributePath{Segments: []string{"x"}}}}}); err == nil || !strings.Contains(err.Error(), "presence") {
		t.Fatalf("presence condition error = %v", err)
	}
}
