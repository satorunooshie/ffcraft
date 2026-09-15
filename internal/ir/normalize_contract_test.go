package ir

import (
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ast"
)

func TestFromASTCanonicalizesConditionAndVariantSurface(t *testing.T) {
	variable := func(path string) *ast.Var { return &ast.Var{Path: path} }
	stringValue := func(value string) *ast.Scalar { return &ast.Scalar{Kind: ast.ScalarKindString, String: value} }
	numericValue := &ast.Scalar{Kind: ast.ScalarKindDouble, Double: 1.5}
	conditions := []ast.Condition{
		&ast.Eq{Left: variable("user.id"), Right: stringValue("beta")},
		&ast.Ne{Left: stringValue("control"), Right: variable("user.id")},
		&ast.Gt{Left: variable("score"), Right: numericValue},
		&ast.Gte{Left: variable("score"), Right: &ast.Scalar{Kind: ast.ScalarKindInt, Int: 1}},
		&ast.Lt{Left: variable("score"), Right: numericValue},
		&ast.Lte{Left: variable("score"), Right: numericValue},
		&ast.In{Target: variable("region"), Candidate: &ast.List{Values: []ast.Value{stringValue("jp"), stringValue("us")}}},
		&ast.Contains{Container: variable("user.tags"), Value: stringValue("beta")},
		&ast.StartsWith{Target: variable("user.id"), Prefix: "test-"},
		&ast.EndsWith{Target: variable("user.id"), Suffix: "-1"},
		&ast.SemverGt{Left: variable("version"), Right: "1.2.3"},
		&ast.SemverLte{Left: variable("version"), Right: "9.0.0"},
		&ast.AllOf{Conditions: []ast.Condition{&ast.LiteralBool{Value: true}}},
		&ast.AnyOf{Conditions: []ast.Condition{&ast.LiteralBool{Value: false}}},
		&ast.OneOf{Conditions: []ast.Condition{&ast.LiteralBool{Value: true}, &ast.LiteralBool{Value: false}}},
		&ast.Not{Condition: &ast.LiteralBool{Value: false}},
	}
	rules := make([]*ast.Rule, 0, len(conditions))
	for _, condition := range conditions {
		rules = append(rules, &ast.Rule{Condition: condition, Action: &ast.ServeAction{Variant: "on"}})
	}
	doc, err := FromAST(&ast.Document{Flags: []*ast.Flag{{
		Key:            "surface",
		DefaultVariant: "off",
		Variants:       map[string]ast.VariantValue{"on": {Kind: ast.VariantValueKindBool, Bool: true}, "off": {Kind: ast.VariantValueKindBool, Bool: false}},
		Environments:   map[string]*ast.Environment{"prod": {Rules: rules, DefaultAction: &ast.ServeAction{Variant: "off"}}},
	}, {
		Key: "object", DefaultVariant: "value",
		Variants:     map[string]ast.VariantValue{"value": {Kind: ast.VariantValueKindObject, Object: map[string]any{"nested": []any{int64(7), nil}}}},
		Environments: map[string]*ast.Environment{"prod": {DefaultAction: &ast.ServeAction{Variant: "value"}}},
	}, {
		Key: "list", DefaultVariant: "value",
		Variants:     map[string]ast.VariantValue{"value": {Kind: ast.VariantValueKindList, List: []ast.VariantValue{{Kind: ast.VariantValueKindString, String: "x"}}}},
		Environments: map[string]*ast.Environment{"prod": {DefaultAction: &ast.ServeAction{Variant: "value"}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	environment := doc.Flags["surface"].Environments["prod"]
	if len(environment.Base.Rules) != len(conditions) {
		t.Fatalf("normalized rule count = %d, want %d", len(environment.Base.Rules), len(conditions))
	}
	if _, ok := environment.Base.Rules[0].Condition.GetKind().(*irv1.Condition_Equality); !ok {
		t.Fatalf("equality condition = %T", environment.Base.Rules[0].Condition.GetKind())
	}
	if _, ok := environment.Base.Rules[6].Condition.GetKind().(*irv1.Condition_Membership); !ok {
		t.Fatalf("membership condition = %T", environment.Base.Rules[6].Condition.GetKind())
	}
	if _, ok := environment.Base.Rules[12].Condition.GetKind().(*irv1.Condition_Constant); !ok {
		t.Fatalf("single-child logical condition was not canonicalized: %T", environment.Base.Rules[12].Condition.GetKind())
	}
	if constant, ok := environment.Base.Rules[15].Condition.GetKind().(*irv1.Condition_Constant); !ok || !constant.Constant {
		t.Fatalf("negated constant = %v, want true", environment.Base.Rules[15].Condition)
	}
	object := doc.Flags["object"].Variants["value"].GetObjectValue()
	if object == nil || object.Fields["nested"].GetListValue().Values[0].GetIntValue() != 7 || object.Fields["nested"].GetListValue().Values[1].GetNullValue() == nil {
		t.Fatalf("nested object variant = %v", object)
	}
}
