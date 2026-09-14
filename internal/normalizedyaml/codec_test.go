package normalizedyaml_test

import (
	"bytes"
	"embed"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/authoring"
	"github.com/satorunooshie/ffcraft/internal/compiler/flagd"
	"github.com/satorunooshie/ffcraft/internal/compiler/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestUnmarshalRejectsUnsafeObjectIntegerBeforeFloatConversion(t *testing.T) {
	t.Parallel()

	input := []byte(`version: normalized/v1
flags:
  - key: test
    variants:
      value:
        users:
          - id: 9007199254740993
    default_variant: value
    environments:
      prod:
        static_variant: value
`)
	_, err := normalizedyaml.Unmarshal(input)
	if err == nil {
		t.Fatal("expected unsafe object integer to be rejected")
	}
	if !strings.Contains(err.Error(), "safe JSON integer range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmarshalRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := normalizedyaml.Unmarshal([]byte(`version: normalized/v1
flags:
  - key: feature-a
    variants:
      on: true
    default_variant: on
    environments:
      prod:
        static_variant: on
        typo: true
`))
	if err == nil || !strings.Contains(err.Error(), "field typo not found") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestNormalizedNumericIngress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "B0N-OBJECT-INT-SAFE-MAX-001", value: "9007199254740991"},
		{name: "B0N-OBJECT-INT-LOSSY-001", value: "9007199254740993", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte("version: normalized/v1\nflags:\n  - key: test\n    variants:\n      value:\n        id: " + tt.value + "\n    default_variant: value\n    environments:\n      prod:\n        static_variant: value\n")
			_, err := normalizedyaml.Unmarshal(input)
			if tt.wantErr && err == nil {
				t.Fatal("expected unsafe object integer to be rejected")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("safe object integer was rejected: %v", err)
			}
		})
	}
}

//go:embed testdata/*
var testdataFS embed.FS

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fixture  string
		validate func(t *testing.T, decoded *ast.Document)
	}{
		{
			name:    "end to end round trip matches golden",
			fixture: "testdata/example.yaml",
			validate: func(t *testing.T, decoded *ast.Document) {
				t.Helper()

				got, err := flagd.CompileJSON(decoded, "prod")
				if err != nil {
					t.Fatalf("compile failed: %v", err)
				}

				want, err := testdataFS.ReadFile("testdata/prod.golden.json")
				if err != nil {
					t.Fatalf("read golden: %v", err)
				}

				if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
					t.Fatalf("golden mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
				}
			},
		},
		{
			name:    "rollout metadata survives round trip",
			fixture: "testdata/valid_rollouts.yaml",
			validate: func(t *testing.T, decoded *ast.Document) {
				t.Helper()

				env := decoded.Flags[0].Environments["prod"]
				if env.Experimentation == nil || len(env.ScheduledRollouts) != 3 {
					t.Fatalf("rollout metadata was not preserved: %#v", env)
				}
				if env.ScheduledRollouts[0].Name != "first snapshot" || !env.ScheduledRollouts[1].Disabled {
					t.Fatalf("scheduled rollout metadata was not preserved: %#v", env.ScheduledRollouts)
				}
				if progressive, ok := env.DefaultAction.(*ast.ProgressiveRolloutAction); !ok || progressive.Steps != 3 {
					t.Fatalf("expected default progressive action, got %#v", env.DefaultAction)
				}
				if _, ok := env.ScheduledRollouts[0].DefaultAction.(*ast.ServeAction); !ok {
					t.Fatalf("expected authored scheduled action, got %#v", env.ScheduledRollouts[0].DefaultAction)
				}

				if _, err := gofeatureflag.CompileYAML(decoded, "prod"); err != nil {
					t.Fatalf("expected gofeatureflag compile to support rollout fields, got %v", err)
				}
				if _, err := flagd.CompileJSON(decoded, "prod"); err == nil {
					t.Fatal("expected flagd compile to reject unsupported rollout fields until compiler support is added")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			decoded := mustRoundTripDoc(t, tt.fixture)
			tt.validate(t, decoded)
		})
	}
}

func TestExtensionsRoundTripLosslessly(t *testing.T) {
	t.Parallel()

	input := []byte(`version: v1
extensions:
  metadata:
    owner: platform-team
    enabled: true
    count: 9007199254740993
    ratio: 1.0
    absent: null
    nested:
      - first
      - 2
variant_sets:
  boolean:
    on: true
    off: false
flags:
  - key: example
    variant_set: boolean
    default_variant: off
    extensions:
      analytics:
        event: evaluated
    environments:
      prod:
        default_action:
          serve: off
        extensions:
          client:
            enabled: true
`)
	authoring, err := authoring.ParseYAML(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	normalized, err := normalize.Normalize(authoring)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(normalized.Extensions) != 1 || len(normalized.Flags[0].Extensions) != 1 || len(normalized.Flags[0].Environments["prod"].Extensions) != 1 {
		t.Fatalf("extensions were not retained at every scope: %#v", normalized)
	}

	encoded, err := normalizedyaml.Marshal(normalized)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded, err := normalizedyaml.Unmarshal(encoded)
	if err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, encoded)
	}
	if diff := cmp.Diff(normalized.Extensions, decoded.Extensions, protocmp.Transform()); diff != "" {
		t.Fatalf("document extensions changed (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(normalized.Flags[0].Extensions, decoded.Flags[0].Extensions, protocmp.Transform()); diff != "" {
		t.Fatalf("flag extensions changed (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(normalized.Flags[0].Environments["prod"].Extensions, decoded.Flags[0].Environments["prod"].Extensions, protocmp.Transform()); diff != "" {
		t.Fatalf("environment extensions changed (-want +got):\n%s", diff)
	}
}

func mustRoundTripDoc(t *testing.T, fixture string) *ast.Document {
	t.Helper()

	src, err := testdataFS.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read authoring fixture: %v", err)
	}

	authoringDoc, err := authoring.ParseYAML(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	normalizedDoc, err := normalize.Normalize(authoringDoc)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	encoded, err := normalizedyaml.Marshal(normalizedDoc)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	decoded, err := normalizedyaml.Unmarshal(encoded)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	return decoded
}
