package runtimeidentity

import (
	"fmt"
	"slices"
)

type NumericTransportShape string

const (
	RootIntShape            NumericTransportShape = "RootInt"
	ObjectNumberShape       NumericTransportShape = "ObjectNumber"
	ObjectListNumberShape   NumericTransportShape = "ObjectListNumber"
	EvaluationContextNumber NumericTransportShape = "EvaluationContextNumber"
)

type NumericBoundary string

const (
	SafeJSONMaxBoundary    NumericBoundary = "SafeJSONMax"
	SafeJSONMinBoundary    NumericBoundary = "SafeJSONMin"
	TwoTo53Boundary        NumericBoundary = "TwoTo53"
	TwoTo53PlusOneBoundary NumericBoundary = "TwoTo53PlusOne"
	Int64MaxBoundary       NumericBoundary = "Int64Max"
	Int64MinBoundary       NumericBoundary = "Int64Min"
)

var officialNumericShapes = []NumericTransportShape{
	RootIntShape,
	ObjectNumberShape,
	ObjectListNumberShape,
	EvaluationContextNumber,
}

var officialNumericBoundaries = []NumericBoundary{
	SafeJSONMaxBoundary,
	SafeJSONMinBoundary,
	TwoTo53Boundary,
	TwoTo53PlusOneBoundary,
	Int64MaxBoundary,
	Int64MinBoundary,
}

type NumericTransportCapability struct {
	Profile            RuntimeProfile
	CompatibilityClass string
	Shape              NumericTransportShape
	Boundary           NumericBoundary
	Status             SupportStatus
	EvidenceIDs        []string
	KnownIssueIDs      []string
}

// DefaultNumericTransportMatrix makes every P1 domain explicit. A runtime is
// never considered numerically supported merely because it is listed as an
// official profile; it needs a measured capability entry with evidence.
func DefaultNumericTransportMatrix() []NumericTransportCapability {
	matrix := make([]NumericTransportCapability, 0, len(officialRuntimeProfiles)*len(officialNumericShapes)*len(officialNumericBoundaries))
	for _, profile := range officialRuntimeProfiles {
		for _, shape := range officialNumericShapes {
			for _, boundary := range officialNumericBoundaries {
				matrix = append(matrix, NumericTransportCapability{
					Profile:            profile,
					CompatibilityClass: compatibilityClass(profile),
					Shape:              shape,
					Boundary:           boundary,
					Status:             StatusNotIndependentlyObservable,
				})
			}
		}
	}
	return matrix
}

func ValidateNumericTransportMatrix(matrix []NumericTransportCapability) error {
	seen := make(map[string]struct{}, len(matrix))
	for _, capability := range matrix {
		if capability.Profile == "" || capability.CompatibilityClass == "" || capability.Shape == "" || capability.Boundary == "" || capability.Status == "" {
			return fmt.Errorf("numeric transport matrix contains an incomplete capability")
		}
		if !slices.Contains(officialRuntimeProfiles, capability.Profile) {
			return fmt.Errorf("numeric transport matrix contains unsupported profile %q", capability.Profile)
		}
		if capability.CompatibilityClass != compatibilityClass(capability.Profile) {
			return fmt.Errorf("numeric profile %q has incorrect compatibility class", capability.Profile)
		}
		if !slices.Contains(officialNumericShapes, capability.Shape) || !slices.Contains(officialNumericBoundaries, capability.Boundary) {
			return fmt.Errorf("numeric transport matrix contains an unknown domain")
		}
		key := fmt.Sprintf("%s\x00%s\x00%s", capability.Profile, capability.Shape, capability.Boundary)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate numeric capability %q", key)
		}
		seen[key] = struct{}{}
		if capability.Status == StatusSupported && len(capability.EvidenceIDs) == 0 {
			return fmt.Errorf("supported numeric capability %q has no evidence", key)
		}
	}
	for _, profile := range officialRuntimeProfiles {
		for _, shape := range officialNumericShapes {
			for _, boundary := range officialNumericBoundaries {
				key := fmt.Sprintf("%s\x00%s\x00%s", profile, shape, boundary)
				if _, exists := seen[key]; !exists {
					return fmt.Errorf("numeric matrix omits %q", key)
				}
			}
		}
	}
	return nil
}
