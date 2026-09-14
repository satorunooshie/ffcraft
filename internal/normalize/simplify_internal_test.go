package normalize

import (
	"testing"

	"github.com/satorunooshie/ffcraft/internal/ast"
)

func literal(value bool) ast.Condition { return &ast.LiteralBool{Value: value} }

func TestSimplifyConditionTruthTable(t *testing.T) {
	marker := &ast.Eq{Left: &ast.Var{Path: "user.id"}, Right: &ast.Scalar{Kind: ast.ScalarKindString, String: "x"}}
	tests := []struct {
		name  string
		input ast.Condition
		want  ast.Condition
	}{
		{"all false short circuits", &ast.AllOf{Conditions: []ast.Condition{marker, literal(false), marker}}, literal(false)},
		{"all removes true", &ast.AllOf{Conditions: []ast.Condition{literal(true), marker}}, marker},
		{"all flattens nested", &ast.AllOf{Conditions: []ast.Condition{&ast.AllOf{Conditions: []ast.Condition{marker}}}}, marker},
		{"all empty", &ast.AllOf{}, literal(true)},
		{"any true short circuits", &ast.AnyOf{Conditions: []ast.Condition{marker, literal(true), marker}}, literal(true)},
		{"any removes false", &ast.AnyOf{Conditions: []ast.Condition{literal(false), marker}}, marker},
		{"any flattens nested", &ast.AnyOf{Conditions: []ast.Condition{&ast.AnyOf{Conditions: []ast.Condition{marker}}}}, marker},
		{"any empty", &ast.AnyOf{}, literal(false)},
		{"not true", &ast.Not{Condition: literal(true)}, literal(false)},
		{"not false", &ast.Not{Condition: literal(false)}, literal(true)},
		{"double negation", &ast.Not{Condition: &ast.Not{Condition: marker}}, marker},
		{"opaque condition", marker, marker},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := simplifyCondition(test.input)
			switch want := test.want.(type) {
			case *ast.LiteralBool:
				value, ok := got.(*ast.LiteralBool)
				if !ok || value.Value != want.Value {
					t.Fatalf("simplifyCondition() = %#v, want literal %v", got, want.Value)
				}
			case *ast.Eq:
				if _, ok := got.(*ast.Eq); !ok {
					t.Fatalf("simplifyCondition() = %#v, want Eq", got)
				}
			default:
				t.Fatalf("unsupported expected type %T", want)
			}
		})
	}
}
