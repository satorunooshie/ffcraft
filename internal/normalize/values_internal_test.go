package normalize

import (
	"strings"
	"testing"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

func TestNormalizeValueKinds(t *testing.T) {
	tests := []struct {
		name  string
		value *ffv1.Value
		want  string
	}{
		{"var", &ffv1.Value{Kind: &ffv1.Value_Var{Var: &ffv1.VarRef{Path: "user.id"}}}, "*ast.Var"},
		{"string", &ffv1.Value{Kind: &ffv1.Value_Scalar{Scalar: &ffv1.Scalar{Kind: &ffv1.Scalar_StringValue{StringValue: "x"}}}}, "*ast.Scalar"},
		{"string list", &ffv1.Value{Kind: &ffv1.Value_StringList{StringList: &ffv1.StringList{Values: []string{"a", "b"}}}}, "*ast.List"},
		{"nested list", &ffv1.Value{Kind: &ffv1.Value_List{List: &ffv1.ValueList{Values: []*ffv1.Value{{Kind: &ffv1.Value_Scalar{Scalar: &ffv1.Scalar{Kind: &ffv1.Scalar_IntValue{IntValue: 1}}}}}}}}, "*ast.List"},
		{"unset", &ffv1.Value{}, "value is unset"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := normalizeValue(test.value)
			if test.name == "unset" {
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("normalizeValue() error = %v, want %q", err, test.want)
				}
				return
			}
			if err != nil || value == nil {
				t.Fatalf("normalizeValue() = %v, %v", value, err)
			}
		})
	}
}

func TestNormalizeScalarAndVariantKinds(t *testing.T) {
	scalars := []*ffv1.Scalar{
		{Kind: &ffv1.Scalar_StringValue{StringValue: "x"}},
		{Kind: &ffv1.Scalar_BoolValue{BoolValue: true}},
		{Kind: &ffv1.Scalar_IntValue{IntValue: 1}},
		{Kind: &ffv1.Scalar_DoubleValue{DoubleValue: 1.5}},
		{Kind: &ffv1.Scalar_NullValue{NullValue: &ffv1.NullValue{}}},
	}
	for _, scalar := range scalars {
		if value, err := normalizeScalar(scalar); err != nil || value == nil {
			t.Fatalf("normalizeScalar(%v) = %v, %v", scalar, value, err)
		}
	}
	variants := []*ffv1.VariantValue{
		{Kind: &ffv1.VariantValue_BoolValue{BoolValue: true}},
		{Kind: &ffv1.VariantValue_StringValue{StringValue: "x"}},
		{Kind: &ffv1.VariantValue_IntValue{IntValue: 1}},
		{Kind: &ffv1.VariantValue_DoubleValue{DoubleValue: 1.5}},
		{Kind: &ffv1.VariantValue_ObjectValue{ObjectValue: &ffv1.ObjectValue{Fields: map[string]*ffv1.VariantValue{"nested": {Kind: &ffv1.VariantValue_BoolValue{BoolValue: true}}}}}},
		{Kind: &ffv1.VariantValue_ListValue{ListValue: &ffv1.ListValue{Values: []*ffv1.VariantValue{{Kind: &ffv1.VariantValue_NullValue{NullValue: &ffv1.NullValue{}}}}}}},
		{Kind: &ffv1.VariantValue_NullValue{NullValue: &ffv1.NullValue{}}},
		{},
	}
	for index, variant := range variants {
		if got := normalizeVariantValue(variant); got.Kind == 0 {
			if index == len(variants)-1 {
				continue
			}
			t.Fatalf("normalizeVariantValue(%v) returned unknown kind", variant)
		}
	}
}

func TestNormalizeActionContracts(t *testing.T) {
	doc := &ffv1.FeatureFlagDocument{Distributions: map[string]*ffv1.Distribution{
		"dist": {Stickiness: "user.id", Allocations: map[string]float64{"on": 1}},
	}}
	tests := []struct {
		name    string
		action  *ffv1.Action
		wantErr bool
	}{
		{"serve", &ffv1.Action{Kind: &ffv1.Action_Serve{Serve: &ffv1.Serve{Variant: "on"}}}, false},
		{"distribution", &ffv1.Action{Kind: &ffv1.Action_Distribute{Distribute: &ffv1.Distribute{Distribution: "dist"}}}, false},
		{"progressive", &ffv1.Action{Kind: &ffv1.Action_ProgressiveRollout{ProgressiveRollout: &ffv1.ProgressiveRollout{Variant: "on", Steps: 2}}}, false},
		{"unset", &ffv1.Action{}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeAction(doc, test.action)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeAction() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
