package codegen

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/satorunooshie/ffcraft/internal/gogen"
)

type Config struct {
	Version string            `yaml:"version"`
	Source  string            `yaml:"source"`
	Targets map[string]Target `yaml:"targets"`
}

type Target struct {
	PackageName   string                          `yaml:"package"`
	Output        string                          `yaml:"output"`
	ContextType   string                          `yaml:"context_type"`
	ClientType    string                          `yaml:"client_type"`
	EvaluatorType string                          `yaml:"evaluator_type"`
	Context       ContextConfig                   `yaml:"context"`
	Accessors     map[string]gogen.AccessorConfig `yaml:"accessors"`
}

type ContextConfig struct {
	Defaults gogen.ContextDefaultsConfig `yaml:"defaults"`
	Fields   []gogen.ContextFieldConfig  `yaml:"fields"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.Version != "v1" {
		return nil, fmt.Errorf("version must be v1")
	}
	if cfg.Source == "" {
		return nil, fmt.Errorf("source is required")
	}
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("targets are required")
	}
	return &cfg, nil
}

func ResolvePath(configPath, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(filepath.Dir(configPath), value)
}
