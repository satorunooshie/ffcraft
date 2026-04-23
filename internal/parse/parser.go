package parse

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
	return parseRootDocument(root.Content[0], "$")
}
