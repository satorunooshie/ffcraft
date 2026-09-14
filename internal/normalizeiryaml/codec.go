// Package normalizeiryaml is the deterministic human-readable adapter for
// ffcraft.ir.v1. It serializes the protobuf JSON mapping, not an internal Go
// model, so oneof and numeric domains remain owned by the IR contract.
package normalizeiryaml

import (
	"bytes"
	"encoding/json"
	"fmt"

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
	value["version"] = version
	return yaml.Marshal(value)
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
