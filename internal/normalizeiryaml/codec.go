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
