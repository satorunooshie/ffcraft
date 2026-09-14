package v1_test

import (
	_ "embed"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
	"gopkg.in/yaml.v3"
)

//go:embed testdata/invalid/invalid_cases.yaml
var invalidCasesFixture []byte

func TestV1InvalidFixtures(t *testing.T) {
	var fixture struct {
		Version string `yaml:"version"`
		Cases   []struct {
			Name  string `yaml:"name"`
			Input string `yaml:"input"`
		} `yaml:"cases"`
	}
	if err := yaml.Unmarshal(invalidCasesFixture, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != "conformance/v1" {
		t.Fatalf("fixture version = %q", fixture.Version)
	}
	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			if _, err := normalizedyaml.Unmarshal([]byte(test.Input)); err == nil {
				t.Fatal("expected invalid fixture to be rejected")
			}
		})
	}
}
