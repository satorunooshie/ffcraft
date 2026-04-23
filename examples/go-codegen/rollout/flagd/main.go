package main

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	flagd "github.com/open-feature/go-sdk-contrib/providers/flagd/pkg"
	"github.com/open-feature/go-sdk/openfeature"

	"github.com/satorunooshie/ffcraft/examples/go-codegen/rollout/adapter"
	featureflags "github.com/satorunooshie/ffcraft/examples/go-codegen/rollout/gen"
)

func main() {
	ctx := context.Background()
	flagFile, err := filepath.Abs("./rollout/gen/prod.flagd.json")
	if err != nil {
		panic(err)
	}

	provider, err := flagd.NewProvider(
		flagd.WithFileResolver(),
		flagd.WithOfflineFilePath(flagFile),
	)
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
