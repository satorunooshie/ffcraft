package flagd

import (
	"fmt"

	"github.com/satorunooshie/ffcraft/internal/ast"
)

// ValidateFlag checks constraints imposed by the flagd target that are not
// part of the target-neutral intermediate representation.
func ValidateFlag(flag *ast.Flag) error {
	for name, value := range flag.Variants {
		if value.Kind == ast.VariantValueKindList {
			return fmt.Errorf("variant %q: flagd does not support top-level array variant values; use an object with array fields instead", name)
		}
	}
	return nil
}
