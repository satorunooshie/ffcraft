package validate

import (
	"strings"
	"testing"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

func TestValidateReferenceAndScheduleContracts(t *testing.T) {
	variants := &ffv1.VariantSet{Variants: map[string]*ffv1.VariantValue{
		"on":  {Kind: &ffv1.VariantValue_BoolValue{BoolValue: true}},
		"off": {Kind: &ffv1.VariantValue_BoolValue{BoolValue: false}},
	}}
	doc := &ffv1.FeatureFlagDocument{Rules: map[string]*ffv1.Condition{
		"known": {Kind: &ffv1.Condition_LiteralBool{LiteralBool: &ffv1.LiteralBool{Value: true}}},
	}}
	knownRule := &ffv1.Condition{Kind: &ffv1.Condition_Rule{Rule: &ffv1.RuleRef{Name: "known"}}}
	missingRule := &ffv1.Condition{Kind: &ffv1.Condition_Rule{Rule: &ffv1.RuleRef{Name: "missing"}}}
	nested := &ffv1.Condition{Kind: &ffv1.Condition_AllOf{AllOf: &ffv1.AllOf{Conditions: []*ffv1.Condition{
		knownRule,
		{Kind: &ffv1.Condition_Not{Not: &ffv1.Not{Condition: missingRule}}},
	}}}}
	if err := validateConditionRefs(doc, nested); err == nil || !strings.Contains(err.Error(), `referenced rule "missing" not found`) {
		t.Fatalf("validateConditionRefs() = %v", err)
	}
	if refs := collectRuleRefs(nested); len(refs) != 2 || refs[0] != "known" || refs[1] != "missing" {
		t.Fatalf("collectRuleRefs() = %#v", refs)
	}

	serve := func(name string) *ffv1.Action {
		return &ffv1.Action{Kind: &ffv1.Action_Serve{Serve: &ffv1.Serve{Variant: name}}}
	}
	if err := validateActionRefs(doc, variants, serve("on"), false); err != nil {
		t.Fatalf("valid serve action = %v", err)
	}
	if err := validateActionRefs(doc, variants, serve("unknown"), false); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown serve action = %v", err)
	}
	progressive := &ffv1.Action{Kind: &ffv1.Action_ProgressiveRollout{ProgressiveRollout: &ffv1.ProgressiveRollout{Variant: "on", Stickiness: "user.id", Start: "2026-01-01T00:00:00Z", End: "2026-01-02T00:00:00Z", Steps: 2}}}
	if err := validateActionRefs(doc, variants, progressive, false); err == nil || !strings.Contains(err.Error(), "only supported in default_action") {
		t.Fatalf("progressive rule action = %v", err)
	}
	if err := validateActionRefs(doc, variants, progressive, true); err != nil {
		t.Fatalf("valid progressive default action = %v", err)
	}
}

func TestValidateTimeAndScheduleTables(t *testing.T) {
	for _, test := range []struct {
		name string
		exp  *ffv1.Experimentation
		want string
	}{
		{"invalid start", &ffv1.Experimentation{Start: "not-time", End: "2026-01-02T00:00:00Z"}, "start"},
		{"reversed", &ffv1.Experimentation{Start: "2026-01-02T00:00:00Z", End: "2026-01-01T00:00:00Z"}, "before end"},
		{"valid", &ffv1.Experimentation{Start: "2026-01-01T00:00:00Z", End: "2026-01-02T00:00:00Z"}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateExperimentation(test.exp)
			if test.want == "" && err != nil {
				t.Fatalf("validateExperimentation() = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("validateExperimentation() = %v, want %q", err, test.want)
			}
		})
	}
	steps := func(first, second string) []*ffv1.ScheduledStep {
		return []*ffv1.ScheduledStep{{Date: first, DefaultAction: &ffv1.Action{Kind: &ffv1.Action_Serve{Serve: &ffv1.Serve{Variant: "on"}}}}, {Date: second, DefaultAction: &ffv1.Action{Kind: &ffv1.Action_Serve{Serve: &ffv1.Serve{Variant: "on"}}}}}
	}
	for _, test := range []struct {
		name  string
		steps []*ffv1.ScheduledStep
		want  string
	}{
		{"duplicate", steps("2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"), "must be unique"},
		{"descending", steps("2026-01-02T00:00:00Z", "2026-01-01T00:00:00Z"), "ascending"},
		{"valid", steps("2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateScheduledRollouts(test.steps)
			if test.want == "" && err != nil {
				t.Fatalf("validateScheduledRollouts() = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("validateScheduledRollouts() = %v, want %q", err, test.want)
			}
		})
	}
}
