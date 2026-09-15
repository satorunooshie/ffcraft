package flagd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestIRConditionOperatorTable(t *testing.T) {
	a := &irv1.AttributePath{Segments: []string{"user", "id"}}
	s := &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "beta"}}
	n := &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 3}}
	conditions := []struct {
		name  string
		value *irv1.Condition
		want  string
	}{
		{"constant", &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}, "true"},
		{"eq", &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: a, Literal: s}}}, "missing"},
		{"ne", &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_NE, Attribute: a, Literal: s}}}, "missing"},
		{"numeric", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE, Attribute: a, Literal: n}}}, `\u003e=`},
		{"membership", &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: a, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{s}}}}}, "in"},
		{"contains", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS, Attribute: a, Literal: "be"}}}, "in"},
		{"starts", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH, Attribute: a, Literal: "be"}}}, "starts_with"},
		{"ends", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_ENDS_WITH, Attribute: a, Literal: "ta"}}}, "ends_with"},
		{"semver", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LT, Attribute: a, Semver: "2.0.0"}}}, "sem_ver"},
		{"all", irLogical(irv1.LogicalOperator_LOGICAL_OPERATOR_ALL), "and"}, {"any", irLogical(irv1.LogicalOperator_LOGICAL_OPERATOR_ANY), "or"}, {"exactly one", irLogical(irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE), "or"},
		{"not", &irv1.Condition{Kind: &irv1.Condition_Negation{Negation: &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}}}, "!"},
	}
	for _, test := range conditions {
		t.Run(test.name, func(t *testing.T) {
			got, err := compileIRCondition(test.value)
			if err != nil || !strings.Contains(string(mustJSONFlagd(got)), test.want) {
				t.Fatalf("compileIRCondition() = %#v, %v; want %q", got, err, test.want)
			}
		})
	}
	for _, test := range []struct {
		name  string
		value *irv1.Condition
		want  string
	}{
		{"nil", nil, "condition is required"}, {"unset", &irv1.Condition{}, "unsupported IR condition"},
		{"presence", &irv1.Condition{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: a}}}, "presence"},
		{"unknown string", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Attribute: a, Operator: irv1.StringMatchOperator(99)}}}, "unsupported string match"},
		{"unknown logical", &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator(99)}}}, "unsupported logical"},
	} {
		t.Run("error/"+test.name, func(t *testing.T) {
			if _, err := compileIRCondition(test.value); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func irLogical(operator irv1.LogicalOperator) *irv1.Condition {
	return &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: operator, Conditions: []*irv1.Condition{{Kind: &irv1.Condition_Constant{Constant: true}}, {Kind: &irv1.Condition_Constant{Constant: false}}}}}}
}

func mustJSONFlagd(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func TestIRActionVariantAndBoundaryTables(t *testing.T) {
	a := &irv1.AttributePath{Segments: []string{"user", "id"}}
	actions := []struct {
		name   string
		action *irv1.Action
		want   string
	}{
		{"serve", &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}, "on"},
		{"distribution", &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: a, Weights: map[string]uint32{"on": 2, "off": 1}}}}, "fractional"},
	}
	for _, test := range actions {
		t.Run(test.name, func(t *testing.T) {
			got, err := compileIRAction(test.action)
			if err != nil || !strings.Contains(string(mustJSONFlagd(got)), test.want) {
				t.Fatalf("compileIRAction() = %#v, %v", got, err)
			}
		})
	}
	for _, test := range []struct {
		name   string
		action *irv1.Action
		want   string
	}{
		{"nil", nil, "action is required"}, {"missing key", &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{}}}, "allocation_key"}, {"unsupported", &irv1.Action{}, "unsupported IR action"},
		{"weight total exceeds flagd limit", &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: a, Weights: map[string]uint32{"on": 2_000_000_000, "off": 2_000_000_000}}}}, "exceeds maximum"},
	} {
		t.Run("action/"+test.name, func(t *testing.T) {
			if _, err := compileIRAction(test.action); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	variants := map[string]*irv1.VariantValue{
		"bool": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, "string": {Kind: &irv1.VariantValue_StringValue{StringValue: "x"}}, "int": {Kind: &irv1.VariantValue_IntValue{IntValue: 1}}, "double": {Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 1.5}}, "null": {Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}},
		"object": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"x": {Kind: &irv1.VariantValue_StringValue{StringValue: "y"}}}}}},
		"list":   {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}}}}},
	}
	if got := compileIRVariants(variants); len(got) != len(variants) {
		t.Fatalf("compileIRVariants() = %#v", got)
	}
}

func TestIRDistributionWeightsPreservePrecision(t *testing.T) {
	for _, test := range []struct {
		name    string
		weights map[string]uint32
	}{
		{"one-to-one", map[string]uint32{"a": 1, "b": 1}},
		{"one-to-two", map[string]uint32{"a": 1, "b": 2}},
		{"three-variant", map[string]uint32{"a": 1, "b": 3, "c": 7}},
		{"high-precision", map[string]uint32{"a": 1, "b": 1_000_000}},
	} {
		t.Run(test.name, func(t *testing.T) {
			action := &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: test.weights}}}
			got, err := compileIRAction(action)
			if err != nil {
				t.Fatal(err)
			}
			encoded := string(mustJSONFlagd(got))
			for variant, want := range test.weights {
				if !strings.Contains(encoded, `"`+variant+`",`+fmt.Sprint(want)) {
					t.Fatalf("encoded weight %q missing from %s", variant, encoded)
				}
			}
		})
	}
}

