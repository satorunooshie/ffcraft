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

func TestCompileIRScalarAndPercentageContracts(t *testing.T) {
	attribute := &irv1.AttributePath{Segments: []string{"user", "id"}}
	for _, test := range []struct {
		name  string
		value *irv1.ScalarValue
		want  string
	}{
		{"string", &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}, `"x"`},
		{"bool", &irv1.ScalarValue{Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}, "true"},
		{"int", &irv1.ScalarValue{Kind: &irv1.ScalarValue_IntValue{IntValue: 7}}, "7"},
		{"double", &irv1.ScalarValue{Kind: &irv1.ScalarValue_DoubleValue{DoubleValue: 1.25}}, "1.25"},
		{"null", &irv1.ScalarValue{Kind: &irv1.ScalarValue_NullValue{NullValue: &irv1.ScalarNull{}}}, "null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			condition := &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: attribute, Literal: test.value}}}
			got, err := compileIRCondition(condition)
			encoded, marshalErr := json.Marshal(got)
			if err != nil || marshalErr != nil || !strings.Contains(string(encoded), test.want) {
				t.Fatalf("compileIRCondition() = %s, %v, %v, want %q", encoded, err, marshalErr, test.want)
			}
		})
	}
	distribution, err := compileIRAction(&irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: attribute, Weights: map[string]uint32{"b": 1, "a": 2}}}})
	if err != nil || !strings.Contains(string(mustJSON(distribution)), "33") || !strings.Contains(string(mustJSON(distribution)), "67") {
		t.Fatalf("distribution output = %#v, %v", distribution, err)
	}
	if got := compileIRVar(attribute); got.(map[string]any)["var"] != "user.id" {
		t.Fatalf("compileIRVar() = %#v", got)
	}
}

func TestCompileIRVariantNestedShapeContracts(t *testing.T) {
	variants := map[string]*irv1.VariantValue{
		"null": {Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}},
		"object": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{
			"enabled": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
			"nested":  {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_IntValue{IntValue: 7}}, {Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}}}}}},
		}}}},
		"list": {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{
			{Kind: &irv1.VariantValue_StringValue{StringValue: "x"}},
			{Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 1.5}},
		}}}},
	}
	encoded := string(mustJSON(compileIRVariants(variants)))
	for _, fragment := range []string{`"null":null`, `"enabled":true`, `"nested":[7,null]`, `"list":["x",1.5]`} {
		if !strings.Contains(encoded, fragment) {
			t.Fatalf("nested variant output = %s, missing %q", encoded, fragment)
		}
	}
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func TestCompileIRConditionErrorPropagation(t *testing.T) {
	if _, err := compileIRCondition(nil); err == nil || !strings.Contains(err.Error(), "condition is required") {
		t.Fatalf("nil condition error = %v", err)
	}
	if _, err := compileIRCondition(&irv1.Condition{}); err == nil || !strings.Contains(err.Error(), "unsupported IR condition") {
		t.Fatalf("unset condition error = %v", err)
	}
	if _, err := compileIRCondition(&irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Attribute: &irv1.AttributePath{Segments: []string{"x"}}, Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}}}}); err == nil || !strings.Contains(err.Error(), "unsupported equality") {
		t.Fatalf("unset equality operator error = %v", err)
	}
	if _, err := compileIRCondition(&irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_ALL, Conditions: []*irv1.Condition{{}}}}}); err == nil || !strings.Contains(err.Error(), "unsupported IR condition") {
		t.Fatalf("nested condition error = %v", err)
	}
}

func TestCompileIRDocumentEnvironmentSelection(t *testing.T) {
	doc := directCompilerFixture()
	doc.Flags["static"] = &irv1.Flag{
		Variants: map[string]*irv1.VariantValue{
			"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
			"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
		},
		Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}},
	}
	output, warnings, err := CompileIR(doc, "prod", CompileOptions{})
	if err != nil || len(warnings) != 0 || !strings.Contains(string(output), `"static"`) {
		t.Fatalf("CompileIR(prod) = %s, %#v, %v", output, warnings, err)
	}
	if _, _, err := CompileIR(doc, "staging", CompileOptions{}); err == nil || !strings.Contains(err.Error(), `environment "staging" not found`) {
		t.Fatalf("missing environment error = %v", err)
	}
	output, warnings, err = CompileIR(doc, "staging", CompileOptions{AllowMissingEnvironment: true})
	if err != nil || len(warnings) != 2 || !strings.Contains(warnings[0], "skipping flag") || len(output) == 0 {
		t.Fatalf("missing environment warning mode = %s, %#v, %v", output, warnings, err)
	}
}
