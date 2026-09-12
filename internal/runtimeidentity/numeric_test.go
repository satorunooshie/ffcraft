package runtimeidentity

import "testing"

func TestDefaultNumericTransportMatrixHasNoSilentOmissions(t *testing.T) {
	matrix := DefaultNumericTransportMatrix()
	if err := ValidateNumericTransportMatrix(matrix); err != nil {
		t.Fatal(err)
	}
	want := len(officialRuntimeProfiles) * len(officialNumericShapes) * len(officialNumericBoundaries)
	if len(matrix) != want {
		t.Fatalf("numeric matrix entries = %d, want %d", len(matrix), want)
	}
}

func TestSupportedNumericTransportRequiresEvidence(t *testing.T) {
	matrix := DefaultNumericTransportMatrix()
	matrix[0].Status = StatusSupported
	if err := ValidateNumericTransportMatrix(matrix); err == nil {
		t.Fatal("expected supported numeric capability without evidence to fail")
	}
	matrix[0].EvidenceIDs = []string{"B8-GOFF-INPROCESS-NUMERIC-001"}
	if err := ValidateNumericTransportMatrix(matrix); err != nil {
		t.Fatal(err)
	}
}
