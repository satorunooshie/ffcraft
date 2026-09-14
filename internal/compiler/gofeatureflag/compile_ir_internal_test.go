package gofeatureflag

import (
	"strings"
	"testing"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func TestCompileIRScheduleAndBucketingContracts(t *testing.T) {
	serve := func(variant string) *irv1.Action { return &irv1.Action{Kind: &irv1.Action_Serve{Serve: variant}} }
	distribute := func(path string) *irv1.Action {
		return &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{path}}, Weights: map[string]uint32{"on": 1, "off": 1}}}}
	}
	steps, key, err := compileIRSchedule([]*irv1.ScheduledEvaluation{{
		EffectiveAt: timestamppb.New(time.Date(2028, 1, 1, 0, 0, 0, 123, time.UTC)),
		Evaluation:  &irv1.Evaluation{DefaultAction: distribute("user.id")},
	}, {
		EffectiveAt: timestamppb.New(time.Date(2028, 1, 2, 0, 0, 0, 0, time.UTC)),
		Evaluation:  &irv1.Evaluation{DefaultAction: serve("on")},
	}})
	if err != nil || len(steps) != 2 || key != "user.id" || steps[0].Date != "2028-01-01T00:00:00.000000123Z" || steps[1].DefaultRule.Variation != "on" {
		t.Fatalf("compileIRSchedule() = %#v, %q, %v", steps, key, err)
	}
	if _, _, err := compileIRSchedule([]*irv1.ScheduledEvaluation{{}}); err == nil || !strings.Contains(err.Error(), "schedule[0] is incomplete") {
		t.Fatalf("incomplete schedule error = %v", err)
	}
	if _, _, err := compileIRSchedule([]*irv1.ScheduledEvaluation{{EffectiveAt: timestamppb.Now(), Evaluation: &irv1.Evaluation{}}}); err == nil || !strings.Contains(err.Error(), "default_action is required") {
		t.Fatalf("invalid schedule evaluation error = %v", err)
	}
	if got, err := mergeBucketingKeys("", "user.id"); err != nil || got != "user.id" {
		t.Fatalf("mergeBucketingKeys(empty) = %q, %v", got, err)
	}
	if _, err := mergeBucketingKeys("user.id", "device.id"); err == nil || !strings.Contains(err.Error(), "multiple distribute") {
		t.Fatalf("mergeBucketingKeys(conflict) = %v", err)
	}
}

func TestCompileIRDefaultRuleContracts(t *testing.T) {
	if rule, key, err := compileIRDefaultRule(&irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}); err != nil || rule.Variation != "on" || key != "" {
		t.Fatalf("serve default rule = %#v, %q, %v", rule, key, err)
	}
	if _, _, err := compileIRDefaultRule(nil); err == nil || !strings.Contains(err.Error(), "default_action is required") {
		t.Fatalf("nil default rule error = %v", err)
	}
	if _, _, err := compileIRDefaultRule(&irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{}}}); err == nil || !strings.Contains(err.Error(), "allocation_key") {
		t.Fatalf("invalid distribution default error = %v", err)
	}
}

func TestCompileIRDocumentEnvironmentSelection(t *testing.T) {
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{
		"static": {
			Variants: map[string]*irv1.VariantValue{
				"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
				"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
			},
			Environments: map[string]*irv1.Environment{
				"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}},
			},
		},
	}}
	output, warnings, err := CompileIR(doc, "prod", CompileOptions{})
	if err != nil || len(warnings) != 0 || !strings.Contains(string(output), "defaultRule:\n        variation: \"on\"") {
		t.Fatalf("CompileIR(prod) = %s, %#v, %v", output, warnings, err)
	}
	if _, _, err := CompileIR(doc, "staging", CompileOptions{}); err == nil || !strings.Contains(err.Error(), `environment "staging" not found`) {
		t.Fatalf("missing environment error = %v", err)
	}
	output, warnings, err = CompileIR(doc, "staging", CompileOptions{AllowMissingEnvironment: true})
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "skipping flag") || len(output) == 0 {
		t.Fatalf("missing environment warning mode = %s, %#v, %v", output, warnings, err)
	}
}
