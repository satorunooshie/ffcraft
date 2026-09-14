package ir_test

import (
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
)

func TestValidateExtensionDetailed(t *testing.T) {
	doc := &irv1.Document{Extensions: map[string]*irv1.ExtensionValue{
		"com.example.v1": {Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: map[string]*irv1.ExtensionValue{
			"event": {Kind: &irv1.ExtensionValue_StringValue{StringValue: "purchase"}},
		}}}},
	}}

	status, diagnostics := ir.ValidateExtensionDetailed(doc, ir.ExtensionSelector{Scope: ir.DocumentScope}, "com.example.v1", func(_ *irv1.Document, selector ir.ExtensionSelector, namespace string, value *irv1.ExtensionValue) []ir.ExtensionDiagnostic {
		if selector.Scope != ir.DocumentScope || namespace != "com.example.v1" || value.GetObjectValue().Fields["event"].GetStringValue() != "purchase" {
			t.Fatal("validator did not receive normalized IR scope and value")
		}
		return []ir.ExtensionDiagnostic{{Code: "EXAMPLE_INVALID", Severity: "error", Message: "invalid event", Path: "/event"}}
	})
	if status != ir.ExtensionInvalid || len(diagnostics) != 1 || diagnostics[0].Path != "/event" {
		t.Fatalf("status = %v, diagnostics = %#v", status, diagnostics)
	}

	status, diagnostics = ir.ValidateExtensionDetailed(doc, ir.ExtensionSelector{Scope: ir.DocumentScope}, "missing", func(_ *irv1.Document, _ ir.ExtensionSelector, _ string, _ *irv1.ExtensionValue) []ir.ExtensionDiagnostic {
		return nil
	})
	if status != ir.ExtensionNotApplicable || len(diagnostics) != 0 {
		t.Fatalf("missing namespace status = %v, diagnostics = %#v", status, diagnostics)
	}
}
