package authoring

import (
	"strings"
	"testing"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

func TestParseValueSequenceContracts(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		stringList bool
		length     int
	}{
		{"non-empty strings use compact representation", "[alpha, beta]", true, 2},
		{"mixed values preserve individual values", "[alpha, 2, true]", false, 3},
		{"empty sequence remains a sequence", "[]", true, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := parseValue(yamlNode(t, test.source), "$.value")
			if err != nil {
				t.Fatal(err)
			}
			if test.stringList {
				list := value.GetStringList()
				if list == nil || len(list.Values) != test.length {
					t.Fatalf("string list = %v, want length %d", list, test.length)
				}
				return
			}
			list := value.GetList()
			if list == nil || len(list.Values) != test.length {
				t.Fatalf("value list = %v, want length %d", list, test.length)
			}
		})
	}
}

func TestParseValueAndVariantShapeContracts(t *testing.T) {
	value, err := parseValue(yamlNode(t, "{var: user.id}"), "$.value")
	if err != nil || value.GetVar().GetPath() != "user.id" {
		t.Fatalf("variable value = %v, %v", value, err)
	}
	if _, err := parseValue(yamlNode(t, "{literal: x}"), "$.value"); err == nil || !strings.Contains(err.Error(), "only support {var") {
		t.Fatalf("invalid value mapping error = %v", err)
	}
	variant, err := parseVariantValue(yamlNode(t, "{enabled: true, ids: [1, 2], missing: null}"), "$.variant")
	if err != nil {
		t.Fatal(err)
	}
	object := variant.GetObjectValue()
	if object == nil || object.Fields["enabled"].GetBoolValue() != true || len(object.Fields["ids"].GetListValue().Values) != 2 || object.Fields["missing"].GetNullValue() == nil {
		t.Fatalf("nested variant = %v", variant)
	}
	if _, err := parseVariantValue(yamlNode(t, "{value: &anchor x, copy: *anchor}"), "$.variant"); err == nil || !strings.Contains(err.Error(), "aliases are not supported") {
		t.Fatalf("variant alias error = %v", err)
	}
}

func TestParseConditionListAndScalarContracts(t *testing.T) {
	condition, err := parseCondition(yamlNode(t, "{all_of: [{literal_bool: true}, {literal_bool: false}]}"), "$.if")
	if err != nil || len(condition.GetAllOf().Conditions) != 2 {
		t.Fatalf("all_of condition = %v, %v", condition, err)
	}
	for _, test := range []struct {
		name   string
		source string
		check  func(*ffv1.Scalar) bool
	}{
		{"null", "null", func(value *ffv1.Scalar) bool { return value.GetNullValue() != nil }},
		{"bool", "true", func(value *ffv1.Scalar) bool { return value.GetBoolValue() }},
		{"integer", "7", func(value *ffv1.Scalar) bool { return value.GetIntValue() == 7 }},
		{"double", "1.25", func(value *ffv1.Scalar) bool { return value.GetDoubleValue() == 1.25 }},
		{"string", "'7'", func(value *ffv1.Scalar) bool { return value.GetStringValue() == "7" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := parseScalarValue(yamlNode(t, test.source)).GetScalar()
			if value == nil || !test.check(value) {
				t.Fatalf("scalar %q = %v", test.source, value)
			}
		})
	}
}
