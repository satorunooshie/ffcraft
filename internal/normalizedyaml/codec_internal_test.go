package normalizedyaml

import (
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/proto"
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

func TestRejectUnrepresentableExtensionScopes(t *testing.T) {
	unknown := func(value proto.Message) {
		value.ProtoReflect().SetUnknown([]byte{0x80, 0x01, 0x01})
	}
	tests := []struct {
		name   string
		mutate func(*irv1.Document)
		want   string
	}{
		{"document", func(doc *irv1.Document) {
			doc.Extensions = map[string]*irv1.ExtensionValue{"x": {}}
			unknown(doc.Extensions["x"])
		}, "document extension"},
		{"flag", func(doc *irv1.Document) {
			doc.Flags["f"].Extensions = map[string]*irv1.ExtensionValue{"x": {}}
			unknown(doc.Flags["f"].Extensions["x"])
		}, "flag f extension"},
		{"environment", func(doc *irv1.Document) {
			doc.Flags["f"].Environments["prod"].Extensions = map[string]*irv1.ExtensionValue{"x": {}}
			unknown(doc.Flags["f"].Environments["prod"].Extensions["x"])
		}, "environment f/prod extension"},
		{"nested object", func(doc *irv1.Document) {
			doc.Extensions = map[string]*irv1.ExtensionValue{"x": {Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: map[string]*irv1.ExtensionValue{"child": {}}}}}}
			unknown(doc.Extensions["x"].GetObjectValue().Fields["child"])
		}, "document extension"},
		{"nested list", func(doc *irv1.Document) {
			doc.Extensions = map[string]*irv1.ExtensionValue{"x": {Kind: &irv1.ExtensionValue_ListValue{ListValue: &irv1.ExtensionList{Values: []*irv1.ExtensionValue{{}}}}}}
			unknown(doc.Extensions["x"].GetListValue().Values[0])
		}, "document extension"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}}}}}
			test.mutate(doc)
			if err := rejectUnrepresentableExtensionFields(doc); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("rejectUnrepresentableExtensionFields() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNormalizedYAMLNodeValidationContracts(t *testing.T) {
	tests := []struct {
		name string
		node *yaml.Node
		want string
	}{
		{"custom tag", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!custom", Value: "x"}, "custom YAML tags"},
		{"non scalar key", &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.SequenceNode}, {Kind: yaml.ScalarNode, Value: "x"}}}, "mapping keys must be scalars"},
		{"duplicate key", &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "x"}, {Kind: yaml.ScalarNode, Value: "1"}, {Kind: yaml.ScalarNode, Value: "x"}, {Kind: yaml.ScalarNode, Value: "2"}}}, "duplicate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateYAMLNode(test.node); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateYAMLNode() = %v, want %q", err, test.want)
			}
		})
	}
	var value any
	if err := decodeYAMLValue(&yaml.Node{Kind: yaml.DocumentNode}, &value); err == nil || !strings.Contains(err.Error(), "unsupported normalized YAML node kind") {
		t.Fatalf("decodeYAMLValue() = %v, want unsupported node kind", err)
	}
	for _, tag := range []string{"!!map", "!!seq", "!!str", "!!bool", "!!int", "!!float", "!!null"} {
		if !isCoreYAMLTag(tag) {
			t.Errorf("isCoreYAMLTag(%q) = false", tag)
		}
	}
	if isCoreYAMLTag("!custom") {
		t.Error("isCoreYAMLTag(!custom) = true")
	}
}
