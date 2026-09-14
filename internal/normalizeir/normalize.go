package normalizeir

import (
	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalize"
)

// Normalize is the authoring adapter into the normative protobuf IR.
func Normalize(doc *ffv1.FeatureFlagDocument) (*irv1.Document, error) {
	legacy, err := normalize.Normalize(doc)
	if err != nil {
		return nil, err
	}
	return ir.FromAST(legacy)
}
