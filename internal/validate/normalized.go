package validate

import (
	"fmt"
	"math"

	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/flagd"
)

// CompileTarget identifies a target-specific capability policy.
type CompileTarget string

const (
	CompileTargetFlagd         CompileTarget = "flagd"
	CompileTargetGOFeatureFlag CompileTarget = "gofeatureflag"
)

// ValidateNormalizedIR checks the target-neutral AST after normalization.
// It is intentionally independent of any provider compiler.
func ValidateNormalizedIR(doc *ast.Document) error {
	if doc == nil {
		return fmt.Errorf("normalized document is nil")
	}
	seen := make(map[string]struct{}, len(doc.Flags))
	for index, featureFlag := range doc.Flags {
		if featureFlag == nil {
			return fmt.Errorf("flag[%d] is nil", index)
		}
		if featureFlag.Key == "" {
			return fmt.Errorf("flag[%d] has empty key", index)
		}
		if _, exists := seen[featureFlag.Key]; exists {
			return fmt.Errorf("duplicate normalized flag key %q", featureFlag.Key)
		}
		seen[featureFlag.Key] = struct{}{}
		if len(featureFlag.Variants) == 0 {
			return fmt.Errorf("flag %q has no variants", featureFlag.Key)
		}
		if _, exists := featureFlag.Variants[featureFlag.DefaultVariant]; !exists {
			return fmt.Errorf("flag %q default variant %q is not defined", featureFlag.Key, featureFlag.DefaultVariant)
		}
		for name, value := range featureFlag.Variants {
			if err := validateVariantValue(value); err != nil {
				return fmt.Errorf("flag %q variant %q: %w", featureFlag.Key, name, err)
			}
		}
		for environment, value := range featureFlag.Environments {
			if value == nil {
				return fmt.Errorf("flag %q environment %q is nil", featureFlag.Key, environment)
			}
			if value.StaticVariant != "" {
				if _, exists := featureFlag.Variants[value.StaticVariant]; !exists {
					return fmt.Errorf("flag %q environment %q static variant %q is not defined", featureFlag.Key, environment, value.StaticVariant)
				}
			}
			if err := validateAction(value.DefaultAction, featureFlag.Variants); err != nil {
				return fmt.Errorf("flag %q environment %q: %w", featureFlag.Key, environment, err)
			}
			for index, rule := range value.Rules {
				if rule == nil || rule.Action == nil {
					return fmt.Errorf("flag %q environment %q rule[%d] is incomplete", featureFlag.Key, environment, index)
				}
				if err := validateCondition(rule.Condition); err != nil {
					return fmt.Errorf("flag %q environment %q rule[%d]: %w", featureFlag.Key, environment, index, err)
				}
				if err := validateAction(rule.Action, featureFlag.Variants); err != nil {
					return fmt.Errorf("flag %q environment %q rule[%d]: %w", featureFlag.Key, environment, index, err)
				}
			}
		}
	}
	return nil
}

// ValidateCompileTarget applies normalized-IR validation and the target's
// shape capability policy before serialization.
func ValidateCompileTarget(target CompileTarget, doc *ast.Document) error {
	if err := ValidateNormalizedIR(doc); err != nil {
		return err
	}
	switch target {
	case CompileTargetFlagd:
		for _, featureFlag := range doc.Flags {
			if err := flagd.ValidateFlag(featureFlag); err != nil {
				return fmt.Errorf("flag %q: %w", featureFlag.Key, err)
			}
		}
	case CompileTargetGOFeatureFlag:
		return nil
	default:
		return fmt.Errorf("unsupported compile target %q", target)
	}
	return nil
}

func validateVariantValue(value ast.VariantValue) error {
	switch value.Kind {
	case ast.VariantValueKindBool, ast.VariantValueKindString, ast.VariantValueKindInt, ast.VariantValueKindNull:
		return nil
	case ast.VariantValueKindDouble:
		if math.IsNaN(value.Double) || math.IsInf(value.Double, 0) {
			return fmt.Errorf("non-finite double is not valid")
		}
		return nil
	case ast.VariantValueKindObject:
		return validateAnyValue(value.Object)
	case ast.VariantValueKindList:
		for index, child := range value.List {
			if err := validateVariantValue(child); err != nil {
				return fmt.Errorf("list[%d]: %w", index, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown variant value kind %d", value.Kind)
	}
}

func validateAnyValue(value any) error {
	switch value := value.(type) {
	case nil, bool, string:
		return nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("non-finite object number is not valid")
		}
		return nil
	case []any:
		for index, child := range value {
			if err := validateAnyValue(child); err != nil {
				return fmt.Errorf("list[%d]: %w", index, err)
			}
		}
		return nil
	case map[string]any:
		for key, child := range value {
			if err := validateAnyValue(child); err != nil {
				return fmt.Errorf("field %q: %w", key, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported object value type %T", value)
	}
}

func validateAction(action ast.Action, variants map[string]ast.VariantValue) error {
	switch action := action.(type) {
	case nil:
		return nil
	case *ast.ServeAction:
		if _, exists := variants[action.Variant]; !exists {
			return fmt.Errorf("serve variant %q is not defined", action.Variant)
		}
	case *ast.DistributeAction:
		for variant := range action.Allocations {
			if _, exists := variants[variant]; !exists {
				return fmt.Errorf("distribution variant %q is not defined", variant)
			}
		}
	case *ast.ProgressiveRolloutAction:
		if _, exists := variants[action.Variant]; !exists {
			return fmt.Errorf("progressive rollout variant %q is not defined", action.Variant)
		}
	default:
		return fmt.Errorf("unsupported action type %T", action)
	}
	return nil
}

func validateCondition(condition ast.Condition) error {
	if condition == nil {
		return fmt.Errorf("condition is nil")
	}
	return nil
}
