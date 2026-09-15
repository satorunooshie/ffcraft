package gofeatureflag

import (
	"strings"
	"testing"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestIRConditionExpressionTable(t *testing.T) {
	a := &irv1.AttributePath{Segments: []string{"user", "id"}}
	s := &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "beta"}}
	conditions := []struct {
		name  string
		value *irv1.Condition
		want  string
	}{
		{"true", &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}, "true"},
		{"false", &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: false}}, "false"},
		{"eq", &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: a, Literal: s}}}, `user.id eq "beta"`},
		{"ne", &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_NE, Attribute: a, Literal: s}}}, `user.id ne "beta"`},
		{"numeric", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT, Attribute: a, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 1.5}}}}}, "user.id gt 1.5"},
		{"membership", &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: a, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{s}}}}}, `user.id in ["beta"]`},
		{"contains", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS, Attribute: a, Literal: "be"}}}, `user.id co "be"`},
		{"starts", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH, Attribute: a, Literal: "be"}}}, `user.id sw "be"`},
		{"ends", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_ENDS_WITH, Attribute: a, Literal: "ta"}}}, `user.id ew "ta"`},
		{"semver", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE, Attribute: a, Semver: "1.2.3"}}}, "user.id ge 1.2.3"},
		{"all", goffLogical(irv1.LogicalOperator_LOGICAL_OPERATOR_ALL), "AND"}, {"any", goffLogical(irv1.LogicalOperator_LOGICAL_OPERATOR_ANY), "OR"}, {"exactly one", goffLogical(irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE), "not"},
		{"not", &irv1.Condition{Kind: &irv1.Condition_Negation{Negation: &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}}}, "not"},
	}
	for _, test := range conditions {
		t.Run(test.name, func(t *testing.T) {
			got, err := compileIRCondition(test.value)
			if err != nil || !strings.Contains(got, test.want) {
				t.Fatalf("compileIRCondition() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
	for _, test := range []struct {
		name  string
		value *irv1.Condition
		want  string
	}{
		{"nil", nil, "condition is required"}, {"unset", &irv1.Condition{}, "unsupported IR condition"}, {"presence", &irv1.Condition{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: a}}}, "presence"}, {"unknown logical", &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator(99)}}}, "unsupported logical"},
	} {
		t.Run("error/"+test.name, func(t *testing.T) {
			if _, err := compileIRCondition(test.value); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	for _, test := range []struct {
		name  string
		value *irv1.Condition
	}{
		{"numeric gte", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE, Attribute: a, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}}}}},
		{"numeric lt", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LT, Attribute: a, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}}}}},
		{"numeric lte", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LTE, Attribute: a, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}}}}},
		{"semver gt", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT, Attribute: a, Semver: "1.0.0"}}}},
		{"semver lt", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LT, Attribute: a, Semver: "2.0.0"}}}},
		{"semver lte", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LTE, Attribute: a, Semver: "2.0.0"}}}},
	} {
		t.Run("additional/"+test.name, func(t *testing.T) {
			if _, err := compileIRCondition(test.value); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func goffLogical(operator irv1.LogicalOperator) *irv1.Condition {
	return &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: operator, Conditions: []*irv1.Condition{{Kind: &irv1.Condition_Constant{Constant: true}}, {Kind: &irv1.Condition_Constant{Constant: false}}}}}}
}

