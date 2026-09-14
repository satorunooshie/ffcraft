package capability

import (
	"strings"
	"testing"
)

func TestConditionCapabilityMatrix(t *testing.T) {
	matrix := ConditionCapabilityMatrix()
	if err := ValidateConditionCapabilityMatrix(matrix); err != nil {
		t.Fatal(err)
	}
	for _, entry := range matrix {
		if entry.Condition == ConditionPresence && entry.Supported {
			t.Fatal("presence must remain fail-closed for both v1 targets")
		}
	}
}

func TestConditionCapabilityMatrixRejectsMalformedEntries(t *testing.T) {
	valid := ConditionCapabilityMatrix()
	tests := []struct {
		name   string
		mutate func([]ConditionCapability)
		want   string
	}{
		{"incomplete target", func(matrix []ConditionCapability) { matrix[0].Target = "" }, "incomplete"},
		{"incomplete condition", func(matrix []ConditionCapability) { matrix[0].Condition = "" }, "incomplete"},
		{"duplicate", func(matrix []ConditionCapability) { matrix[1] = matrix[0] }, "duplicate"},
		{"unsupported diagnostic", func(matrix []ConditionCapability) { matrix[0].Supported = false; matrix[0].Diagnostic = "wrong" }, "diagnostic"},
		{"missing entry", func(matrix []ConditionCapability) { matrix = matrix[:len(matrix)-1] }, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matrix := append([]ConditionCapability(nil), valid...)
			if test.name == "missing entry" {
				matrix = matrix[:len(matrix)-1]
			} else {
				test.mutate(matrix)
			}
			err := ValidateConditionCapabilityMatrix(matrix)
			if err == nil || test.want != "" && !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateConditionCapabilityMatrix() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestUnsupportedConditionErrorContract(t *testing.T) {
	err := &UnsupportedConditionError{Target: TargetFlagd, Condition: ConditionPresence}
	if err.Code() != UnsupportedConditionCode || !strings.Contains(err.Error(), UnsupportedConditionCode) || !strings.Contains(err.Error(), "presence") {
		t.Fatalf("UnsupportedConditionError = %v", err)
	}
}
