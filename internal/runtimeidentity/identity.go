// Package runtimeidentity describes the concrete artifacts used by runtime
// compatibility evidence.
package runtimeidentity

import "fmt"

type RuntimeProfile string

type ModuleArtifact struct {
	Module  string
	Version string
	Sum     string
}

type ContainerArtifact struct {
	Image  string
	Tag    string
	Digest string
}

type ProtocolIdentity struct {
	ID              string
	ProtoFile       string
	ProtoPackage    string
	Service         string
	ProtocolVersion string
	Methods         []string
}

type RuntimeArtifactTuple struct {
	ID                 string
	RuntimeProfile     RuntimeProfile
	CompatibilityClass string
	RuntimeModule      *ModuleArtifact
	ProviderModule     *ModuleArtifact
	SDKModule          *ModuleArtifact
	CoreModule         *ModuleArtifact
	RuntimeContainer   *ContainerArtifact
	Protocol           ProtocolIdentity
}

const (
	FlagdEvaluationV2ProtocolID      = "flagd-evaluation-v2"
	FlagdEvaluationV2ProtoFile       = "flagd/evaluation/v2/evaluation.proto"
	FlagdEvaluationV2ProtoPackage    = "flagd.evaluation.v2"
	FlagdEvaluationV2Service         = "flagd.evaluation.v2.Service"
	FlagdEvaluationV2ProtocolVersion = "v2"
)

var FlagdEvaluationV2Methods = []string{
	"ResolveBoolean",
	"ResolveString",
	"ResolveFloat",
	"ResolveInt",
	"ResolveObject",
}

func FlagdEvaluationV2Protocol() ProtocolIdentity {
	return ProtocolIdentity{
		ID:              FlagdEvaluationV2ProtocolID,
		ProtoFile:       FlagdEvaluationV2ProtoFile,
		ProtoPackage:    FlagdEvaluationV2ProtoPackage,
		Service:         FlagdEvaluationV2Service,
		ProtocolVersion: FlagdEvaluationV2ProtocolVersion,
		Methods:         append([]string(nil), FlagdEvaluationV2Methods...),
	}
}

func (p ProtocolIdentity) Validate() error {
	if p.ID == "" {
		return nil
	}
	if p.ProtoFile == "" || p.ProtoPackage == "" || p.Service == "" || p.ProtocolVersion == "" {
		return fmt.Errorf("protocol identity requires id, proto file, package, service, and version")
	}
	if len(p.Methods) == 0 {
		return fmt.Errorf("protocol identity requires typed methods")
	}
	return nil
}

// Validate enforces the minimum identity needed to treat evidence as
// reproducible. Optional artifact categories are allowed to be nil.
func (t RuntimeArtifactTuple) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("runtime artifact tuple id is required")
	}
	if t.RuntimeProfile == "" {
		return fmt.Errorf("runtime profile is required")
	}
	if t.CompatibilityClass == "" {
		return fmt.Errorf("compatibility class is required")
	}
	if err := t.Protocol.Validate(); err != nil {
		return err
	}
	if t.RuntimeProfile == "flagd/rpc" {
		if t.Protocol.ID == "" {
			return fmt.Errorf("flagd/rpc artifact requires protocol identity")
		}
		if t.Protocol.ID != FlagdEvaluationV2ProtocolID || t.Protocol.ProtoFile != FlagdEvaluationV2ProtoFile || t.Protocol.ProtoPackage != FlagdEvaluationV2ProtoPackage || t.Protocol.Service != FlagdEvaluationV2Service || t.Protocol.ProtocolVersion != FlagdEvaluationV2ProtocolVersion {
			return fmt.Errorf("flagd/rpc artifact uses unsupported protocol identity")
		}
	}
	if err := validateModule("runtime module", t.RuntimeModule); err != nil {
		return err
	}
	if err := validateModule("provider module", t.ProviderModule); err != nil {
		return err
	}
	if err := validateModule("sdk module", t.SDKModule); err != nil {
		return err
	}
	if err := validateModule("core module", t.CoreModule); err != nil {
		return err
	}
	if c := t.RuntimeContainer; c != nil && (c.Image == "" || c.Tag == "" || c.Digest == "") {
		return fmt.Errorf("runtime container requires image, tag, and digest")
	}
	return nil
}

func validateModule(name string, module *ModuleArtifact) error {
	if module != nil && (module.Module == "" || module.Version == "") {
		return fmt.Errorf("%s requires module path and version", name)
	}
	return nil
}
