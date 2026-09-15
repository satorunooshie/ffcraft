package codegen

import (
	_ "embed"
	"fmt"
	"strings"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

type Config struct {
	PackageName     string
	ContextType     string
	ClientType      string
	EvaluatorType   string
	ContextDefaults ContextDefaultsConfig
	ContextFields   []ContextFieldConfig
	Accessors       map[string]AccessorConfig
}

// CompileIR compiles normalized semantic IR into Go source.
func CompileIR(doc *irv1.Document, cfg Config) ([]byte, error) {
	return compileIRDocument(doc, cfg)
}

type AccessorConfig struct {
	Name        string `yaml:"name"`
	VariantType string `yaml:"variant_type"`
}

type ContextFieldConfig struct {
	Path string `yaml:"path"`
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type ContextDefaultsConfig struct {
	ScalarTypes     map[string]string `yaml:"scalar_types"`
	CollectionTypes map[string]string `yaml:"collection_types"`
}

type flagKind int

const (
	flagKindBool flagKind = iota
	flagKindString
	flagKindInt
	flagKindFloat
	flagKindObject
	flagKindList
)

type compiledVariant struct {
	Name      string
	ConstName string
}

type compiledFlag struct {
	Key                  string
	AccessorName         string
	ConstName            string
	DefaultVariant       string
	DefaultLiteral       string
	Kind                 flagKind
	VariantType          string
	Variants             []compiledVariant
	UsesContext          bool
	RequiresTargetingKey bool
	TargetingKeyPaths    []string
}

type compiledContextField struct {
	Path      string
	FieldName string
	FieldType string
}

type templateData struct {
	PackageName             string
	ContextType             string
	ClientType              string
	EvaluatorType           string
	ContextFields           []compiledContextField
	HasAttributePaths       bool
	HasContextFields        bool
	HasErrors               bool
	HasCollectionFlags      bool
	HasTargetingKeyPaths    bool
	HasRequiredTargetingKey bool
	Flags                   []compiledFlag
}

func hasRequiredTargetingKey(flags []compiledFlag) bool {
	for _, flag := range flags {
		if flag.RequiresTargetingKey {
			return true
		}
	}
	return false
}

func hasTargetingKeyPaths(flags []compiledFlag) bool {
	for _, flag := range flags {
		if len(flag.TargetingKeyPaths) > 0 {
			return true
		}
	}
	return false
}

func hasErrors(flags []compiledFlag) bool {
	for _, flag := range flags {
		if flag.RequiresTargetingKey || flag.Kind == flagKindObject || flag.Kind == flagKindList {
			return true
		}
	}
	return false
}

func hasCollectionFlags(flags []compiledFlag) bool {
	for _, flag := range flags {
		if flag.Kind == flagKindObject || flag.Kind == flagKindList {
			return true
		}
	}
	return false
}

func normalizeFieldType(value string) string {
	switch value = strings.TrimSpace(value); value {
	case "string", "bool", "int", "int64", "float64", "[]string", "[]bool", "[]int", "[]int64", "[]float64", "[]any", "map[string]any":
		return value
	default:
		return "string"
	}
}

func isSupportedFieldType(value string) bool {
	return normalizeFieldType(value) == strings.TrimSpace(value)
}

func applyContextDefaults(value string, defaults ContextDefaultsConfig) string {
	value = strings.TrimSpace(value)
	if key := inferredScalarTypeKey(value); key != "" {
		if override := strings.TrimSpace(defaults.ScalarTypes[key]); override != "" {
			return override
		}
	}
	if key := inferredCollectionTypeKey(value); key != "" {
		if override := strings.TrimSpace(defaults.CollectionTypes[key]); override != "" {
			return override
		}
	}
	return value
}

func validateContextDefaults(defaults ContextDefaultsConfig) error {
	for key, value := range defaults.ScalarTypes {
		switch key {
		case "string", "bool", "int", "float":
		default:
			return fmt.Errorf("context.defaults.scalar_types.%s: unsupported key", key)
		}
		if !isSupportedFieldType(value) {
			return fmt.Errorf("context.defaults.scalar_types.%s: unsupported type %q", key, value)
		}
	}
	for key, value := range defaults.CollectionTypes {
		switch key {
		case "string", "bool", "int", "float", "any":
		default:
			return fmt.Errorf("context.defaults.collection_types.%s: unsupported key", key)
		}
		if !isSupportedFieldType(value) {
			return fmt.Errorf("context.defaults.collection_types.%s: unsupported type %q", key, value)
		}
	}
	return nil
}

func inferredScalarTypeKey(value string) string {
	switch value {
	case "string":
		return "string"
	case "bool":
		return "bool"
	case "int", "int64":
		return "int"
	case "float64":
		return "float"
	default:
		return ""
	}
}

func inferredCollectionTypeKey(value string) string {
	switch value {
	case "[]string":
		return "string"
	case "[]bool":
		return "bool"
	case "[]int", "[]int64":
		return "int"
	case "[]float64":
		return "float"
	case "[]any":
		return "any"
	default:
		return ""
	}
}

func toExportedName(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == ' ' })
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		upper := strings.ToUpper(part)
		if upper == "ID" {
			b.WriteString(upper)
		} else if upper == "IDS" {
			b.WriteString("IDs")
		} else {
			b.WriteString(strings.ToUpper(part[:1]))
			b.WriteString(part[1:])
		}
	}
	if b.Len() == 0 {
		return "Generated"
	}
	return b.String()
}

//go:embed templates/go.tmpl
var fileTemplate string
