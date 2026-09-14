package ir_test

import (
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
)

func TestValidateSemanticContracts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, want string
		mutate     func(*irv1.Document)
	}{
		{name: "variant kinds", want: "homogeneous", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Variants["off"] = &irv1.VariantValue{Kind: &irv1.VariantValue_StringValue{StringValue: "off"}}
		}},
		{name: "membership kinds", want: "homogeneous", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: &irv1.AttributePath{Segments: []string{"x"}}, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{{Kind: &irv1.ScalarValue_StringValue{StringValue: "a"}}, {Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}}}}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "semver literal", want: "invalid SemVer", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT, Attribute: &irv1.AttributePath{Segments: []string{"version"}}, Semver: "1.0"}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := minimalIR()
			test.mutate(doc)
			if err := ir.Validate(doc); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func minimalIR() *irv1.Document {
	return &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, "off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}}}}}
}
