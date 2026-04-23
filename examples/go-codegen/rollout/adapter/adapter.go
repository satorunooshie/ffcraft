package adapter

import (
	"github.com/open-feature/go-sdk/openfeature"

	"github.com/satorunooshie/ffcraft/examples/go-codegen/adapter"
	featureflags "github.com/satorunooshie/ffcraft/examples/go-codegen/rollout/gen"
)

func NewClientAdapter(client *openfeature.Client, hooks ...openfeature.Hook) *adapter.Adapter[featureflags.EvaluationContext] {
	return adapter.NewClientAdapter(client, toOpenFeatureEvaluationContext, hooks...)
}

func toOpenFeatureEvaluationContext(evalCtx featureflags.EvaluationContext) openfeature.EvaluationContext {
	attrs := evalCtx.Attributes
	if attrs == nil {
		attrs = make(map[string]any)
	}
	if evalCtx.TargetingKey == "" {
		return openfeature.NewTargetlessEvaluationContext(attrs)
	}
	if _, ok := attrs["targetingKey"]; !ok {
		attrs["targetingKey"] = evalCtx.TargetingKey
	}
	return openfeature.NewEvaluationContext(evalCtx.TargetingKey, attrs)
}
