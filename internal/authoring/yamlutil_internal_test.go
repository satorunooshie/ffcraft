package authoring

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func yamlNode(t *testing.T, source string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(source), &root); err != nil {
		t.Fatal(err)
	}
	return root.Content[0]
}

func TestYAMLScalarConversionContracts(t *testing.T) {
	tests := []struct {
		name  string
		value string
		parse func(*yaml.Node) error
		want  string
	}{
		{"bool true", "true", func(node *yaml.Node) error { _, err := scalarBool(node, "$.value"); return err }, ""},
		{"bool false", "false", func(node *yaml.Node) error { _, err := scalarBool(node, "$.value"); return err }, ""},
		{"integer", "-42", func(node *yaml.Node) error { _, err := scalarInt(node, "$.value"); return err }, ""},
		{"float", "1.25", func(node *yaml.Node) error { _, err := scalarFloat(node, "$.value"); return err }, ""},
		{"string", "hello", func(node *yaml.Node) error { _, err := scalarString(node, "$.value"); return err }, ""},
		{"invalid bool", "maybe", func(node *yaml.Node) error { _, err := scalarBool(node, "$.value"); return err }, "expected bool"},
		{"invalid integer", "1.5", func(node *yaml.Node) error { _, err := scalarInt(node, "$.value"); return err }, "expected integer"},
		{"invalid float", "not-a-number", func(node *yaml.Node) error { _, err := scalarFloat(node, "$.value"); return err }, "expected numeric"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.parse(yamlNode(t, test.value))
			if test.want == "" && err != nil {
				t.Fatalf("conversion error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("conversion error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestYAMLStructureAndValueContracts(t *testing.T) {
	if _, err := mapping(yamlNode(t, "[1, 2]"), "$.object"); err == nil {
		t.Fatal("mapping accepted sequence")
	}
	if _, err := stringSequence(yamlNode(t, "[one, 2]"), "$.tags"); err != nil {
		t.Fatalf("stringSequence rejected scalar values: %v", err)
	}
	if _, err := stringSequence(yamlNode(t, "one"), "$.tags"); err == nil {
		t.Fatal("stringSequence accepted scalar")
	}
	if _, err := nodeToAny(yamlNode(t, "{a: [true, null, 1.5], b: text}"), "$"); err != nil {
		t.Fatalf("nodeToAny rejected nested value: %v", err)
	}
	if _, err := nodeToAny(yamlNode(t, "{a: &x 1, b: *x}"), "$"); err == nil {
		t.Fatal("nodeToAny accepted alias")
	}
	if got := sortedKeys(map[string]int{"b": 1, "a": 2}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("sortedKeys() = %#v", got)
	}
}
