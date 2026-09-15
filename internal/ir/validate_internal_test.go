package ir

import (
	"fmt"
	"math"
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

func TestValidateConditionContracts(t *testing.T) {
	attr := &irv1.AttributePath{Segments: []string{"user", "id"}}
	tests := []struct {
		name      string
		condition *irv1.Condition
		want      string
	}{
		{"missing kind", &irv1.Condition{}, "condition kind"},
		{"equality missing operands", &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ}}}, "attribute equality"},
		{"numeric missing operands", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT}}}, "numeric comparison"},
		{"numeric nonfinite", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT, Attribute: attr, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: math.Inf(1)}}}}}, "not finite"},
		{"membership missing operands", &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{}}}, "membership"},
		{"membership heterogeneous", &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: attr, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}, {Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}}}}}}, "homogeneous"},
		{"string match missing attribute", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{}}}, "string match"},
		{"semver missing literal", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT, Attribute: attr}}}, "semver comparison"},
		{"semver invalid literal", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT, Attribute: attr, Semver: "1.0"}}}, "invalid SemVer"},
		{"presence missing attribute", &irv1.Condition{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{}}}, "presence attribute"},
		{"logical too short", &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_ANY, Conditions: []*irv1.Condition{}}}}, "at least two"},
		{"negation missing child", &irv1.Condition{Kind: &irv1.Condition_Negation{}}, "condition kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateCondition(test.condition); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateCondition() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsUnknownConditionEnums(t *testing.T) {
	attribute := &irv1.AttributePath{Segments: []string{"value"}}
	tests := []struct {
		name      string
		condition *irv1.Condition
		want      string
	}{
		{"equality", &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator(99), Attribute: attribute, Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}}}}, "unsupported equality"},
		{"numeric", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator(99), Attribute: attribute, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}}}}, "unsupported numeric"},
		{"string match", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator(99), Attribute: attribute}}}, "unsupported string match"},
		{"semver", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator(99), Attribute: attribute, Semver: "1.2.3"}}}, "unsupported semver"},
		{"logical", &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator(99), Conditions: []*irv1.Condition{{Kind: &irv1.Condition_Constant{Constant: true}}, {Kind: &irv1.Condition_Constant{Constant: false}}}}}}, "unsupported logical"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateCondition(test.condition); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateCondition() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateValueAndExtensionContracts(t *testing.T) {
	tests := []struct {
		name  string
		check func() error
		want  string
	}{
		{"nil variant", func() error { return validateVariant(nil) }, "value is nil"},
		{"missing variant kind", func() error { return validateVariant(&irv1.VariantValue{}) }, "value kind"},
		{"nonfinite variant", func() error {
			return validateVariant(&irv1.VariantValue{Kind: &irv1.VariantValue_DoubleValue{DoubleValue: math.NaN()}})
		}, "not finite"},
		{"nil extension", func() error { return validateExtensionDepth(nil, 0) }, "extension value"},
		{"missing extension kind", func() error { return validateExtensionDepth(&irv1.ExtensionValue{}, 0) }, "value kind"},
		{"nonfinite extension", func() error {
			return validateExtensionDepth(&irv1.ExtensionValue{Kind: &irv1.ExtensionValue_DoubleValue{DoubleValue: math.Inf(-1)}}, 0)
		}, "not finite"},
		{"nil extension object", func() error {
			return validateExtensionDepth(&irv1.ExtensionValue{Kind: &irv1.ExtensionValue_ObjectValue{}}, 0)
		}, "object value"},
		{"nil extension list", func() error {
			return validateExtensionDepth(&irv1.ExtensionValue{Kind: &irv1.ExtensionValue_ListValue{}}, 0)
		}, "list value"},
		{"invalid extension string", func() error {
			return validateExtensionDepth(&irv1.ExtensionValue{Kind: &irv1.ExtensionValue_StringValue{StringValue: strings.Repeat("x", 257)}}, 0)
		}, "256 bytes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.check(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateActionContracts(t *testing.T) {
	variants := map[string]*irv1.VariantValue{
		"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
		"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
	}
	tests := []struct {
		name   string
		action *irv1.Action
		want   string
	}{
		{"missing kind", &irv1.Action{}, "action kind"},
		{"unknown serve", &irv1.Action{Kind: &irv1.Action_Serve{Serve: "missing"}}, "unknown serve"},
		{"missing distribution", &irv1.Action{Kind: &irv1.Action_Distribute{}}, "invalid distribution"},
		{"zero weight", &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"id"}}, Weights: map[string]uint32{"on": 0, "off": 1}}}}, "weight"},
		{"unknown variant", &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"id"}}, Weights: map[string]uint32{"on": 1, "missing": 1}}}}, "unknown distribution"},
		{"noncanonical gcd", &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"id"}}, Weights: map[string]uint32{"on": 2, "off": 4}}}}, "GCD=1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateAction(test.action, variants); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateAction() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateAcceptsEverySemanticKind(t *testing.T) {
	attribute := &irv1.AttributePath{Segments: []string{"user", "value"}}
	conditionCases := []*irv1.Condition{
		{Kind: &irv1.Condition_Constant{Constant: true}},
		{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: attribute, Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_IntValue{IntValue: 7}}}}},
		{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT, Attribute: attribute, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 1.5}}}}},
		{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: attribute, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{{Kind: &irv1.ScalarValue_StringValue{StringValue: "a"}}, {Kind: &irv1.ScalarValue_StringValue{StringValue: "b"}}}}}}},
		{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS, Attribute: attribute, Literal: "x"}}},
		{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE, Attribute: attribute, Semver: "1.2.3-rc.1+build.7"}}},
		{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: attribute}}},
		{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_ANY, Conditions: []*irv1.Condition{
			{Kind: &irv1.Condition_Constant{Constant: true}},
			{Kind: &irv1.Condition_Constant{Constant: false}},
		}}}},
		{Kind: &irv1.Condition_Negation{Negation: &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: false}}}},
	}
	for index, condition := range conditionCases {
		if err := validateCondition(condition); err != nil {
			t.Fatalf("condition[%d] rejected: %v", index, err)
		}
	}

	variantCases := []*irv1.VariantValue{
		{Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
		{Kind: &irv1.VariantValue_StringValue{StringValue: "x"}},
		{Kind: &irv1.VariantValue_IntValue{IntValue: 7}},
		{Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 1.5}},
		{Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}},
		{Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"nested": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}}}}},
		{Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_IntValue{IntValue: 1}}}}}},
	}
	for index, variant := range variantCases {
		if err := validateVariant(variant); err != nil {
			t.Fatalf("variant[%d] rejected: %v", index, err)
		}
	}
	if err := validateExtensions(map[string]*irv1.ExtensionValue{
		"metadata": {Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: map[string]*irv1.ExtensionValue{
			"enabled": {Kind: &irv1.ExtensionValue_BoolValue{BoolValue: true}},
			"items":   {Kind: &irv1.ExtensionValue_ListValue{ListValue: &irv1.ExtensionList{Values: []*irv1.ExtensionValue{{Kind: &irv1.ExtensionValue_NullValue{NullValue: &irv1.ExtensionNull{}}}}}}},
		}}}},
	}); err != nil {
		t.Fatalf("valid extension rejected: %v", err)
	}
}

