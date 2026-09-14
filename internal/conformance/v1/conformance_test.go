package v1_test

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/capability"
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
	fixtureNames, err := fs.Glob(fixtures, "testdata/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtureNames {
		t.Run(fixture, func(t *testing.T) {
			data, err := fixtures.ReadFile(fixture)
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
		})
	}
}

func TestV1CompilerOutputIgnoresExtensions(t *testing.T) {
	fixtureNames, err := fs.Glob(fixtures, "testdata/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtureNames {
		t.Run(fixture, func(t *testing.T) {
			data, err := fixtures.ReadFile(fixture)
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
		})
	}
}

func TestV1TargetCompilersFailClosedForUnrepresentablePresence(t *testing.T) {
	data, err := fixtures.ReadFile("testdata/core_conditions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := normalizedyaml.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	flag := doc.Flags["conditions"]
	flag.Environments["prod"].Base.Rules[0].Condition = &irv1.Condition{
		Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{
			Attribute: &irv1.AttributePath{Segments: []string{"user", "id"}},
		}},
	}
	if err := ir.Validate(doc); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		name string
		call func(*irv1.Document) error
	}{
		{
			name: "flagd",
			call: func(doc *irv1.Document) error {
				_, _, err := flagd.CompileIR(doc, "prod", flagd.CompileOptions{})
				return err
			},
		},
		{
			name: "gofeatureflag",
			call: func(doc *irv1.Document) error {
				_, _, err := gofeatureflag.CompileIR(doc, "prod", gofeatureflag.CompileOptions{})
				return err
			},
		},
	} {
		t.Run(target.name, func(t *testing.T) {
			err := target.call(doc)
			var capabilityError *capability.UnsupportedConditionError
			if !errors.As(err, &capabilityError) {
				t.Fatalf("error = %v, want UnsupportedConditionError", err)
			}
			if capabilityError.Code() != capability.UnsupportedConditionCode {
				t.Fatalf("diagnostic code = %q, want %q", capabilityError.Code(), capability.UnsupportedConditionCode)
			}
			if err == nil {
				t.Fatal("expected unsupported presence condition to fail closed")
			}
		})
	}
}