func TestIRDocumentEnvironmentAndEvaluationBoundaries(t *testing.T) {
	if _, _, err := CompileIR(&irv1.Document{}, "prod", CompileOptions{}); err == nil || !strings.Contains(err.Error(), "FFCRAFT_IR_INVALID_CORE") {
		t.Fatalf("invalid IR document = %v", err)
	}
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{
		"f": {
			Variants: map[string]*irv1.VariantValue{
				"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
				"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
			},
			Environments: map[string]*irv1.Environment{"prod": {
				Base: &irv1.Evaluation{
					Rules: []*irv1.Rule{{
						Condition: &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: &irv1.AttributePath{Segments: []string{"user", "segment"}}, Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "beta"}}}}},
						Action:    &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}},
					}},
					DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}},
				},
				Schedule: []*irv1.ScheduledEvaluation{{
					EffectiveAt: timestamppb.New(time.Unix(1767225600, 0)),
					Evaluation:  &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: map[string]uint32{"on": 2, "off": 1}}}}},
				}},
			}},
		},
	}}
	if _, _, err := CompileIR(doc, "prod", CompileOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CompileIR(doc, "missing", CompileOptions{}); err == nil || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("missing environment error = %v", err)
	}
	if output, warnings, err := CompileIR(doc, "missing", CompileOptions{AllowMissingEnvironment: true}); err != nil || len(warnings) != 1 || len(output) == 0 {
		t.Fatalf("missing environment warning mode = %s, %#v, %v", output, warnings, err)
	}
	if _, err := compileIREvaluation(nil); err == nil || !strings.Contains(err.Error(), "default_action") {
		t.Fatalf("nil evaluation error = %v", err)
	}
	if _, err := compileIREvaluation(&irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}, Rules: []*irv1.Rule{{}}}); err == nil || !strings.Contains(err.Error(), "rule is incomplete") {
		t.Fatalf("incomplete rule error = %v", err)
	}
	if _, err := compileIRBinaryNumeric(&irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator(99)}); err == nil || !strings.Contains(err.Error(), "unsupported numeric") {
		t.Fatalf("unknown numeric operator = %v", err)
	}
	if _, err := compileIRBinary(&irv1.AttributePath{}, &irv1.ScalarValue{}, ""); err == nil || !strings.Contains(err.Error(), "unsupported equality") {
		t.Fatalf("empty binary operator = %v", err)
	}
	if _, err := compileIRCondition(&irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator(99)}}}); err == nil || !strings.Contains(err.Error(), "unsupported equality") {
		t.Fatalf("unknown equality operator = %v", err)
	}
	if _, err := compileIREvaluation(&irv1.Evaluation{DefaultAction: &irv1.Action{}}); err == nil || !strings.Contains(err.Error(), "unsupported IR action") {
		t.Fatalf("unsupported evaluation action = %v", err)
	}
}

func TestIRScalarAndBoundaryHelpers(t *testing.T) {
	for _, test := range []struct {
		name  string
		value *irv1.ScalarValue
		want  string
	}{
		{"bool", &irv1.ScalarValue{Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}, "true"},
		{"int", &irv1.ScalarValue{Kind: &irv1.ScalarValue_IntValue{IntValue: 7}}, "7"},
		{"double", &irv1.ScalarValue{Kind: &irv1.ScalarValue_DoubleValue{DoubleValue: 1.5}}, "1.5"},
		{"string", &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}, "x"},
		{"null", &irv1.ScalarValue{Kind: &irv1.ScalarValue_NullValue{NullValue: &irv1.ScalarNull{}}}, "<nil>"},
		{"unset", &irv1.ScalarValue{}, "<nil>"},
	} {
		if got := compileIRScalar(test.value); fmt.Sprint(got) != test.want {
			t.Errorf("compileIRScalar(%s) = %#v, want %q", test.name, got, test.want)
		}
	}
	for _, test := range []struct {
		value *irv1.NumericValue
		want  string
	}{
		{&irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 7}}, "7"},
		{&irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 1.5}}, "1.5"},
		{&irv1.NumericValue{}, "<nil>"},
	} {
		if got := compileIRNumeric(test.value); fmt.Sprint(got) != test.want {
			t.Errorf("compileIRNumeric() = %#v, want %q", got, test.want)
		}
	}
	if _, err := compileIRServeVariant(nil); err == nil || !strings.Contains(err.Error(), "default_action") {
		t.Fatalf("nil serve variant error = %v", err)
	}
	if _, err := compileIRServeVariant(&irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{}}}); err == nil || !strings.Contains(err.Error(), "default_action.serve") {
		t.Fatalf("non-serve variant error = %v", err)
	}
}
