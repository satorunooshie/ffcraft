package validate

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"buf.build/go/protovalidate"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

var expiryPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

var (
	validatorOnce sync.Once
	validatorInst protovalidate.Validator
	validatorErr  error
)

func Validate(doc *ffv1.FeatureFlagDocument) error {
	validator, err := validator()
	if err != nil {
		return err
	}
	if err := validator.Validate(doc); err != nil {
		return err
	}
	return validateReferences(doc)
}

func validator() (protovalidate.Validator, error) {
	validatorOnce.Do(func() {
		validatorInst, validatorErr = protovalidate.New()
	})
	return validatorInst, validatorErr
}

func validateReferences(doc *ffv1.FeatureFlagDocument) error {
	var errs []error
	seenFlags := map[string]struct{}{}

	for _, flag := range doc.Flags {
		if _, exists := seenFlags[flag.Key]; exists {
			errs = append(errs, fmt.Errorf("duplicate flag key %q", flag.Key))
		} else {
			seenFlags[flag.Key] = struct{}{}
		}

		vs, ok := doc.VariantSets[flag.VariantSet]
		if !ok {
			errs = append(errs, fmt.Errorf("flag %q: unknown variant_set %q", flag.Key, flag.VariantSet))
			continue
		}

		if _, ok := vs.Variants[flag.DefaultVariant]; !ok {
			errs = append(errs, fmt.Errorf("flag %q: default_variant %q not found in variant_set %q", flag.Key, flag.DefaultVariant, flag.VariantSet))
		}
		if metadata := flag.Metadata; metadata != nil && metadata.Expiry != "" && !expiryPattern.MatchString(metadata.Expiry) {
			errs = append(errs, fmt.Errorf("flag %q: metadata.expiry must be YYYY-MM-DD", flag.Key))
		}

		for envName, env := range flag.Environments {
			if fixed := env.GetFixedServe(); fixed != nil {
				if _, ok := vs.Variants[fixed.Variant]; !ok {
					errs = append(errs, fmt.Errorf("flag %q env %q: serve variant %q not found in variant_set %q", flag.Key, envName, fixed.Variant, flag.VariantSet))
				}
				continue
			}

			eval := env.GetRuleEvaluation()
			if eval == nil {
				errs = append(errs, fmt.Errorf("flag %q env %q: environment must define serve or rules", flag.Key, envName))
				continue
			}
			if len(eval.Rules) == 0 && eval.DefaultAction == nil && eval.Experimentation == nil && len(eval.ScheduledRollouts) == 0 {
				errs = append(errs, fmt.Errorf("flag %q env %q: rule_evaluation must define rules, default_action, experimentation, or scheduled_rollouts", flag.Key, envName))
				continue
			}

			if eval.DefaultAction != nil {
				if err := validateActionRefs(doc, vs, eval.DefaultAction, true); err != nil {
					errs = append(errs, fmt.Errorf("flag %q env %q: default action: %w", flag.Key, envName, err))
				}
			}
			if eval.Experimentation != nil {
				if err := validateExperimentation(eval.Experimentation); err != nil {
					errs = append(errs, fmt.Errorf("flag %q env %q: experimentation: %w", flag.Key, envName, err))
				}
			}

			for idx, entry := range eval.Rules {
				if err := validateConditionRefs(doc, entry.If); err != nil {
					errs = append(errs, fmt.Errorf("flag %q env %q rule[%d]: %w", flag.Key, envName, idx, err))
				}
				if err := validateActionRefs(doc, vs, entry.Action, false); err != nil {
					errs = append(errs, fmt.Errorf("flag %q env %q rule[%d]: %w", flag.Key, envName, idx, err))
				}
			}
			for idx, step := range eval.ScheduledRollouts {
				if err := validateScheduledStep(doc, vs, step); err != nil {
					errs = append(errs, fmt.Errorf("flag %q env %q scheduled_rollouts[%d]: %w", flag.Key, envName, idx, err))
				}
			}
			if err := validateScheduledRollouts(eval.ScheduledRollouts); err != nil {
				errs = append(errs, fmt.Errorf("flag %q env %q: %w", flag.Key, envName, err))
			}
		}
	}

	if err := detectRuleCycles(doc); err != nil {
		errs = append(errs, err)
	}

	for name, dist := range doc.Distributions {
		if err := validateStickiness(dist.Stickiness); err != nil {
			errs = append(errs, fmt.Errorf("distribution %q: %w", name, err))
		}
		total := 0.0
		for _, value := range dist.Allocations {
			total += value
		}
		if math.Abs(total-100.0) > 1e-9 {
			errs = append(errs, fmt.Errorf("distribution %q: allocation total must equal 100", name))
		}
	}

	return errors.Join(errs...)
}

