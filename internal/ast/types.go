package ast

import irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"

type Document struct {
	Flags      []*Flag
	Extensions map[string]*irv1.ExtensionValue
}

type Flag struct {
	Key            string
	Variants       map[string]VariantValue
	DefaultVariant string
	Environments   map[string]*Environment
	Extensions     map[string]*irv1.ExtensionValue
}

type VariantValue struct {
	Kind   VariantValueKind
	Bool   bool
	String string
	Int    int64
	Double float64
	Object map[string]any
	List   []VariantValue
}

type VariantValueKind int

const (
	VariantValueKindUnknown VariantValueKind = iota
	VariantValueKindBool
	VariantValueKindString
	VariantValueKindInt
	VariantValueKindDouble
	VariantValueKindObject
	VariantValueKindList
	VariantValueKindNull
)

type Environment struct {
	StaticVariant     string
	Rules             []*Rule
	DefaultAction     Action
	ScheduledRollouts []*ScheduledStep
	Extensions        map[string]*irv1.ExtensionValue
}

type Rule struct {
	Condition Condition
	Action    Action
}

type Action interface {
	isAction()
}

type ServeAction struct {
	Variant string
}

func (*ServeAction) isAction() {}

type DistributeAction struct {
	Stickiness string
	Weights    map[string]uint32
}

func (*DistributeAction) isAction() {}

type ProgressiveRolloutAction struct {
	Variant    string
	Stickiness string
	Start      string
	End        string
	Steps      uint32
}

func (*ProgressiveRolloutAction) isAction() {}

type ScheduledStep struct {
	Name          string
	Description   string
	Disabled      bool
	Date          string
	Rules         []*Rule
	DefaultAction Action
}
