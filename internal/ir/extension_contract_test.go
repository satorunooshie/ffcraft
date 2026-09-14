package ir_test

import (
	"errors"
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
)

func TestValidateExtensionScopeContracts(t *testing.T) {
	value := &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_StringValue{StringValue: "x"}}
	doc := &irv1.Document{
		Extensions: map[string]*irv1.ExtensionValue{"document": value},
		Flags: map[string]*irv1.Flag{"flag": {
			Extensions: map[string]*irv1.ExtensionValue{"flag": value},
			Environments: map[string]*irv1.Environment{"prod": {
				Extensions: map[string]*irv1.ExtensionValue{"environment": value},
			}},
		}},
	}
	validator := func(_ *irv1.Document, got *irv1.ExtensionValue) error {
		if got.GetStringValue() != "x" {
			return errors.New("unexpected extension value")
		}
		return nil
	}
	tests := []struct {
		name        string
		scope       ir.ExtensionScope
		flag        string
		environment string
		namespace   string
		want        ir.ExtensionValidationStatus
		wantError   string
	}{
		{"document valid", ir.DocumentScope, "", "", "document", ir.ExtensionValid, ""},
		{"flag valid", ir.FlagScope, "flag", "", "flag", ir.ExtensionValid, ""},
		{"environment valid", ir.EnvironmentScope, "flag", "prod", "environment", ir.ExtensionValid, ""},
		{"not applicable", ir.DocumentScope, "", "", "missing", ir.ExtensionNotApplicable, ""},
		{"missing flag", ir.FlagScope, "missing", "", "flag", ir.ExtensionInvalid, `flag "missing" not found`},
		{"missing environment", ir.EnvironmentScope, "flag", "staging", "environment", ir.ExtensionInvalid, `environment "staging" not found`},
		{"unknown scope", ir.ExtensionScope(99), "", "", "document", ir.ExtensionInvalid, "unknown extension scope"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, err := ir.ValidateExtension(doc, test.scope, test.flag, test.environment, test.namespace, validator)
			if status != test.want || (test.wantError == "" && err != nil) || (test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError))) {
				t.Fatalf("ValidateExtension() = %v, %v; want %v, %q", status, err, test.want, test.wantError)
			}
		})
	}
	if status, err := ir.ValidateExtension(doc, ir.DocumentScope, "", "", "document", nil); status != ir.ExtensionInvalid || err == nil || !strings.Contains(err.Error(), "validator is nil") {
		t.Fatalf("nil validator = %v, %v", status, err)
	}
	if status, err := ir.ValidateExtension(nil, ir.DocumentScope, "", "", "document", validator); status != ir.ExtensionInvalid || err == nil || !strings.Contains(err.Error(), "document is nil") {
		t.Fatalf("nil document = %v, %v", status, err)
	}
	if status, err := ir.ValidateExtension(doc, ir.DocumentScope, "", "", "document", func(_ *irv1.Document, _ *irv1.ExtensionValue) error { return errors.New("owner rejected") }); status != ir.ExtensionInvalid || err == nil || !strings.Contains(err.Error(), "owner rejected") {
		t.Fatalf("owner rejection = %v, %v", status, err)
	}
}

func TestValidateExtensionDetailedDiagnosticContracts(t *testing.T) {
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{"flag": {Environments: map[string]*irv1.Environment{"prod": {}}}}}
	for _, test := range []struct {
		name    string
		selectr ir.ExtensionSelector
		code    string
	}{
		{"missing flag", ir.ExtensionSelector{Scope: ir.FlagScope, FlagKey: "missing"}, "FFCRAFT_EXTENSION_FLAG_NOT_FOUND"},
		{"missing environment", ir.ExtensionSelector{Scope: ir.EnvironmentScope, FlagKey: "flag", Environment: "staging"}, "FFCRAFT_EXTENSION_ENVIRONMENT_NOT_FOUND"},
		{"unknown scope", ir.ExtensionSelector{Scope: ir.ExtensionScope(99)}, "FFCRAFT_EXTENSION_SCOPE_UNKNOWN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, diagnostics := ir.ValidateExtensionDetailed(doc, test.selectr, "x", func(_ *irv1.Document, _ ir.ExtensionSelector, _ string, _ *irv1.ExtensionValue) []ir.ExtensionDiagnostic {
				return nil
			})
			if status != ir.ExtensionInvalid || len(diagnostics) != 1 || diagnostics[0].Code != test.code || diagnostics[0].Severity != "error" {
				t.Fatalf("ValidateExtensionDetailed() = %v, %#v", status, diagnostics)
			}
		})
	}
	if status, diagnostics := ir.ValidateExtensionDetailed(nil, ir.ExtensionSelector{}, "x", nil); status != ir.ExtensionInvalid || len(diagnostics) != 1 || diagnostics[0].Code != "FFCRAFT_EXTENSION_DOCUMENT_NIL" {
		t.Fatalf("nil inputs = %v, %#v", status, diagnostics)
	}
}
