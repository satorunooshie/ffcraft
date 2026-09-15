package flagd

import irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"

const schemaURL = "https://flagd.dev/schema/v0/flags.json"

type document struct {
	Schema string           `json:"$schema"`
	Flags  map[string]*flag `json:"flags"`
}

type flag struct {
	State          string         `json:"state"`
	Variants       map[string]any `json:"variants"`
	DefaultVariant string         `json:"defaultVariant"`
	Targeting      any            `json:"targeting,omitempty"`
}

type CompileOptions struct {
	AllowMissingEnvironment bool
}

// CompileIR compiles normalized semantic IR into flagd JSON.
func CompileIR(doc *irv1.Document, environment string, opts CompileOptions) ([]byte, []string, error) {
	return compileIRDocument(doc, environment, opts)
}
