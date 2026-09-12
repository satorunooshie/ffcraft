package featureflags

import (
	"context"
	"errors"
	"testing"
)

type booleanClient struct {
	value func(string, bool, EvaluationContext) (bool, error)
}

func (c booleanClient) BooleanValue(_ context.Context, key string, defaultValue bool, evalCtx EvaluationContext) (bool, error) {
	return c.value(key, defaultValue, evalCtx)
}

func (booleanClient) StringValue(context.Context, string, string, EvaluationContext) (string, error) {
	return "", errors.New("unexpected StringValue call")
}

func (booleanClient) IntValue(context.Context, string, int64, EvaluationContext) (int64, error) {
	return 0, errors.New("unexpected IntValue call")
}

func (booleanClient) FloatValue(context.Context, string, float64, EvaluationContext) (float64, error) {
	return 0, errors.New("unexpected FloatValue call")
}

func (booleanClient) ObjectValue(context.Context, string, any, EvaluationContext) (any, error) {
	return nil, errors.New("unexpected ObjectValue call")
}

func TestShowSampleBanner(t *testing.T) {
	client := booleanClient{
		value: func(key string, defaultValue bool, evalCtx EvaluationContext) (bool, error) {
			if key != FlagShowSampleBanner || defaultValue {
				t.Errorf("BooleanValue arguments = %q, %v; want %q, false", key, defaultValue, FlagShowSampleBanner)
			}
			if evalCtx.TargetingKey != "" || len(evalCtx.Attributes) != 0 {
				t.Errorf("evaluation context = %#v; want empty context", evalCtx)
			}
			return true, nil
		},
	}

	got, err := New(client).ShowSampleBanner(context.Background())
	if err != nil || !got {
		t.Fatalf("ShowSampleBanner() = %v, %v; want true, nil", got, err)
	}
}
