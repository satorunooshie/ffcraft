package ast

type Document struct {
	Flags []*Flag
}

type Flag struct {
	Key            string
	Variants       map[string]VariantValue
	DefaultVariant string
	Environments   map[string]*Environment
	Metadata       *Metadata
}

type Metadata struct {
	Owner       string
	Description string
	Expiry      string
	Tags        []string
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
	Experimentation   *Experimentation
	ScheduledRollouts []*ScheduledStep
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
	Stickiness  string
	Allocations map[string]float64
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

type Experimentation struct {
	Start string
	End   string
}

type ScheduledStep struct {
	Name            string
	Description     string
	Disabled        bool
	Date            string
	Rules           []*Rule
	DefaultAction   Action
	Experimentation *Experimentation
}