func TestIRActionVariantScheduleAndNumericContracts(t *testing.T) {
	a := &irv1.AttributePath{Segments: []string{"user", "id"}}
	serve := &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}
	distribute := &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: a, Weights: map[string]uint32{"on": 2, "off": 1}}}}
	if rule, key, err := compileIRDefaultRule(serve); err != nil || rule.Variation != "on" || key != "" {
		t.Fatalf("serve default = %#v, %q, %v", rule, key, err)
	}
	if rule, key, err := compileIRDefaultRule(distribute); err != nil || key != "user.id" || len(rule.Percentage) != 2 {
		t.Fatalf("distribution default = %#v, %q, %v", rule, key, err)
	}
	steps, key, err := compileIRSchedule([]*irv1.ScheduledEvaluation{{EffectiveAt: timestamppb.New(time.Date(2028, 1, 1, 0, 0, 0, 1, time.UTC)), Evaluation: &irv1.Evaluation{DefaultAction: serve}}})
	if err != nil || key != "" || len(steps) != 1 || steps[0].Date != "2028-01-01T00:00:00.000000001Z" {
		t.Fatalf("schedule = %#v, %q, %v", steps, key, err)
	}
	if _, _, err := compileIRSchedule([]*irv1.ScheduledEvaluation{{}}); err == nil || !strings.Contains(err.Error(), "schedule[0] is incomplete") {
		t.Fatalf("incomplete schedule = %v", err)
	}
	if _, _, err := compileIRSchedule([]*irv1.ScheduledEvaluation{{EffectiveAt: timestamppb.Now(), Evaluation: &irv1.Evaluation{}}}); err == nil || !strings.Contains(err.Error(), "default_action") {
		t.Fatalf("invalid schedule evaluation = %v", err)
	}
	if _, _, err := compileIRSchedule([]*irv1.ScheduledEvaluation{{EffectiveAt: timestamppb.Now(), Evaluation: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: map[string]uint32{"on": 1}}}}}}, {EffectiveAt: timestamppb.Now(), Evaluation: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"device", "id"}}, Weights: map[string]uint32{"on": 1}}}}}}}); err == nil || !strings.Contains(err.Error(), "multiple distribute") {
		t.Fatalf("conflicting schedule keys = %v", err)
	}
	if err := validateIRNumericTransport(map[string]*irv1.VariantValue{"x": {Kind: &irv1.VariantValue_IntValue{IntValue: 1 << 53}}}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("unsafe numeric = %v", err)
	}
	if _, _, err := compileIRDefaultRule(nil); err == nil || !strings.Contains(err.Error(), "default_action") {
		t.Fatalf("nil default = %v", err)
	}
	if _, _, err := compileIRDefaultRule(&irv1.Action{}); err == nil || !strings.Contains(err.Error(), "unsupported IR action") {
		t.Fatalf("unsupported default = %v", err)
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
			got := compileIRPercentages(test.weights)
			for variant, want := range test.weights {
				if got[variant] != float64(want) {
					t.Fatalf("weight %q = %v, want %d", variant, got[variant], want)
				}
			}
		})
	}
}

func TestIRDocumentAndBoundaryContracts(t *testing.T) {
	if _, _, err := CompileIR(&irv1.Document{}, "prod", CompileOptions{}); err == nil || !strings.Contains(err.Error(), "FFCRAFT_IR_INVALID_CORE") {
		t.Fatalf("invalid IR document = %v", err)
	}
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}}}}}
	if _, _, err := CompileIR(doc, "prod", CompileOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CompileIR(doc, "missing", CompileOptions{}); err == nil || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("missing environment = %v", err)
	}
	if output, warnings, err := CompileIR(doc, "missing", CompileOptions{AllowMissingEnvironment: true}); err != nil || len(warnings) != 1 || len(output) == 0 {
		t.Fatalf("missing environment warning mode = %s, %#v, %v", output, warnings, err)
	}
	if _, _, err := compileIRRules([]*irv1.Rule{{}}); err == nil || !strings.Contains(err.Error(), "rule is incomplete") {
		t.Fatalf("incomplete rule = %v", err)
	}
	if _, err := compileIRActionResult(&irv1.Action{}, &targetRule{}); err == nil || !strings.Contains(err.Error(), "unsupported IR action") {
		t.Fatalf("unsupported action = %v", err)
	}
	if _, _, err := compileIRDistribution(&irv1.Distribution{}); err == nil || !strings.Contains(err.Error(), "allocation_key") {
		t.Fatalf("missing distribution key = %v", err)
	}
	if _, err := compileIRBinary(&irv1.AttributePath{}, &irv1.ScalarValue{}, ""); err == nil || !strings.Contains(err.Error(), "unsupported equality") {
		t.Fatalf("empty binary operator = %v", err)
	}
}

func TestIRDocumentTargetingAndTransportShapes(t *testing.T) {
	attr := &irv1.AttributePath{Segments: []string{"user", "id"}}
	serve := func(name string) *irv1.Action { return &irv1.Action{Kind: &irv1.Action_Serve{Serve: name}} }
	distribute := &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: attr, Weights: map[string]uint32{"on": 1, "off": 3}}}}
	condition := &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{
		"a": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, "off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{Rules: []*irv1.Rule{{Condition: condition, Action: distribute}}, DefaultAction: serve("off")}, Schedule: []*irv1.ScheduledEvaluation{{EffectiveAt: timestamppb.New(time.Date(2028, 1, 1, 0, 0, 0, 1, time.UTC)), Evaluation: &irv1.Evaluation{DefaultAction: distribute}}}}}},
		"b": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_StringValue{StringValue: "on"}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: serve("on")}}}},
	}}
	output, warnings, err := CompileIR(doc, "prod", CompileOptions{})
	if err != nil || len(warnings) != 0 || !strings.Contains(string(output), "scheduledRollout") || !strings.Contains(string(output), "percentage") {
		t.Fatalf("targeted document = %s, %#v, %v", output, warnings, err)
	}
}

