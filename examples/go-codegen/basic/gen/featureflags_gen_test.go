package featureflags

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recordingClient struct {
	booleanValue func(context.Context, string, bool, EvaluationContext) (bool, error)
	stringValue  func(context.Context, string, string, EvaluationContext) (string, error)
}

func (c recordingClient) BooleanValue(ctx context.Context, key string, defaultValue bool, evalCtx EvaluationContext) (bool, error) {
	if c.booleanValue == nil {
		return defaultValue, errors.New("unexpected BooleanValue call")
	}
	return c.booleanValue(ctx, key, defaultValue, evalCtx)
}

func (c recordingClient) StringValue(ctx context.Context, key string, defaultValue string, evalCtx EvaluationContext) (string, error) {
	if c.stringValue == nil {
		return defaultValue, errors.New("unexpected StringValue call")
	}
	return c.stringValue(ctx, key, defaultValue, evalCtx)
}

func (recordingClient) IntValue(context.Context, string, int64, EvaluationContext) (int64, error) {
	return 0, errors.New("unexpected IntValue call")
}

func (recordingClient) FloatValue(context.Context, string, float64, EvaluationContext) (float64, error) {
	return 0, errors.New("unexpected FloatValue call")
}

func (recordingClient) ObjectValue(context.Context, string, any, EvaluationContext) (any, error) {
	return nil, errors.New("unexpected ObjectValue call")
}

func TestCheckoutMode(t *testing.T) {
	ctx := context.Background()
	client := recordingClient{
		stringValue: func(_ context.Context, key, defaultValue string, evalCtx EvaluationContext) (string, error) {
			if key != FlagCheckoutMode || defaultValue != string(CheckoutModeVariantControl) {
				t.Errorf("StringValue arguments = %q, %q; want %q, %q", key, defaultValue, FlagCheckoutMode, CheckoutModeVariantControl)
			}
			wantAttributes := map[string]any{"user": map[string]any{"id": "user-123"}}
			if !reflect.DeepEqual(evalCtx.Attributes, wantAttributes) {
				t.Errorf("evaluation attributes = %#v; want %#v", evalCtx.Attributes, wantAttributes)
			}
			if evalCtx.TargetingKey != "user-123" {
				t.Errorf("targeting key = %q; want %q", evalCtx.TargetingKey, "user-123")
			}
			return "treatment", nil
		},
	}

	got, err := New(client).CheckoutMode(ctx, "user-123")
	if err != nil {
		t.Fatal(err)
	}
	if got != CheckoutModeVariantTreatment {
		t.Fatalf("CheckoutMode() = %q; want %q", got, CheckoutModeVariantTreatment)
	}
}

func TestCheckoutModeRequiresTargetingKey(t *testing.T) {
	called := false
	client := recordingClient{
		stringValue: func(context.Context, string, string, EvaluationContext) (string, error) {
			called = true
			return "control", nil
		},
	}

	_, err := New(client).CheckoutMode(context.Background(), "")
	if !errors.Is(err, ErrMissingTargetingKey) {
		t.Fatalf("CheckoutMode() error = %v; want %v", err, ErrMissingTargetingKey)
	}
	if called {
		t.Fatal("CheckoutMode() called the client without a targeting key")
	}
}

func TestEnableNewHomeBuildsEvaluationAttributes(t *testing.T) {
	client := recordingClient{
		booleanValue: func(_ context.Context, key string, defaultValue bool, evalCtx EvaluationContext) (bool, error) {
			if key != FlagEnableNewHome || defaultValue {
				t.Errorf("BooleanValue arguments = %q, %v; want %q, false", key, defaultValue, FlagEnableNewHome)
			}
			wantAttributes := map[string]any{"device": map[string]any{"platform": "ios"}}
			if !reflect.DeepEqual(evalCtx.Attributes, wantAttributes) {
				t.Errorf("evaluation attributes = %#v; want %#v", evalCtx.Attributes, wantAttributes)
			}
			return true, nil
		},
	}

	got, err := New(client).EnableNewHome(context.Background(), EvalContext{DevicePlatform: "ios"})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("EnableNewHome() = false; want true")
	}
}
