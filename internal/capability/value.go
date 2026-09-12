// Package capability models target value support as deterministic predicates.
package capability

import (
	"fmt"
	"math"

	"github.com/satorunooshie/ffcraft/internal/numeric"
)

type ValueKind uint8

const (
	KindUnknown ValueKind = iota
	KindBool
	KindString
	KindInt
	KindDouble
	KindObject
	KindList
	KindNull
)

type ValuePosition uint8

const (
	PositionRoot ValuePosition = iota
	PositionObjectField
	PositionListElement
)

type NumericClass uint8

const (
	NumericNotNumeric NumericClass = iota
	NumericInt64
	NumericSafeJSONInteger
	NumericUnsafeJSONInteger
	NumericFiniteDouble
	NumericNonFiniteDouble
)

type ValueFacts struct {
	Kind         ValueKind
	RootKind     ValueKind
	ParentKind   *ValueKind
	Depth        int
	Position     ValuePosition
	NumericClass NumericClass
	Path         []ValueKind
}

type CapabilityDecision uint8

const (
	Supported CapabilityDecision = iota
	Unsupported
)

type ValuePredicate struct {
	Kind         *ValueKind
	Position     *ValuePosition
	ParentKind   *ValueKind
	NumericClass *NumericClass
}

func (p ValuePredicate) Matches(f ValueFacts) bool {
	return (p.Kind == nil || *p.Kind == f.Kind) &&
		(p.Position == nil || *p.Position == f.Position) &&
		(p.ParentKind == nil || (f.ParentKind != nil && *p.ParentKind == *f.ParentKind)) &&
		(p.NumericClass == nil || *p.NumericClass == f.NumericClass)
}

type ValueCapabilityRule struct {
	Match       ValuePredicate
	Decision    CapabilityDecision
	ErrorCode   string
	EvidenceIDs []string
	Priority    int
}

// Walk visits every node in a normalized value, including the root. It is
// deliberately representation-neutral so providers can apply the same
// capability predicates to scalar, object, and list shapes.
func Walk(value any, visit func(ValueFacts) error) error {
	kind := kindOf(value)
	return walk(value, ValueFacts{Kind: kind, RootKind: kind, Position: PositionRoot, Path: []ValueKind{kind}}, visit)
}

func walk(value any, facts ValueFacts, visit func(ValueFacts) error) error {
	facts.Kind = kindOf(value)
	facts.NumericClass = numericClassFor(value, facts)
	if err := visit(facts); err != nil {
		return err
	}
	switch value := value.(type) {
	case map[string]any:
		for _, child := range value {
			childFacts := childFacts(facts, kindOf(child), PositionObjectField)
			if err := walk(child, childFacts, visit); err != nil {
				return err
			}
		}
	case []any:
		parent := facts.Kind
		for _, child := range value {
			childFacts := childFacts(facts, kindOf(child), PositionListElement)
			childFacts.ParentKind = &parent
			if err := walk(child, childFacts, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func numericClassFor(value any, facts ValueFacts) NumericClass {
	class := NumericClassOf(value)
	if facts.Kind != KindInt {
		return class
	}
	if facts.RootKind != KindObject {
		return NumericInt64
	}
	switch value := value.(type) {
	case int:
		return integerClass(int64(value))
	case int8:
		return integerClass(int64(value))
	case int16:
		return integerClass(int64(value))
	case int32:
		return integerClass(int64(value))
	case int64:
		return integerClass(value)
	case uint8, uint16, uint32, uint64:
		return class
	}
	return class
}
func childFacts(parent ValueFacts, kind ValueKind, position ValuePosition) ValueFacts {
	parentKind := parent.Kind
	path := append(append([]ValueKind(nil), parent.Path...), kind)
	return ValueFacts{
		Kind:       kind,
		RootKind:   parent.RootKind,
		ParentKind: &parentKind,
		Depth:      parent.Depth + 1,
		Position:   position,
		Path:       path,
	}
}

func kindOf(value any) ValueKind {
	switch value.(type) {
	case bool:
		return KindBool
	case string:
		return KindString
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return KindInt
	case float32, float64:
		return KindDouble
	case map[string]any:
		return KindObject
	case []any:
		return KindList
	case nil:
		return KindNull
	default:
		return KindUnknown
	}
}

// ValidateRules ensures that every rule which declares support is backed by
// at least one evidence identifier.
func ValidateRules(rules []ValueCapabilityRule) error {
	if len(rules) == 0 {
		return fmt.Errorf("no capability rules configured")
	}
	for _, rule := range rules {
		if rule.Decision == Supported && len(rule.EvidenceIDs) == 0 {
			return fmt.Errorf("supported capability rule %q has no evidence", rule.ErrorCode)
		}
	}
	return nil
}

// Resolve returns the single highest-priority decision. Rules at the same
// effective priority must not disagree or silently shadow one another.
func Resolve(rules []ValueCapabilityRule, facts ValueFacts) (CapabilityDecision, ValueCapabilityRule, error) {
	if len(rules) == 0 {
		return Unsupported, ValueCapabilityRule{}, fmt.Errorf("no capability rules configured")
	}
	matched := make([]ValueCapabilityRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Match.Matches(facts) {
			matched = append(matched, rule)
		}
	}
	if len(matched) == 0 {
		return Unsupported, ValueCapabilityRule{}, fmt.Errorf("no capability rule matched value shape")
	}
	highest := matched[0].Priority
	for _, rule := range matched[1:] {
		if rule.Priority > highest {
			highest = rule.Priority
		}
	}
	var selected *ValueCapabilityRule
	for i := range matched {
		if matched[i].Priority != highest {
			continue
		}
		if selected != nil {
			return Unsupported, ValueCapabilityRule{}, fmt.Errorf("multiple capability rules matched value shape at priority %d", highest)
		}
		selected = &matched[i]
	}
	return selected.Decision, *selected, nil
}

func NumericClassOf(value any) NumericClass {
	switch value := value.(type) {
	case int:
		return integerClass(int64(value))
	case int8:
		return integerClass(int64(value))
	case int16:
		return integerClass(int64(value))
	case int32:
		return integerClass(int64(value))
	case int64:
		return integerClass(value)
	case uint:
		if uint64(value) > uint64(numeric.SafeJSONIntegerMax) {
			return NumericUnsafeJSONInteger
		}
		return NumericSafeJSONInteger
	case uint8:
		return NumericSafeJSONInteger
	case uint16:
		return NumericSafeJSONInteger
	case uint32:
		return NumericSafeJSONInteger
	case uint64:
		if value > uint64(numeric.SafeJSONIntegerMax) {
			return NumericUnsafeJSONInteger
		}
		return NumericSafeJSONInteger
	case float32, float64:
		var f float64
		switch value := value.(type) {
		case float32:
			f = float64(value)
		case float64:
			f = value
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return NumericNonFiniteDouble
		}
		return NumericFiniteDouble
	default:
		return NumericNotNumeric
	}
}

func integerClass(value int64) NumericClass {
	if numeric.IsSafeJSONInteger(value) {
		return NumericSafeJSONInteger
	}
	return NumericUnsafeJSONInteger
}
