package featureflags

import (
	"context"
	"errors"
	"testing"
)

type stringClient struct {
	value func(string, string, EvaluationContext) (string, error)
}

func (c stringClient) BooleanValue(context.Context, string, bool, EvaluationContext) (bool, error) {
	return false, errors.New("unexpected BooleanValue call")
}

func (c stringClient) StringValue(_ context.Context, key, defaultValue string, evalCtx EvaluationContext) (string, error) {
	return c.value(key, defaultValue, evalCtx)
}

func (c stringClient) IntValue(context.Context, string, int64, EvaluationContext) (int64, error) {
	return 0, errors.New("unexpected IntValue call")
}

func (c stringClient) FloatValue(context.Context, string, float64, EvaluationContext) (float64, error) {
	return 0, errors.New("unexpected FloatValue call")
}

func (c stringClient) ObjectValue(context.Context, string, any, EvaluationContext) (any, error) {
	return nil, errors.New("unexpected ObjectValue call")
}

func TestRolloutAccessors(t *testing.T) {
	client := stringClient{
		value: func(key, defaultValue string, evalCtx EvaluationContext) (string, error) {
			if defaultValue != "control" && defaultValue != "default" {
				t.Errorf("default value for %q = %q", key, defaultValue)
			}
			if evalCtx.TargetingKey != "user-123" {
				t.Errorf("targeting key for %q = %q; want user-123", key, evalCtx.TargetingKey)
			}
			if evalCtx.Attributes["user"].(map[string]any)["country"] != "JP" ||
				evalCtx.Attributes["user"].(map[string]any)["type"] != "internal" ||
				evalCtx.Attributes["user"].(map[string]any)["id"] != "user-123" {
				t.Errorf("attributes for %q = %#v; want user attributes", key, evalCtx.Attributes)
			}
			if key == FlagExperimentRollout {
				return "treatment", nil
			}
			return "modern", nil
		},
	}
	evaluator := New(client)

	experiment, err := evaluator.ExperimentRollout(context.Background(), EvalContext{UserCountry: "JP", UserType: "internal"}, "user-123")
	if err != nil || experiment != ExperimentVariantTreatment {
		t.Fatalf("ExperimentRollout() = %q, %v; want treatment, nil", experiment, err)
	}
	theme, err := evaluator.HomepageTheme(context.Background(), EvalContext{UserCountry: "JP", UserType: "internal"}, "user-123")
	if err != nil || theme != HomepageThemeVariantModern {
		t.Fatalf("HomepageTheme() = %q, %v; want modern, nil", theme, err)
	}
}

func TestRolloutAccessorsRequireTargetingKey(t *testing.T) {
	client := stringClient{value: func(string, string, EvaluationContext) (string, error) {
		return "control", nil
	}}
	evaluator := New(client)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "experiment rollout",
			call: func() error {
				_, err := evaluator.ExperimentRollout(context.Background(), EvalContext{}, "")
				return err
			},
		},
		{
			name: "homepage theme",
			call: func() error {
				_, err := evaluator.HomepageTheme(context.Background(), EvalContext{}, "")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrMissingTargetingKey) {
				t.Fatalf("accessor error = %v; want %v", err, ErrMissingTargetingKey)
			}
		})
	}
}
