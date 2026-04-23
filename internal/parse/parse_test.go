package parse

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
			name: "parses rollout extensions",
			file: "testdata/valid_rollouts.yaml",
			assert: func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
				t.Helper()
				env := doc.Flags[0].GetEnvironments()["prod"].GetRuleEvaluation()
				if env.GetDefaultAction().GetProgressiveRollout() == nil {
					t.Fatalf("unexpected default action: %#v", env.GetDefaultAction())
				}
				if env.GetExperimentation().GetStart() == "" || env.GetExperimentation().GetEnd() == "" {
					t.Fatalf("expected experimentation, got %#v", env.GetExperimentation())
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
				if !env.GetScheduledRollouts()[1].GetDisabled() {
					t.Fatalf("expected second step to be disabled: %#v", env.GetScheduledRollouts()[1])
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
