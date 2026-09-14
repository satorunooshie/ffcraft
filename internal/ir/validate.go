package ir

import (
	"fmt"
	"math"
	"sort"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

// Validate checks only target-independent IR invariants.
func Validate(doc *irv1.Document) error {
	if doc == nil {
		return fmt.Errorf("IR document is nil")
	}
	if len(doc.Flags) == 0 {
		return fmt.Errorf("IR document has no flags")
	}
	for key, flag := range doc.Flags {
		if key == "" || flag == nil || len(flag.Variants) == 0 || len(flag.Environments) == 0 {
			return fmt.Errorf("flag %q is incomplete", key)
		}
		if err := validateExtensions(flag.Extensions); err != nil {
			return fmt.Errorf("flag %q extensions: %w", key, err)
		}
		for variant, value := range flag.Variants {
			if err := validateVariant(value); err != nil {
				return fmt.Errorf("flag %q variant %q: %w", key, variant, err)
			}
		}
		for name, env := range flag.Environments {
			if err := validateEnvironment(env, flag.Variants); err != nil {
				return fmt.Errorf("flag %q environment %q: %w", key, name, err)
			}
		}
	}
	return validateExtensions(doc.Extensions)
}

func validateEnvironment(env *irv1.Environment, variants map[string]*irv1.VariantValue) error {
	if env == nil || env.Base == nil {
		return fmt.Errorf("base evaluation is required")
	}
	if err := validateEvaluation(env.Base, variants); err != nil {
		return err
	}
	var previous int64 = -1
	for _, scheduled := range env.Schedule {
		if scheduled == nil || scheduled.EffectiveAt == nil || scheduled.Evaluation == nil {
			return fmt.Errorf("schedule entry is incomplete")
		}
		current := scheduled.EffectiveAt.Seconds*1_000_000_000 + int64(scheduled.EffectiveAt.Nanos)
		if current <= previous {
			return fmt.Errorf("schedule timestamps must be strictly increasing")
		}
		previous = current
		if err := validateEvaluation(scheduled.Evaluation, variants); err != nil {
			return err
		}
	}
	return validateExtensions(env.Extensions)
}

func validateEvaluation(eval *irv1.Evaluation, variants map[string]*irv1.VariantValue) error {
	if eval == nil {
		return fmt.Errorf("evaluation is nil")
	}
	if err := validateAction(eval.DefaultAction, variants); err != nil {
		return fmt.Errorf("default action: %w", err)
	}
	for index, rule := range eval.Rules {
		if rule == nil {
			return fmt.Errorf("rule[%d] is nil", index)
		}
		if err := validateCondition(rule.Condition); err != nil {
			return fmt.Errorf("rule[%d] condition: %w", index, err)
		}
		if err := validateAction(rule.Action, variants); err != nil {
			return fmt.Errorf("rule[%d] action: %w", index, err)
		}
	}
	return nil
}

func validateAction(action *irv1.Action, variants map[string]*irv1.VariantValue) error {
	switch kind := action.GetKind().(type) {
	case *irv1.Action_Serve:
		if _, ok := variants[kind.Serve]; !ok {
			return fmt.Errorf("unknown serve variant %q", kind.Serve)
		}
	case *irv1.Action_Distribute:
		if len(kind.Distribute.Weights) < 2 || kind.Distribute.AllocationKey == nil {
			return fmt.Errorf("invalid distribution")
		}
		for name, weight := range kind.Distribute.Weights {
			if weight == 0 {
				return fmt.Errorf("distribution weight %q is zero", name)
			}
			if _, ok := variants[name]; !ok {
				return fmt.Errorf("unknown distribution variant %q", name)
			}
		}
	default:
		return fmt.Errorf("action kind is required")
	}
	return nil
}

func validateCondition(condition *irv1.Condition) error {
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_Constant:
		return nil
	case *irv1.Condition_Equality:
		return validateAttributeLiteral(kind.Equality.Attribute, kind.Equality.Literal)
	case *irv1.Condition_NumericComparison:
		if kind.NumericComparison.Attribute == nil || kind.NumericComparison.Literal == nil {
			return fmt.Errorf("numeric comparison is incomplete")
		}
	case *irv1.Condition_Membership:
		if kind.Membership.Attribute == nil || kind.Membership.Literals == nil || len(kind.Membership.Literals.Values) == 0 {
			return fmt.Errorf("membership is incomplete")
		}
	case *irv1.Condition_StringMatch:
		if kind.StringMatch.Attribute == nil {
			return fmt.Errorf("string match attribute is required")
		}
	case *irv1.Condition_SemverComparison:
		if kind.SemverComparison.Attribute == nil || kind.SemverComparison.Semver == "" {
			return fmt.Errorf("semver comparison is incomplete")
		}
	case *irv1.Condition_Presence:
		if kind.Presence.Attribute == nil {
			return fmt.Errorf("presence attribute is required")
		}
	case *irv1.Condition_Logical:
		if len(kind.Logical.Conditions) < 2 {
			return fmt.Errorf("logical condition needs at least two children")
		}
		for _, child := range kind.Logical.Conditions {
			if err := validateCondition(child); err != nil {
				return err
			}
		}
	case *irv1.Condition_Negation:
		return validateCondition(kind.Negation)
	default:
		return fmt.Errorf("condition kind is required")
	}
	return nil
}

