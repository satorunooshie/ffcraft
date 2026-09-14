package codegen

import (
	"strings"
	"testing"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCompileIRRejectsEnvironmentSpecificDefaults(t *testing.T) {
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{
		"checkout": {
			Variants: map[string]*irv1.VariantValue{
				"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
				"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
			},
			Environments: map[string]*irv1.Environment{
				"prod":    {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}},
				"staging": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}}},
			},
		},
	}}
	_, err := CompileIR(doc, Config{PackageName: "flags"})
	if err == nil || !strings.Contains(err.Error(), "environment-specific default variants") {
		t.Fatalf("CompileIR() error = %v, want environment-specific default rejection", err)
	}
}

func TestCompileIRScansMatchingDefaultsAcrossEnvironments(t *testing.T) {
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{
		"checkout": {
			Variants: map[string]*irv1.VariantValue{
				"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
				"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
			},
			Environments: map[string]*irv1.Environment{
				"prod": {Base: &irv1.Evaluation{
					Rules: []*irv1.Rule{{
						Condition: &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{
							Operator:  irv1.EqualityOperator_EQUALITY_OPERATOR_EQ,
							Attribute: &irv1.AttributePath{Segments: []string{"user", "segment"}},
							Literal:   &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "beta"}},
						}}},
						Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}},
					}},
					DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}},
				}},
				"staging": {
					Base: &irv1.Evaluation{
						Rules: []*irv1.Rule{{
							Condition: &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{
								Operator:  irv1.StringMatchOperator_STRING_MATCH_OPERATOR_STARTS_WITH,
								Attribute: &irv1.AttributePath{Segments: []string{"user", "id"}},
								Literal:   "test-",
							}}},
							Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}},
						}},
						DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}},
					},
					Schedule: []*irv1.ScheduledEvaluation{{
						EffectiveAt: timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 123, time.UTC)),
						Evaluation: &irv1.Evaluation{
							Rules: []*irv1.Rule{{
								Condition: &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{
									Operator:  irv1.EqualityOperator_EQUALITY_OPERATOR_EQ,
									Attribute: &irv1.AttributePath{Segments: []string{"user", "type"}},
									Literal:   &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "internal"}},
								}}},
								Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}},
							}},
							DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}},
						},
					}},
				},
			},
		}}}

	output, err := CompileIR(doc, Config{PackageName: "flags"})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"UserSegment", "UserID", "UserType"} {
		if !strings.Contains(string(output), fragment) {
			t.Fatalf("generated source does not contain %q", fragment)
		}
	}
}
