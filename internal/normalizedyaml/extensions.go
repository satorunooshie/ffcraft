package normalizedyaml

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"gopkg.in/yaml.v3"
)

type extensionYAML struct{ Value *irv1.ExtensionValue }

func (v extensionYAML) MarshalYAML() (any, error) { return extensionNode(v.Value), nil }
func (v *extensionYAML) UnmarshalYAML(node *yaml.Node) error {
	value, err := parseExtensionNode(node)
	if err != nil {
		return err
	}
	v.Value = value
	return nil
}

func wrapExtensions(values map[string]*irv1.ExtensionValue) map[string]extensionYAML {
	out := make(map[string]extensionYAML, len(values))
	for key, value := range values {
		out[key] = extensionYAML{Value: value}
	}
	return out
}
func unwrapExtensions(values map[string]extensionYAML) map[string]*irv1.ExtensionValue {
	out := make(map[string]*irv1.ExtensionValue, len(values))
	for key, value := range values {
		out[key] = value.Value
	}
	return out
}

func extensionNode(value *irv1.ExtensionValue) *yaml.Node {
	node := &yaml.Node{}
	switch kind := value.GetKind().(type) {
	case *irv1.ExtensionValue_StringValue:
		node.Kind, node.Tag, node.Value = yaml.ScalarNode, "!!str", kind.StringValue
	case *irv1.ExtensionValue_BoolValue:
		node.Kind, node.Tag, node.Value = yaml.ScalarNode, "!!bool", strconv.FormatBool(kind.BoolValue)
	case *irv1.ExtensionValue_IntValue:
		node.Kind, node.Tag, node.Value = yaml.ScalarNode, "!!int", strconv.FormatInt(kind.IntValue, 10)
	case *irv1.ExtensionValue_DoubleValue:
		node.Kind, node.Tag, node.Value = yaml.ScalarNode, "!!float", strconv.FormatFloat(kind.DoubleValue, 'g', -1, 64)
	case *irv1.ExtensionValue_NullValue:
		node.Kind, node.Tag, node.Value = yaml.ScalarNode, "!!null", "null"
	case *irv1.ExtensionValue_ObjectValue:
		node.Kind, node.Tag = yaml.MappingNode, "!!map"
		keys := make([]string, 0, len(kind.ObjectValue.Fields))
		for key := range kind.ObjectValue.Fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, extensionNode(kind.ObjectValue.Fields[key]))
		}
	case *irv1.ExtensionValue_ListValue:
		node.Kind, node.Tag = yaml.SequenceNode, "!!seq"
		for _, child := range kind.ListValue.Values {
			node.Content = append(node.Content, extensionNode(child))
		}
	}
	return node
}

func parseExtensionNode(node *yaml.Node) (*irv1.ExtensionValue, error) {
	switch node.Kind {
	case yaml.MappingNode:
		fields := make(map[string]*irv1.ExtensionValue, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Value == "" {
				return nil, fmt.Errorf("extension object keys must be non-empty strings")
			}
			value, err := parseExtensionNode(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			fields[key.Value] = value
		}
		return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: fields}}}, nil
	case yaml.SequenceNode:
		values := make([]*irv1.ExtensionValue, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := parseExtensionNode(child)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_ListValue{ListValue: &irv1.ExtensionList{Values: values}}}, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!null":
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_NullValue{NullValue: &irv1.ExtensionNull{}}}, nil
		case "!!str":
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_StringValue{StringValue: node.Value}}, nil
		case "!!bool":
			v, err := strconv.ParseBool(node.Value)
			if err != nil {
				return nil, err
			}
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_BoolValue{BoolValue: v}}, nil
		case "!!int":
			v, err := strconv.ParseInt(node.Value, 10, 64)
			if err != nil {
				return nil, err
			}
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_IntValue{IntValue: v}}, nil
		case "!!float":
			v, err := strconv.ParseFloat(node.Value, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("extension float must be finite")
			}
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_DoubleValue{DoubleValue: v}}, nil
		}
	}
	return nil, fmt.Errorf("unsupported extension YAML node")
}
