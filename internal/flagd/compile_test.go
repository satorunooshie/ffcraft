package flagd_test

import (
	"bytes"
	"embed"
	"encoding/json"
	"strings"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/flagd"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/parse"
)

//go:embed testdata/*.yaml testdata/*.json
var testdataFS embed.FS

func TestCompileJSON(t *testing.T) {
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
				`"=="`,
				`"!="`,
				`\u003e`,
				`\u003e=`,
				`\u003c`,
				`\u003c=`,
				`"defaultVariant": "off"`,
			},
		},
		{
			name: "collection and string operators",
			file: "testdata/collection_string_ops.yaml",
			wantContains: []string{
				`"in"`,
				`"starts_with"`,
				`"ends_with"`,
				`"var": "user.tags"`,
				`"var": "user.email"`,
			},
		},
		{
			name: "semver and logical operators",
			file: "testdata/semver_logical_ops.yaml",
			wantContains: []string{
				`"sem_ver"`,
				`"and"`,
				`"or"`,
				`"!"`,
				`"var": "user.plan"`,
			},
		},
		{
			name: "one_of compiles to or of and clauses",
			file: "testdata/one_of.yaml",
			wantContains: []string{
				`"or"`,
				`"and"`,
				`"!"`,
			},
		},
		{
			name: "distribute action",
			file: "testdata/distribute.yaml",
			wantContains: []string{
				`"fractional"`,
				`"$flagd.flagKey"`,
				`"var": "user.id"`,
			},
		},
		{
			name: "static environment omits targeting",
			file: "testdata/static_serve.yaml",
			wantContains: []string{
				`"defaultVariant": "on"`,
			},
		},
		{
			name: "scheduled rollouts compile to descending timestamp chain",
			file: "testdata/scheduled_rollouts.yaml",
			wantContains: []string{
				`"$flagd.timestamp"`,
				`1779440400`,
				`1778835600`,
				`1778230800`,
				`1777626000`,
				`"var": "user.id"`,
			},
		},
		{
			name: "scheduled experimentation compiles as time window overlay",
			file: "testdata/scheduled_experimentation.yaml",
			wantContains: []string{
				`"$flagd.timestamp"`,
				`1778230800`,
				`1778403600`,
				`1779267600`,
				`"var": "user.id"`,
				`"var": "user.segment"`,
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
			out, err := flagd.CompileJSON(doc, "prod")
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
			if tt.file == "testdata/scheduled_rollouts.yaml" && strings.Contains(text, `1778144400`) {
				t.Fatalf("expected disabled step timestamp to be omitted\noutput:\n%s", text)
			}
			if tt.file == "testdata/static_serve.yaml" && strings.Contains(text, `"targeting"`) {
				t.Fatalf("expected static serve output to omit targeting\noutput:\n%s", text)
			}
		})
	}
}

func TestCompileJSONStructuredCases(t *testing.T) {
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

				want, err := testdataFS.ReadFile("testdata/prod.golden.json")
				if err != nil {
					t.Fatalf("read golden: %v", err)
				}
				if !bytes.Equal(bytes.TrimSpace(out), bytes.TrimSpace(want)) {
					t.Fatalf("golden mismatch\n--- got ---\n%s\n--- want ---\n%s", out, want)
				}
			},
		},
		{
			name:    "scheduled rollouts compile to descending timestamp chain",
			fixture: "testdata/scheduled_rollouts.yaml",
			validate: func(t *testing.T, _ []byte, decoded map[string]any) {
				t.Helper()

				flags := decoded["flags"].(map[string]any)
				targeting := flags["new-checkout"].(map[string]any)["targeting"].(map[string]any)

				outer := targeting["if"].([]any)
				assertUnixThreshold(t, outer[0], 1777626000)

				second := outer[2].(map[string]any)["if"].([]any)
				assertUnixThreshold(t, second[0], 1778230800)

				third := second[2].(map[string]any)["if"].([]any)
				assertUnixThreshold(t, third[0], 1778835600)

				fourth := third[2].(map[string]any)["if"].([]any)
				assertUnixThreshold(t, fourth[0], 1779440400)

				if got := fourth[2]; got != "off" {
					t.Fatalf("unexpected final fallback: %#v", got)
				}
			},
		},
		{
			name:    "scheduled experimentation compiles as overlay window",
			fixture: "testdata/scheduled_experimentation.yaml",
			validate: func(t *testing.T, _ []byte, decoded map[string]any) {
				t.Helper()

				flags := decoded["flags"].(map[string]any)
				targeting := flags["experiment-checkout"].(map[string]any)["targeting"].(map[string]any)
				outer := targeting["if"].([]any)
				condition := outer[0].(map[string]any)["and"].([]any)

				assertUnixThreshold(t, condition[0], 1778230800)
				assertUnixThreshold(t, condition[1], 1778403600)
				assertUnixUpperBound(t, condition[2], 1779267600)

				if got := outer[2]; got != "off" {
					t.Fatalf("unexpected experimentation fallback: %#v", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, decoded := mustCompiledJSON(t, tt.fixture)
			tt.validate(t, out, decoded)
		})
	}
}

func assertUnixThreshold(t *testing.T, value any, want float64) {
	t.Helper()
	cond := value.(map[string]any)[">="].([]any)
	if got := cond[1].(float64); got != want {
		t.Fatalf("unexpected threshold: got %v want %v", got, want)
	}
}

func assertUnixUpperBound(t *testing.T, value any, want float64) {
	t.Helper()
	cond := value.(map[string]any)["<"].([]any)
	if got := cond[1].(float64); got != want {
		t.Fatalf("unexpected upper bound: got %v want %v", got, want)
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

func mustCompiledJSON(t *testing.T, path string) ([]byte, map[string]any) {
	t.Helper()

	doc := mustNormalizedDoc(t, path)
	out, err := flagd.CompileJSON(doc, "prod")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	return out, decoded
}
