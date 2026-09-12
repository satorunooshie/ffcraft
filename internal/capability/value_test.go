package capability

import "testing"

func TestResolveUsesDeterministicPriority(t *testing.T) {
	kind := KindList
	rules := []ValueCapabilityRule{
		{Match: ValuePredicate{}, Decision: Supported},
		{Match: ValuePredicate{Kind: &kind}, Decision: Unsupported, Priority: 10},
	}
	decision, _, err := Resolve(rules, ValueFacts{Kind: KindList})
	if err != nil {
		t.Fatal(err)
	}
	if decision != Unsupported {
		t.Fatalf("decision = %v, want unsupported", decision)
	}
}

func TestResolveRejectsAmbiguousRules(t *testing.T) {
	kind := KindObject
	rules := []ValueCapabilityRule{
		{Match: ValuePredicate{Kind: &kind}, Decision: Supported},
		{Match: ValuePredicate{Kind: &kind}, Decision: Unsupported},
	}
	if _, _, err := Resolve(rules, ValueFacts{Kind: KindObject}); err == nil {
		t.Fatal("expected ambiguous capability rules to fail")
	}
}

func TestWalkProducesRootAndNestedValueFacts(t *testing.T) {
	var facts []ValueFacts
	err := Walk(map[string]any{"users": []any{map[string]any{"id": int64(9007199254740993)}}}, func(got ValueFacts) error {
		facts = append(facts, got)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 4 {
		t.Fatalf("visited facts = %d, want 4", len(facts))
	}
	last := facts[len(facts)-1]
	if last.Position != PositionObjectField || last.ParentKind == nil || *last.ParentKind != KindObject || last.Depth != 3 || last.NumericClass != NumericUnsafeJSONInteger {
		t.Fatalf("unexpected nested facts: %#v", last)
	}
	if NumericClassOf(int64(9007199254740993)) != NumericUnsafeJSONInteger {
		t.Fatal("raw integer domain should expose unsafe JSON classification")
	}
	var rootFacts []ValueFacts
	if err := Walk(int64(9007199254740993), func(got ValueFacts) error {
		rootFacts = append(rootFacts, got)
		return nil
	}); err != nil || len(rootFacts) != 1 || rootFacts[0].NumericClass != NumericInt64 {
		t.Fatalf("root typed integer facts = %#v, err=%v", rootFacts, err)
	}
}

func TestValidateRulesRequiresEvidenceForSupportedRules(t *testing.T) {
	if err := ValidateRules([]ValueCapabilityRule{{Decision: Supported}}); err == nil {
		t.Fatal("expected supported rule without evidence to fail")
	}
	if err := ValidateRules([]ValueCapabilityRule{{Decision: Supported, EvidenceIDs: []string{"B0-001"}}}); err != nil {
		t.Fatal(err)
	}
}
