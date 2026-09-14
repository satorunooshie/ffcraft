package normalizedyaml

import (
	"strings"
	"testing"
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
