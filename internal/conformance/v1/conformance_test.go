package v1_test

import (
	"bytes"
	"embed"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/compiler/flagd"
	"github.com/satorunooshie/ffcraft/internal/compiler/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
	"google.golang.org/protobuf/proto"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

//go:embed testdata/*.yaml
var fixtures embed.FS

func TestV1NormalizedFixture(t *testing.T) {
	data, err := fixtures.ReadFile("testdata/extensions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := normalizedyaml.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := ir.Validate(doc); err != nil {
		t.Fatal(err)
	}
	encoded, err := normalizedyaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := normalizedyaml.Unmarshal(encoded)
	if err != nil || !proto.Equal(doc, roundTrip) {
		t.Fatalf("normalized YAML round trip mismatch: %v", err)
	}
}

func TestV1CompilerOutputIgnoresExtensions(t *testing.T) {
	data, err := fixtures.ReadFile("testdata/extensions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	withExtensions, err := normalizedyaml.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	withoutExtensions := proto.Clone(withExtensions).(*irv1.Document)
	withoutExtensions.Extensions = nil

	flagdWith, _, err := flagd.CompileIR(withExtensions, "prod", flagd.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	flagdWithout, _, err := flagd.CompileIR(withoutExtensions, "prod", flagd.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(flagdWith, flagdWithout) {
		t.Fatal("flagd output changed after stripping extensions")
	}

	goffWith, _, err := gofeatureflag.CompileIR(withExtensions, "prod", gofeatureflag.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	goffWithout, _, err := gofeatureflag.CompileIR(withoutExtensions, "prod", gofeatureflag.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(goffWith, goffWithout) {
		t.Fatal("GO Feature Flag output changed after stripping extensions")
	}
}
