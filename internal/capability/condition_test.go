package capability

import "testing"

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
