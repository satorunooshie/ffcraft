package ast

type Value interface {
	isValue()
}

type Var struct {
	Path string
}

func (*Var) isValue() {}

type Scalar struct {
	Kind   ScalarKind
	String string
	Bool   bool
	Int    int64
	Double float64
}

func (*Scalar) isValue() {}

type List struct {
	Values []Value
}

func (*List) isValue() {}

type ScalarKind int

const (
	ScalarKindUnknown ScalarKind = iota
	ScalarKindString
	ScalarKindBool
	ScalarKindInt
	ScalarKindDouble
	ScalarKindNull
)
