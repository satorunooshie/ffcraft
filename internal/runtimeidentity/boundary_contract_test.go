package runtimeidentity

import (
	"strings"
	"testing"
)

func validTuple() RuntimeArtifactTuple {
	return RuntimeArtifactTuple{
		ID:                 "runtime",
		RuntimeProfile:     "flagd/in-process",
		CompatibilityClass: "v1",
		RuntimeModule:      &ModuleArtifact{Module: "example/runtime", Version: "v1.0.0"},
		RuntimeContainer:   &ContainerArtifact{Image: "example/runtime", Tag: "v1", Digest: "sha256:abc"},
	}
}

func TestRuntimeArtifactTupleValidationTable(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RuntimeArtifactTuple)
		want   string
	}{
		{"missing id", func(tuple *RuntimeArtifactTuple) { tuple.ID = "" }, "tuple id"},
		{"missing profile", func(tuple *RuntimeArtifactTuple) { tuple.RuntimeProfile = "" }, "runtime profile"},
		{"missing compatibility", func(tuple *RuntimeArtifactTuple) { tuple.CompatibilityClass = "" }, "compatibility class"},
		{"incomplete runtime module", func(tuple *RuntimeArtifactTuple) { tuple.RuntimeModule = &ModuleArtifact{Module: "example/runtime"} }, "runtime module"},
		{"incomplete container image", func(tuple *RuntimeArtifactTuple) { tuple.RuntimeContainer.Image = "" }, "runtime container"},
		{"incomplete container tag", func(tuple *RuntimeArtifactTuple) { tuple.RuntimeContainer.Tag = "" }, "runtime container"},
		{"incomplete container digest", func(tuple *RuntimeArtifactTuple) { tuple.RuntimeContainer.Digest = "" }, "runtime container"},
		{"incomplete protocol", func(tuple *RuntimeArtifactTuple) { tuple.Protocol = ProtocolIdentity{ID: "custom"} }, "protocol identity"},
		{"rpc wrong protocol", func(tuple *RuntimeArtifactTuple) {
			tuple.RuntimeProfile = "flagd/rpc"
			tuple.Protocol = ProtocolIdentity{ID: "wrong", ProtoFile: "file", ProtoPackage: "pkg", Service: "svc", ProtocolVersion: "v1", Methods: []string{"Resolve"}}
		}, "unsupported protocol"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tuple := validTuple()
			test.mutate(&tuple)
			if err := tuple.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want %q", err, test.want)
			}
		})
	}
	if err := validTuple().Validate(); err != nil {
		t.Fatalf("valid tuple rejected: %v", err)
	}
}

func TestProtocolAndBoundaryEvidenceContracts(t *testing.T) {
	protocol := FlagdEvaluationV2Protocol()
	for _, mutate := range []func(*ProtocolIdentity){
		func(value *ProtocolIdentity) { value.ProtoFile = "" },
		func(value *ProtocolIdentity) { value.ProtoPackage = "" },
		func(value *ProtocolIdentity) { value.Service = "" },
		func(value *ProtocolIdentity) { value.ProtocolVersion = "" },
		func(value *ProtocolIdentity) { value.Methods = nil },
	} {
		candidate := protocol
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid protocol accepted: %#v", candidate)
		}
	}
	evidence := DefaultBoundaryEvidence()
	evidence[0].Boundary = "unknown"
	if err := ValidateBoundaryEvidence(evidence); err == nil || !strings.Contains(err.Error(), "unknown runtime boundary") {
		t.Fatalf("unknown boundary error = %v", err)
	}
	evidence = DefaultBoundaryEvidence()
	evidence[0].Boundary = evidence[1].Boundary
	if err := ValidateBoundaryEvidence(evidence); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate boundary error = %v", err)
	}
	evidence = DefaultBoundaryEvidence()[:len(DefaultBoundaryEvidence())-1]
	if err := ValidateBoundaryEvidence(evidence); err == nil || !strings.Contains(err.Error(), "omitted") {
		t.Fatalf("omitted boundary error = %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*BoundaryEvidence)
		want   string
	}{
		{"empty boundary", func(value *BoundaryEvidence) { value.Boundary = "" }, "incomplete"},
		{"empty status", func(value *BoundaryEvidence) { value.Status = "" }, "incomplete"},
		{"supported no evidence", func(value *BoundaryEvidence) { value.Status = StatusSupported }, "no evidence"},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := DefaultBoundaryEvidence()
			test.mutate(&candidate[0])
			if err := ValidateBoundaryEvidence(candidate); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateBoundaryEvidence() = %v, want %q", err, test.want)
			}
		})
	}
}
