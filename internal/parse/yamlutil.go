package parse

import (
	"fmt"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

func mapping(node *yaml.Node, path string) (map[string]*yaml.Node, error) {
	if err := expectMapping(node, path); err != nil {
		return nil, err
	}
	out := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s: expected scalar map key", path)
		}
		if _, exists := out[key.Value]; exists {
			return nil, fmt.Errorf("%s.%s: duplicate key", path, key.Value)
		}
		out[key.Value] = value
	}
	return out, nil
}

func expectMapping(node *yaml.Node, path string) error {
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("%s: yaml aliases are not supported", path)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: expected mapping", path)
	}
	return nil
}

func scalarString(node *yaml.Node, path string) (string, error) {
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("%s: expected string scalar", path)
	}
	return node.Value, nil
}

func scalarBool(node *yaml.Node, path string) (bool, error) {
	if node.Kind != yaml.ScalarNode {
		return false, fmt.Errorf("%s: expected bool scalar", path)
	}
	value, err := strconv.ParseBool(node.Value)
	if err != nil {
		return false, fmt.Errorf("%s: expected bool scalar", path)
	}
	return value, nil
}

func scalarFloat(node *yaml.Node, path string) (float64, error) {
	if node.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("%s: expected numeric scalar", path)
	}
	value, err := strconv.ParseFloat(node.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: expected numeric scalar", path)
	}
	return value, nil
}

func scalarInt(node *yaml.Node, path string) (int64, error) {
	if node.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("%s: expected integer scalar", path)
	}
	value, err := strconv.ParseInt(node.Value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: expected integer scalar", path)
	}
	return value, nil
}

func stringSequence(node *yaml.Node, path string) ([]string, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected sequence", path)
	}
	out := make([]string, 0, len(node.Content))
	for i, item := range node.Content {
		value, err := scalarString(item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
