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

// ValidateExtension selects one namespace at one scope and invokes its owner
// validator. No inheritance or merging is performed.
func ValidateExtension(document *irv1.Document, scope ExtensionScope, flagKey, environment, namespace string, validator ExtensionValidator) error {
	if document == nil {
		return fmt.Errorf("document is nil")
	}
	if validator == nil {
		return fmt.Errorf("extension validator is nil")
	}
	var value *irv1.ExtensionValue
	switch scope {
	case DocumentScope:
		value = document.Extensions[namespace]
	case FlagScope:
		flag := document.Flags[flagKey]
		if flag == nil {
			return fmt.Errorf("flag %q not found", flagKey)
		}
		value = flag.Extensions[namespace]
	case EnvironmentScope:
		flag := document.Flags[flagKey]
		if flag == nil {
			return fmt.Errorf("flag %q not found", flagKey)
		}
		env := flag.Environments[environment]
		if env == nil {
			return fmt.Errorf("environment %q not found", environment)
		}
		value = env.Extensions[namespace]
	default:
		return fmt.Errorf("unknown extension scope %d", scope)
	}
	if value == nil {
		return nil
	} // not applicable
	return validator(document, value)
}
