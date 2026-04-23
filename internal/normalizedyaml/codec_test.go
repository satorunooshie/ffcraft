package normalizedyaml_test

import (
	"bytes"
	"embed"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/flagd"
	"github.com/satorunooshie/ffcraft/internal/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
	"github.com/satorunooshie/ffcraft/internal/parse"
)

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

func mustRoundTripDoc(t *testing.T, fixture string) *ast.Document {
	t.Helper()

	src, err := testdataFS.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read authoring fixture: %v", err)
	}

	authoringDoc, err := parse.ParseYAML(src)
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
