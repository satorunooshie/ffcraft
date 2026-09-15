package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProjectConfigContracts(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"invalid version", "version: v2\nsource: flags.yaml\ntargets: {go: {package: flags}}\n", "version must be v1"},
		{"missing source", "version: v1\ntargets: {go: {package: flags}}\n", "source is required"},
		{"missing targets", "version: v1\nsource: flags.yaml\n", "targets are required"},
		{"invalid yaml", "version: [\n", "unmarshal config"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ffcodegen.yaml")
			if err := os.WriteFile(path, []byte(test.source), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() = %v, want %q", err, test.want)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "ffcodegen.yaml")
	source := "version: v1\nsource: authoring.yaml\ntargets:\n  go:\n    package: featureflags\n    output: generated/flags.go\n    context_type: RequestContext\n    client_type: FeatureClient\n    evaluator_type: FeatureEvaluator\n    context:\n      fields:\n        - path: user.id\n          name: UserID\n          type: string\n    accessors:\n      checkout:\n        name: CheckoutMode\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	target, ok := config.Targets["go"]
	if !ok || target.PackageName != "featureflags" || target.ContextType != "RequestContext" || target.Context.Fields[0].Name != "UserID" || target.Accessors["checkout"].Name != "CheckoutMode" {
		t.Fatalf("loaded config = %#v", config)
	}
}

func TestLoadAndResolvePathContracts(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("missing config error = %v", err)
	}
	configPath := filepath.Join("/tmp", "project", "ffcodegen.yaml")
	if got := ResolvePath(configPath, "generated/flags.go"); got != filepath.Join("/tmp", "project", "generated/flags.go") {
		t.Fatalf("ResolvePath(relative) = %q", got)
	}
	if got := ResolvePath(configPath, "/absolute/flags.go"); got != "/absolute/flags.go" {
		t.Fatalf("ResolvePath(absolute) = %q", got)
	}
}