func validateAttributeLiteral(attribute *irv1.AttributePath, literal *irv1.ScalarValue) error {
	if attribute == nil || len(attribute.Segments) == 0 || literal == nil {
		return fmt.Errorf("attribute equality is incomplete")
	}
	return nil
}
func validateVariant(value *irv1.VariantValue) error {
	if value == nil {
		return fmt.Errorf("value is nil")
	}
	switch kind := value.GetKind().(type) {
	case *irv1.VariantValue_DoubleValue:
		if math.IsNaN(kind.DoubleValue) || math.IsInf(kind.DoubleValue, 0) {
			return fmt.Errorf("double is not finite")
		}
	case *irv1.VariantValue_ObjectValue:
		for name, child := range kind.ObjectValue.Fields {
			if name == "" {
				return fmt.Errorf("object key is empty")
			}
			if err := validateVariant(child); err != nil {
				return err
			}
		}
	case *irv1.VariantValue_ListValue:
		for _, child := range kind.ListValue.Values {
			if err := validateVariant(child); err != nil {
				return err
			}
		}
	case nil:
		return fmt.Errorf("value kind is required")
	}
	return nil
}
func validateExtensions(values map[string]*irv1.ExtensionValue) error {
	if len(values) > 256 {
		return fmt.Errorf("extension namespace count exceeds 256")
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "" || values[name] == nil {
			return fmt.Errorf("invalid namespace %q", name)
		}
		if err := validateExtensionDepth(values[name], 0); err != nil {
			return fmt.Errorf("namespace %q: %w", name, err)
		}
	}
	return nil
}
func validateExtensionDepth(value *irv1.ExtensionValue, depth int) error {
	if depth > 64 {
		return fmt.Errorf("extension nesting depth exceeds 64")
	}
	switch kind := value.GetKind().(type) {
	case *irv1.ExtensionValue_DoubleValue:
		if math.IsNaN(kind.DoubleValue) || math.IsInf(kind.DoubleValue, 0) {
			return fmt.Errorf("double is not finite")
		}
	case *irv1.ExtensionValue_ObjectValue:
		for name, child := range kind.ObjectValue.Fields {
			if name == "" {
				return fmt.Errorf("object key is empty")
			}
			if err := validateExtensionDepth(child, depth+1); err != nil {
				return err
			}
		}
	case *irv1.ExtensionValue_ListValue:
		if len(kind.ListValue.Values) > 256 {
			return fmt.Errorf("extension list length exceeds 256")
		}
		for _, child := range kind.ListValue.Values {
			if err := validateExtensionDepth(child, depth+1); err != nil {
				return err
			}
		}
	case nil:
		return fmt.Errorf("value kind is required")
	}
	return nil
}
