package runtimeidentity

import "testing"

func TestSupportedLifecycleRequiresEvidenceForEveryArtifact(t *testing.T) {
	tuples := []RuntimeArtifactTuple{
		{ID: "one", RuntimeProfile: "flagd/in-process", CompatibilityClass: "flagd-v2"},
		{ID: "two", RuntimeProfile: "flagd/in-process", CompatibilityClass: "flagd-v2"},
	}
	capability := RuntimeLifecycleCapability{
		Profile:            "flagd/in-process",
		CompatibilityClass: "flagd-v2",
		Scenario:           HotConfigUpdate,
		Status:             StatusSupported,
	}
	if err := capability.ValidateEvidence(tuples, map[string][]string{"one": {"B8-FLAGD-INPROCESS-HOTRELOAD-001"}}); err == nil {
		t.Fatal("expected missing evidence to be rejected")
	}
}

func TestNonSupportedLifecycleDoesNotClaimEvidence(t *testing.T) {
	capability := RuntimeLifecycleCapability{
		Profile:            "flagd/in-process",
		CompatibilityClass: "flagd-v2",
		Scenario:           InitialConnectionFailure,
		Status:             StatusKnownBroken,
	}
	if err := capability.ValidateEvidence(nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultLifecycleMatrixHasNoSilentOmissions(t *testing.T) {
	if err := ValidateLifecycleMatrix(DefaultLifecycleMatrix()); err != nil {
		t.Fatal(err)
	}
	if got, want := len(DefaultLifecycleMatrix()), len(officialRuntimeProfiles)*len(officialLifecycleScenarios); got != want {
		t.Fatalf("matrix entries = %d, want %d", got, want)
	}
}

func TestLifecycleEvidenceMatrixValidatesEverySupportedEntry(t *testing.T) {
	matrix := DefaultLifecycleMatrix()
	matrix[0].Status = StatusSupported
	matrix[0].EvidenceIDs = []string{"B8-FLAGD-INPROCESS-HOTRELOAD-001"}
	set := LifecycleEvidenceSet{
		Artifacts: []RuntimeArtifactTuple{{ID: "flagd-inprocess-v1", RuntimeProfile: "flagd/in-process", CompatibilityClass: "flagd-v2"}},
		Evidence:  map[string][]string{"flagd-inprocess-v1": {"B8-FLAGD-INPROCESS-HOTRELOAD-001"}},
	}
	if err := ValidateLifecycleEvidenceMatrix(matrix, set); err != nil {
		t.Fatal(err)
	}
}
