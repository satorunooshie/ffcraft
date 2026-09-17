package codegen_test

import (
	"errors"
	"fmt"
	"github.com/satorunooshie/ffcraft/internal/capability"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/authoring"
	"github.com/satorunooshie/ffcraft/internal/codegen"
	"github.com/satorunooshie/ffcraft/internal/compiler/flagd"
	"github.com/satorunooshie/ffcraft/internal/compiler/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
	"github.com/satorunooshie/ffcraft/internal/runtimeeval"
)

func TestContextInferenceAndContainsSemantics(t *testing.T) {
	for _, tc := range []struct {
		condition, wantType string
		match, miss         any
	}{
		{"gt: [{var: user.value}, 1.5]", "float64", 2.0, 1.0},
		{"eq: [{var: user.value}, 1.5]", "float64", 1.5, "1.5"},
		{"eq: [{var: user.value}, true]", "bool", true, false},
		{"in: [{var: user.value}, [1.5, 2.5]]", "float64", 1.5, 2.0},
		{"in: [{var: user.value}, [-1, -2]]", "int64", int64(-1), int64(0)},
		{"in: [{var: user.value}, [true]]", "bool", true, false},
		{"contains: [{var: user.value}, beta]", "[]string", []string{"beta", "staff"}, []string{"beta-user"}},
		{"contains: [{var: user.value}, 3]", "[]int64", []int64{3}, []any{"3", []any{2, 4}}},
		{"contains: [{var: user.value}, 1.5]", "[]float64", []float64{1.5}, []float64{2.5}},
		{"contains: [{var: user.value}, true]", "[]bool", []bool{true}, []bool{false}},
		{"in: [beta, {var: user.value}]", "[]string", []any{"beta"}, []any{"staff"}},
		{"in: [3, {var: user.value}]", "[]int64", []any{float64(3)}, []any{"3"}},
		{"string_contains: [{var: user.value}, beta]", "string", "beta-user", "staff"},
	} {
		t.Run(tc.condition, func(t *testing.T) {
			source := fmt.Sprintf(`version: v1
variant_sets:
  boolean: {on: true, off: false}
flags:
  - key: f
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        rules:
          - if: {%s}
            serve: on
        default_action: {serve: off}
`, tc.condition)
			authored, err := authoring.ParseYAML([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := normalize.Normalize(authored)
			if err != nil {
				t.Fatal(err)
			}
			// Both interchange formats must preserve array versus string semantics.
			for _, codec := range []struct {
				marshal   func(*irv1.Document) ([]byte, error)
				unmarshal func([]byte) (*irv1.Document, error)
			}{{ir.Marshal, ir.Unmarshal}, {normalizedyaml.Marshal, normalizedyaml.Unmarshal}} {
				data, err := codec.marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				doc, err = codec.unmarshal(data)
				if err != nil {
					t.Fatal(err)
				}
			}
			cond := doc.Flags["f"].Environments["prod"].Base.Rules[0].Condition
			if !runtimeeval.Evaluate(cond, runtimeeval.Context{"user": map[string]any{"value": tc.match}}) {
				t.Fatal("matching context rejected")
			}
			for _, miss := range []any{tc.miss, nil, map[string]any{}} {
				if runtimeeval.Evaluate(cond, runtimeeval.Context{"user": map[string]any{"value": miss}}) {
					t.Fatalf("unexpected match for %#v", miss)
				}
			}
			if runtimeeval.Evaluate(cond, runtimeeval.Context{}) {
				t.Fatal("missing attribute matched")
			}
			if strings.HasPrefix(tc.wantType, "[]") && runtimeeval.Evaluate(cond, runtimeeval.Context{"user": map[string]any{"value": "beta-user"}}) {
				t.Fatal("array condition accepted a string")
			}
			if strings.HasPrefix(tc.condition, "string_contains") && runtimeeval.Evaluate(cond, runtimeeval.Context{"user": map[string]any{"value": []string{"beta"}}}) {
				t.Fatal("substring condition accepted an array")
			}
			cfg := codegen.Config{PackageName: "generated", InferSDKFallback: true}
			output, err := codegen.CompileIR(doc, cfg)
			if err != nil {
				t.Fatal(err)
			}
			assertContextType(t, output, tc.wantType)
			cfg.ContextDefaults = codegen.ContextDefaultsConfig{ScalarTypes: map[string]string{"float": "int"}, CollectionTypes: map[string]string{"float": "[]int"}}
			output, err = codegen.CompileIR(doc, cfg)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.ReplaceAll(tc.wantType, "float64", "int")
			assertContextType(t, output, want)
			for targetIndex, compile := range []func(*irv1.Document, string) ([]byte, []string, error){
				func(d *irv1.Document, e string) ([]byte, []string, error) {
					return flagd.CompileIR(d, e, flagd.CompileOptions{})
				},
				func(d *irv1.Document, e string) ([]byte, []string, error) {
					return gofeatureflag.CompileIR(d, e, gofeatureflag.CompileOptions{})
				},
			} {
				output, _, err := compile(doc, "prod")
				if strings.HasPrefix(tc.wantType, "[]") || (targetIndex == 1 && strings.HasPrefix(tc.condition, "string_contains")) {
					if !errors.Is(err, capability.ErrUnsupportedCondition) || len(output) != 0 {
						t.Fatalf("unsafe output %s, error %v", output, err)
					}
				} else if err != nil || len(output) == 0 {
					t.Fatal(err)
				}
			}
		})
	}
}

func assertContextType(t *testing.T, source []byte, want string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || len(field.Names) != 1 || field.Names[0].Name != "UserValue" {
			return true
		}
		found = true
		typ := field.Type
		prefix := ""
		if array, ok := typ.(*ast.ArrayType); ok {
			prefix = "[]"
			typ = array.Elt
		}
		ident, ok := typ.(*ast.Ident)
		if !ok || prefix+ident.Name != want {
			t.Fatalf("field type = %v, want %s", field.Type, want)
		}
		return true
	})
	if !found {
		t.Fatal("UserValue field missing")
	}
}
