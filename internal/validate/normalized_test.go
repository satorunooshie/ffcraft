package validate

import (
	"math"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/ast"
)

func TestValidateNormalizedIRRejectsBrokenReferences(t *testing.T) {
	doc := &ast.Document{Flags: []*ast.Flag{{
		Key:            "flag",
		DefaultVariant: "missing",
		Variants:       map[string]ast.VariantValue{"off": {Kind: ast.VariantValueKindBool}},
	}}}
	if err := ValidateNormalizedIR(doc); err == nil {
		t.Fatal("expected broken normalized IR to be rejected")
	}
}

func TestValidateCompileTargetAppliesFlagdShapePolicy(t *testing.T) {
	doc := &ast.Document{Flags: []*ast.Flag{{
		Key:            "flag",
		DefaultVariant: "items",
		Variants:       map[string]ast.VariantValue{"items": {Kind: ast.VariantValueKindList}},
	}}}
	if err := ValidateCompileTarget(CompileTargetFlagd, doc); err == nil {
		t.Fatal("expected flagd root list capability rejection")
	}
	if err := ValidateCompileTarget(CompileTargetGOFeatureFlag, doc); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNormalizedIRRejectsNonFiniteObjectNumber(t *testing.T) {
	doc := &ast.Document{Flags: []*ast.Flag{{
		Key:            "flag",
		DefaultVariant: "object",
		Variants:       map[string]ast.VariantValue{"object": {Kind: ast.VariantValueKindObject, Object: map[string]any{"x": math.NaN()}}},
	}}}
	if err := ValidateNormalizedIR(doc); err == nil {
		t.Fatal("expected non-finite object number to be rejected")
	}
}
