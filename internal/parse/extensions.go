package parse

import (
	"fmt"
	"math"
	"strconv"

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
		switch node.Tag {
		case "!!null":
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_NullValue{NullValue: &irv1.ExtensionNull{}}}, nil
		case "!!bool":
			v, err := strconv.ParseBool(node.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: invalid boolean", path)
			}
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_BoolValue{BoolValue: v}}, nil
		case "!!int":
			v, err := strconv.ParseInt(node.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%s: integer %q cannot be represented as int64", path, node.Value)
			}
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_IntValue{IntValue: v}}, nil
		case "!!float":
			v, err := strconv.ParseFloat(node.Value, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("%s: floating-point value must be finite", path)
			}
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_DoubleValue{DoubleValue: v}}, nil
		case "!!str":
			return &irv1.ExtensionValue{Kind: &irv1.ExtensionValue_StringValue{StringValue: node.Value}}, nil
		default:
			return nil, fmt.Errorf("%s: unsupported YAML scalar tag %q", path, node.Tag)
		}
	case yaml.AliasNode:
		return nil, fmt.Errorf("%s: yaml aliases are not supported", path)
	default:
		return nil, fmt.Errorf("%s: unsupported yaml node kind %v", path, node.Kind)
	}
}
