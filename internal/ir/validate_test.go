package ir_test

import (
	"math"
	"strings"
	"testing"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"google.golang.org/protobuf/types/known/timestamppb"
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
		{name: "semver numeric prerelease leading zero", want: "invalid SemVer", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT, Attribute: &irv1.AttributePath{Segments: []string{"version"}}, Semver: "1.2.3-01"}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "distribution weights are reduced", want: "GCD=1", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.DefaultAction = &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: map[string]uint32{"on": 2, "off": 4}}}}
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

func TestValidateRejectsDotsInsideAttributePathSegments(t *testing.T) {
	doc := minimalIR()
	doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{
		Condition: &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{
			Operator:  irv1.EqualityOperator_EQUALITY_OPERATOR_EQ,
			Attribute: &irv1.AttributePath{Segments: []string{"user.name"}},
			Literal:   &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "alice"}},
		}}},
		Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}},
	}}
	if err := ir.Validate(doc); err == nil || !strings.Contains(err.Error(), "does not match regex pattern") {
		t.Fatalf("Validate() error = %v, want invalid attribute segment", err)
	}
}

func TestValidateMalformedMapsReturnsErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		doc  *irv1.Document
	}{
		{name: "nil flag", doc: &irv1.Document{Flags: map[string]*irv1.Flag{"f": nil}}},
		{name: "nil environment", doc: &irv1.Document{Flags: map[string]*irv1.Flag{"f": {
			Variants:     map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}},
			Environments: map[string]*irv1.Environment{"prod": nil},
		}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ir.Validate(test.doc); err == nil {
				t.Fatal("Validate() unexpectedly accepted malformed IR")
			}
		})
	}
}

func TestValidateScheduleSupportsPreEpochTimestamps(t *testing.T) {
	doc := minimalIR()
	base := doc.Flags["f"].Environments["prod"].Base
	doc.Flags["f"].Environments["prod"].Schedule = []*irv1.ScheduledEvaluation{
		{EffectiveAt: timestamppb.New(time.Unix(-2, 900_000_000)), Evaluation: base},
		{EffectiveAt: timestamppb.New(time.Unix(-1, 100_000_000)), Evaluation: base},
	}
	if err := ir.Validate(doc); err != nil {
		t.Fatalf("pre-epoch schedule rejected: %v", err)
	}
}

func TestValidateBoundaryContracts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*irv1.Document)
	}{
		{name: "nil document", mutate: func(*irv1.Document) {}},
		{name: "no flags", mutate: func(doc *irv1.Document) { doc.Flags = nil }},
		{name: "empty flag key", mutate: func(doc *irv1.Document) { doc.Flags[""] = doc.Flags["f"]; delete(doc.Flags, "f") }},
		{name: "missing variants", mutate: func(doc *irv1.Document) { doc.Flags["f"].Variants = nil }},
		{name: "missing environments", mutate: func(doc *irv1.Document) { doc.Flags["f"].Environments = nil }},
		{name: "missing base", mutate: func(doc *irv1.Document) { doc.Flags["f"].Environments["prod"].Base = nil }},
		{name: "incomplete schedule", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Schedule = []*irv1.ScheduledEvaluation{{}}
		}},
		{name: "duplicate schedule timestamp", mutate: func(doc *irv1.Document) {
			timestamp := timestamppb.New(testTime())
			doc.Flags["f"].Environments["prod"].Schedule = []*irv1.ScheduledEvaluation{
				{EffectiveAt: timestamp, Evaluation: doc.Flags["f"].Environments["prod"].Base},
				{EffectiveAt: timestamppb.New(testTime()), Evaluation: doc.Flags["f"].Environments["prod"].Base},
			}
		}},
		{name: "nil rule", mutate: func(doc *irv1.Document) { doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{nil} }},
		{name: "unknown serve variant", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.DefaultAction = &irv1.Action{Kind: &irv1.Action_Serve{Serve: "missing"}}
		}},
		{name: "missing action kind", mutate: func(doc *irv1.Document) { doc.Flags["f"].Environments["prod"].Base.DefaultAction = &irv1.Action{} }},
		{name: "invalid distribution", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.DefaultAction = &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{}}}
		}},
		{name: "zero distribution weight", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.DefaultAction = &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"id"}}, Weights: map[string]uint32{"on": 0, "off": 1}}}}
		}},
		{name: "unknown distribution variant", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.DefaultAction = &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"id"}}, Weights: map[string]uint32{"on": 1, "missing": 1}}}}
		}},
		{name: "nil condition", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: nil, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "numeric incomplete", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "numeric nonfinite", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Attribute: &irv1.AttributePath{Segments: []string{"x"}}, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: math.NaN()}}}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "empty membership", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: &irv1.AttributePath{Segments: []string{"x"}}, Literals: &irv1.ScalarList{}}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "string match attribute", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "invalid semver", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Attribute: &irv1.AttributePath{Segments: []string{"version"}}, Semver: "1"}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "presence attribute", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "logical arity", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Base.Rules = []*irv1.Rule{{Condition: &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Conditions: []*irv1.Condition{}}}}, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}
		}},
		{name: "nil variant", mutate: func(doc *irv1.Document) { doc.Flags["f"].Variants["on"] = nil }},
		{name: "nonfinite variant", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Variants["on"] = &irv1.VariantValue{Kind: &irv1.VariantValue_DoubleValue{DoubleValue: math.Inf(1)}}
		}},
		{name: "empty object key", mutate: func(doc *irv1.Document) {
			doc.Flags["f"].Variants["on"] = &irv1.VariantValue{Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}}}}}
		}},
		{name: "invalid extension namespace", mutate: func(doc *irv1.Document) {
			doc.Extensions = map[string]*irv1.ExtensionValue{"": {Kind: &irv1.ExtensionValue_StringValue{StringValue: "x"}}}
		}},
		{name: "extension missing kind", mutate: func(doc *irv1.Document) { doc.Extensions = map[string]*irv1.ExtensionValue{"x": {}} }},
		{name: "extension nonfinite", mutate: func(doc *irv1.Document) {
			doc.Extensions = map[string]*irv1.ExtensionValue{"x": {Kind: &irv1.ExtensionValue_DoubleValue{DoubleValue: math.NaN()}}}
		}},
		{name: "extension object nil", mutate: func(doc *irv1.Document) {
			doc.Extensions = map[string]*irv1.ExtensionValue{"x": {Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: nil}}}
		}},
		{name: "extension list nil", mutate: func(doc *irv1.Document) {
			doc.Extensions = map[string]*irv1.ExtensionValue{"x": {Kind: &irv1.ExtensionValue_ListValue{ListValue: nil}}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var doc *irv1.Document
			if test.name == "nil document" {
				doc = nil
			} else {
				doc = minimalIR()
				test.mutate(doc)
			}
			if err := ir.Validate(doc); err == nil {
				t.Fatal("Validate() unexpectedly accepted malformed IR")
			}
		})
	}
}

func testTime() (t time.Time) { return time.Unix(100, 0).UTC() }

func minimalIR() *irv1.Document {
	return &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, "off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}}}}}
}
