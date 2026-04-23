package gogen

import (
	"bytes"
	"embed"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/parse"
	"github.com/satorunooshie/ffcraft/internal/testhelper"
)

//go:embed testdata/*.yaml testdata/*.golden.go
var testdataFS embed.FS

func TestCompile(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		config     Config
		goldenFile string
	}{
		{
			name:    "with overrides",
			fixture: "example.yaml",
			config: Config{
				PackageName: "featuretoggle",
				Accessors: map[string]AccessorConfig{
					"enable-new-home": {
						Name: "EnableNewHome",
					},
					"checkout-mode": {
						Name:        "CheckoutMode",
						VariantType: "CheckoutModeVariant",
					},
				},
			},
			goldenFile: "generated.golden.go",
		},
		{
			name:    "with defaults",
			fixture: "example.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "generated.default.golden.go",
		},
		{
			name:    "no context",
			fixture: "no_context.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "no_context.golden.go",
		},
		{
			name:    "context overrides",
			fixture: "context_overrides.yaml",
			config: Config{
				PackageName: "featuretoggle",
				ContextFields: []ContextFieldConfig{
					{Path: "user.id", Name: "ActorID", Type: "string"},
					{Path: "device.platform", Name: "Platform", Type: "string"},
					{Path: "app.version", Name: "AppVersion", Type: "string"},
				},
				Accessors: map[string]AccessorConfig{
					"enable-new-home": {Name: "EnableNewHome"},
					"checkout-mode":   {Name: "CheckoutMode", VariantType: "CheckoutModeVariant"},
				},
			},
			goldenFile: "context_overrides.golden.go",
		},
		{
			name:    "required targeting key",
			fixture: "required_targeting_key.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "required_targeting_key.golden.go",
		},
		{
			name:    "scheduled rollout",
			fixture: "scheduled_rollout.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "scheduled_rollout.golden.go",
		},
		{
			name:    "deep rule refs",
			fixture: "deep_rule_refs.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "deep_rule_refs.golden.go",
		},
		{
			name:    "compound conditions",
			fixture: "compound_conditions.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "compound_conditions.golden.go",
		},
		{
			name:    "multi scheduled steps",
			fixture: "multi_scheduled_steps.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "multi_scheduled_steps.golden.go",
		},
		{
			name:    "int type",
			fixture: "int_type.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "int_type.golden.go",
		},
		{
			name:    "float type",
			fixture: "float_type.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "float_type.golden.go",
		},
		{
			name:    "object type",
			fixture: "object_type.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "object_type.golden.go",
		},
		{
			name:    "list type",
			fixture: "list_type.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "list_type.golden.go",
		},
		{
			name:    "additional types",
			fixture: "additional_types.yaml",
			config: Config{
				PackageName: "featureflags",
			},
			goldenFile: "additional_types.golden.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := mustNormalizeFixture(t, tt.fixture)
			got, err := Compile(doc, tt.config)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}

			if testhelper.ShouldUpdateGolden() {
				testhelper.WriteGolden(t, filepath.Join("testdata", tt.goldenFile), got)
				return
			}
			want := testhelper.MustReadFile(t, testdataFS, filepath.Join("testdata", tt.goldenFile))

			if diff := cmp.Diff(string(bytes.TrimSpace(want)), string(bytes.TrimSpace(got))); diff != "" {
				t.Fatalf("Compile() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCompile_TargetingKeySignaturesAndGuards(t *testing.T) {
	tests := []struct {
		name         string
		doc          *ast.Document
		wantContains []string
		wantAbsent   []string
	}{
		{
			name: "non rollout flag stays targetless by default",
			doc:  mustNormalizeFixture(t, "example.yaml"),
			wantContains: []string{
				"func newEvaluationContext(attrs map[string]any) EvaluationContext {",
				"func newEvaluationContextWithTargetingKey(attrs map[string]any, targetingKey string) EvaluationContext {",
				"func (e Evaluator) EnableNewHome(ctx context.Context, ec EvalContext) (bool, error)",
				`return e.client.BooleanValue(ctx, FlagEnableNewHome, false, newEvaluationContext(attrs))`,
				"func (e EvalContext) toAttributes() map[string]any {",
				"type EvaluationContext struct {",
			},
			wantAbsent: []string{
				"flag " + `"enable-new-home"` + " requires targetingKey",
				"type EvalOption func(*evalOptions)",
				"func WithTargetingKey(targetingKey string) EvalOption",
			},
		},
		{
			name: "int flag uses IntValue",
			doc:  mustNormalizeFixture(t, "int_type.yaml"),
			wantContains: []string{
				"func (e Evaluator) QuotaLimit(ctx context.Context) (int64, error)",
				`return e.client.IntValue(ctx, FlagQuotaLimit, 1, newEvaluationContext(attrs))`,
			},
		},
		{
			name: "float flag uses FloatValue",
			doc:  mustNormalizeFixture(t, "float_type.yaml"),
			wantContains: []string{
				"func (e Evaluator) ScoreThreshold(ctx context.Context) (float64, error)",
				`return e.client.FloatValue(ctx, FlagScoreThreshold, 0.5, newEvaluationContext(attrs))`,
			},
		},
		{
			name: "object flag uses ObjectValue and map return",
			doc:  mustNormalizeFixture(t, "object_type.yaml"),
			wantContains: []string{
				"func (e Evaluator) ThemeConfig(ctx context.Context) (map[string]any, error)",
				`var ErrInvalidObjectValue = errors.New("invalid object value")`,
				"value, err := e.client.ObjectValue(ctx, FlagThemeConfig, map[string]any{",
				"return asObjectValue(value)",
			},
		},
		{
			name: "list flag uses ObjectValue and slice return",
			doc:  mustNormalizeFixture(t, "list_type.yaml"),
			wantContains: []string{
				"func (e Evaluator) StarterBadges(ctx context.Context) ([]any, error)",
				`var ErrInvalidListValue = errors.New("invalid list value")`,
				`value, err := e.client.ObjectValue(ctx, FlagStarterBadges, []any{}, newEvaluationContext(attrs))`,
				"return asListValue(value)",
			},
		},
		{
			name: "distribute action requires targeting key",
			doc: &ast.Document{Flags: []*ast.Flag{
				{
					Key:            "checkout-rollout",
					DefaultVariant: "control",
					Variants: map[string]ast.VariantValue{
						"control":   {Kind: ast.VariantValueKindString, String: "control"},
						"treatment": {Kind: ast.VariantValueKindString, String: "treatment"},
					},
					Environments: map[string]*ast.Environment{
						"prod": {
							DefaultAction: &ast.DistributeAction{
								Stickiness:  "user.id",
								Allocations: map[string]float64{"control": 50, "treatment": 50},
							},
						},
					},
				},
			}},
			wantContains: []string{
				"func (e Evaluator) CheckoutRollout(ctx context.Context, targetingKey string) (CheckoutRolloutVariant, error)",
				"if targetingKey == \"\" {",
				`return "", fmt.Errorf("flag %s: %w", FlagCheckoutRollout, ErrMissingTargetingKey)`,
				`attrs = withTargetingKeyAttributes(attrs, targetingKey, "user.id")`,
				`v, err := e.client.StringValue(ctx, FlagCheckoutRollout, "control", newEvaluationContextWithTargetingKey(attrs, targetingKey))`,
			},
		},
		{
			name: "progressive rollout requires targeting key",
			doc: &ast.Document{Flags: []*ast.Flag{
				{
					Key:            "new-home-rollout",
					DefaultVariant: "off",
					Variants: map[string]ast.VariantValue{
						"off": {Kind: ast.VariantValueKindBool, Bool: false},
						"on":  {Kind: ast.VariantValueKindBool, Bool: true},
					},
					Environments: map[string]*ast.Environment{
						"prod": {
							DefaultAction: &ast.ProgressiveRolloutAction{
								Variant:    "on",
								Stickiness: "user.id",
								Start:      "2026-01-01T00:00:00Z",
								End:        "2026-02-01T00:00:00Z",
								Steps:      5,
							},
						},
					},
				},
			}},
			wantContains: []string{
				"func (e Evaluator) NewHomeRollout(ctx context.Context, targetingKey string) (bool, error)",
				`return false, fmt.Errorf("flag %s: %w", FlagNewHomeRollout, ErrMissingTargetingKey)`,
				`attrs = withTargetingKeyAttributes(attrs, targetingKey, "user.id")`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Compile(tt.doc, Config{PackageName: "featureflags"})
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}

			text := string(got)
			for _, want := range tt.wantContains {
				if !strings.Contains(text, want) {
					t.Fatalf("expected generated code to contain %q:\n%s", want, text)
				}
			}
			for _, unwanted := range tt.wantAbsent {
				if strings.Contains(text, unwanted) {
					t.Fatalf("expected generated code not to contain %q:\n%s", unwanted, text)
				}
			}
		})
	}
}

func mustNormalizeFixture(t *testing.T, name string) *ast.Document {
	t.Helper()
	input := testhelper.MustReadFile(t, testdataFS, filepath.Join("testdata", name))
	doc, err := parse.ParseYAML(input)
	if err != nil {
		t.Fatalf("parse input: %v", err)
	}
	normalized, err := normalize.Normalize(doc)
	if err != nil {
		t.Fatalf("normalize input: %v", err)
	}
	return normalized
}
