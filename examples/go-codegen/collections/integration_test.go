package collections_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gofeatureflag "github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
	"github.com/open-feature/go-sdk/openfeature"

	"github.com/satorunooshie/ffcraft/examples/go-codegen/adapter"
	featureflags "github.com/satorunooshie/ffcraft/examples/go-codegen/collections/gen"
)

func TestB9GoffStructuredValuesKeepListAndObjectShapes(t *testing.T) {
	response := map[string]any{
		"flags": map[string]any{
			"starter-badges": map[string]any{
				"variations":  map[string]any{"empty": []any{}, "starter": []any{"new", "vip"}},
				"defaultRule": map[string]any{"variation": "starter"},
			},
			"theme-config": map[string]any{
				"variations":  map[string]any{"default": map[string]any{"primary": "blue", "nested": map[string]any{"enabled": true}}},
				"defaultRule": map[string]any{"variation": "default"},
			},
			"numeric-object-safe": map[string]any{
				"variations":  map[string]any{"object_safe": map[string]any{"id": float64(9007199254740991)}},
				"defaultRule": map[string]any{"variation": "object_safe"},
			},
			"numeric-object-list-safe": map[string]any{
				"variations":  map[string]any{"object_list_safe": map[string]any{"ids": []any{float64(9007199254740991)}}},
				"defaultRule": map[string]any{"variation": "object_list_safe"},
			},
		},
	}
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/flag/configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	provider, err := gofeatureflag.NewProviderWithContext(context.Background(), gofeatureflag.ProviderOptions{
		Endpoint:              server.URL,
		HTTPClient:            server.Client(),
		DataCollectorDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatal(err)
	}
	defer openfeature.Shutdown()

	evaluator := featureflags.New(adapter.NewClientAdapter(openfeature.NewDefaultClient(), func(ctx featureflags.EvaluationContext) openfeature.EvaluationContext {
		return openfeature.NewTargetlessEvaluationContext(ctx.Attributes)
	}))
	ctx := context.Background()
	badges, err := evaluator.StarterBadges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(badges) != 2 || badges[0] != "new" || badges[1] != "vip" {
		t.Fatalf("unexpected generated list value: %#v", badges)
	}
	config, err := evaluator.ThemeConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nested, ok := config["nested"].(map[string]any)
	if !ok || config["primary"] != "blue" || nested["enabled"] != true {
		t.Fatalf("unexpected generated object value: %#v", config)
	}
	numericObject, err := evaluator.NumericObjectSafe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if numericObject["id"] != float64(9007199254740991) {
		t.Fatalf("object number = %#v, want safe boundary", numericObject["id"])
	}
	numericList, err := evaluator.NumericObjectListSafe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids, ok := numericList["ids"].([]any)
	if !ok || len(ids) != 1 || ids[0] != float64(9007199254740991) {
		t.Fatalf("object-list number = %#v, want safe boundary", numericList["ids"])
	}
}
