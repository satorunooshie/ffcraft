package runtimeidentity

type SupportStatus string

const (
	StatusSupported                  SupportStatus = "Supported"
	StatusUnsupported                SupportStatus = "Unsupported"
	StatusKnownBroken                SupportStatus = "KnownBroken"
	StatusNotApplicable              SupportStatus = "NotApplicable"
	StatusNotIndependentlyObservable SupportStatus = "NotIndependentlyObservable"
)
