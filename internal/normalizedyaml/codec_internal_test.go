package normalizedyaml

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUnmarshalRejectsYAMLBoundaryForms(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"alias", "version: normalized/v1\nvalue: &x 1\nother: *x\n", "aliases"},
		{"custom tag", "version: normalized/v1\nvalue: !custom 1\n", "custom YAML tags"},
		{"duplicate key", "version: normalized/v1\nversion: normalized/v1\n", "duplicate"},
		{"root sequence", "- version\n- normalized/v1\n", "root must be a mapping"},
		{"wrong version", "version: normalized/v2\n", "unsupported normalized yaml version"},
		{"integer overflow", "version: normalized/v1\nflags:\n  '9223372036854775808': 1\n", "syntax error"},
		{"nonfinite float", "version: normalized/v1\nvalue: 1e999\n", "finite"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Unmarshal([]byte(test.yaml)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Unmarshal() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNormalizeNumericLexemesRecurses(t *testing.T) {
	value := map[string]any{
		"int_value": "9007199254740993",
		"nested":    []any{map[string]any{"int_value": "42"}},
	}
	normalizeNumericLexemes(value)
	if _, ok := value["int_value"].(int64); !ok {
		t.Fatalf("root integer type = %T", value["int_value"])
	}
	nested := value["nested"].([]any)[0].(map[string]any)
	if _, ok := nested["int_value"].(int64); !ok {
		t.Fatalf("nested integer type = %T", nested["int_value"])
	}
}

func TestNormalizedYAMLScalarContracts(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   any
	}{
		{"quoted scalar", "value: '001'\n", "001"},
		{"boolean", "value: true\n", true},
		{"null", "value: null\n", nil},
		{"integer", "value: -42\n", int64(-42)},
		{"float", "value: 1.25e2\n", float64(125)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var root yaml.Node
			if err := yaml.Unmarshal([]byte(test.source), &root); err != nil {
				t.Fatal(err)
			}
			var value any
			if err := decodeYAMLValue(root.Content[0].Content[1], &value); err != nil {
				t.Fatal(err)
			}
			if value != test.want {
				t.Fatalf("decodeYAMLValue() = %#v (%T), want %#v (%T)", value, value, test.want, test.want)
			}
		})
	}
	encoded, err := preserveDoubleLexemes([]byte("double_value: 1\nitems:\n  - double_value: 2e1\n"))
	if err != nil || !strings.Contains(string(encoded), "double_value: 1.0") || !strings.Contains(string(encoded), "double_value: 2e1") {
		t.Fatalf("preserveDoubleLexemes() = %v, %s", err, encoded)
	}
}
