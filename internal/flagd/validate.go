package flagd

import (
	"fmt"

	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/capability"
)

// ValidateFlag checks constraints imposed by the flagd target that are not
// part of the target-neutral intermediate representation.
func ValidateFlag(flag *ast.Flag) error {
	listKind := capability.KindList
	rootPosition := capability.PositionRoot
	rules := []capability.ValueCapabilityRule{
		{
			Match:     capability.ValuePredicate{Kind: &listKind, Position: &rootPosition},
			Decision:  capability.Unsupported,
			ErrorCode: "FLAGD-ROOT-LIST-UNSUPPORTED",
			Priority:  100,
		},
		{
			Match:       capability.ValuePredicate{},
			Decision:    capability.Supported,
			EvidenceIDs: []string{"B0A-OBJECT-INT-SAFE-MAX-001", "B0A-OBJECT-LIST-INT-SAFE-001"},
		},
	}
	if err := capability.ValidateRules(rules); err != nil {
		return fmt.Errorf("flagd capability policy: %w", err)
	}
	for name, value := range flag.Variants {
		if err := capability.Walk(variantAny(value), func(facts capability.ValueFacts) error {
			decision, rule, err := capability.Resolve(rules, facts)
			if err != nil {
				return err
			}
			if decision == capability.Unsupported && rule.ErrorCode == "FLAGD-ROOT-LIST-UNSUPPORTED" {
				return fmt.Errorf("flagd does not support top-level array variant values; use an object with array fields instead")
			}
			return nil
		}); err != nil {
			return fmt.Errorf("variant %q: resolve capability: %w", name, err)
		}
	}
	return nil
}

func variantAny(value ast.VariantValue) any {
	switch value.Kind {
	case ast.VariantValueKindBool:
		return value.Bool
	case ast.VariantValueKindString:
		return value.String
	case ast.VariantValueKindInt:
		return value.Int
	case ast.VariantValueKindDouble:
		return value.Double
	case ast.VariantValueKindObject:
		return value.Object
	case ast.VariantValueKindList:
		list := make([]any, len(value.List))
		for i, child := range value.List {
			list[i] = variantAny(child)
		}
		return list
	case ast.VariantValueKindNull:
		return nil
	default:
		return nil
	}
}
