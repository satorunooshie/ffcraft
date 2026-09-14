package ir

import (
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
		{"equality missing operands", &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{}}}, "attribute equality"},
		{"numeric missing operands", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{}}}, "numeric comparison"},
		{"numeric nonfinite", &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Attribute: attr, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: math.Inf(1)}}}}}, "not finite"},
		{"membership missing operands", &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{}}}, "membership"},
		{"membership heterogeneous", &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: attr, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}, {Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}}}}}}, "homogeneous"},
		{"string match missing attribute", &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{}}}, "string match"},
		{"semver missing literal", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Attribute: attr}}}, "semver comparison"},
		{"semver invalid literal", &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Attribute: attr, Semver: "1.0"}}}, "invalid SemVer"},
		{"presence missing attribute", &irv1.Condition{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{}}}, "presence attribute"},
		{"logical too short", &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Conditions: []*irv1.Condition{}}}}, "at least two"},
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
