package v1_test

import (
	"errors"
	"testing"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/capability"
	"github.com/satorunooshie/ffcraft/internal/compiler/flagd"
	"github.com/satorunooshie/ffcraft/internal/compiler/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/runtimeeval"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Target restrictions must also apply to nested and scheduled conditions.
func TestContainsTargetCapabilities(t *testing.T) {
	for _, kind := range []capability.ConditionKind{capability.ConditionCollectionContains, capability.ConditionStringContains} {
		for _, wrapper := range []string{"direct", "not", "all", "any", "exactly_one"} {
			for _, scheduled := range []bool{false, true} {
				doc := conditionCapabilityFixture(kind)
				env := doc.Flags["condition"].Environments["prod"]
				rule := env.Base.Rules[0]
				wrongType := any("beta-user")
				if kind == capability.ConditionStringContains {
					rule.Condition.GetStringMatch().Literal = "beta"
					wrongType = []any{"beta"}
				}
				if runtimeeval.Evaluate(rule.Condition, runtimeeval.Context{"value": wrongType}) {
					t.Fatal("IR accepted wrong container type")
				}
				switch wrapper {
				case "not":
					rule.Condition = &irv1.Condition{Kind: &irv1.Condition_Negation{Negation: rule.Condition}}
				case "all", "any", "exactly_one":
					operator := map[string]irv1.LogicalOperator{"all": irv1.LogicalOperator_LOGICAL_OPERATOR_ALL, "any": irv1.LogicalOperator_LOGICAL_OPERATOR_ANY, "exactly_one": irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE}[wrapper]
					rule.Condition = &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Operator: operator, Conditions: []*irv1.Condition{rule.Condition, {Kind: &irv1.Condition_Constant{Constant: true}}}}}}
				}
				location := "base"
				if scheduled {
					location = "schedule"
					env.Schedule = []*irv1.ScheduledEvaluation{{EffectiveAt: timestamppb.New(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)), Evaluation: env.Base}}
					env.Base = &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}}
				}
				for _, target := range []capability.Target{capability.TargetFlagd, capability.TargetGOFeatureFlag} {
					t.Run(string(target)+"/"+string(kind)+"/"+wrapper+"/"+location, func(t *testing.T) {
						var output []byte
						var err error
						if target == capability.TargetFlagd {
							output, _, err = flagd.CompileIR(doc, "prod", flagd.CompileOptions{})
						} else {
							output, _, err = gofeatureflag.CompileIR(doc, "prod", gofeatureflag.CompileOptions{})
						}
						if target == capability.TargetFlagd && kind == capability.ConditionStringContains {
							if err != nil || len(output) == 0 {
								t.Fatalf("guarded substring compilation failed: %v", err)
							}
							return
						}
						var unsupported *capability.UnsupportedConditionError
						if !errors.Is(err, capability.ErrUnsupportedCondition) || !errors.As(err, &unsupported) {
							t.Fatalf("expected explicit rejection: output %s, error %v", output, err)
						}
						if unsupported.Target != target || unsupported.Condition != kind || len(output) != 0 {
							t.Fatalf("incorrect rejection: output %s, error %v", output, err)
						}
					})
				}
			}
		}
	}
}
