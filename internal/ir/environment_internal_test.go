package ir

import (
	"strings"
	"testing"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestValidateEnvironmentScheduleContracts(t *testing.T) {
	variants := map[string]*irv1.VariantValue{
		"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
		"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
	}
	base := &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}}
	timestamp := func(second int64) *timestamppb.Timestamp { return timestamppb.New(time.Unix(second, 0).UTC()) }
	validEvaluation := &irv1.Evaluation{
		Rules: []*irv1.Rule{{
			Condition: &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}},
			Action:    &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}},
		}},
		DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}},
	}
	tests := []struct {
		name string
		env  *irv1.Environment
		want string
	}{
		{"valid schedule", &irv1.Environment{Base: base, Schedule: []*irv1.ScheduledEvaluation{{EffectiveAt: timestamp(1), Evaluation: validEvaluation}, {EffectiveAt: timestamp(2), Evaluation: base}}}, ""},
		{"nil environment", nil, "base evaluation"},
		{"nil base", &irv1.Environment{}, "base evaluation"},
		{"incomplete schedule", &irv1.Environment{Base: base, Schedule: []*irv1.ScheduledEvaluation{{EffectiveAt: timestamp(1)}}}, "schedule entry"},
		{"duplicate timestamps", &irv1.Environment{Base: base, Schedule: []*irv1.ScheduledEvaluation{{EffectiveAt: timestamp(1), Evaluation: base}, {EffectiveAt: timestamp(1), Evaluation: base}}}, "strictly increasing"},
		{"nil rule", &irv1.Environment{Base: &irv1.Evaluation{DefaultAction: base.DefaultAction, Rules: []*irv1.Rule{nil}}}, "rule[0]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateEnvironment(test.env, variants)
			if test.want == "" && err != nil {
				t.Fatalf("validateEnvironment() = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("validateEnvironment() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateExtensionsNameAndValueContracts(t *testing.T) {
	tests := []struct {
		name       string
		extensions map[string]*irv1.ExtensionValue
		want       string
	}{
		{"empty namespace", map[string]*irv1.ExtensionValue{"": {Kind: &irv1.ExtensionValue_BoolValue{BoolValue: true}}}, "invalid namespace"},
		{"overlong namespace", map[string]*irv1.ExtensionValue{strings.Repeat("x", 129): {Kind: &irv1.ExtensionValue_BoolValue{BoolValue: true}}}, "invalid namespace"},
		{"nil namespace value", map[string]*irv1.ExtensionValue{"x": nil}, "invalid namespace"},
		{"empty object field", map[string]*irv1.ExtensionValue{"x": {Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: map[string]*irv1.ExtensionValue{"": {Kind: &irv1.ExtensionValue_BoolValue{BoolValue: true}}}}}}}, "object key"},
		{"overlong string", map[string]*irv1.ExtensionValue{"x": {Kind: &irv1.ExtensionValue_StringValue{StringValue: strings.Repeat("x", 257)}}}, "256 bytes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateExtensions(test.extensions); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateExtensions() = %v, want %q", err, test.want)
			}
		})
	}
}
