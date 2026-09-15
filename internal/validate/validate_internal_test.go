package validate

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/ast"
)

func TestValidateAnyValueTable(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, ""},
		{"bool", true, ""},
		{"string", "x", ""},
		{"finite number", float64(1.5), ""},
		{"nested list", []any{true, map[string]any{"x": "y"}}, ""},
		{"nonfinite number", math.Inf(1), "non-finite"},
		{"nested nonfinite", []any{map[string]any{"x": math.NaN()}}, "field \"x\""},
		{"unsupported type", struct{}{}, "unsupported object value type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAnyValue(test.value)
			if test.want == "" && err != nil {
				t.Fatalf("validateAnyValue() = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("validateAnyValue() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateActionAndTimestampContracts(t *testing.T) {
	variants := map[string]ast.VariantValue{"on": {Kind: ast.VariantValueKindBool}}
	for _, test := range []struct {
		name   string
		action ast.Action
		want   string
	}{
		{"nil action", nil, ""},
		{"serve", &ast.ServeAction{Variant: "on"}, ""},
		{"missing serve", &ast.ServeAction{Variant: "off"}, "not defined"},
		{"distribution", &ast.DistributeAction{Weights: map[string]uint32{"on": 50}}, ""},
		{"missing distribution variant", &ast.DistributeAction{Weights: map[string]uint32{"off": 50}}, "not defined"},
		{"progressive", &ast.ProgressiveRolloutAction{Variant: "on"}, ""},
		{"missing progressive variant", &ast.ProgressiveRolloutAction{Variant: "off"}, "not defined"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateAction(test.action, variants)
			if test.want == "" && err != nil {
				t.Fatalf("validateAction() = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("validateAction() = %v, want %q", err, test.want)
			}
		})
	}
	if _, err := parseTimestamp("2028-01-01T00:00:00.123456789Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseTimestamp("2028-01-01"); err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("parseTimestamp() = %v", err)
	}
}

func TestValidateNormalizedIRShapeTable(t *testing.T) {
	base := &ast.Document{Flags: []*ast.Flag{{
		Key: "flag", DefaultVariant: "on", Variants: map[string]ast.VariantValue{"on": {Kind: ast.VariantValueKindBool}},
		Environments: map[string]*ast.Environment{"prod": {DefaultAction: &ast.ServeAction{Variant: "on"}}},
	}}}
	tests := []struct {
		name   string
		mutate func(*ast.Document)
		want   string
	}{
		{"nil doc", func(*ast.Document) {}, "normalized document is nil"},
		{"nil flag", func(doc *ast.Document) { doc.Flags = []*ast.Flag{nil} }, "is nil"},
		{"empty key", func(doc *ast.Document) { doc.Flags[0].Key = "" }, "empty key"},
		{"no variants", func(doc *ast.Document) { doc.Flags[0].Variants = nil }, "no variants"},
		{"nil environment", func(doc *ast.Document) { doc.Flags[0].Environments["prod"] = nil }, "environment"},
		{"missing serve action", func(doc *ast.Document) {
			doc.Flags[0].Environments["prod"].DefaultAction = &ast.ServeAction{Variant: "off"}
		}, "not defined"},
		{"nil rule", func(doc *ast.Document) { doc.Flags[0].Environments["prod"].Rules = []*ast.Rule{nil} }, "incomplete"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "nil doc" {
				if err := ValidateNormalizedIR(nil); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("ValidateNormalizedIR() = %v", err)
				}
				return
			}
			doc := cloneValidationDocument(base)
			test.mutate(doc)
			if err := ValidateNormalizedIR(doc); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateNormalizedIR() = %v, want %q", err, test.want)
			}
		})
	}
	if err := ValidateCompileTarget(CompileTarget("unknown"), cloneValidationDocument(base)); err == nil || !errors.Is(err, ErrUnsupportedCompileTarget) {
		t.Fatalf("ValidateCompileTarget(unknown) = %v", err)
	}
}

func cloneValidationDocument(doc *ast.Document) *ast.Document {
	flag := doc.Flags[0]
	return &ast.Document{Flags: []*ast.Flag{{Key: flag.Key, DefaultVariant: flag.DefaultVariant, Variants: map[string]ast.VariantValue{"on": flag.Variants["on"]}, Environments: map[string]*ast.Environment{"prod": {DefaultAction: &ast.ServeAction{Variant: "on"}}}}}}
}
