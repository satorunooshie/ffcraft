package authoring

import (
	"embed"
	"strings"
	"testing"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

//go:embed testdata/*.yaml
var testdataFS embed.FS

func TestParseYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    string
		wantErr string
		assert  func(t *testing.T, doc *ffv1.FeatureFlagDocument)
	}{
		{
			name: "parses authoring document",
			file: "testdata/valid_authoring.yaml",
			assert: func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
				t.Helper()
				if doc.Version != "v1" {
					t.Fatalf("unexpected version: %q", doc.Version)
				}
				if len(doc.VariantSets) != 1 || doc.VariantSets["boolean"] == nil {
					t.Fatalf("expected boolean variant set, got %#v", doc.VariantSets)
				}
				if len(doc.Rules) != 1 || doc.Rules["internal_user"] == nil {
					t.Fatalf("expected internal_user rule, got %#v", doc.Rules)
				}
				if len(doc.Distributions) != 1 || doc.Distributions["rollout"] == nil {
					t.Fatalf("expected rollout distribution, got %#v", doc.Distributions)
				}
				if len(doc.Flags) != 1 {
					t.Fatalf("expected one flag, got %d", len(doc.Flags))
				}
				flag := doc.Flags[0]
				if flag.Key != "feature-a" {
					t.Fatalf("unexpected flag key: %q", flag.Key)
				}
				if flag.GetEnvironments()["prod"].GetRuleEvaluation().GetDefaultAction().GetServe().GetVariant() != "off" {
					t.Fatalf("unexpected default action: %#v", flag.GetEnvironments()["prod"].GetRuleEvaluation().GetDefaultAction())
				}
			},
		},
		{
			name:    "rejects aliases",
			file:    "testdata/error_alias.yaml",
			wantErr: "yaml aliases are not supported",
		},
		{
			name:    "rejects condition with multiple operators",
			file:    "testdata/error_multiple_operators.yaml",
			wantErr: "condition must contain exactly one operator",
		},
		{
			name:    "rejects rule without action",
			file:    "testdata/error_missing_action.yaml",
			wantErr: "one of serve, distribute, or progressive_rollout is required",
		},
		{
			name: "parses lists and objects",
			file: "testdata/valid_complex_values.yaml",
			assert: func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
				t.Helper()
				variantSet := doc.VariantSets["complex"]
				if _, ok := variantSet.Variants["arr"].Kind.(*ffv1.VariantValue_ListValue); !ok {
					t.Fatalf("expected list variant, got %#v", variantSet.Variants["arr"].Kind)
				}
				if _, ok := variantSet.Variants["obj"].Kind.(*ffv1.VariantValue_ObjectValue); !ok {
					t.Fatalf("expected object variant, got %#v", variantSet.Variants["obj"].Kind)
				}
			},
		},
		{
			name: "parses rollout features",
			file: "testdata/valid_rollouts.yaml",
			assert: func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
				t.Helper()
				env := doc.Flags[0].GetEnvironments()["prod"].GetRuleEvaluation()
				if env.GetDefaultAction().GetProgressiveRollout() == nil {
					t.Fatalf("unexpected default action: %#v", env.GetDefaultAction())
				}
				if len(env.GetScheduledRollouts()) != 3 {
					t.Fatalf("expected three scheduled steps, got %d", len(env.GetScheduledRollouts()))
				}
				rollout := env.GetDefaultAction().GetProgressiveRollout()
				if rollout.GetVariant() != "on" || rollout.GetStickiness() != "user.id" || rollout.GetSteps() != 3 {
					t.Fatalf("unexpected progressive rollout: %#v", rollout)
				}
				if env.GetScheduledRollouts()[0].GetName() != "first snapshot" {
					t.Fatalf("unexpected first snapshot: %#v", env.GetScheduledRollouts()[0])
				}
				if env.GetScheduledRollouts()[0].GetDescription() != "serve on for everyone" || env.GetScheduledRollouts()[0].GetDate() != "2026-05-03T00:00:00Z" {
					t.Fatalf("unexpected first snapshot metadata: %#v", env.GetScheduledRollouts()[0])
				}
				if !env.GetScheduledRollouts()[1].GetDisabled() {
					t.Fatalf("expected second step to be disabled: %#v", env.GetScheduledRollouts()[1])
				}
				third := env.GetScheduledRollouts()[2]
				if len(third.GetRules()) != 1 || third.GetRules()[0].GetAction().GetServe().GetVariant() != "on" || third.GetDefaultAction().GetServe().GetVariant() != "off" {
					t.Fatalf("unexpected third snapshot rule/default: %#v", third)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, err := testdataFS.ReadFile(tt.file)
			if err != nil {
				t.Fatalf("read fixture %s: %v", tt.file, err)
			}
			doc, err := ParseYAML(src)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			tt.assert(t, doc)
		})
	}
}

