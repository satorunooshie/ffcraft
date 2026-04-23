package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/satorunooshie/ffcraft/internal/ast"
	"github.com/satorunooshie/ffcraft/internal/codegen"
	"github.com/satorunooshie/ffcraft/internal/gogen"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
	"github.com/satorunooshie/ffcraft/internal/parse"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "ffcodegen: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("target is required")
	}

	switch args[0] {
	case "go":
		return runGo(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown target %q", args[0])
	}
}

func runGo(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("go", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	inPath := fs.String("in", "", "input path")
	configPath := fs.String("config", "", "input ffcodegen YAML path")
	outPath := fs.String("out", "", "output Go path; stdout when omitted or '-'")
	dumpPath := fs.String("dump", "", "when reading authoring YAML, write normalized YAML to this path; use '-' for stderr")
	inputFormat := fs.String("format", "auto", "input format: auto, authoring, normalized")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inPath == "" {
		return errors.New("--in is required")
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}

	target := codegen.Target{
		PackageName: "featureflags",
	}
	if *configPath != "" {
		cfg, err := codegen.Load(*configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		var ok bool
		target, ok = cfg.Targets["go"]
		if !ok {
			return errors.New("targets.go is required")
		}
		if target.PackageName == "" {
			target.PackageName = "featureflags"
		}
	}

	input, err := os.ReadFile(*inPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	doc, wasAuthoring, err := loadInput(input, *inputFormat)
	if err != nil {
		return err
	}

	if wasAuthoring && *dumpPath != "" {
		dump, err := normalizedyaml.Marshal(doc)
		if err != nil {
			return fmt.Errorf("marshal normalized yaml: %w", err)
		}
		dump = append(dump, '\n')
		if *dumpPath == "-" {
			if _, err := stderr.Write(dump); err != nil {
				return err
			}
		} else if err := os.WriteFile(*dumpPath, dump, 0o644); err != nil {
			return fmt.Errorf("write dump: %w", err)
		}
	}

	output, err := gogen.Compile(doc, gogen.Config{
		PackageName:     target.PackageName,
		ContextType:     target.ContextType,
		ClientType:      target.ClientType,
		EvaluatorType:   target.EvaluatorType,
		ContextDefaults: target.Context.Defaults,
		ContextFields:   target.Context.Fields,
		Accessors:       target.Accessors,
	})
	if err != nil {
		return fmt.Errorf("compile go code: %w", err)
	}

	output = append(output, '\n')
	return writeOutput(stdout, *outPath, output)
}

func loadInput(input []byte, formatName string) (*ast.Document, bool, error) {
	switch formatName {
	case "auto":
		doc, err := normalizedyaml.Unmarshal(input)
		if err == nil {
			return doc, false, nil
		}
		return loadAuthoring(input)
	case "authoring":
		return loadAuthoring(input)
	case "normalized":
		doc, err := normalizedyaml.Unmarshal(input)
		if err != nil {
			return nil, false, fmt.Errorf("read normalized yaml: %w", err)
		}
		return doc, false, nil
	default:
		return nil, false, fmt.Errorf("unsupported --format %q", formatName)
	}
}

func loadAuthoring(input []byte) (*ast.Document, bool, error) {
	doc, err := parse.ParseYAML(input)
	if err != nil {
		return nil, false, fmt.Errorf("parse input: %w", err)
	}
	normalizedDoc, err := normalize.Normalize(doc)
	if err != nil {
		return nil, false, fmt.Errorf("normalize input: %w", err)
	}
	return normalizedDoc, true, nil
}

func writeOutput(stdout io.Writer, outPath string, output []byte) error {
	if outPath == "" || outPath == "-" {
		_, err := stdout.Write(output)
		return err
	}
	if err := os.WriteFile(outPath, output, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  ffcodegen go --in ffcompile.yaml [--config ffcodegen.yaml] [--format auto|authoring|normalized] [--out flags.gen.go] [--dump normalized.yaml]")
}
