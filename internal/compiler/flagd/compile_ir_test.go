package flagd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCompileIRDirectSemanticSurface(t *testing.T) {
	doc := directCompilerFixture()
	output, _, err := CompileIR(doc, "prod", CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"user.segment", "user.id", "fractional", "1767225600", "\"off\",", "\"on\","} {
		if !strings.Contains(string(output), fragment) {
			t.Fatalf("flagd output missing %q: %s", fragment, output)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("decode flagd JSON: %v", err)
	}
	want := map[string]any{
		"$schema": schemaURL,
		"flags": map[string]any{
			"direct": map[string]any{
				"state":          "ENABLED",
				"variants":       map[string]any{"off": false, "on": true},
				"defaultVariant": "off",
				"targeting": map[string]any{"if": []any{
					map[string]any{">=": []any{map[string]any{"var": "$flagd.timestamp"}, float64(1767225600)}},
					map[string]any{"fractional": []any{
						map[string]any{"var": "user.id"},
						[]any{"off", float64(1)},
						[]any{"on", float64(2)},
					}},
					map[string]any{"if": []any{
						map[string]any{"if": []any{map[string]any{"missing": []any{"user.segment"}}, false, map[string]any{"===": []any{map[string]any{"var": "user.segment"}, "beta"}}}},
						"on",
						"off",
					}},
				}},
			},
		},
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("compiled flagd structure = %#v, want %#v", decoded, want)
	}
}

func TestCompileIRSupportsBaseDistributionWithoutDefaultVariant(t *testing.T) {
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{"distributed": {
		Variants: map[string]*irv1.VariantValue{
			"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
			"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
		},
		Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{
			AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: map[string]uint32{"off": 1, "on": 1},
		}}}}}},
	}}}
	output, _, err := CompileIR(doc, "prod", CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatal(err)
	}
	flag := decoded["flags"].(map[string]any)["distributed"].(map[string]any)
	if _, ok := flag["defaultVariant"]; ok {
		t.Fatal("base distribution must not require defaultVariant")
	}
	if flag["targeting"] == nil {
		t.Fatal("base distribution must be emitted as targeting")
	}
}

func directCompilerFixture() *irv1.Document {
	return &irv1.Document{Flags: map[string]*irv1.Flag{
		"direct": {
			Variants: map[string]*irv1.VariantValue{
				"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
				"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
			},
			Environments: map[string]*irv1.Environment{"prod": {
				Base: &irv1.Evaluation{Rules: []*irv1.Rule{{
					Condition: &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: &irv1.AttributePath{Segments: []string{"user", "segment"}}, Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "beta"}}}}},
					Action:    &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}},
				}}, DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}},
				Schedule: []*irv1.ScheduledEvaluation{{EffectiveAt: timestamppb.New(time.Unix(1767225600, 1).UTC()), Evaluation: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: map[string]uint32{"off": 1, "on": 2}}}}}}},
			}},
		},
	}}
}

func TestScheduleSelectsNewestElapsedSnapshot(t *testing.T) {
	evaluation := func(name string) *irv1.Evaluation {
		return &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: name}}}
	}
	env := &irv1.Environment{Base: evaluation("base")}
	for i, day := range []int{1, 10, 22} {
		env.Schedule = append(env.Schedule, &irv1.ScheduledEvaluation{
			EffectiveAt: timestamppb.New(time.Date(2026, 5, day, 0, 0, 0, 0, time.UTC)),
			Evaluation:  evaluation([]string{"internal", "ten-percent", "public"}[i]),
		})
	}
	targeting, err := compileIREnvironment(env)
	if err != nil {
		t.Fatal(err)
	}
	// Interpret the emitted if/>= tree to check actual branch selection.
	var evaluate func(any, int64) string
	evaluate = func(expr any, now int64) string {
		if variant, ok := expr.(string); ok {
			return variant
		}
		args := expr.(map[string]any)["if"].([]any)
		comparison := args[0].(map[string]any)[">="].([]any)
		if now >= comparison[1].(int64) {
			return evaluate(args[1], now)
		}
		return evaluate(args[2], now)
	}
	previous := "base"
	for i, step := range env.Schedule {
		at := step.EffectiveAt.AsTime().Unix()
		want := []string{"internal", "ten-percent", "public"}[i]
		for _, tc := range []struct {
			at   int64
			want string
		}{{at - 1, previous}, {at, want}, {at + 1, want}} {
			if got := evaluate(targeting, tc.at); got != tc.want {
				t.Errorf("at %s: got %s, want %s", time.Unix(tc.at, 0), got, tc.want)
			}
		}
		previous = want
	}
	if got := evaluate(targeting, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).Unix()); got != "public" {
		t.Fatalf("after all dates: %s", got)
	}
}
