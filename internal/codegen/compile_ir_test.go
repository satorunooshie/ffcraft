package codegen

import (
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
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
