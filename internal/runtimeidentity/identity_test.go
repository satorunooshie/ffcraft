package runtimeidentity

import "testing"

func TestFlagdEvaluationV2ProtocolIdentity(t *testing.T) {
	got := FlagdEvaluationV2Protocol()
	if got.ID != FlagdEvaluationV2ProtocolID || got.ProtoFile != FlagdEvaluationV2ProtoFile || got.ProtoPackage != FlagdEvaluationV2ProtoPackage || got.Service != FlagdEvaluationV2Service {
		t.Fatalf("unexpected flagd protocol identity: %#v", got)
	}
	if len(got.Methods) != 5 || got.Methods[0] != "ResolveBoolean" || got.Methods[4] != "ResolveObject" {
		t.Fatalf("unexpected flagd typed methods: %#v", got.Methods)
	}
}

func TestRuntimeArtifactTupleRequiresImmutableIdentity(t *testing.T) {
	tuple := RuntimeArtifactTuple{
		ID:                 "flagd-test",
		RuntimeProfile:     "flagd/in-process",
		CompatibilityClass: "flagd-v2",
		ProviderModule:     &ModuleArtifact{Module: "example/provider"},
	}
	if err := tuple.Validate(); err == nil {
		t.Fatal("expected module version to be required")
	}
}

func TestFlagdRPCArtifactRequiresExactProtocolIdentity(t *testing.T) {
	tuple := RuntimeArtifactTuple{
		ID:                 "flagd-rpc-test",
		RuntimeProfile:     "flagd/rpc",
		CompatibilityClass: "flagd-v2",
	}
	if err := tuple.Validate(); err == nil {
		t.Fatal("expected flagd rpc protocol identity to be required")
	}
	tuple.Protocol = FlagdEvaluationV2Protocol()
	if err := tuple.Validate(); err != nil {
		t.Fatal(err)
	}
}
