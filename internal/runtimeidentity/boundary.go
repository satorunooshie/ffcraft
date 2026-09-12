package runtimeidentity

import (
	"fmt"
	"slices"
)

type RuntimeBoundary string

const (
	BoundaryAuthoringIngress   RuntimeBoundary = "B0-A"
	BoundaryNormalizedIngress  RuntimeBoundary = "B0-N"
	BoundarySemanticValidation RuntimeBoundary = "B1"
	BoundaryNormalize          RuntimeBoundary = "B2"
	BoundaryIRValidation       RuntimeBoundary = "B3"
	BoundaryCapability         RuntimeBoundary = "B4"
	BoundaryCompile            RuntimeBoundary = "B5"
	BoundaryNativeLoader       RuntimeBoundary = "B6"
	BoundaryNativeRuntime      RuntimeBoundary = "B7"
	BoundaryOpenFeature        RuntimeBoundary = "B8"
	BoundaryGeneratedAccessor  RuntimeBoundary = "B9"
	BoundaryCLIArtifact        RuntimeBoundary = "B10"
)

var officialRuntimeBoundaries = []RuntimeBoundary{
	BoundaryAuthoringIngress, BoundaryNormalizedIngress, BoundarySemanticValidation,
	BoundaryNormalize, BoundaryIRValidation, BoundaryCapability, BoundaryCompile,
	BoundaryNativeLoader, BoundaryNativeRuntime, BoundaryOpenFeature,
	BoundaryGeneratedAccessor, BoundaryCLIArtifact,
}

type BoundaryEvidence struct {
	Boundary    RuntimeBoundary
	Status      SupportStatus
	EvidenceIDs []string
}

func DefaultBoundaryEvidence() []BoundaryEvidence {
	out := make([]BoundaryEvidence, 0, len(officialRuntimeBoundaries))
	for _, boundary := range officialRuntimeBoundaries {
		out = append(out, BoundaryEvidence{Boundary: boundary, Status: StatusNotIndependentlyObservable})
	}
	return out
}

func ValidateBoundaryEvidence(evidence []BoundaryEvidence) error {
	seen := make(map[RuntimeBoundary]struct{}, len(evidence))
	for _, entry := range evidence {
		if entry.Boundary == "" || entry.Status == "" {
			return fmt.Errorf("boundary evidence is incomplete")
		}
		if !slices.Contains(officialRuntimeBoundaries, entry.Boundary) {
			return fmt.Errorf("unknown runtime boundary %q", entry.Boundary)
		}
		if _, exists := seen[entry.Boundary]; exists {
			return fmt.Errorf("duplicate runtime boundary %q", entry.Boundary)
		}
		seen[entry.Boundary] = struct{}{}
		if entry.Status == StatusSupported && len(entry.EvidenceIDs) == 0 {
			return fmt.Errorf("supported runtime boundary %q has no evidence", entry.Boundary)
		}
	}
	for _, boundary := range officialRuntimeBoundaries {
		if _, exists := seen[boundary]; !exists {
			return fmt.Errorf("runtime boundary %q is omitted", boundary)
		}
	}
	return nil
}
