package main

import (
	"context"
	"fmt"
	"path/filepath"

	flagd "github.com/open-feature/go-sdk-contrib/providers/flagd/pkg"
	"github.com/open-feature/go-sdk/openfeature"

	"github.com/satorunooshie/ffcraft/examples/go-codegen/hooks"
	"github.com/satorunooshie/ffcraft/examples/go-codegen/withhooks/adapter"
	featureflags "github.com/satorunooshie/ffcraft/examples/go-codegen/withhooks/gen"
)

func main() {
	ctx := context.Background()
	flagFile, err := filepath.Abs("./withhooks/gen/prod.flagd.json")
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
	evaluator := featureflags.New(adapter.NewClientAdapter(client, hooks.LoggingHook{}))

	enabled, err := evaluator.ShowSampleBanner(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("show_sample_banner = %t\n", enabled)
}
