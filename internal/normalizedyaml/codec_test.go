package normalizedyaml_test

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:embed testdata/conformance_extensions.yaml
var conformanceFixture []byte

func TestRoundTripPreservesIRKinds(t *testing.T) {
	doc := &irv1.Document{
		Flags: map[string]*irv1.Flag{
			"example": {
				Variants: map[string]*irv1.VariantValue{
					"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
				},
				Environments: map[string]*irv1.Environment{
					"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}},
				},
			},
		},
		Extensions: map[string]*irv1.ExtensionValue{
			"meta": {Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: map[string]*irv1.ExtensionValue{
				"integer": {Kind: &irv1.ExtensionValue_IntValue{IntValue: 9007199254740993}},
				"double":  {Kind: &irv1.ExtensionValue_DoubleValue{DoubleValue: 1}},
			}}}},
		},
	}
	encoded, err := normalizedyaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `int_value: "`) || !strings.Contains(string(encoded), "double_value: 1.0") {
		t.Fatalf("numeric lexical kinds were not preserved:\n%s", encoded)
	}
	decoded, err := normalizedyaml.Unmarshal(encoded)
	if err != nil {
		t.Fatalf("%v\n%s", err, encoded)
	}
	if diff := cmp.Diff(doc, decoded, protocmp.Transform()); diff != "" {
		t.Fatalf("IR changed (-want +got):\n%s", diff)
	}
}

func TestConformanceExtensionFixture(t *testing.T) {
	doc, err := normalizedyaml.Unmarshal(conformanceFixture)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := normalizedyaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := normalizedyaml.Unmarshal(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(doc, decoded, protocmp.Transform()); diff != "" {
		t.Fatalf("fixture changed across round trip:\n%s", diff)
	}
}

func TestRoundTripPreservesAllVariantKindsAndSchedule(t *testing.T) {
	path := &irv1.AttributePath{Segments: []string{"user", "id"}}
	condition := &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{
		Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_ALL,
		Conditions: []*irv1.Condition{{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{
			Operator:  irv1.EqualityOperator_EQUALITY_OPERATOR_EQ,
			Attribute: path,
			Literal:   &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "beta"}},
		}}}, {Kind: &irv1.Condition_Constant{Constant: true}}},
	}}}
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{
		"object": {
			Variants: map[string]*irv1.VariantValue{
				"value": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{
					"enabled": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
					"items":   {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_IntValue{IntValue: 7}}, {Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}}}}}},
				}}}},
			},
			Environments: map[string]*irv1.Environment{"prod": {
				Base:     &irv1.Evaluation{Rules: []*irv1.Rule{{Condition: condition, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "value"}}}}, DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "value"}}},
				Schedule: []*irv1.ScheduledEvaluation{{EffectiveAt: timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 123, time.UTC)), Evaluation: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "value"}}}}},
			}},
		},
	}}
	encoded, err := normalizedyaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := normalizedyaml.Unmarshal(encoded)
	if err != nil {
		t.Fatalf("Unmarshal() = %v\n%s", err, encoded)
	}
	if diff := cmp.Diff(doc, decoded, protocmp.Transform()); diff != "" {
		t.Fatalf("IR changed across full semantic round trip (-want +got):\n%s", diff)
	}
}
