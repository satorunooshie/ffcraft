package flagd

import (
	"fmt"

	"github.com/satorunooshie/ffcraft/internal/ast"
)

// ValidateDocument checks constraints imposed by the flagd target that are
// not part of the target-neutral intermediate representation.
func ValidateDocument(doc *ast.Document) error {
	for _, flag := range doc.Flags {
		for name, value := range flag.Variants {
			if value.Kind == ast.VariantValueKindList {
				return fmt.Errorf("flag %q variant %q: flagd does not support top-level array variant values; use an object with array fields instead", flag.Key, name)
			}
		}
	}
	return nil
}
