package normalize

import (
	"fmt"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/validate"
)

func Normalize(doc *ffv1.FeatureFlagDocument) (*ast.Document, error) {
	if err := validate.Validate(doc); err != nil {
		return nil, err
	}

	out := &ast.Document{
		Flags: make([]*ast.Flag, 0, len(doc.Flags)),
	}
	for _, flag := range doc.Flags {
		value, err := normalizeFlag(doc, flag)
		if err != nil {
			return nil, fmt.Errorf("flag %q: %w", flag.Key, err)
		}
		out.Flags = append(out.Flags, value)
	}
	return out, nil
}

func normalizeFlag(doc *ffv1.FeatureFlagDocument, flag *ffv1.Flag) (*ast.Flag, error) {
	out := &ast.Flag{
		Key:            flag.Key,
		Variants:       normalizeVariantSet(doc.VariantSets[flag.VariantSet]),
		DefaultVariant: flag.DefaultVariant,
		Environments:   map[string]*ast.Environment{},
		Metadata:       normalizeMetadata(flag.Metadata),
	}
	for envName, env := range flag.Environments {
		value, err := normalizeEnvironment(doc, flag, env)
		if err != nil {
			return nil, fmt.Errorf("env %q: %w", envName, err)
		}
		out.Environments[envName] = value
	}
	return out, nil
}

func normalizeEnvironment(doc *ffv1.FeatureFlagDocument, flag *ffv1.Flag, env *ffv1.Environment) (*ast.Environment, error) {
	if fixed := env.GetFixedServe(); fixed != nil {
		return &ast.Environment{
			StaticVariant: fixed.Variant,
			DefaultAction: &ast.ServeAction{Variant: fixed.Variant},
		}, nil
	}

	eval := env.GetRuleEvaluation()
	if eval == nil {
		return nil, fmt.Errorf("environment must define fixed_serve or rule_evaluation")
	}

	out := &ast.Environment{
		DefaultAction:     normalizeDefaultAction(doc, eval, flag),
		Experimentation:   normalizeExperimentation(eval.Experimentation),
		ScheduledRollouts: make([]*ast.ScheduledStep, 0, len(eval.ScheduledRollouts)),
		Rules:             make([]*ast.Rule, 0, len(eval.Rules)),
	}
	for _, entry := range eval.Rules {
		rule, keep, err := normalizeRuleEntry(doc, entry)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		out.Rules = append(out.Rules, rule)
		if isLiteralTrue(rule.Condition) {
			break
		}
	}
	for _, step := range eval.ScheduledRollouts {
		normalized, err := normalizeScheduledStep(doc, step)
		if err != nil {
			return nil, err
		}
		out.ScheduledRollouts = append(out.ScheduledRollouts, normalized)
	}
	if len(out.Rules) == 0 {
		if serve, ok := out.DefaultAction.(*ast.ServeAction); ok {
			out.StaticVariant = serve.Variant
		}
	}
	return out, nil
}

func normalizeDefaultAction(doc *ffv1.FeatureFlagDocument, eval *ffv1.RuleEvaluation, flag *ffv1.Flag) ast.Action {
	if eval.GetDefaultAction() != nil {
		action, err := normalizeAction(doc, eval.GetDefaultAction())
		if err == nil {
			return action
		}
	}
	return &ast.ServeAction{Variant: flag.DefaultVariant}
}

func normalizeRuleEntry(doc *ffv1.FeatureFlagDocument, entry *ffv1.RuleEntry) (*ast.Rule, bool, error) {
	condition, err := normalizeCondition(doc, entry.If)
	if err != nil {
		return nil, false, fmt.Errorf("normalize condition: %w", err)
	}
	condition = simplifyCondition(condition)
	if isLiteralFalse(condition) {
		return nil, false, nil
	}

	action, err := normalizeAction(doc, entry.Action)
	if err != nil {
		return nil, false, fmt.Errorf("normalize action: %w", err)
	}
	return &ast.Rule{Condition: condition, Action: action}, true, nil
}

func normalizeScheduledStep(doc *ffv1.FeatureFlagDocument, step *ffv1.ScheduledStep) (*ast.ScheduledStep, error) {
	out := &ast.ScheduledStep{
		Name:            step.Name,
		Description:     step.Description,
		Disabled:        step.Disabled,
		Date:            step.Date,
		Experimentation: normalizeExperimentation(step.Experimentation),
		Rules:           make([]*ast.Rule, 0, len(step.Rules)),
	}
	if step.DefaultAction != nil {
		action, err := normalizeAction(doc, step.DefaultAction)
		if err != nil {
			return nil, fmt.Errorf("normalize scheduled default action: %w", err)
		}
		out.DefaultAction = action
	}
	for _, entry := range step.Rules {
		rule, keep, err := normalizeRuleEntry(doc, entry)
		if err != nil {
			return nil, fmt.Errorf("normalize scheduled rule: %w", err)
		}
		if !keep {
			continue
		}
		out.Rules = append(out.Rules, rule)
	}
	return out, nil
}
