package authoring

import (
	"fmt"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"gopkg.in/yaml.v3"
)

func parseExtensions(node *yaml.Node, path string) (map[string]*irv1.ExtensionValue, error) {
	if node == nil {
		return map[string]*irv1.ExtensionValue{}, nil
	}
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*irv1.ExtensionValue, len(fields))
	for name, child := range fields {
		if name == "" {
			return nil, fmt.Errorf("%s: extension namespace must not be empty", path)
		}
		value, err := parseExtensionValue(child, path+"."+name)
		if err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}

func parseExtensionValue(node *yaml.Node, path string) (*irv1.ExtensionValue, error) {
	switch node.Kind {
	case yaml.MappingNode:
		fields, err := mapping(node, path)
		if err != nil {
			return nil, err
		}
		values := make(map[string]*irv1.ExtensionValue, len(fields))
		for name, child := range fields {
			if name == "" {
				return nil, fmt.Errorf("%s: extension object field must not be empty", path)
			}
			value, err := parseExtensionValue(child, path+"."+name)
			if err != nil {
				return nil, err
			}
			values[name] = value
		}
		return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: values}}}, nil
	case yaml.SequenceNode:
		values := make([]*irv1.ExtensionValue, 0, len(node.Content))
		for i, child := range node.Content {
			value, err := parseExtensionValue(child, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_ListValue{ListValue: &irv1.ExtensionList{Values: values}}}, nil
	case yaml.ScalarNode:
		if node.Style != 0 {
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_StringValue{StringValue: node.Value}}, nil
		}
		if isExactNull(node) {
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_NullValue{NullValue: &irv1.ExtensionNull{}}}, nil
		}
		if value, ok := parseBoolScalar(node); ok {
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_BoolValue{BoolValue: value}}, nil
		}
		if value, ok := parseIntScalar(node); ok {
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_IntValue{IntValue: value}}, nil
		}
		if yamlIntegerPattern.MatchString(node.Value) {
			return nil, fmt.Errorf("%s: integer %q cannot be represented as int64", path, node.Value)
		}
		if value, ok := parseFloatScalar(node); ok {
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_DoubleValue{DoubleValue: value}}, nil
		}
		if yamlFloatPattern.MatchString(node.Value) {
			return nil, fmt.Errorf("%s: floating-point value must be finite", path)
		}
		return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_StringValue{StringValue: node.Value}}, nil
	case yaml.AliasNode:
		return nil, fmt.Errorf("%s: yaml aliases are not supported", path)
	default:
		return nil, fmt.Errorf("%s: unsupported yaml node kind %v", path, node.Kind)
	}
}
