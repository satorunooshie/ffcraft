package compiler_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-feature/flagd/core/pkg/evaluator"
	"github.com/open-feature/flagd/core/pkg/logger"
	"github.com/open-feature/flagd/core/pkg/store"
	flagsync "github.com/open-feature/flagd/core/pkg/sync"
)

// Compile real authoring input through the CLI, then evaluate its output using
// the flagd core v0.15.0 dependency pinned by this module. No copied lowering.
func TestCompiledStringContainsWithFlagdCore(t *testing.T) {
	temp := t.TempDir()
	binary := filepath.Join(temp, "ffcompile")
	build := exec.Command("go", "build", "-o", binary, "./cmd/ffcompile")
	build.Dir = "../../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build compiler: %v\n%s", err, output)
	}
	var source strings.Builder
	source.WriteString("version: v1\nvariant_sets:\n  boolean: {on: true, off: false}\nflags:\n")
	needles := map[string]string{"substring": "beta", "empty": "", "numeric": "3", "unicode": "日本"}
	for key, needle := range needles {
		for _, negated := range []bool{false, true} {
			name, condition := key, fmt.Sprintf("string_contains: [{var: user.value}, %q]", needle)
			if negated {
				name += "-not"
				condition = "not: {" + condition + "}"
			}
			fmt.Fprintf(&source, `  - key: %s
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        rules:
          - if: {%s}
            serve: on
        default_action: {serve: off}
`, name, condition)
		}
	}
	input := filepath.Join(temp, "flags.yaml")
	if err := os.WriteFile(input, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "build", "flagd", "--in", input, "--env", "prod")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("compile: %v\n%s", err, output)
	}
	engine := evaluator.NewJSON(logger.NewLogger(nil, false), store.NewFlags())
	if err := engine.SetState(flagsync.DataSync{FlagData: string(output), Source: "test://compiled-contains"}); err != nil {
		t.Fatalf("load compiled configuration: %v", err)
	}
	cases := []struct {
		name    string
		value   any
		missing bool
	}{
		{"match", "beta-user", false}, {"miss", "staff", false}, {"empty", "", false},
		{"numeric-string", "123", false}, {"unicode", "日本語", false},
		{"array", []any{"beta"}, false}, {"empty-array", []any{}, false},
		{"nested-array", []any{[]any{2, 4}}, false}, {"number", 3, false},
		{"bool", true, false}, {"null", nil, false}, {"object", map[string]any{"value": "beta"}, false},
		{"missing", nil, true},
	}
	for _, tc := range cases {
		for key, needle := range needles {
			for _, negated := range []bool{false, true} {
				name := key
				text, isString := tc.value.(string)
				want := !tc.missing && isString && strings.Contains(text, needle)
				if negated {
					name += "-not"
					want = !want
				}
				t.Run(name+"/"+tc.name, func(t *testing.T) {
					ctx := map[string]any{}
					if !tc.missing {
						ctx["user"] = map[string]any{"value": tc.value}
					}
					got, _, _, _, err := engine.ResolveBooleanValue(context.Background(), "test", name, ctx)
					if err != nil || got != want {
						t.Fatalf("value=%#v: got %v, error %v; want %v", tc.value, got, err, want)
					}
				})
			}
		}
	}
}