func validateActionRefs(doc *ffv1.FeatureFlagDocument, variants *ffv1.VariantSet, action *ffv1.Action, allowProgressive bool) error {
	switch kind := action.Kind.(type) {
	case *ffv1.Action_Serve:
		if _, ok := variants.Variants[kind.Serve.Variant]; !ok {
			return fmt.Errorf("serve variant %q not found", kind.Serve.Variant)
		}
	case *ffv1.Action_Distribute:
		dist, ok := doc.Distributions[kind.Distribute.Distribution]
		if !ok {
			return fmt.Errorf("distribution %q not found", kind.Distribute.Distribution)
		}
		for variant := range dist.Allocations {
			if _, ok := variants.Variants[variant]; !ok {
				return fmt.Errorf("distribution %q references unknown variant %q", kind.Distribute.Distribution, variant)
			}
		}
	case *ffv1.Action_ProgressiveRollout:
		if !allowProgressive {
			return fmt.Errorf("progressive_rollout is only supported in default_action")
		}
		if err := validateStickiness(kind.ProgressiveRollout.GetStickiness()); err != nil {
			return fmt.Errorf("progressive_rollout: %w", err)
		}
		if err := validateProgressiveRollout(variants, kind.ProgressiveRollout); err != nil {
			return err
		}
	default:
		return fmt.Errorf("action must define serve, distribute, or progressive_rollout")
	}
	return nil
}

func validateStickiness(stickiness string) error {
	// targetingKey is provider-specific and does not behave like a normal attribute path across runtimes.
	if stickiness == "targetingKey" {
		return errors.New(`stickiness "targetingKey" is not supported; use a regular attribute path such as "user.id"`)
	}
	return nil
}

func validateConditionRefs(doc *ffv1.FeatureFlagDocument, cond *ffv1.Condition) error {
	switch kind := cond.Kind.(type) {
	case *ffv1.Condition_Rule:
		if _, ok := doc.Rules[kind.Rule.Name]; !ok {
			return fmt.Errorf("referenced rule %q not found", kind.Rule.Name)
		}
	case *ffv1.Condition_AllOf:
		for _, child := range kind.AllOf.Conditions {
			if err := validateConditionRefs(doc, child); err != nil {
				return err
			}
		}
	case *ffv1.Condition_AnyOf:
		for _, child := range kind.AnyOf.Conditions {
			if err := validateConditionRefs(doc, child); err != nil {
				return err
			}
		}
	case *ffv1.Condition_OneOf:
		for _, child := range kind.OneOf.Conditions {
			if err := validateConditionRefs(doc, child); err != nil {
				return err
			}
		}
	case *ffv1.Condition_Not:
		return validateConditionRefs(doc, kind.Not.Condition)
	}
	return nil
}

func detectRuleCycles(doc *ffv1.FeatureFlagDocument) error {
	state := map[string]int{}
	var stack []string

	var dfs func(name string) error
	dfs = func(name string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("rule cycle detected: %s -> %s", strings.Join(stack, " -> "), name)
		case 2:
			return nil
		}

		rule, ok := doc.Rules[name]
		if !ok {
			return fmt.Errorf("rule %q not found", name)
		}

		state[name] = 1
		stack = append(stack, name)
		for _, ref := range collectRuleRefs(rule) {
			if err := dfs(ref); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[name] = 2
		return nil
	}

	for name := range doc.Rules {
		if err := dfs(name); err != nil {
			return err
		}
	}
	return nil
}

