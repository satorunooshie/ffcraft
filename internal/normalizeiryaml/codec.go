package normalizeiryaml

import (
	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
)

// Marshal emits normalized/v1 from the canonical protobuf IR.
func Marshal(doc *irv1.Document) ([]byte, error) {
	legacy, err := ir.ToAST(doc)
	if err != nil {
		return nil, err
	}
	return normalizedyaml.Marshal(legacy)
}

// Unmarshal returns canonical protobuf IR, never the legacy AST.
func Unmarshal(data []byte) (*irv1.Document, error) {
	legacy, err := normalizedyaml.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return ir.FromAST(legacy)
}
