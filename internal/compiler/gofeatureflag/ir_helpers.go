package gofeatureflag

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

func mergeBucketingKeys(left, right string) (string, error) {
	if left == "" {
		return right, nil
	}
	if right == "" {
		return left, nil
	}
	if left != right {
		return "", fmt.Errorf("multiple distribute stickiness values are not supported by GO Feature Flag: %q and %q", left, right)
	}
	return left, nil
}

func sortedAllocations(in map[string]float64) map[string]float64 {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]float64, len(in))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}

func marshalDocument(flags map[string]flagFile) ([]byte, error) {
	keys := make([]string, 0, len(flags))
	for key := range flags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	root := &yaml.Node{Kind: yaml.MappingNode}
	for _, key := range keys {
		value, err := yaml.Marshal(flags[key])
		if err != nil {
			return nil, fmt.Errorf("marshal flag %q: %w", key, err)
		}
		var node yaml.Node
		if err := yaml.Unmarshal(value, &node); err != nil {
			return nil, fmt.Errorf("unmarshal flag %q: %w", key, err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			node.Content[0],
		)
	}
	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal document: %w", err)
	}
	return out, nil
}
