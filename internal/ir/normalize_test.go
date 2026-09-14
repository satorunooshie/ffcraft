package ir_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/satorunooshie/ffcraft/internal/ir"
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
	want, err := ir.Normalize(doc)
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
