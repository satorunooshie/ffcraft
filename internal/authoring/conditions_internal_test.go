package authoring

import (
	"strings"
	"testing"
)

func TestConditionParserShapeContracts(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"unknown operator", "{regex: [x, y]}", "unsupported condition operator"},
		{"multiple operators", "{eq: [x, y], ne: [x, y]}", "exactly one operator"},
		{"all_of expects sequence", "{all_of: true}", "expected sequence"},
		{"nested invalid condition", "{not: {regex: x}}", "unsupported condition operator"},
		{"binary arity", "{eq: [x]}", "two-item sequence"},
		{"binary mapping", "{eq: x}", "two-item sequence"},
		{"string arity", "{starts_with: [x]}", "two-item sequence"},
		{"string literal type", "{starts_with: [x, {var: y}]}", "expected string scalar"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseCondition(yamlNode(t, test.source), "$.if"); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseCondition() error = %v, want %q", err, test.want)
			}
		})
	}
}
