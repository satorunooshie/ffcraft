package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	gofeatureflag "github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
	"github.com/open-feature/go-sdk/openfeature"
	"go.yaml.in/yaml/v3"

	"github.com/satorunooshie/ffcraft/examples/go-codegen/basic/adapter"
	featureflags "github.com/satorunooshie/ffcraft/examples/go-codegen/basic/gen"
)

func main() {
	ctx := context.Background()
	flagFile, err := filepath.Abs("./basic/gen/prod.goff.yaml")
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
