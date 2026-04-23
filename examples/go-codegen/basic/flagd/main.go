package main

import (
	"context"
	"fmt"
	"path/filepath"

	flagd "github.com/open-feature/go-sdk-contrib/providers/flagd/pkg"
	"github.com/open-feature/go-sdk/openfeature"

	"github.com/satorunooshie/ffcraft/examples/go-codegen/basic/adapter"
	featureflags "github.com/satorunooshie/ffcraft/examples/go-codegen/basic/gen"
)

func main() {
	ctx := context.Background()
	flagFile, err := filepath.Abs("./basic/gen/prod.flagd.json")
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

	enabled, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"})
	if err != nil {
		panic(err)
	}

	variant, err := evaluator.CheckoutMode(ctx, "user-123")
	if err != nil {
		panic(err)
	}

	fmt.Printf("enable-new-home(ios) = %t\n", enabled)
	fmt.Printf("checkout-mode(user-123) = %s\n", variant)
}
