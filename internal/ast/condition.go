package ast

type Condition interface {
	isCondition()
}

type LiteralBool struct {
	Value bool
}

func (*LiteralBool) isCondition() {}

type Eq struct {
	Left  Value
	Right Value
}

func (*Eq) isCondition() {}

type Ne struct {
	Left  Value
	Right Value
}

func (*Ne) isCondition() {}

type Gt struct {
	Left  Value
	Right Value
}

func (*Gt) isCondition() {}

type Gte struct {
	Left  Value
	Right Value
}

func (*Gte) isCondition() {}

type Lt struct {
	Left  Value
	Right Value
}

func (*Lt) isCondition() {}

type Lte struct {
	Left  Value
	Right Value
}

func (*Lte) isCondition() {}

type In struct {
	Target    Value
	Candidate Value
}

func (*In) isCondition() {}

type Contains struct {
	Container Value
	Value     Value
}

func (*Contains) isCondition() {}

type StartsWith struct {
	Target Value
	Prefix string
}

func (*StartsWith) isCondition() {}

type EndsWith struct {
	Target Value
	Suffix string
}

func (*EndsWith) isCondition() {}

type Matches struct {
	Target  Value
	Pattern string
}

func (*Matches) isCondition() {}

type SemverGt struct {
	Left  Value
	Right string
}

func (*SemverGt) isCondition() {}

type SemverGte struct {
	Left  Value
	Right string
}

func (*SemverGte) isCondition() {}

type SemverLt struct {
	Left  Value
	Right string
}

func (*SemverLt) isCondition() {}

type SemverLte struct {
	Left  Value
	Right string
}

func (*SemverLte) isCondition() {}

type AllOf struct {
	Conditions []Condition
}

func (*AllOf) isCondition() {}

type AnyOf struct {
	Conditions []Condition
}

func (*AnyOf) isCondition() {}

type OneOf struct {
	Conditions []Condition
}

func (*OneOf) isCondition() {}

type Not struct {
	Condition Condition
}

func (*Not) isCondition() {}
