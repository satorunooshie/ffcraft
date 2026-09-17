package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/authoring"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/testhelper"
)

func TestCollectionContextGolden(t *testing.T) {
	for _, tc := range []struct {
		name, input, golden string
		defaults            ContextDefaultsConfig
		fields              []ContextFieldConfig
	}{
		{name: "inference", input: "context_collection_inference.yaml", golden: "context_collection_inference.golden.go"},
		{name: "override", input: "context_collection_overrides.yaml", golden: "context_collection_overrides.golden.go", fields: []ContextFieldConfig{{Path: "user.scores", Type: "[]int"}}},
		{name: "defaults", input: "context_collection_inference.yaml", golden: "context_defaults.golden.go", defaults: ContextDefaultsConfig{CollectionTypes: map[string]string{"int": "[]int"}}},
		{name: "override precedence", input: "context_collection_inference.yaml", golden: "context_defaults_precedence.golden.go", defaults: ContextDefaultsConfig{CollectionTypes: map[string]string{"int": "[]int"}}, fields: []ContextFieldConfig{{Path: "user.scores", Type: "[]int64"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tc.input))
			if err != nil {
				t.Fatal(err)
			}
			authored, err := authoring.ParseYAML(data)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := normalize.Normalize(authored)
			if err != nil {
				t.Fatal(err)
			}
			output, err := CompileIR(doc, Config{PackageName: "featureflags", InferSDKFallback: true, ContextDefaults: tc.defaults, ContextFields: tc.fields})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", tc.golden)
			if testhelper.ShouldUpdateGolden() {
				testhelper.WriteGolden(t, path, output)
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(output, want) {
				t.Fatalf("generated context differs from %s; run UPDATE_GOLDEN=1 go test ./internal/codegen", path)
			}
		})
	}
}
