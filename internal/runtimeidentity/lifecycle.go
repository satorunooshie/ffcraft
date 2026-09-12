package runtimeidentity

import (
	"fmt"
	"slices"
)

type RuntimeLifecycleScenario string

const (
	ColdStart                   RuntimeLifecycleScenario = "ColdStart"
	InitialLoad                 RuntimeLifecycleScenario = "InitialLoad"
	HotConfigUpdate             RuntimeLifecycleScenario = "HotConfigUpdate"
	DisconnectReconnect         RuntimeLifecycleScenario = "DisconnectReconnect"
	InitialConnectionFailure    RuntimeLifecycleScenario = "InitialConnectionFailure"
	RecoveryAfterInitialFailure RuntimeLifecycleScenario = "RecoveryAfterInitialFailure"
	InvalidConfigUpdate         RuntimeLifecycleScenario = "InvalidConfigUpdate"
	RecoveryAfterInvalidUpdate  RuntimeLifecycleScenario = "RecoveryAfterInvalidUpdate"
	Shutdown                    RuntimeLifecycleScenario = "Shutdown"
)

type RuntimeLifecycleCapability struct {
	Profile            RuntimeProfile
	CompatibilityClass string
	Scenario           RuntimeLifecycleScenario
	Status             SupportStatus
	EvidenceIDs        []string
	KnownIssueIDs      []string
}

type LifecycleEvidenceSet struct {
	Artifacts []RuntimeArtifactTuple
	Evidence  map[string][]string
}

var officialRuntimeProfiles = []RuntimeProfile{
	"flagd/in-process",
	"flagd/file",
	"flagd/rpc",
	"gofeatureflag/in-process",
	"gofeatureflag/remote",
}

var officialLifecycleScenarios = []RuntimeLifecycleScenario{
	ColdStart,
	InitialLoad,
	HotConfigUpdate,
	DisconnectReconnect,
	InitialConnectionFailure,
	RecoveryAfterInitialFailure,
	InvalidConfigUpdate,
	RecoveryAfterInvalidUpdate,
	Shutdown,
}

// DefaultLifecycleMatrix is intentionally conservative. A profile/scenario
// is explicitly present even when this repository cannot independently
// observe it because the upstream runtime is not a dependency.
func DefaultLifecycleMatrix() []RuntimeLifecycleCapability {
	out := make([]RuntimeLifecycleCapability, 0, len(officialRuntimeProfiles)*len(officialLifecycleScenarios))
	for _, profile := range officialRuntimeProfiles {
		for _, scenario := range officialLifecycleScenarios {
			out = append(out, RuntimeLifecycleCapability{
				Profile:            profile,
				CompatibilityClass: compatibilityClass(profile),
				Scenario:           scenario,
				Status:             StatusNotIndependentlyObservable,
			})
		}
	}
	return out
}

func ValidateLifecycleMatrix(matrix []RuntimeLifecycleCapability) error {
	seen := make(map[string]struct{}, len(matrix))
	for _, capability := range matrix {
		if capability.Profile == "" || capability.CompatibilityClass == "" || capability.Scenario == "" || capability.Status == "" {
			return fmt.Errorf("lifecycle matrix contains an incomplete capability")
		}
		if !slices.Contains(officialRuntimeProfiles, capability.Profile) {
			return fmt.Errorf("lifecycle matrix contains unsupported profile %q", capability.Profile)
		}
		if expected := compatibilityClass(capability.Profile); capability.CompatibilityClass != expected {
			return fmt.Errorf("lifecycle matrix profile %q requires compatibility class %q", capability.Profile, expected)
		}
		key := string(capability.Profile) + "\x00" + string(capability.Scenario)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate lifecycle capability for %q and %q", capability.Profile, capability.Scenario)
		}
		seen[key] = struct{}{}
	}
	for _, profile := range officialRuntimeProfiles {
		for _, scenario := range officialLifecycleScenarios {
			key := string(profile) + "\x00" + string(scenario)
			if _, ok := seen[key]; !ok {
				return fmt.Errorf("lifecycle matrix omits %q for %q", scenario, profile)
			}
		}
	}
	return nil
}

func compatibilityClass(profile RuntimeProfile) string {
	switch profile {
	case "flagd/in-process", "flagd/file", "flagd/rpc":
		return "flagd-v2"
	case "gofeatureflag/in-process", "gofeatureflag/remote":
		return "goff-v1"
	default:
		return ""
	}
}

// ValidateEvidence applies the lifecycle evidence gate. A scenario may be
// declared Supported only when every representative artifact has positive
// evidence associated with it.
func (c RuntimeLifecycleCapability) ValidateEvidence(tuples []RuntimeArtifactTuple, evidence map[string][]string) error {
	if c.Profile == "" || c.CompatibilityClass == "" || c.Scenario == "" || c.Status == "" {
		return fmt.Errorf("lifecycle capability requires profile, compatibility class, scenario, and status")
	}
	if c.Status != StatusSupported {
		return nil
	}
	if len(c.EvidenceIDs) == 0 {
		return fmt.Errorf("supported lifecycle capability requires evidence identifiers")
	}
	if len(tuples) == 0 {
		return fmt.Errorf("supported lifecycle capability requires representative artifacts")
	}
	for _, tuple := range tuples {
		if err := tuple.Validate(); err != nil {
			return fmt.Errorf("artifact %q: %w", tuple.ID, err)
		}
		if tuple.RuntimeProfile != c.Profile || tuple.CompatibilityClass != c.CompatibilityClass {
			return fmt.Errorf("artifact %q is outside lifecycle capability domain", tuple.ID)
		}
		artifactEvidence := evidence[tuple.ID]
		if len(artifactEvidence) == 0 {
			return fmt.Errorf("artifact %q has no lifecycle evidence", tuple.ID)
		}
		for _, required := range c.EvidenceIDs {
			found := slices.Contains(artifactEvidence, required)
			if !found {
				return fmt.Errorf("artifact %q is missing lifecycle evidence %q", tuple.ID, required)
			}
		}
	}
	return nil
}

// ValidateLifecycleEvidenceMatrix applies the artifact evidence gate to every
// Supported lifecycle entry in a complete matrix.
func ValidateLifecycleEvidenceMatrix(matrix []RuntimeLifecycleCapability, set LifecycleEvidenceSet) error {
	if err := ValidateLifecycleMatrix(matrix); err != nil {
		return err
	}
	for _, capability := range matrix {
		if capability.Status != StatusSupported {
			continue
		}
		var representatives []RuntimeArtifactTuple
		for _, artifact := range set.Artifacts {
			if artifact.RuntimeProfile == capability.Profile && artifact.CompatibilityClass == capability.CompatibilityClass {
				representatives = append(representatives, artifact)
			}
		}
		if err := capability.ValidateEvidence(representatives, set.Evidence); err != nil {
			return fmt.Errorf("%s/%s: %w", capability.Profile, capability.Scenario, err)
		}
	}
	return nil
}
