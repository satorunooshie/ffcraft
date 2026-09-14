// Package normalizedyaml is the deterministic human-readable adapter for
// ffcraft.ir.v1. It serializes the protobuf JSON mapping, not an internal Go
// model, so oneof and numeric domains remain owned by the IR contract.
package normalizedyaml

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
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
	if err := rejectUnrepresentableExtensionFields(doc); err != nil {
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

// YAML has no portable representation for protobuf unknown fields. Refuse to
// serialize such an extension instead of silently losing opaque namespace data.
func rejectUnrepresentableExtensionFields(doc *irv1.Document) error {
	check := func(scope string, values map[string]*irv1.ExtensionValue) error {
		for namespace, value := range values {
			if extensionHasUnknown(value) {
				return fmt.Errorf("%s extension %q contains unknown protobuf fields that normalized YAML cannot preserve", scope, namespace)
			}
		}
		return nil
	}
	if err := check("document", doc.Extensions); err != nil {
		return err
	}
	for flagKey, flag := range doc.Flags {
		if err := check("flag "+flagKey, flag.Extensions); err != nil {
			return err
		}
		for environment, env := range flag.Environments {
			if err := check("environment "+flagKey+"/"+environment, env.Extensions); err != nil {
				return err
			}
		}
	}
	return nil
}

func extensionHasUnknown(value *irv1.ExtensionValue) bool {
	if value == nil {
		return false
	}
	if len(value.ProtoReflect().GetUnknown()) != 0 {
		return true
	}
	switch kind := value.GetKind().(type) {
	case *irv1.ExtensionValue_ObjectValue:
		if kind.ObjectValue == nil || len(kind.ObjectValue.ProtoReflect().GetUnknown()) != 0 {
			return true
		}
		for _, child := range kind.ObjectValue.Fields {
			if extensionHasUnknown(child) {
				return true
			}
		}
	case *irv1.ExtensionValue_ListValue:
		if kind.ListValue == nil || len(kind.ListValue.ProtoReflect().GetUnknown()) != 0 {
			return true
		}
		for _, child := range kind.ListValue.Values {
			if extensionHasUnknown(child) {
				return true
			}
		}
	case *irv1.ExtensionValue_NullValue:
		return kind.NullValue != nil && len(kind.NullValue.ProtoReflect().GetUnknown()) != 0
	}
	return false
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
	var value any
	if err := decodeYAMLValue(document.Content[0], &value); err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("normalized YAML root must be a mapping")
	}
	value = root
	valueMap := value.(map[string]any)
	if valueMap["version"] != version {
		return nil, fmt.Errorf("unsupported normalized yaml version %q", valueMap["version"])
	}
	delete(valueMap, "version")
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
	if node.Style&yaml.TaggedStyle != 0 || node.Tag != "" && len(node.Tag) > 0 && node.Tag[0] == '!' && node.Tag != "!!map" && node.Tag != "!!seq" && node.Tag != "!!str" && node.Tag != "!!bool" && node.Tag != "!!int" && node.Tag != "!!float" && node.Tag != "!!null" {
		return fmt.Errorf("custom YAML tags are not supported")
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("normalized YAML mapping keys must be scalars")
			}
			if _, exists := seen[key.Value]; exists {
				return fmt.Errorf("duplicate normalized YAML mapping key %q", key.Value)
			}
			seen[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := validateYAMLNode(child); err != nil {
			return err
		}
	}
	return nil
}

var (
	normalizedIntegerPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	normalizedFloatPattern   = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+|[eE][+-]?[0-9]+|\.[0-9]+[eE][+-]?[0-9]+)$`)
)

func decodeYAMLValue(node *yaml.Node, out *any) error {
	switch node.Kind {
	case yaml.MappingNode:
		value := make(map[string]any, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			var child any
			if err := decodeYAMLValue(node.Content[i+1], &child); err != nil {
				return err
			}
			value[key.Value] = child
		}
		*out = value
	case yaml.SequenceNode:
		value := make([]any, len(node.Content))
		for i, child := range node.Content {
			if err := decodeYAMLValue(child, &value[i]); err != nil {
				return err
			}
		}
		*out = value
	case yaml.ScalarNode:
		if node.Style != 0 {
			*out = node.Value
			return nil
		}
		switch {
		case node.Tag == "!!null" && node.Value == "null":
			*out = nil
		case node.Tag == "!!bool" && (node.Value == "true" || node.Value == "false"):
			*out = node.Value == "true"
		case normalizedIntegerPattern.MatchString(node.Value):
			value, err := strconv.ParseInt(node.Value, 10, 64)
			if err != nil {
				return fmt.Errorf("integer %q cannot be represented as int64", node.Value)
			}
			*out = value
		case normalizedFloatPattern.MatchString(node.Value):
			value, err := strconv.ParseFloat(node.Value, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("floating-point value %q must be finite", node.Value)
			}
			*out = value
		default:
			*out = node.Value
		}
	default:
		return fmt.Errorf("unsupported normalized YAML node kind %v", node.Kind)
	}
	return nil
}
