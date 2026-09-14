package codegen

import (
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

func TestVariantKindAndLiteralTables(t *testing.T) {
	tests := []struct {
		name        string
		value       *irv1.VariantValue
		wantKind    flagKind
		wantLiteral string
	}{
		{"bool", &irv1.VariantValue{Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, flagKindBool, "true"},
		{"string", &irv1.VariantValue{Kind: &irv1.VariantValue_StringValue{StringValue: "on"}}, flagKindString, `"on"`},
		{"int", &irv1.VariantValue{Kind: &irv1.VariantValue_IntValue{IntValue: 7}}, flagKindInt, "7"},
		{"double", &irv1.VariantValue{Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 1.5}}, flagKindFloat, "1.5"},
		{"object", &irv1.VariantValue{Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"b": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}, "a": {Kind: &irv1.VariantValue_StringValue{StringValue: "x"}}}}}}, flagKindObject, `map[string]any{"a": "x", "b": false}`},
		{"list", &irv1.VariantValue{Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_IntValue{IntValue: 1}}, {Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}}}}}}, flagKindList, "[]any{1, nil}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, ok := variantKindOfIR(test.value)
			if !ok || kind != test.wantKind || goLiteralIR(test.value) != test.wantLiteral {
				t.Fatalf("variant = %v, %v, %q; want %v, true, %q", kind, ok, goLiteralIR(test.value), test.wantKind, test.wantLiteral)
			}
			got, err := variantKindIR(map[string]*irv1.VariantValue{"x": test.value})
			if err != nil || got != test.wantKind {
				t.Fatalf("variantKindIR() = %v, %v", got, err)
			}
		})
	}
	if _, err := variantKindIR(map[string]*irv1.VariantValue{"x": {}}); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("unsupported variant error = %v", err)
	}
	if _, err := variantKindIR(map[string]*irv1.VariantValue{
		"a": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
		"b": {Kind: &irv1.VariantValue_StringValue{StringValue: "x"}},
	}); err == nil || !strings.Contains(err.Error(), "homogeneous") {
		t.Fatalf("heterogeneous variant error = %v", err)
	}
}

func TestCollectIRContextFieldsTraversesConditions(t *testing.T) {
	attr := func(path []string) *irv1.AttributePath { return &irv1.AttributePath{Segments: path} }
	condition := &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{
		Conditions: []*irv1.Condition{
			{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Attribute: attr([]string{"user", "name"}), Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}}}},
			{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: attr([]string{"device", "id"})}}},
		},
	}}}
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{
		"f": {
			Variants: map[string]*irv1.VariantValue{
				"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
				"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
			},
			Environments: map[string]*irv1.Environment{
				"prod": {Base: &irv1.Evaluation{
					Rules:         []*irv1.Rule{{Condition: condition, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}},
					DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}},
				}},
			},
		},
	}}
	fields, err := collectIRContextFields(doc, ContextDefaultsConfig{}, nil)
	if err != nil || len(fields) != 2 || fields[0].Path != "device.id" || fields[1].Path != "user.name" {
		t.Fatalf("collectIRContextFields() = %#v, %v", fields, err)
	}
}

func TestContextTypeContractTables(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{" string ", "string"},
		{"int", "int"},
		{"unknown", "string"},
		{"[]string", "[]string"},
		{"map[string]any", "map[string]any"},
		{"[]unknown", "string"},
	} {
		if got := normalizeFieldType(test.input); got != test.want {
			t.Errorf("normalizeFieldType(%q) = %q, want %q", test.input, got, test.want)
		}
	}
	if !isSupportedFieldType("[]int64") || isSupportedFieldType("custom") {
		t.Fatal("isSupportedFieldType() contract violated")
	}
	if got := applyContextDefaults("int64", ContextDefaultsConfig{ScalarTypes: map[string]string{"int": "string"}}); got != "string" {
		t.Fatalf("applyContextDefaults() = %q", got)
	}
	tests := []struct {
		name     string
		defaults ContextDefaultsConfig
		want     string
	}{
		{"scalar key", ContextDefaultsConfig{ScalarTypes: map[string]string{"date": "string"}}, "unsupported key"},
		{"scalar type", ContextDefaultsConfig{ScalarTypes: map[string]string{"string": "date"}}, "unsupported type"},
		{"collection key", ContextDefaultsConfig{CollectionTypes: map[string]string{"date": "[]string"}}, "unsupported key"},
		{"collection type", ContextDefaultsConfig{CollectionTypes: map[string]string{"any": "date"}}, "unsupported type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateContextDefaults(test.defaults); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateContextDefaults() = %v, want %q", err, test.want)
			}
		})
	}
}
