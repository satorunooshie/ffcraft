package normalizeiryaml_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/normalizeiryaml"
)

func TestRoundTripPreservesIRKinds(t *testing.T) {
	doc := &irv1.Document{
		Flags: map[string]*irv1.Flag{
			"example": {
				Variants: map[string]*irv1.VariantValue{
					"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
				},
				Environments: map[string]*irv1.Environment{
					"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}},
				},
			},
		},
		Extensions: map[string]*irv1.ExtensionValue{
			"meta": {Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: map[string]*irv1.ExtensionValue{
				"integer": {Kind: &irv1.ExtensionValue_IntValue{IntValue: 9007199254740993}},
				"double":  {Kind: &irv1.ExtensionValue_DoubleValue{DoubleValue: 1}},
			}}}},
		},
	}
	encoded, err := normalizeiryaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `int_value: "`) || !strings.Contains(string(encoded), "double_value: 1.0") {
		t.Fatalf("numeric lexical kinds were not preserved:\n%s", encoded)
	}
	decoded, err := normalizeiryaml.Unmarshal(encoded)
	if err != nil {
		t.Fatalf("%v\n%s", err, encoded)
	}
	if diff := cmp.Diff(doc, decoded, protocmp.Transform()); diff != "" {
		t.Fatalf("IR changed (-want +got):\n%s", diff)
	}
}
