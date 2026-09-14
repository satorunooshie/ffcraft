// Package normalizeiryaml is the deterministic human-readable adapter for
// ffcraft.ir.v1. It serializes the protobuf JSON mapping, not an internal Go
// model, so oneof and numeric domains remain owned by the IR contract.
package normalizeiryaml

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

const version = "normalized/v1"

var jsonOptions = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: false}

func Marshal(doc *irv1.Document) ([]byte, error) {
	if err := ir.Validate(doc); err != nil {
		return nil, err
	}
	payload, err := jsonOptions.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	normalizeNumericLexemes(value)
	value["version"] = version
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	return preserveDoubleLexemes(encoded)
}

func normalizeNumericLexemes(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "int_value" {
				if text, ok := child.(string); ok {
					if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
						value[key] = integer
						continue
					}
				}
			}
			normalizeNumericLexemes(child)
		}
	case []any:
		for _, child := range value {
			normalizeNumericLexemes(child)
		}
	}
}

func preserveDoubleLexemes(data []byte) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var visit func(*yaml.Node)
	visit = func(node *yaml.Node) {
		if node.Kind == yaml.MappingNode {
			for index := 0; index+1 < len(node.Content); index += 2 {
				key, value := node.Content[index], node.Content[index+1]
				if key.Value == "double_value" && value.Kind == yaml.ScalarNode {
					if _, err := strconv.ParseFloat(value.Value, 64); err == nil && !strings.ContainsAny(value.Value, ".eE") {
						value.Value += ".0"
					}
					value.Tag = "!!float"
				}
				visit(value)
			}
		} else {
			for _, child := range node.Content {
				visit(child)
			}
		}
	}
	visit(&root)
	return yaml.Marshal(&root)
}

func Unmarshal(data []byte) (*irv1.Document, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if err := validateYAMLNode(&document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("normalized YAML root must be a mapping")
	}
	data, err := yaml.Marshal(document.Content[0])
	if err != nil {
		return nil, err
	}
	decoder = yaml.NewDecoder(bytes.NewReader(data))
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if value["version"] != version {
		return nil, fmt.Errorf("unsupported normalized yaml version %q", value["version"])
	}
	delete(value, "version")
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	doc := new(irv1.Document)
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(payload, doc); err != nil {
		return nil, err
	}
	if err := ir.Validate(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func validateYAMLNode(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("YAML aliases are not supported")
	}
	if node.Tag != "" && len(node.Tag) > 0 && node.Tag[0] == '!' && node.Tag != "!!map" && node.Tag != "!!seq" && node.Tag != "!!str" && node.Tag != "!!bool" && node.Tag != "!!int" && node.Tag != "!!float" && node.Tag != "!!null" {
		return fmt.Errorf("custom YAML tags are not supported")
	}
	for _, child := range node.Content {
		if err := validateYAMLNode(child); err != nil {
			return err
		}
	}
	return nil
}
