package ir

import (
	"fmt"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

// ExtensionScope identifies one of the three non-inherited extension scopes.
type ExtensionScope uint8

const (
	DocumentScope ExtensionScope = iota
	FlagScope
	EnvironmentScope
)

// ExtensionValidator is intentionally namespace-owned. Core only selects the
// value and supplies its surrounding normalized IR context.
type ExtensionValidator func(document *irv1.Document, value *irv1.ExtensionValue) error

// ExtensionSelector identifies the scope at which a namespace is evaluated.
// FlagKey and Environment are intentionally empty outside their applicable
// scopes; extension values are never inherited or merged across scopes.
type ExtensionSelector struct {
	Scope       ExtensionScope
	FlagKey     string
	Environment string
}

type ExtensionDiagnostic struct {
	Code     string
	Severity string
	Message  string
	Path     string
}

// ExtensionDiagnosticValidator is the lossless extension validation API.
// Path is a scope-relative JSON Pointer into the selected extension value.
type ExtensionDiagnosticValidator func(document *irv1.Document, selector ExtensionSelector, namespace string, value *irv1.ExtensionValue) []ExtensionDiagnostic

type ExtensionValidationStatus uint8

const (
	ExtensionNotApplicable ExtensionValidationStatus = iota
	ExtensionValid
	ExtensionInvalid
)

// ValidateExtension selects one namespace at one scope and invokes its owner
// validator. No inheritance or merging is performed.
func ValidateExtension(document *irv1.Document, scope ExtensionScope, flagKey, environment, namespace string, validator ExtensionValidator) (ExtensionValidationStatus, error) {
	if document == nil {
		return ExtensionInvalid, fmt.Errorf("document is nil")
	}
	if validator == nil {
		return ExtensionInvalid, fmt.Errorf("extension validator is nil")
	}
	var value *irv1.ExtensionValue
	switch scope {
	case DocumentScope:
		value = document.Extensions[namespace]
	case FlagScope:
		flag := document.Flags[flagKey]
		if flag == nil {
			return ExtensionInvalid, fmt.Errorf("flag %q not found", flagKey)
		}
		value = flag.Extensions[namespace]
	case EnvironmentScope:
		flag := document.Flags[flagKey]
		if flag == nil {
			return ExtensionInvalid, fmt.Errorf("flag %q not found", flagKey)
		}
		env := flag.Environments[environment]
		if env == nil {
			return ExtensionInvalid, fmt.Errorf("environment %q not found", environment)
		}
		value = env.Extensions[namespace]
	default:
		return ExtensionInvalid, fmt.Errorf("unknown extension scope %d", scope)
	}
	if value == nil {
		return ExtensionNotApplicable, nil
	} // not applicable
	if err := validator(document, value); err != nil {
		return ExtensionInvalid, err
	}
	return ExtensionValid, nil
}

func ValidateExtensionDetailed(document *irv1.Document, selector ExtensionSelector, namespace string, validator ExtensionDiagnosticValidator) (ExtensionValidationStatus, []ExtensionDiagnostic) {
	if document == nil {
		return ExtensionInvalid, []ExtensionDiagnostic{{Code: "FFCRAFT_EXTENSION_DOCUMENT_NIL", Severity: "error", Message: "document is nil"}}
	}
	if validator == nil {
		return ExtensionInvalid, []ExtensionDiagnostic{{Code: "FFCRAFT_EXTENSION_VALIDATOR_NIL", Severity: "error", Message: "extension validator is nil"}}
	}
	value, diagnostics := extensionAt(document, selector, namespace)
	if len(diagnostics) != 0 {
		return ExtensionInvalid, diagnostics
	}
	if value == nil {
		return ExtensionNotApplicable, nil
	}
	diagnostics = validator(document, selector, namespace, value)
	if len(diagnostics) != 0 {
		return ExtensionInvalid, diagnostics
	}
	return ExtensionValid, nil
}

func extensionAt(document *irv1.Document, selector ExtensionSelector, namespace string) (*irv1.ExtensionValue, []ExtensionDiagnostic) {
	invalid := func(code, message string) (*irv1.ExtensionValue, []ExtensionDiagnostic) {
		return nil, []ExtensionDiagnostic{{Code: code, Severity: "error", Message: message}}
	}
	switch selector.Scope {
	case DocumentScope:
		return document.Extensions[namespace], nil
	case FlagScope:
		flag := document.Flags[selector.FlagKey]
		if flag == nil {
			return invalid("FFCRAFT_EXTENSION_FLAG_NOT_FOUND", fmt.Sprintf("flag %q not found", selector.FlagKey))
		}
		return flag.Extensions[namespace], nil
	case EnvironmentScope:
		flag := document.Flags[selector.FlagKey]
		if flag == nil {
			return invalid("FFCRAFT_EXTENSION_FLAG_NOT_FOUND", fmt.Sprintf("flag %q not found", selector.FlagKey))
		}
		env := flag.Environments[selector.Environment]
		if env == nil {
			return invalid("FFCRAFT_EXTENSION_ENVIRONMENT_NOT_FOUND", fmt.Sprintf("environment %q not found", selector.Environment))
		}
		return env.Extensions[namespace], nil
	default:
		return invalid("FFCRAFT_EXTENSION_SCOPE_UNKNOWN", fmt.Sprintf("unknown extension scope %d", selector.Scope))
	}
}
