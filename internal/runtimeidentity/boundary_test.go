package runtimeidentity

import "testing"

func TestDefaultBoundaryEvidenceHasNoSilentOmissions(t *testing.T) {
	if err := ValidateBoundaryEvidence(DefaultBoundaryEvidence()); err != nil {
		t.Fatal(err)
	}
}

func TestSupportedBoundaryRequiresEvidence(t *testing.T) {
	evidence := DefaultBoundaryEvidence()
	evidence[0].Status = StatusSupported
	if err := ValidateBoundaryEvidence(evidence); err == nil {
		t.Fatal("expected supported boundary without evidence to fail")
	}
}
