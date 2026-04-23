package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"

	gofeatureflag "github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
	"github.com/open-feature/go-sdk/openfeature"
	"go.yaml.in/yaml/v3"

	"github.com/satorunooshie/ffcraft/examples/go-codegen/rollout/adapter"
	featureflags "github.com/satorunooshie/ffcraft/examples/go-codegen/rollout/gen"
)

func main() {
	ctx := context.Background()
	flagFile, err := filepath.Abs("./rollout/gen/prod.goff.yaml")
	if err != nil {
		panic(err)
	}

	flagConfigResponse, err := loadRelayProxyResponse(flagFile)
	if err != nil {
		panic(err)
	}

	relayProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/flag/configuration":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(flagConfigResponse); err != nil {
				panic(err)
			}
		case "/v1/data/collector":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer relayProxy.Close()

	provider, err := gofeatureflag.NewProviderWithContext(ctx, gofeatureflag.ProviderOptions{
		Endpoint:              relayProxy.URL,
		DataCollectorDisabled: true,
	})
	if err != nil {
		panic(err)
	}
	defer openfeature.Shutdown()

	if err := openfeature.SetProviderAndWait(provider); err != nil {
		panic(err)
	}

	client := openfeature.NewDefaultClient()
	evaluator := featureflags.New(adapter.NewClientAdapter(client))

	experimentCounts, err := collectExperimentDistribution(ctx, evaluator, 100)
	if err != nil {
		panic(err)
	}

	themeCounts, err := collectThemeDistribution(ctx, evaluator, 100)
	if err != nil {
		panic(err)
	}

	fmt.Printf("experiment_rollout (100 internal users) = %s\n", formatCounts(experimentCounts))
	fmt.Printf("homepage_theme (100 JP users) = %s\n", formatCounts(themeCounts))
}

func loadRelayProxyResponse(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	flags := make(map[string]any)
	if err := yaml.Unmarshal(raw, &flags); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		"flags": flags,
	})
}

func collectExperimentDistribution(ctx context.Context, evaluator featureflags.Evaluator, sampleSize int) (map[string]int, error) {
	counts := make(map[string]int)
	for i := 0; i < sampleSize; i++ {
		variant, err := evaluator.ExperimentRollout(
			ctx,
			featureflags.EvalContext{UserType: "internal"},
			fmt.Sprintf("user-%03d", i),
		)
		if err != nil {
			return nil, err
		}
		counts[string(variant)]++
	}
	return counts, nil
}

func collectThemeDistribution(ctx context.Context, evaluator featureflags.Evaluator, sampleSize int) (map[string]int, error) {
	counts := make(map[string]int)
	for i := 0; i < sampleSize; i++ {
		variant, err := evaluator.HomepageTheme(
			ctx,
			featureflags.EvalContext{UserCountry: "JP"},
			fmt.Sprintf("jp-user-%03d", i),
		)
		if err != nil {
			return nil, err
		}
		counts[string(variant)]++
	}
	return counts, nil
}

func formatCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	result := ""
	for i, key := range keys {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf("%s:%d", key, counts[key])
	}
	return result
}
