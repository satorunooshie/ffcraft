package validate_test

import (
	"embed"
	"strings"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/parse"
	"github.com/satorunooshie/ffcraft/internal/validate"
)

//go:embed testdata/*.yaml
var testdataFS embed.FS

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    string
		wantErr string
	}{
		{name: "accepts valid document", file: "testdata/valid_document.yaml"},
		{name: "rejects distribution unknown variant", file: "testdata/error_distribution_unknown_variant.yaml", wantErr: `distribution "rollout" references unknown variant "missing"`},
		{name: "rejects default action unknown variant", file: "testdata/error_fallback_unknown_variant.yaml", wantErr: `default action: serve variant "missing" not found`},
		{name: "rejects environment without branch", file: "testdata/error_environment_without_branch.yaml", wantErr: `rule_evaluation must define rules, default_action, experimentation, or scheduled_rollouts`},
		{name: "rejects rule cycle", file: "testdata/error_rule_cycle.yaml", wantErr: "rule cycle detected"},
		{name: "rejects duplicate flag keys", file: "testdata/error_duplicate_flag_keys.yaml", wantErr: `duplicate flag key "duplicate"`},
		{name: "rejects unknown variant set", file: "testdata/error_unknown_variant_set.yaml", wantErr: `unknown variant_set "boolean"`},
		{name: "rejects unknown rule reference", file: "testdata/error_unknown_rule_reference.yaml", wantErr: `referenced rule "missing_rule" not found`},
		{name: "rejects bad distribution sum", file: "testdata/error_bad_distribution_sum.yaml", wantErr: `distribution "bad_rollout": allocation total must equal 100`},
		{name: "rejects invalid expiry format", file: "testdata/error_invalid_expiry.yaml", wantErr: `metadata.expiry must be YYYY-MM-DD`},
		{name: "rejects normalize rule cycle fixture", file: "testdata/error_rule_cycle_normalize.yaml", wantErr: "rule cycle detected"},
		{name: "rejects invalid progressive rollout", file: "testdata/error_invalid_progressive_rollout.yaml", wantErr: `variant "missing" not found`},
		{name: "rejects progressive rollout with zero steps", file: "testdata/error_progressive_rollout_zero_steps.yaml", wantErr: `steps: must be greater than or equal to 1`},
		{name: "rejects progressive rollout invalid window", file: "testdata/error_progressive_rollout_invalid_window.yaml", wantErr: `start must be before end`},
		{name: "rejects progressive rollout in rule action", file: "testdata/error_progressive_rollout_in_rule.yaml", wantErr: `progressive_rollout is only supported in default_action`},
		{name: "rejects targetingKey stickiness", file: "testdata/error_targeting_key_stickiness.yaml", wantErr: `stickiness "targetingKey" is not supported`},
		{name: "rejects scheduled step missing default action", file: "testdata/error_scheduled_step_missing_default_action.yaml", wantErr: `default_action: value is required`},
		{name: "rejects duplicate scheduled rollout dates", file: "testdata/error_scheduled_rollouts_duplicate_date.yaml", wantErr: `scheduled_rollouts dates must be unique`},
		{name: "rejects unordered scheduled rollouts", file: "testdata/error_scheduled_rollouts_out_of_order.yaml", wantErr: `scheduled_rollouts must be sorted in ascending date order`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, err := testdataFS.ReadFile(tt.file)
			if err != nil {
				t.Fatalf("read fixture %s: %v", tt.file, err)
			}
			doc, err := parse.ParseYAML(src)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			err = validate.Validate(doc)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate failed: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
