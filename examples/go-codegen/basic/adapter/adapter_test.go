package adapter

import (
	"reflect"
	"testing"

	featureflags "github.com/satorunooshie/ffcraft/examples/go-codegen/basic/gen"
)

func TestToOpenFeatureEvaluationContext(t *testing.T) {
	tests := []struct {
		name      string
		input     featureflags.EvaluationContext
		wantKey   string
		wantAttrs map[string]any
	}{
		{
			name:      "targetless with nil attributes",
			input:     featureflags.EvaluationContext{},
			wantKey:   "",
			wantAttrs: map[string]any{},
		},
		{
			name: "targeting key is added as an attribute",
			input: featureflags.EvaluationContext{
				TargetingKey: "user-123",
				Attributes:   map[string]any{"device": map[string]any{"platform": "ios"}},
			},
			wantKey:   "user-123",
			wantAttrs: map[string]any{"device": map[string]any{"platform": "ios"}, "targetingKey": "user-123"},
		},
		{
			name: "existing targeting key attribute is preserved",
			input: featureflags.EvaluationContext{
				TargetingKey: "user-123",
				Attributes:   map[string]any{"targetingKey": "custom"},
			},
			wantKey:   "user-123",
			wantAttrs: map[string]any{"targetingKey": "custom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toOpenFeatureEvaluationContext(tt.input)
			if got.TargetingKey() != tt.wantKey {
				t.Errorf("TargetingKey() = %q; want %q", got.TargetingKey(), tt.wantKey)
			}
			if attrs := got.Attributes(); !reflect.DeepEqual(attrs, tt.wantAttrs) {
				t.Errorf("Attributes() = %#v; want %#v", attrs, tt.wantAttrs)
			}
		})
	}
}
