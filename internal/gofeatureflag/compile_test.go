package gofeatureflag_test

import (
	"bytes"
	"embed"
	"strings"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/parse"
	"gopkg.in/yaml.v3"
)

//go:embed testdata/*.yaml
var testdataFS embed.FS

func TestCompileYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		file         string
		wantContains []string
		wantErr      string
	}{
		{
			name: "comparison operators",
			file: "testdata/comparison_ops.yaml",
			wantContains: []string{
				" eq ",
				" ne ",
				" gt ",
				" ge ",
				" lt ",
				" le ",
				"defaultRule:",
			},
		},
		{
			name: "collection and string operators",
			file: "testdata/collection_string_ops.yaml",
			wantContains: []string{
				" in ",
				" co ",
				" sw ",
				" ew ",
				"user.tags",
				"user.email",
			},
		},
		{
			name: "semver and logical operators",
			file: "testdata/semver_logical_ops.yaml",
			wantContains: []string{
				" AND ",
				" OR ",
				"not (",
				"user.plan",
			},
		},
		{
			name: "one_of compiles to or of and clauses",
			file: "testdata/one_of.yaml",
			wantContains: []string{
				" OR ",
				" AND ",
				"not (",
			},
		},
		{
			name: "distribute action",
			file: "testdata/distribute.yaml",
			wantContains: []string{
				"bucketingKey: user.id",
				"percentage:",
				"treatment_a: 10",
			},
		},
		{
			name: "static environment omits targeting",
			file: "testdata/static_serve.yaml",
			wantContains: []string{
				"defaultRule:",
				`variation: "on"`,
			},
		},
		{
			name: "rollout strategies",
			file: "testdata/rollouts.yaml",
			wantContains: []string{
				"progressiveRollout:",
				"scheduledRollout:",
				"experimentation:",
				"targeting:",
				"bucketingKey: user.id",
			},
		},
		{
			name:    "matches returns error",
			file:    "testdata/matches_error.yaml",
			wantErr: "matches is not compiled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := mustNormalizedDoc(t, tt.file)
			out, err := gofeatureflag.CompileYAML(doc, "prod")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected compile error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("compile failed: %v", err)
			}

			text := string(out)
			for _, needle := range tt.wantContains {
				if !strings.Contains(text, needle) {
					t.Fatalf("expected output to contain %q\noutput:\n%s", needle, text)
				}
			}
			if tt.file == "testdata/static_serve.yaml" && strings.Contains(text, "targeting:") {
				t.Fatalf("expected static serve output to omit targeting\noutput:\n%s", text)
			}
			if tt.file == "testdata/rollouts.yaml" && strings.Contains(text, "disabled snapshot") {
				t.Fatalf("expected disabled scheduled step to be omitted\noutput:\n%s", text)
			}
		})
	}
}

func TestCompileYAMLStructuredCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fixture  string
		validate func(t *testing.T, out []byte, decoded map[string]any)
	}{
		{
			name:    "end to end round trip matches golden",
			fixture: "testdata/example.yaml",
			validate: func(t *testing.T, out []byte, _ map[string]any) {
				t.Helper()

				want, err := testdataFS.ReadFile("testdata/prod.golden.yaml")
				if err != nil {
					t.Fatalf("read golden: %v", err)
				}
				if !bytes.Equal(bytes.TrimSpace(out), bytes.TrimSpace(want)) {
					t.Fatalf("golden mismatch\n--- got ---\n%s\n--- want ---\n%s", out, want)
				}
			},
		},
		{
			name:    "rollout strategies compile to native fields",
			fixture: "testdata/rollouts.yaml",
			validate: func(t *testing.T, _ []byte, decoded map[string]any) {
				t.Helper()

				flag := decoded["rollout-flag"].(map[string]any)
				if got := flag["bucketingKey"]; got != "user.id" {
					t.Fatalf("unexpected bucketingKey: %#v", got)
				}

				defaultRule := flag["defaultRule"].(map[string]any)
				progressive := defaultRule["progressiveRollout"].(map[string]any)
				initial := progressive["initial"].(map[string]any)
				if got := initial["variation"]; got != "off" {
					t.Fatalf("unexpected progressive initial variation: %#v", got)
				}
				if got := initial["percentage"]; got != 0 {
					t.Fatalf("unexpected progressive initial percentage: %#v", got)
				}
				end := progressive["end"].(map[string]any)
				if got := end["variation"]; got != "on" {
					t.Fatalf("unexpected progressive end variation: %#v", got)
				}
				if got := end["percentage"]; got != 100 {
					t.Fatalf("unexpected progressive end percentage: %#v", got)
				}

				experimentation := flag["experimentation"].(map[string]any)
				if experimentation["start"] != "2026-05-01T00:00:00Z" || experimentation["end"] != "2026-05-20T00:00:00Z" {
					t.Fatalf("unexpected experimentation: %#v", experimentation)
				}

				scheduled := flag["scheduledRollout"].([]any)
				if got := len(scheduled); got != 2 {
					t.Fatalf("unexpected scheduled rollout count: %d", got)
				}

				first := scheduled[0].(map[string]any)
				if first["date"] != "2026-05-03T00:00:00Z" {
					t.Fatalf("unexpected first scheduled date: %#v", first["date"])
				}
				if first["defaultRule"].(map[string]any)["variation"] != "on" {
					t.Fatalf("unexpected first scheduled defaultRule: %#v", first["defaultRule"])
				}

				second := scheduled[1].(map[string]any)
				if second["date"] != "2026-05-09T00:00:00Z" {
					t.Fatalf("unexpected second scheduled date: %#v", second["date"])
				}
				targeting := second["targeting"].([]any)
				if len(targeting) != 1 {
					t.Fatalf("unexpected second scheduled targeting: %#v", targeting)
				}
				if targeting[0].(map[string]any)["query"] != `user.country eq "JP"` {
					t.Fatalf("unexpected second scheduled query: %#v", targeting[0])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, decoded := mustCompiledYAML(t, tt.fixture)
			tt.validate(t, out, decoded)
		})
	}
}

func mustNormalizedDoc(t *testing.T, path string) *ast.Document {
	t.Helper()

	data, err := testdataFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	doc, err := parse.ParseYAML(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	normalizedDoc, err := normalize.Normalize(doc)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	return normalizedDoc
}

func mustCompiledYAML(t *testing.T, path string) ([]byte, map[string]any) {
	t.Helper()

	doc := mustNormalizedDoc(t, path)
	out, err := gofeatureflag.CompileYAML(doc, "prod")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	var decoded map[string]any
	if err := yaml.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	return out, decoded
}