func collectRuleRefs(cond *ffv1.Condition) []string {
	switch kind := cond.Kind.(type) {
	case *ffv1.Condition_Rule:
		return []string{kind.Rule.Name}
	case *ffv1.Condition_AllOf:
		return nestedRuleRefs(kind.AllOf.Conditions)
	case *ffv1.Condition_AnyOf:
		return nestedRuleRefs(kind.AnyOf.Conditions)
	case *ffv1.Condition_OneOf:
		return nestedRuleRefs(kind.OneOf.Conditions)
	case *ffv1.Condition_Not:
		return collectRuleRefs(kind.Not.Condition)
	}
	return nil
}

func nestedRuleRefs(conditions []*ffv1.Condition) []string {
	var out []string
	for _, cond := range conditions {
		out = append(out, collectRuleRefs(cond)...)
	}
	return out
}

func validateProgressiveRollout(variants *ffv1.VariantSet, rollout *ffv1.ProgressiveRollout) error {
	if rollout == nil {
		return errors.New("progressive_rollout is required")
	}
	if _, ok := variants.Variants[rollout.Variant]; !ok {
		return fmt.Errorf("variant %q not found", rollout.Variant)
	}
	start, err := parseTimestamp(rollout.Start)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	end, err := parseTimestamp(rollout.End)
	if err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if !start.Before(end) {
		return fmt.Errorf("start must be before end")
	}
	if rollout.Steps == 0 {
		return fmt.Errorf("steps must be greater than 0")
	}
	return nil
}

func validateExperimentation(exp *ffv1.Experimentation) error {
	start, err := parseTimestamp(exp.Start)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	end, err := parseTimestamp(exp.End)
	if err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if !start.Before(end) {
		return fmt.Errorf("start must be before end")
	}
	return nil
}

func validateScheduledStep(doc *ffv1.FeatureFlagDocument, variants *ffv1.VariantSet, step *ffv1.ScheduledStep) error {
	if _, err := parseTimestamp(step.Date); err != nil {
		return fmt.Errorf("date: %w", err)
	}
	if step.DefaultAction == nil {
		return fmt.Errorf("default_action: required")
	}
	for idx, entry := range step.Rules {
		if err := validateConditionRefs(doc, entry.If); err != nil {
			return fmt.Errorf("rule[%d]: %w", idx, err)
		}
		if err := validateActionRefs(doc, variants, entry.Action, false); err != nil {
			return fmt.Errorf("rule[%d]: %w", idx, err)
		}
	}
	if step.DefaultAction != nil {
		if err := validateActionRefs(doc, variants, step.DefaultAction, false); err != nil {
			return fmt.Errorf("default action: %w", err)
		}
	}
	if step.Experimentation != nil {
		if err := validateExperimentation(step.Experimentation); err != nil {
			return fmt.Errorf("experimentation: %w", err)
		}
	}
	return nil
}

func validateScheduledRollouts(steps []*ffv1.ScheduledStep) error {
	if len(steps) == 0 {
		return nil
	}

	var prev time.Time
	for i, step := range steps {
		current, err := parseTimestamp(step.Date)
		if err != nil {
			return fmt.Errorf("scheduled_rollouts[%d].date: %w", i, err)
		}
		if i == 0 {
			prev = current
			continue
		}
		if current.Equal(prev) {
			return fmt.Errorf("scheduled_rollouts dates must be unique; duplicate %q", step.Date)
		}
		if current.Before(prev) {
			return fmt.Errorf("scheduled_rollouts must be sorted in ascending date order")
		}
		prev = current
	}
	return nil
}

func parseTimestamp(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("must be RFC3339 timestamp")
	}
	return t, nil
}
