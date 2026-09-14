package authoring

import (
	"fmt"

	"gopkg.in/yaml.v3"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

func ParseYAML(data []byte) (*ffv1.FeatureFlagDocument, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if len(root.Content) != 1 {
		return nil, fmt.Errorf("yaml document must contain a single root node")
	}
	if err := validateYAMLTree(root.Content[0], "$"); err != nil {
		return nil, err
	}
	return parseRootDocument(root.Content[0], "$")
}

func validateYAMLTree(node *yaml.Node, path string) error {
	if node == nil {
		return fmt.Errorf("%s: missing YAML node", path)
	}
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("%s: yaml aliases are not supported", path)
	}
	if node.Style&yaml.TaggedStyle != 0 && !isCoreYAMLTag(node.Tag) {
		return fmt.Errorf("%s: custom YAML tags are not supported", path)
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!float" && !yamlFloatPattern.MatchString(node.Value) {
		return fmt.Errorf("%s: floating-point value must be a finite JSON number", path)
	}
	for index, child := range node.Content {
		if err := validateYAMLTree(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func isCoreYAMLTag(tag string) bool {
	switch tag {
	case "!!map", "!!seq", "!!str", "!!bool", "!!int", "!!float", "!!null":
		return true
	default:
		return false
	}
}
