package ir

import (
	"fmt"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/proto"
)

// Marshal encodes a validated IR document using the normative protobuf wire
// format. Unknown core fields are never emitted by this package.
func Marshal(doc *irv1.Document) ([]byte, error) {
	if err := Validate(doc); err != nil {
		return nil, err
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(doc)
}

// Unmarshal decodes and validates a normalized IR document. Unknown fields in
// core messages fail closed; extension values remain opaque and are preserved
// by the protobuf runtime.
func Unmarshal(data []byte) (*irv1.Document, error) {
	doc := new(irv1.Document)
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	if err := rejectUnknownCore(doc); err != nil {
		return nil, err
	}
	if err := Validate(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func rejectUnknownCore(doc *irv1.Document) error {
	if len(doc.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("unknown fields in Document")
	}
	for key, flag := range doc.Flags {
		if len(flag.ProtoReflect().GetUnknown()) != 0 {
			return fmt.Errorf("unknown fields in flag %q", key)
		}
		for name, env := range flag.Environments {
			if err := rejectEnvironment(name, env); err != nil {
				return fmt.Errorf("flag %q: %w", key, err)
			}
		}
	}
	return nil
}
func rejectEnvironment(name string, env *irv1.Environment) error {
	if len(env.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("environment %q has unknown fields", name)
	}
	if err := rejectEvaluation(env.Base); err != nil {
		return err
	}
	for _, scheduled := range env.Schedule {
		if len(scheduled.ProtoReflect().GetUnknown()) != 0 {
			return fmt.Errorf("scheduled evaluation has unknown fields")
		}
		if err := rejectEvaluation(scheduled.Evaluation); err != nil {
			return err
		}
	}
	return nil
}
func rejectEvaluation(eval *irv1.Evaluation) error {
	if len(eval.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("evaluation has unknown fields")
	}
	for _, rule := range eval.Rules {
		if len(rule.ProtoReflect().GetUnknown()) != 0 {
			return fmt.Errorf("rule has unknown fields")
		}
		if err := rejectCondition(rule.Condition); err != nil {
			return err
		}
		if err := rejectAction(rule.Action); err != nil {
			return err
		}
	}
	return rejectAction(eval.DefaultAction)
}
func rejectAction(action *irv1.Action) error {
	if len(action.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("action has unknown fields")
	}
	if distribution, ok := action.GetKind().(*irv1.Action_Distribute); ok && len(distribution.Distribute.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("distribution has unknown fields")
	}
	return nil
}
func rejectCondition(condition *irv1.Condition) error {
	if len(condition.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("condition has unknown fields")
	}
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_Equality:
		if len(kind.Equality.ProtoReflect().GetUnknown()) != 0 {
			return fmt.Errorf("equality condition has unknown fields")
		}
	case *irv1.Condition_Logical:
		for _, child := range kind.Logical.Conditions {
			if err := rejectCondition(child); err != nil {
				return err
			}
		}
	case *irv1.Condition_Negation:
		return rejectCondition(kind.Negation)
	}
	return nil
}
