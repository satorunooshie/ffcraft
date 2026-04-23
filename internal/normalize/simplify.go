package normalize

import "github.com/satorunooshie/ffcraft/internal/ast"

func simplifyCondition(cond ast.Condition) ast.Condition {
	switch c := cond.(type) {
	case *ast.AllOf:
		var flat []ast.Condition
		for _, child := range c.Conditions {
			s := simplifyCondition(child)
			if isLiteralFalse(s) {
				return &ast.LiteralBool{Value: false}
			}
			if isLiteralTrue(s) {
				continue
			}
			if nested, ok := s.(*ast.AllOf); ok {
				flat = append(flat, nested.Conditions...)
				continue
			}
			flat = append(flat, s)
		}
		switch len(flat) {
		case 0:
			return &ast.LiteralBool{Value: true}
		case 1:
			return flat[0]
		default:
			return &ast.AllOf{Conditions: flat}
		}
	case *ast.AnyOf:
		var flat []ast.Condition
		for _, child := range c.Conditions {
			s := simplifyCondition(child)
			if isLiteralTrue(s) {
				return &ast.LiteralBool{Value: true}
			}
			if isLiteralFalse(s) {
				continue
			}
			if nested, ok := s.(*ast.AnyOf); ok {
				flat = append(flat, nested.Conditions...)
				continue
			}
			flat = append(flat, s)
		}
		switch len(flat) {
		case 0:
			return &ast.LiteralBool{Value: false}
		case 1:
			return flat[0]
		default:
			return &ast.AnyOf{Conditions: flat}
		}
	case *ast.Not:
		child := simplifyCondition(c.Condition)
		if isLiteralTrue(child) {
			return &ast.LiteralBool{Value: false}
		}
		if isLiteralFalse(child) {
			return &ast.LiteralBool{Value: true}
		}
		if nested, ok := child.(*ast.Not); ok {
			return simplifyCondition(nested.Condition)
		}
		return &ast.Not{Condition: child}
	default:
		return cond
	}
}

func isLiteralTrue(cond ast.Condition) bool {
	value, ok := cond.(*ast.LiteralBool)
	return ok && value.Value
}

func isLiteralFalse(cond ast.Condition) bool {
	value, ok := cond.(*ast.LiteralBool)
	return ok && !value.Value
}