func TestParseYAMLRejectsLegacyMetadataField(t *testing.T) {
	_, err := ParseYAML([]byte(`version: v1
variant_sets:
  values: {on: true}
flags:
  - key: example
    variant_set: values
    default_variant: on
    metadata: {owner: platform}
    environments:
      prod: {serve: on}
`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("ParseYAML() error = %v, want unknown field", err)
	}
}

func TestParseYAMLRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := ParseYAML([]byte(`version: v1
variant_sets:
  boolean:
    on: true
flags:
  - key: feature-a
    variant_set: boolean
    default_variatn: on
    environments:
      prod:
        serve: on
`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestParseYAMLNumericDomains(t *testing.T) {
	t.Parallel()

	const prefix = `version: v1
variant_sets:
  values:
`
	tests := []struct {
		name    string
		value   string
		wantErr bool
		assert  func(t *testing.T, doc *ffv1.FeatureFlagDocument)
	}{
		{
			name:  "B0A-OBJECT-INT-SAFE-MAX-001",
			value: "    object:\n      id: 9007199254740991\n",
		},
		{
			name:  "B0A-OBJECT-INT-SAFE-MIN-001",
			value: "    object:\n      id: -9007199254740991\n",
		},
		{
			name:  "B0A-OBJECT-INT-UNSAFE-POSITIVE-001",
			value: "    object:\n      id: 9007199254740992\n",
		},
		{
			name:  "B0A-OBJECT-INT-UNSAFE-2P53-001",
			value: "    object:\n      id: 9007199254740992\n",
		},
		{
			name:  "B0A-OBJECT-INT-LOSSY-POSITIVE-001",
			value: "    object:\n      id: 9007199254740993\n",
		},
		{
			name:  "B0A-OBJECT-INT-LOSSY-2P53P1-001",
			value: "    object:\n      id: 9007199254740993\n",
		},
		{
			name:  "B0A-OBJECT-INT-UNSAFE-NEGATIVE-001",
			value: "    object:\n      id: -9007199254740992\n",
		},
		{
			name:  "B0A-DEEP-OBJECT-INT-LOSSY-001",
			value: "    object:\n      a:\n        b:\n          value: 9007199254740993\n",
		},
		{
			name:  "B0A-OBJECT-LIST-INT-LOSSY-001",
			value: "    object:\n      users:\n        - id: 9007199254740993\n",
		},
		{
			name:  "B0A-OBJECT-LIST-INT-SAFE-001",
			value: "    object:\n      users:\n        - id: 9007199254740991\n",
		},
		{
			name:  "B0A-ROOT-LIST-INT64-001",
			value: "    list:\n      - 9007199254740993\n",
			assert: func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
				t.Helper()
				list := doc.VariantSets["values"].Variants["list"].GetListValue().GetValues()
				if len(list) != 1 || list[0].GetIntValue() != 9007199254740993 {
					t.Fatalf("root list integer was not preserved: %#v", list)
				}
			},
		},
		{
			name:  "B0A-ROOT-INT-2P53P1-001",
			value: "    value: 9007199254740993\n",
			assert: func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
				t.Helper()
				got := doc.VariantSets["values"].Variants["value"].GetIntValue()
				if got != 9007199254740993 {
					t.Fatalf("root integer was not preserved: %d", got)
				}
			},
		},
		{
			name:  "B0A-ROOT-INT64-MAX-001",
			value: "    max: 9223372036854775807\n",
			assert: func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
				t.Helper()
				values := doc.VariantSets["values"].Variants
				if values["max"].GetIntValue() != 9223372036854775807 {
					t.Fatalf("root int64 max was not preserved: %#v", values)
				}
			},
		},
		{
			name:  "B0A-ROOT-INT64-MIN-001",
			value: "    min: -9223372036854775808\n",
			assert: func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
				t.Helper()
				values := doc.VariantSets["values"].Variants
				if values["min"].GetIntValue() != -9223372036854775808 {
					t.Fatalf("root int64 min was not preserved: %#v", values)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			source := []byte(prefix + tt.value + "flags:\n  - key: test\n    variant_set: values\n    default_variant: value\n    environments:\n      prod:\n        serve: value\n")
			doc, err := ParseYAML(source)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected unsafe object integer to be rejected")
				}
				return
			}
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, doc)
			}
		})
	}
}

func TestParseYAMLNestedVariantPreservesNumericKinds(t *testing.T) {
	doc, err := ParseYAML([]byte(`version: v1
variant_sets:
  values:
    object:
      integer: 9007199254740993
      decimal: 1.0
      nested:
        - 9223372036854775807
flags:
  - key: test
    variant_set: values
    default_variant: object
    environments:
      prod:
        serve: object
`))
	if err != nil {
		t.Fatal(err)
	}
	object := doc.VariantSets["values"].Variants["object"].GetObjectValue()
	if got := object.Fields["integer"].GetIntValue(); got != 9007199254740993 {
		t.Fatalf("nested integer kind/value was not preserved: %d", got)
	}
	if got := object.Fields["decimal"].GetDoubleValue(); got != 1.0 {
		t.Fatalf("nested double kind/value was not preserved: %v", got)
	}
	if got := object.Fields["nested"].GetListValue().Values[0].GetIntValue(); got != 9223372036854775807 {
		t.Fatalf("deep integer kind/value was not preserved: %d", got)
	}
}
