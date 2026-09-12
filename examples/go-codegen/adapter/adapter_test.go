package adapter

import (
	"context"
	"reflect"
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
)

type recordingHook struct {
	before  int
	after   int
	error   int
	finally int
}

func (h *recordingHook) Before(context.Context, openfeature.HookContext, openfeature.HookHints) (*openfeature.EvaluationContext, error) {
	h.before++
	return nil, nil
}

func (h *recordingHook) After(context.Context, openfeature.HookContext, openfeature.InterfaceEvaluationDetails, openfeature.HookHints) error {
	h.after++
	return nil
}

func (h *recordingHook) Error(context.Context, openfeature.HookContext, error, openfeature.HookHints) {
	h.error++
}

func (h *recordingHook) Finally(context.Context, openfeature.HookContext, openfeature.InterfaceEvaluationDetails, openfeature.HookHints) {
	h.finally++
}

func TestAdapterDelegatesAllValueTypesAndHooks(t *testing.T) {
	if err := openfeature.SetProviderAndWait(openfeature.NoopProvider{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(openfeature.Shutdown)

	hook := &recordingHook{}
	conversionCalls := 0
	toEvaluation := func(value int) openfeature.EvaluationContext {
		conversionCalls++
		return openfeature.NewTargetlessEvaluationContext(map[string]any{"value": value})
	}
	client := NewClientAdapter(openfeature.NewDefaultClient(), toEvaluation, hook)
	ctx := context.Background()

	boolean, err := client.BooleanValue(ctx, "boolean", false, 1)
	if err != nil || boolean {
		t.Fatalf("BooleanValue() = %v, %v; want false, nil", boolean, err)
	}
	stringValue, err := client.StringValue(ctx, "string", "default", 2)
	if err != nil || stringValue != "default" {
		t.Fatalf("StringValue() = %q, %v; want default, nil", stringValue, err)
	}
	intValue, err := client.IntValue(ctx, "int", 3, 3)
	if err != nil || intValue != 3 {
		t.Fatalf("IntValue() = %d, %v; want 3, nil", intValue, err)
	}
	floatValue, err := client.FloatValue(ctx, "float", 4.5, 4)
	if err != nil || floatValue != 4.5 {
		t.Fatalf("FloatValue() = %v, %v; want 4.5, nil", floatValue, err)
	}
	objectValue, err := client.ObjectValue(ctx, "object", map[string]any{"default": true}, 5)
	if err != nil || !reflect.DeepEqual(objectValue, map[string]any{"default": true}) {
		t.Fatalf("ObjectValue() = %#v, %v; want default object, nil", objectValue, err)
	}

	if conversionCalls != 5 {
		t.Fatalf("evaluation conversion calls = %d; want 5", conversionCalls)
	}
	if hook.before != 5 || hook.after != 5 || hook.error != 0 || hook.finally != 5 {
		t.Fatalf("hook calls = before:%d after:%d error:%d finally:%d; want 5, 5, 0, 5", hook.before, hook.after, hook.error, hook.finally)
	}
}

func TestAdapterWorksWithoutHooks(t *testing.T) {
	if err := openfeature.SetProviderAndWait(openfeature.NoopProvider{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(openfeature.Shutdown)

	client := NewClientAdapter(
		openfeature.NewDefaultClient(),
		func(int) openfeature.EvaluationContext { return openfeature.NewTargetlessEvaluationContext(nil) },
	)
	got, err := client.BooleanValue(context.Background(), "boolean", true, 0)
	if err != nil || !got {
		t.Fatalf("BooleanValue() = %v, %v; want true, nil", got, err)
	}
}
