package flagd

import (
	"strings"
	"testing"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCompileIRDirectSemanticSurface(t *testing.T) {
	doc := directCompilerFixture()
	output, _, err := CompileIR(doc, "prod", CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"user.segment", "user.id", "fractional", "1767225600", "\"off\",", "\"on\","} {
		if !strings.Contains(string(output), fragment) {
			t.Fatalf("flagd output missing %q: %s", fragment, output)
		}
	}
}

func directCompilerFixture() *irv1.Document {
	return &irv1.Document{Flags: map[string]*irv1.Flag{
		"direct": {
			Variants: map[string]*irv1.VariantValue{
				"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
				"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
			},
			Environments: map[string]*irv1.Environment{"prod": {
				Base: &irv1.Evaluation{Rules: []*irv1.Rule{{
					Condition: &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: &irv1.AttributePath{Segments: []string{"user", "segment"}}, Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "beta"}}}}},
					Action:    &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}},
				}}, DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}},
				Schedule: []*irv1.ScheduledEvaluation{{EffectiveAt: timestamppb.New(time.Unix(1767225600, 1).UTC()), Evaluation: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: map[string]uint32{"off": 1, "on": 2}}}}}}},
			}},
		},
	}}
}