func TestIRValueAndHelperTables(t *testing.T) {
	for _, test := range []struct {
		name  string
		value *irv1.ScalarValue
		want  string
	}{
		{"bool", &irv1.ScalarValue{Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}, "true"}, {"int", &irv1.ScalarValue{Kind: &irv1.ScalarValue_IntValue{IntValue: 7}}, "7"}, {"double", &irv1.ScalarValue{Kind: &irv1.ScalarValue_DoubleValue{DoubleValue: 1.5}}, "1.5"}, {"string", &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}, `"x"`}, {"null", &irv1.ScalarValue{}, "<nil>"},
	} {
		if got := compileIRScalar(test.value); test.name == "string" && got != `"x"` || test.name != "string" && got == "" {
			t.Errorf("compileIRScalar(%s) = %#v", test.name, got)
		}
	}
	for _, test := range []struct {
		value *irv1.NumericValue
		want  string
	}{
		{&irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 7}}, "7"}, {&irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 1.5}}, "1.5"}, {&irv1.NumericValue{}, "null"},
	} {
		if got := compileIRNumeric(test.value); got != test.want {
			t.Errorf("compileIRNumeric() = %q, want %q", got, test.want)
		}
	}
	variants := map[string]*irv1.VariantValue{
		"bool": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, "string": {Kind: &irv1.VariantValue_StringValue{StringValue: "x"}}, "int": {Kind: &irv1.VariantValue_IntValue{IntValue: 1}}, "double": {Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 1.5}}, "null": {Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}}, "object": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"x": {Kind: &irv1.VariantValue_StringValue{StringValue: "y"}}}}}}, "list": {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_IntValue{IntValue: 1}}}}}},
	}
	for name, value := range variants {
		if got := compileIRVariant(value); got == nil && name != "null" {
			t.Errorf("compileIRVariant(%s) = nil", name)
		}
	}
	if got := compileIRVariant(&irv1.VariantValue{}); got != nil {
		t.Fatalf("unset variant = %#v", got)
	}
	if got, err := mergeBucketingKeys("", "user.id"); err != nil || got != "user.id" {
		t.Fatalf("merge empty = %q, %v", got, err)
	}
	if got, err := mergeBucketingKeys("user.id", "user.id"); err != nil || got != "user.id" {
		t.Fatalf("merge same = %q, %v", got, err)
	}
	if _, err := mergeBucketingKeys("user.id", "device.id"); err == nil {
		t.Fatal("conflicting bucketing keys accepted")
	}
	if err := validateIRNumericTransport(map[string]*irv1.VariantValue{"nested": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"id": {Kind: &irv1.VariantValue_IntValue{IntValue: 1 << 53}}}}}}}); err == nil || !strings.Contains(err.Error(), "object field") {
		t.Fatalf("unsafe nested object = %v", err)
	}
	if err := validateIRNumericTransport(map[string]*irv1.VariantValue{
		"list":   {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_IntValue{IntValue: 1}}}}}},
		"object": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"nested": {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_StringValue{StringValue: "x"}}}}}}}}}},
	}); err != nil {
		t.Fatalf("safe nested numeric transport = %v", err)
	}
	target := &targetRule{}
	if key, err := compileIRActionResult(&irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}, target); err != nil || target.Variation != "on" || key != "" {
		t.Fatalf("serve action result = %#v, %q, %v", target, key, err)
	}
	if key, err := compileIRActionResult(&irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: map[string]uint32{"on": 1}}}}, target); err != nil || key != "user.id" {
		t.Fatalf("distribution action result = %#v, %q, %v", target, key, err)
	}
	if _, err := marshalDocument(map[string]flagFile{"b": {}, "a": {}}); err != nil {
		t.Fatalf("empty sorted document = %v", err)
	}
	if _, err := compileIRCondition(&irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator(99)}}}); err == nil || !strings.Contains(err.Error(), "unsupported equality") {
		t.Fatalf("unknown equality = %v", err)
	}
	if _, _, err := compileIRRules([]*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}, Action: &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: map[string]uint32{"on": 1}}}}}, {Condition: &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}, Action: &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"device", "id"}}, Weights: map[string]uint32{"on": 1}}}}}}); err == nil || !strings.Contains(err.Error(), "multiple distribute") {
		t.Fatalf("conflicting rule keys = %v", err)
	}
}
