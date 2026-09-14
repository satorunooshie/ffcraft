package runtimeidentity

import (
	"strings"
	"testing"
)

func supportedLifecycle() RuntimeLifecycleCapability {
	return RuntimeLifecycleCapability{
		Profile: "flagd/in-process", CompatibilityClass: "flagd-v2", Scenario: ColdStart,
		Status: StatusSupported, EvidenceIDs: []string{"e1"},
	}
}

func TestLifecycleEvidenceSuccessAndFailureTable(t *testing.T) {
	tuple := validTuple()
	tuple.CompatibilityClass = "flagd-v2"
	evidence := map[string][]string{"runtime": {"e1"}}
	if err := supportedLifecycle().ValidateEvidence([]RuntimeArtifactTuple{tuple}, evidence); err != nil {
		t.Fatalf("valid lifecycle evidence rejected: %v", err)
	}
	tests := []struct {
		name       string
		capability RuntimeLifecycleCapability
		tuples     []RuntimeArtifactTuple
		evidence   map[string][]string
		want       string
	}{
		{"incomplete capability", RuntimeLifecycleCapability{}, nil, nil, "requires profile"},
		{"unsupported status needs no evidence", RuntimeLifecycleCapability{Profile: "flagd/in-process", CompatibilityClass: "flagd-v2", Scenario: ColdStart, Status: StatusUnsupported}, nil, nil, ""},
		{"supported no ids", RuntimeLifecycleCapability{Profile: "flagd/in-process", CompatibilityClass: "flagd-v2", Scenario: ColdStart, Status: StatusSupported}, []RuntimeArtifactTuple{tuple}, evidence, "evidence identifiers"},
		{"supported no artifacts", supportedLifecycle(), nil, evidence, "representative artifacts"},
		{"outside domain", supportedLifecycle(), []RuntimeArtifactTuple{{ID: "other", RuntimeProfile: "gofeatureflag/in-process", CompatibilityClass: "goff-v1"}}, map[string][]string{"other": {"e1"}}, "outside lifecycle"},
		{"missing artifact evidence", supportedLifecycle(), []RuntimeArtifactTuple{tuple}, nil, "no lifecycle evidence"},
		{"missing required evidence", supportedLifecycle(), []RuntimeArtifactTuple{tuple}, map[string][]string{"runtime": {"other"}}, "missing lifecycle evidence"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.capability.ValidateEvidence(test.tuples, test.evidence)
			if test.want == "" {
				if err != nil {
					t.Fatalf("ValidateEvidence() = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateEvidence() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLifecycleMatrixRejectsMalformedEntries(t *testing.T) {
	matrix := DefaultLifecycleMatrix()
	tests := []struct {
		name   string
		mutate func([]RuntimeLifecycleCapability)
		want   string
	}{
		{"incomplete", func(value []RuntimeLifecycleCapability) { value[0].Profile = "" }, "incomplete"},
		{"unsupported profile", func(value []RuntimeLifecycleCapability) { value[0].Profile = "unknown" }, "unsupported profile"},
		{"wrong compatibility", func(value []RuntimeLifecycleCapability) { value[0].CompatibilityClass = "wrong" }, "requires compatibility"},
		{"duplicate", func(value []RuntimeLifecycleCapability) { value[1] = value[0] }, "duplicate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := append([]RuntimeLifecycleCapability(nil), matrix...)
			test.mutate(candidate)
			if err := ValidateLifecycleMatrix(candidate); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateLifecycleMatrix() = %v, want %q", err, test.want)
			}
		})
	}
	missing := matrix[:len(matrix)-1]
	if err := ValidateLifecycleMatrix(missing); err == nil || !strings.Contains(err.Error(), "omits") {
		t.Fatalf("missing lifecycle entry error = %v", err)
	}
}
