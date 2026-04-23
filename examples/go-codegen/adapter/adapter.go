package adapter

import (
	"context"

	"github.com/open-feature/go-sdk/openfeature"
)

type Adapter[C any] struct {
	client        *openfeature.Client
	hooks         []openfeature.Hook
	toEvaluation  func(C) openfeature.EvaluationContext
}

func NewClientAdapter[C any](client *openfeature.Client, toEvaluation func(C) openfeature.EvaluationContext, hooks ...openfeature.Hook) *Adapter[C] {
	return &Adapter[C]{
		client:       client,
		hooks:        hooks,
		toEvaluation: toEvaluation,
	}
}

func (a Adapter[C]) BooleanValue(ctx context.Context, key string, defaultValue bool, evalCtx C) (bool, error) {
	return a.client.BooleanValue(ctx, key, defaultValue, a.toEvaluation(evalCtx), a.withHooks()...)
}

func (a Adapter[C]) StringValue(ctx context.Context, key string, defaultValue string, evalCtx C) (string, error) {
	return a.client.StringValue(ctx, key, defaultValue, a.toEvaluation(evalCtx), a.withHooks()...)
}

func (a Adapter[C]) IntValue(ctx context.Context, key string, defaultValue int64, evalCtx C) (int64, error) {
	return a.client.IntValue(ctx, key, defaultValue, a.toEvaluation(evalCtx), a.withHooks()...)
}

func (a Adapter[C]) FloatValue(ctx context.Context, key string, defaultValue float64, evalCtx C) (float64, error) {
	return a.client.FloatValue(ctx, key, defaultValue, a.toEvaluation(evalCtx), a.withHooks()...)
}

func (a Adapter[C]) ObjectValue(ctx context.Context, key string, defaultValue any, evalCtx C) (any, error) {
	return a.client.ObjectValue(ctx, key, defaultValue, a.toEvaluation(evalCtx), a.withHooks()...)
}

func (a Adapter[C]) withHooks() []openfeature.Option {
	if len(a.hooks) == 0 {
		return nil
	}
	return []openfeature.Option{openfeature.WithHooks(a.hooks...)}
}
