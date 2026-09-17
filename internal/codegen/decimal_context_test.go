package codegen

import (
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

func TestDecimalContextFieldsUseGoFloatType(t *testing.T) {
	attribute := &irv1.AttributePath{Segments: []string{"user", "score"}}
	literal := &irv1.ScalarValue{Kind: &irv1.ScalarValue_DoubleValue{DoubleValue: 1.5}}
	for name, condition := range map[string]*irv1.Condition{
		"comparison": {Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Attribute: attribute, Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_DoubleValue{DoubleValue: 1.5}}}}},
		"equality":   {Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Attribute: attribute, Literal: literal}}},
		"membership": {Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: attribute, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{literal}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			doc := &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{Rules: []*irv1.Rule{{Condition: condition}}}}}}}}
			for _, test := range []struct {
				defaults ContextDefaultsConfig
				want     string
			}{
				{ContextDefaultsConfig{}, "float64"},
				{ContextDefaultsConfig{ScalarTypes: map[string]string{"float": "int64"}}, "int64"},
			} {
				fields, err := collectIRContextFields(doc, test.defaults, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(fields) != 1 || fields[0].FieldType != test.want {
					t.Fatalf("fields = %+v; want one %s field", fields, test.want)
				}
			}
		})
	}
}
