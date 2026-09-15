package capability

import "fmt"

// ConditionKind identifies a normalized IR condition construct.
type ConditionKind string

const (
	ConditionEquality    ConditionKind = "equality"
	ConditionInequality  ConditionKind = "inequality"
	ConditionNumeric     ConditionKind = "numeric_comparison"
	ConditionMembership  ConditionKind = "membership"
	ConditionStringMatch ConditionKind = "string_match"
	ConditionSemver      ConditionKind = "semver_comparison"
	ConditionPresence    ConditionKind = "presence"
	ConditionLogical     ConditionKind = "logical"
	ConditionNegation    ConditionKind = "negation"
	ConditionConstant    ConditionKind = "constant"
)

// Target identifies a compiler target capability profile.
type Target string

const (
	TargetFlagd         Target = "flagd"
	TargetGOFeatureFlag Target = "gofeatureflag"
)

// ConditionCapability is the versioned target condition capability matrix.
type ConditionCapability struct {
	Target     Target
	Condition  ConditionKind
	Supported  bool
	Diagnostic string
}

const UnsupportedConditionCode = "FFCRAFT_TARGET_CONDITION_UNSUPPORTED"

// ConditionCapabilityMatrix returns the v1 condition capability matrix.
func ConditionCapabilityMatrix() []ConditionCapability {
	conditions := []ConditionKind{
		ConditionConstant,
		ConditionEquality,
		ConditionInequality,
		ConditionNumeric,
		ConditionMembership,
		ConditionStringMatch,
		ConditionSemver,
		ConditionPresence,
		ConditionLogical,
		ConditionNegation,
	}
	matrix := make([]ConditionCapability, 0, len(conditions)*2)
	for _, target := range []Target{TargetFlagd, TargetGOFeatureFlag} {
		for _, condition := range conditions {
			capability := ConditionCapability{Target: target, Condition: condition, Supported: true}
			if condition == ConditionPresence || (condition == ConditionInequality && target == TargetFlagd) {
				capability.Supported = false
				capability.Diagnostic = UnsupportedConditionCode
			}
			matrix = append(matrix, capability)
		}
	}
	return matrix
}

// ValidateConditionCapabilityMatrix checks that every v1 target/condition
// pair is declared exactly once and that unsupported entries have a stable
// diagnostic code.
func ValidateConditionCapabilityMatrix(matrix []ConditionCapability) error {
	seen := make(map[string]struct{}, len(matrix))
	for _, entry := range matrix {
		if entry.Target == "" || entry.Condition == "" {
			return fmt.Errorf("condition capability entry is incomplete")
		}
		key := string(entry.Target) + "\x00" + string(entry.Condition)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate condition capability %q", key)
		}
		seen[key] = struct{}{}
		if !entry.Supported && entry.Diagnostic != UnsupportedConditionCode {
			return fmt.Errorf("unsupported condition capability %q has diagnostic %q", key, entry.Diagnostic)
		}
	}
	expected := len([]Target{TargetFlagd, TargetGOFeatureFlag}) * len([]ConditionKind{
		ConditionConstant, ConditionEquality, ConditionInequality, ConditionNumeric, ConditionMembership,
		ConditionStringMatch, ConditionSemver, ConditionPresence, ConditionLogical,
		ConditionNegation,
	})
	if len(seen) != expected {
		return fmt.Errorf("condition capability matrix has %d entries, want %d", len(seen), expected)
	}
	return nil
}

// UnsupportedConditionError is returned when a target cannot faithfully
// represent a normalized condition.
type UnsupportedConditionError struct {
	Target    Target
	Condition ConditionKind
}

func (e *UnsupportedConditionError) Error() string {
	return fmt.Sprintf("%s: target %q cannot represent condition %q", UnsupportedConditionCode, e.Target, e.Condition)
}

func (e *UnsupportedConditionError) Code() string { return UnsupportedConditionCode }
