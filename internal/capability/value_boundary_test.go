package capability

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestNumericClassBoundaryTable(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  NumericClass
	}{
		{"int safe", int64(1), NumericSafeJSONInteger},
		{"int unsafe", int64(1 << 53), NumericUnsafeJSONInteger},
		{"uint safe", uint64(1), NumericSafeJSONInteger},
		{"uint unsafe", uint64(1 << 53), NumericUnsafeJSONInteger},
		{"finite float", 1.25, NumericFiniteDouble},
		{"nan", math.NaN(), NumericNonFiniteDouble},
		{"infinity", math.Inf(1), NumericNonFiniteDouble},
		{"bool", true, NumericNotNumeric},
		{"unknown", struct{}{}, NumericNotNumeric},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NumericClassOf(test.value); got != test.want {
				t.Fatalf("NumericClassOf(%#v) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestWalkPropagatesShapeAndVisitorErrors(t *testing.T) {
	var facts []ValueFacts
	if err := Walk(map[string]any{"items": []any{int64(1), "x"}}, func(f ValueFacts) error {
		facts = append(facts, f)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(facts) != 4 || facts[0].Kind != KindObject || facts[1].Position != PositionObjectField || facts[2].Position != PositionListElement || facts[2].Depth != 2 || facts[2].ParentKind == nil || *facts[2].ParentKind != KindList {
		t.Fatalf("unexpected value facts: %#v", facts)
	}
	wantErr := errors.New("stop")
	if err := Walk([]any{true, false}, func(ValueFacts) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("Walk() error = %v, want %v", err, wantErr)
	}
}

func TestCapabilityPredicateAndRuleBoundaryContracts(t *testing.T) {
	kind := KindInt
	position := PositionListElement
	parent := KindList
	numeric := NumericSafeJSONInteger
	facts := ValueFacts{Kind: kind, Position: position, ParentKind: &parent, NumericClass: numeric}
	if !(ValuePredicate{Kind: &kind, Position: &position, ParentKind: &parent, NumericClass: &numeric}).Matches(facts) {
		t.Fatal("matching predicate did not match")
	}
	wrongParent := KindObject
	if (ValuePredicate{ParentKind: &wrongParent}).Matches(facts) {
		t.Fatal("wrong parent predicate matched")
	}
	if err := ValidateRules(nil); err == nil || !strings.Contains(err.Error(), "no capability rules") {
		t.Fatalf("ValidateRules(nil) = %v", err)
	}
	if _, _, err := Resolve(nil, facts); err == nil || !strings.Contains(err.Error(), "no capability rules") {
		t.Fatalf("Resolve(nil) = %v", err)
	}
	if _, _, err := Resolve([]ValueCapabilityRule{{Match: ValuePredicate{Kind: &kind}}}, ValueFacts{Kind: KindString}); err == nil || !strings.Contains(err.Error(), "no capability rule matched") {
		t.Fatalf("Resolve(no match) = %v", err)
	}
}
