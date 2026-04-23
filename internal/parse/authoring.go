package parse

import (
	"fmt"

	"gopkg.in/yaml.v3"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

func parseRootDocument(node *yaml.Node, path string) (*ffv1.FeatureFlagDocument, error) {
	if err := expectMapping(node, path); err != nil {
		return nil, err
	}

	doc := &ffv1.FeatureFlagDocument{
		VariantSets:   map[string]*ffv1.VariantSet{},
		Rules:         map[string]*ffv1.Condition{},
		Distributions: map[string]*ffv1.Distribution{},
	}

	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}

	if versionNode, ok := fields["version"]; ok {
		doc.Version, err = scalarString(versionNode, path+".version")
		if err != nil {
			return nil, err
		}
	}

	if doc.VariantSets, err = parseNamedVariantSets(fields["variant_sets"], path+".variant_sets"); err != nil {
		return nil, err
	}
	if doc.Rules, err = parseNamedConditions(fields["rules"], path+".rules"); err != nil {
		return nil, err
	}
	if doc.Distributions, err = parseNamedDistributions(fields["distributions"], path+".distributions"); err != nil {
		return nil, err
	}

	flagsNode, ok := fields["flags"]
	if !ok {
		return nil, fmt.Errorf("%s.flags: required field is missing", path)
	}
	doc.Flags, err = parseFlags(flagsNode, path+".flags")
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func parseNamedVariantSets(node *yaml.Node, path string) (map[string]*ffv1.VariantSet, error) {
	if node == nil {
		return map[string]*ffv1.VariantSet{}, nil
	}
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*ffv1.VariantSet, len(fields))
	for _, name := range sortedKeys(fields) {
		value, err := parseVariantSet(fields[name], path+"."+name)
		if err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}

func parseNamedConditions(node *yaml.Node, path string) (map[string]*ffv1.Condition, error) {
	if node == nil {
		return map[string]*ffv1.Condition{}, nil
	}
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*ffv1.Condition, len(fields))
	for _, name := range sortedKeys(fields) {
		value, err := parseCondition(fields[name], path+"."+name)
		if err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}

func parseNamedDistributions(node *yaml.Node, path string) (map[string]*ffv1.Distribution, error) {
	if node == nil {
		return map[string]*ffv1.Distribution{}, nil
	}
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*ffv1.Distribution, len(fields))
	for _, name := range sortedKeys(fields) {
		value, err := parseDistribution(fields[name], path+"."+name)
		if err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}

func parseVariantSet(node *yaml.Node, path string) (*ffv1.VariantSet, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := &ffv1.VariantSet{Variants: map[string]*ffv1.VariantValue{}}
	for _, key := range sortedKeys(fields) {
		value, err := parseVariantValue(fields[key], path+"."+key)
		if err != nil {
			return nil, err
		}
		out.Variants[key] = value
	}
	return out, nil
}

func parseDistribution(node *yaml.Node, path string) (*ffv1.Distribution, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := &ffv1.Distribution{Allocations: map[string]float64{}}
	if scalar := fields["stickiness"]; scalar != nil {
		out.Stickiness, err = scalarString(scalar, path+".stickiness")
		if err != nil {
			return nil, err
		}
	}
	if allocNode := fields["allocations"]; allocNode != nil {
		allocs, err := mapping(allocNode, path+".allocations")
		if err != nil {
			return nil, err
		}
		for _, key := range sortedKeys(allocs) {
			value, err := scalarFloat(allocs[key], path+".allocations."+key)
			if err != nil {
				return nil, err
			}
			out.Allocations[key] = value
		}
	}
	return out, nil
}

func parseFlags(node *yaml.Node, path string) ([]*ffv1.Flag, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected sequence", path)
	}
	out := make([]*ffv1.Flag, 0, len(node.Content))
	for i, item := range node.Content {
		flag, err := parseFlag(item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		out = append(out, flag)
	}
	return out, nil
}

func parseFlag(node *yaml.Node, path string) (*ffv1.Flag, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := &ffv1.Flag{Environments: map[string]*ffv1.Environment{}}

	if scalar := fields["key"]; scalar != nil {
		out.Key, err = scalarString(scalar, path+".key")
		if err != nil {
			return nil, err
		}
	}
	if scalar := fields["variant_set"]; scalar != nil {
		out.VariantSet, err = scalarString(scalar, path+".variant_set")
		if err != nil {
			return nil, err
		}
	}
	if scalar := fields["default_variant"]; scalar != nil {
		out.DefaultVariant, err = scalarString(scalar, path+".default_variant")
		if err != nil {
			return nil, err
		}
	}
	if metaNode := fields["metadata"]; metaNode != nil {
		out.Metadata, err = parseMetadata(metaNode, path+".metadata")
		if err != nil {
			return nil, err
		}
	}
	if envNode := fields["environments"]; envNode != nil {
		out.Environments, err = parseEnvironments(envNode, path+".environments")
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func parseMetadata(node *yaml.Node, path string) (*ffv1.Metadata, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := &ffv1.Metadata{}
	if scalar := fields["owner"]; scalar != nil {
		out.Owner, err = scalarString(scalar, path+".owner")
		if err != nil {
			return nil, err
		}
	}
	if scalar := fields["description"]; scalar != nil {
		out.Description, err = scalarString(scalar, path+".description")
		if err != nil {
			return nil, err
		}
	}
	if scalar := fields["expiry"]; scalar != nil {
		out.Expiry, err = scalarString(scalar, path+".expiry")
		if err != nil {
			return nil, err
		}
	}
	if tagsNode := fields["tags"]; tagsNode != nil {
		out.Tags, err = stringSequence(tagsNode, path+".tags")
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func parseEnvironments(node *yaml.Node, path string) (map[string]*ffv1.Environment, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*ffv1.Environment, len(fields))
	for _, name := range sortedKeys(fields) {
		value, err := parseEnvironment(fields[name], path+"."+name)
		if err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}

func parseEnvironment(node *yaml.Node, path string) (*ffv1.Environment, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	if serveNode := fields["serve"]; serveNode != nil {
		if fields["rules"] != nil || fields["default_action"] != nil || fields["experimentation"] != nil || fields["scheduled_rollouts"] != nil {
			return nil, fmt.Errorf("%s: serve cannot be combined with rules, default_action, experimentation, or scheduled_rollouts", path)
		}
		variant, err := scalarString(serveNode, path+".serve")
		if err != nil {
			return nil, err
		}
		return &ffv1.Environment{
			Kind: &ffv1.Environment_FixedServe{
				FixedServe: &ffv1.FixedServe{Variant: variant},
			},
		}, nil
	}

	ruleEval, err := parseRuleEvaluation(fields, path)
	if err != nil {
		return nil, err
	}
	return &ffv1.Environment{
		Kind: &ffv1.Environment_RuleEvaluation{RuleEvaluation: ruleEval},
	}, nil
}

func parseRuleEvaluation(fields map[string]*yaml.Node, path string) (*ffv1.RuleEvaluation, error) {
	out := &ffv1.RuleEvaluation{}
	var err error
	if rulesNode := fields["rules"]; rulesNode != nil {
		out.Rules, err = parseRuleEntries(rulesNode, path+".rules")
		if err != nil {
			return nil, err
		}
	}
	if defaultNode := fields["default_action"]; defaultNode != nil {
		out.DefaultAction, err = parseActionNode(defaultNode, path+".default_action")
		if err != nil {
			return nil, err
		}
	}
	if expNode := fields["experimentation"]; expNode != nil {
		out.Experimentation, err = parseExperimentation(expNode, path+".experimentation")
		if err != nil {
			return nil, err
		}
	}
	if scheduledNode := fields["scheduled_rollouts"]; scheduledNode != nil {
		out.ScheduledRollouts, err = parseScheduledSteps(scheduledNode, path+".scheduled_rollouts")
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func parseRuleEntries(node *yaml.Node, path string) ([]*ffv1.RuleEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected sequence", path)
	}
	out := make([]*ffv1.RuleEntry, 0, len(node.Content))
	for i, item := range node.Content {
		entry, err := parseRuleEntry(item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

func parseRuleEntry(node *yaml.Node, path string) (*ffv1.RuleEntry, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	ifNode, ok := fields["if"]
	if !ok {
		return nil, fmt.Errorf("%s.if: required field is missing", path)
	}
	condition, err := parseCondition(ifNode, path+".if")
	if err != nil {
		return nil, err
	}
	action, err := parseAction(fields, path)
	if err != nil {
		return nil, err
	}
	return &ffv1.RuleEntry{If: condition, Action: action}, nil
}

func parseAction(fields map[string]*yaml.Node, path string) (*ffv1.Action, error) {
	return parseActionMap(fields, path)
}

func parseActionNode(node *yaml.Node, path string) (*ffv1.Action, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	return parseActionMap(fields, path)
}

func parseActionMap(fields map[string]*yaml.Node, path string) (*ffv1.Action, error) {
	if serveNode := fields["serve"]; serveNode != nil {
		variant, err := scalarString(serveNode, path+".serve")
		if err != nil {
			return nil, err
		}
		return &ffv1.Action{Kind: &ffv1.Action_Serve{Serve: &ffv1.Serve{Variant: variant}}}, nil
	}
	if distNode := fields["distribute"]; distNode != nil {
		name, err := scalarString(distNode, path+".distribute")
		if err != nil {
			return nil, err
		}
		return &ffv1.Action{Kind: &ffv1.Action_Distribute{Distribute: &ffv1.Distribute{Distribution: name}}}, nil
	}
	if rolloutNode := fields["progressive_rollout"]; rolloutNode != nil {
		rollout, err := parseProgressiveRollout(rolloutNode, path+".progressive_rollout")
		if err != nil {
			return nil, err
		}
		return &ffv1.Action{Kind: &ffv1.Action_ProgressiveRollout{ProgressiveRollout: rollout}}, nil
	}
	return nil, fmt.Errorf("%s: one of serve, distribute, or progressive_rollout is required", path)
}

func parseProgressiveRollout(node *yaml.Node, path string) (*ffv1.ProgressiveRollout, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := &ffv1.ProgressiveRollout{}
	if variantNode := fields["variant"]; variantNode != nil {
		out.Variant, err = scalarString(variantNode, path+".variant")
		if err != nil {
			return nil, err
		}
	}
	if stickinessNode := fields["stickiness"]; stickinessNode != nil {
		out.Stickiness, err = scalarString(stickinessNode, path+".stickiness")
		if err != nil {
			return nil, err
		}
	}
	if startNode := fields["start"]; startNode != nil {
		out.Start, err = scalarString(startNode, path+".start")
		if err != nil {
			return nil, err
		}
	}
	if endNode := fields["end"]; endNode != nil {
		out.End, err = scalarString(endNode, path+".end")
		if err != nil {
			return nil, err
		}
	}
	if stepsNode := fields["steps"]; stepsNode != nil {
		value, err := scalarInt(stepsNode, path+".steps")
		if err != nil {
			return nil, err
		}
		out.Steps = uint32(value)
	}
	return out, nil
}

func parseExperimentation(node *yaml.Node, path string) (*ffv1.Experimentation, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := &ffv1.Experimentation{}
	if startNode := fields["start"]; startNode != nil {
		out.Start, err = scalarString(startNode, path+".start")
		if err != nil {
			return nil, err
		}
	}
	if endNode := fields["end"]; endNode != nil {
		out.End, err = scalarString(endNode, path+".end")
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func parseScheduledSteps(node *yaml.Node, path string) ([]*ffv1.ScheduledStep, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected sequence", path)
	}
	out := make([]*ffv1.ScheduledStep, 0, len(node.Content))
	for i, item := range node.Content {
		step, err := parseScheduledStep(item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		out = append(out, step)
	}
	return out, nil
}

func parseScheduledStep(node *yaml.Node, path string) (*ffv1.ScheduledStep, error) {
	fields, err := mapping(node, path)
	if err != nil {
		return nil, err
	}
	out := &ffv1.ScheduledStep{}
	if nameNode := fields["name"]; nameNode != nil {
		out.Name, err = scalarString(nameNode, path+".name")
		if err != nil {
			return nil, err
		}
	}
	if descNode := fields["description"]; descNode != nil {
		out.Description, err = scalarString(descNode, path+".description")
		if err != nil {
			return nil, err
		}
	}
	if disabledNode := fields["disabled"]; disabledNode != nil {
		out.Disabled, err = scalarBool(disabledNode, path+".disabled")
		if err != nil {
			return nil, err
		}
	}
	if dateNode := fields["date"]; dateNode != nil {
		out.Date, err = scalarString(dateNode, path+".date")
		if err != nil {
			return nil, err
		}
	}
	if rulesNode := fields["rules"]; rulesNode != nil {
		out.Rules, err = parseRuleEntries(rulesNode, path+".rules")
		if err != nil {
			return nil, err
		}
	}
	if defaultNode := fields["default_action"]; defaultNode != nil {
		out.DefaultAction, err = parseActionNode(defaultNode, path+".default_action")
		if err != nil {
			return nil, err
		}
	}
	if expNode := fields["experimentation"]; expNode != nil {
		out.Experimentation, err = parseExperimentation(expNode, path+".experimentation")
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
