package normalizedyaml

import (
	"fmt"
	"math"
	"strconv"

	"gopkg.in/yaml.v3"
)

func mapping(node *yaml.Node, path string) (map[string]*yaml.Node, error) {
	if node.Kind == yaml.AliasNode {
		return nil, fmt.Errorf("%s: yaml aliases are not supported", path)
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected mapping", path)
	}
	out := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s: expected scalar map key", path)
		}
		out[key.Value] = value
	}
	return out, nil
}

func scalarString(node *yaml.Node, path string) (string, error) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("%s: expected string scalar", path)
	}
	return node.Value, nil
}

func scalarBool(node *yaml.Node, path string) (bool, error) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return false, fmt.Errorf("%s: expected bool scalar", path)
	}
	value, err := strconv.ParseBool(node.Value)
	if err != nil {
		return false, fmt.Errorf("%s: expected bool scalar", path)
	}
	return value, nil
}

func scalarFloat(node *yaml.Node, path string) (float64, error) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("%s: expected numeric scalar", path)
	}
	value, err := strconv.ParseFloat(node.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: expected numeric scalar", path)
	}
	return value, nil
}

func scalarInt(node *yaml.Node, path string) (int64, error) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("%s: expected integer scalar", path)
	}
	value, err := strconv.ParseInt(node.Value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: expected integer scalar", path)
	}
	return value, nil
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

func nodeToAny(node *yaml.Node, path string) (any, error) {
	switch node.Kind {
	case yaml.MappingNode:
		fields, err := mapping(node, path)
		if err != nil {
			return nil, err
		}
		out := make(map[string]any, len(fields))
		for key, child := range fields {
			value, err := nodeToAny(child, path+"."+key)
			if err != nil {
				return nil, err
			}
			out[key] = value
		}
		return out, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(node.Content))
		for i, child := range node.Content {
			value, err := nodeToAny(child, fmt.Sprintf("%s[%d]", path, i))
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
