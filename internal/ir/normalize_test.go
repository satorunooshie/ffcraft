package ir_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalizeir"
	"github.com/satorunooshie/ffcraft/internal/parse"
)

func TestNormalizeAndProtoRoundTrip(t *testing.T) {
	t.Parallel()
	doc, err := parse.ParseYAML([]byte(`version: v1
variant_sets:
  boolean:
    on: true
    off: false
flags:
  - key: example
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        default_action:
          serve: on
`))
	if err != nil {
		t.Fatal(err)
	}
	want, err := normalizeir.Normalize(doc)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := ir.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ir.Unmarshal(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("IR changed across protobuf round trip (-want +got):\n%s", diff)
	}
}

func TestUnknownExtensionFieldsRemainOpaque(t *testing.T) {
	t.Parallel()
	doc := &irv1.Document{
		Flags: map[string]*irv1.Flag{
			"example": {
				Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}},
				Environments: map[string]*irv1.Environment{
					"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}},
				},
			},
		},
		Extensions: map[string]*irv1.ExtensionValue{"vendor": {}},
	}
	doc.Extensions["vendor"].ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	encoded, err := ir.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.Unmarshal(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(doc.Extensions, decoded.Extensions, protocmp.Transform()); diff != "" {
		t.Fatalf("opaque extension changed (-want +got):\n%s", diff)
	}
}
