package gofeatureflag

import irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"

type CompileOptions struct {
	AllowMissingEnvironment bool
}

// CompileIR compiles normalized semantic IR into GO Feature Flag YAML.
func CompileIR(doc *irv1.Document, environment string, opts CompileOptions) ([]byte, []string, error) {
	return compileIRDocument(doc, environment, opts)
}

type flagFile struct {
	Variations       map[string]any     `yaml:"variations"`
	DefaultRule      ruleResult         `yaml:"defaultRule"`
	Targeting        []targetRule       `yaml:"targeting,omitempty"`
	BucketingKey     string             `yaml:"bucketingKey,omitempty"`
	ScheduledRollout []scheduledStepOut `yaml:"scheduledRollout,omitempty"`
}

type ruleResult struct {
	Variation          string              `yaml:"variation,omitempty"`
	Percentage         map[string]float64  `yaml:"percentage,omitempty"`
	ProgressiveRollout *progressiveRollout `yaml:"progressiveRollout,omitempty"`
}

type targetRule struct {
	Query      string             `yaml:"query"`
	Variation  string             `yaml:"variation,omitempty"`
	Percentage map[string]float64 `yaml:"percentage,omitempty"`
}

type progressiveRollout struct {
	Initial progressiveState `yaml:"initial"`
	End     progressiveState `yaml:"end"`
}

type progressiveState struct {
	Variation  string   `yaml:"variation"`
	Date       string   `yaml:"date"`
	Percentage *float64 `yaml:"percentage"`
}

type scheduledStepOut struct {
	Date         string       `yaml:"date"`
	Targeting    []targetRule `yaml:"targeting,omitempty"`
	DefaultRule  *ruleResult  `yaml:"defaultRule,omitempty"`
	BucketingKey string       `yaml:"bucketingKey,omitempty"`
}
