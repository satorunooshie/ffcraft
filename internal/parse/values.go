package parse

import (
	"fmt"
	"math"
	"strconv"

	"google.golang.org/protobuf/types/known/structpb"
	"gopkg.in/yaml.v3"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

func parseBinaryValueOperands(node *yaml.Node, path string) (*ffv1.Value, *ffv1.Value, error) {
	if node.Kind != yaml.SequenceNode || len(node.Content) != 2 {
		return nil, nil, fmt.Errorf("%s: expected a two-item sequence", path)
	}
	left, err := parseValue(node.Content[0], path+"[0]")
	if err != nil {
		return nil, nil, err
	}
	right, err := parseValue(node.Content[1], path+"[1]")
	if err != nil {
		return nil, nil, err
	}
	return left, right, nil
}

func parseValueAndStringOperands(node *yaml.Node, path string) (*ffv1.Value, string, error) {
	if node.Kind != yaml.SequenceNode || len(node.Content) != 2 {
		return nil, "", fmt.Errorf("%s: expected a two-item sequence", path)
	}
	left, err := parseValue(node.Content[0], path+"[0]")
	if err != nil {
		return nil, "", err
	}
	right, err := scalarString(node.Content[1], path+"[1]")
	if err != nil {
		return nil, "", err
	}
	return left, right, nil
}

func parseValue(node *yaml.Node, path string) (*ffv1.Value, error) {
	switch node.Kind {
	case yaml.MappingNode:
		return parseMappedValue(node, path)
	case yaml.SequenceNode:
		return parseSequenceValue(node, path)
	case yaml.ScalarNode:
		return parseScalarValue(node), nil
	case yaml.AliasNode:
		return nil, fmt.Errorf("%s: yaml aliases are not supported", path)
	default:
		return nil, fmt.Errorf("%s: unsupported yaml node kind %v", path, node.Kind)
	}
}

func parseMappedValue(node *yaml.Node, path string) (*ffv1.Value, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	if len(fields) == 1 {
		if varNode, ok := fields["var"]; ok {
			ref, err := scalarString(varNode, path+".var")
			if err != nil {
				return nil, err
			}
			return &ffv1.Value{Kind: &ffv1.Value_Var{Var: &ffv1.VarRef{Path: ref}}}, nil
		}
	}
	return nil, fmt.Errorf("%s: value mappings only support {var: ...}", path)
}

func parseSequenceValue(node *yaml.Node, path string) (*ffv1.Value, error) {
	items := make([]*ffv1.Value, 0, len(node.Content))
	stringValues := make([]string, 0, len(node.Content))
	allStrings := true

	for i, item := range node.Content {
		value, err := parseValue(item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		items = append(items, value)

		scalar := value.GetScalar()
		if scalar == nil {
			allStrings = false
			continue
		}
		stringValue, ok := scalar.Kind.(*ffv1.Scalar_StringValue)
		if !ok || stringValue.StringValue == "" {
			allStrings = false
			continue
		}
		stringValues = append(stringValues, stringValue.StringValue)
	}

	if allStrings && len(stringValues) == len(items) {
		return &ffv1.Value{Kind: &ffv1.Value_StringList{StringList: &ffv1.StringList{Values: stringValues}}}, nil
	}
	return &ffv1.Value{Kind: &ffv1.Value_List{List: &ffv1.ValueList{Values: items}}}, nil
}

func parseScalarValue(node *yaml.Node) *ffv1.Value {
	if node.Tag == "!!null" || node.Value == "null" {
		return &ffv1.Value{Kind: &ffv1.Value_Scalar{Scalar: &ffv1.Scalar{Kind: &ffv1.Scalar_NullValue{NullValue: &ffv1.NullValue{}}}}}
	}
	if value, ok := parseBoolScalar(node); ok {
		return &ffv1.Value{Kind: &ffv1.Value_Scalar{Scalar: &ffv1.Scalar{Kind: &ffv1.Scalar_BoolValue{BoolValue: value}}}}
	}
	if value, ok := parseIntScalar(node); ok {
		return &ffv1.Value{Kind: &ffv1.Value_Scalar{Scalar: &ffv1.Scalar{Kind: &ffv1.Scalar_IntValue{IntValue: value}}}}
	}
	if value, ok := parseFloatScalar(node); ok {
		return &ffv1.Value{Kind: &ffv1.Value_Scalar{Scalar: &ffv1.Scalar{Kind: &ffv1.Scalar_DoubleValue{DoubleValue: value}}}}
	}
	return &ffv1.Value{Kind: &ffv1.Value_Scalar{Scalar: &ffv1.Scalar{Kind: &ffv1.Scalar_StringValue{StringValue: node.Value}}}}
}

func parseVariantValue(node *yaml.Node, path string) (*ffv1.VariantValue, error) {
	switch node.Kind {
	case yaml.MappingNode:
		return parseObjectVariantValue(node, path)
	case yaml.SequenceNode:
		return parseListVariantValue(node, path)
	case yaml.ScalarNode:
		return parseScalarVariantValue(node), nil
	case yaml.AliasNode:
		return nil, fmt.Errorf("%s: yaml aliases are not supported", path)
	default:
		return nil, fmt.Errorf("%s: unsupported yaml node kind %v", path, node.Kind)
	}
}

func parseObjectVariantValue(node *yaml.Node, path string) (*ffv1.VariantValue, error) {
	objectValue, err := nodeToAny(node, path)
	if err != nil {
		return nil, err
	}
	fields, ok := objectValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected object", path)
	}
	value, err := structpb.NewStruct(fields)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &ffv1.VariantValue{Kind: &ffv1.VariantValue_ObjectValue{ObjectValue: value}}, nil
}

func parseListVariantValue(node *yaml.Node, path string) (*ffv1.VariantValue, error) {
	values := make([]*ffv1.VariantValue, 0, len(node.Content))
	for i, item := range node.Content {
		value, err := parseVariantValue(item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return &ffv1.VariantValue{Kind: &ffv1.VariantValue_ListValue{ListValue: &ffv1.ListValue{Values: values}}}, nil
}

func parseScalarVariantValue(node *yaml.Node) *ffv1.VariantValue {
	if node.Tag == "!!null" || node.Value == "null" {
		return &ffv1.VariantValue{Kind: &ffv1.VariantValue_NullValue{NullValue: &ffv1.NullValue{}}}
	}
	if value, ok := parseBoolScalar(node); ok {
		return &ffv1.VariantValue{Kind: &ffv1.VariantValue_BoolValue{BoolValue: value}}
	}
	if value, ok := parseIntScalar(node); ok {
		return &ffv1.VariantValue{Kind: &ffv1.VariantValue_IntValue{IntValue: value}}
	}
	if value, ok := parseFloatScalar(node); ok {
		return &ffv1.VariantValue{Kind: &ffv1.VariantValue_DoubleValue{DoubleValue: value}}
	}
	return &ffv1.VariantValue{Kind: &ffv1.VariantValue_StringValue{StringValue: node.Value}}
}

func nodeToAny(node *yaml.Node, path string) (any, error) {
	switch node.Kind {
	case yaml.MappingNode:
		fields, err := mapping(node, path)
		if err != nil {
			return nil, err
		}
		out := make(map[string]any, len(fields))
		for _, key := range sortedKeys(fields) {
			value, err := nodeToAny(fields[key], path+"."+key)
			if err != nil {
				return nil, err
			}
			out[key] = value
		}
		return out, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(node.Content))
		for i, item := range node.Content {
			value, err := nodeToAny(item, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case yaml.ScalarNode:
		if node.Tag == "!!null" || node.Value == "null" {
			return nil, nil
		}
		if value, ok := parseBoolScalar(node); ok {
			return value, nil
		}
		if value, ok := parseIntScalar(node); ok {
			return float64(value), nil
		}
		if value, ok := parseFloatScalar(node); ok {
			return value, nil
		}
		return node.Value, nil
	case yaml.AliasNode:
		return nil, fmt.Errorf("%s: yaml aliases are not supported", path)
	default:
		return nil, fmt.Errorf("%s: unsupported yaml node kind %v", path, node.Kind)
	}
}

func parseBoolScalar(node *yaml.Node) (bool, bool) {
	if node.Tag != "!!bool" && node.Value != "true" && node.Value != "false" {
		return false, false
	}
	value, err := strconv.ParseBool(node.Value)
	return value, err == nil
}

func parseIntScalar(node *yaml.Node) (int64, bool) {
	if node.Tag == "!!str" {
		return 0, false
	}
	value, err := strconv.ParseInt(node.Value, 10, 64)
	return value, err == nil
}

func parseFloatScalar(node *yaml.Node) (float64, bool) {
	if node.Tag == "!!str" {
		return 0, false
	}
	value, err := strconv.ParseFloat(node.Value, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, false
	}
	return value, true
}