func TestValidateDepthAndSizeLimits(t *testing.T) {
	deepVariant := &irv1.VariantValue{Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}
	for index := 0; index < 66; index++ {
		deepVariant = &irv1.VariantValue{Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{deepVariant}}}}
	}
	if err := validateVariant(deepVariant); err == nil || !strings.Contains(err.Error(), "nesting depth") {
		t.Fatalf("deep variant error = %v", err)
	}
	deepCondition := &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}
	for index := 0; index < 66; index++ {
		deepCondition = &irv1.Condition{Kind: &irv1.Condition_Negation{Negation: deepCondition}}
	}
	if err := validateCondition(deepCondition); err == nil || !strings.Contains(err.Error(), "nesting depth") {
		t.Fatalf("deep condition error = %v", err)
	}
	deepExtension := &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_StringValue{StringValue: "x"}}
	for index := 0; index < 66; index++ {
		deepExtension = &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_ListValue{ListValue: &irv1.ExtensionList{Values: []*irv1.ExtensionValue{deepExtension}}}}
	}
	if err := validateExtensionDepth(deepExtension, 0); err == nil || !strings.Contains(err.Error(), "nesting depth") {
		t.Fatalf("deep extension error = %v", err)
	}

	manyNamespaces := make(map[string]*irv1.ExtensionValue, 257)
	for index := 0; index < 257; index++ {
		manyNamespaces[fmt.Sprintf("ns-%d", index)] = &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_BoolValue{BoolValue: true}}
	}
	if err := validateExtensions(manyNamespaces); err == nil || !strings.Contains(err.Error(), "namespace count") {
		t.Fatalf("namespace limit error = %v", err)
	}
	manyFields := make(map[string]*irv1.ExtensionValue, 257)
	for index := 0; index < 257; index++ {
		manyFields[fmt.Sprintf("field-%d", index)] = &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_BoolValue{BoolValue: true}}
	}
	if err := validateExtensionDepth(&irv1.ExtensionValue{Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: manyFields}}}, 0); err == nil || !strings.Contains(err.Error(), "field count") {
		t.Fatalf("field limit error = %v", err)
	}
	manyItems := make([]*irv1.ExtensionValue, 257)
	for index := range manyItems {
		manyItems[index] = &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_BoolValue{BoolValue: true}}
	}
	if err := validateExtensionDepth(&irv1.ExtensionValue{Kind: &irv1.ExtensionValue_ListValue{ListValue: &irv1.ExtensionList{Values: manyItems}}}, 0); err == nil || !strings.Contains(err.Error(), "list length") {
		t.Fatalf("list limit error = %v", err)
	}
}
