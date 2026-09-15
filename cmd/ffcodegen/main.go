package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/authoring"
	"github.com/satorunooshie/ffcraft/internal/codegen"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
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
	dumpPath := fs.String("dump", "", "write a normalized YAML view to this path; use '-' for stderr")
	inputFormat := fs.String("format", "authoring", "input format: authoring or protobuf")

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

	doc, err := loadInput(input, *inputFormat)
	if err != nil {
		return err
	}

	if *dumpPath != "" {
		dump, err := normalizedyaml.Marshal(doc.IR)
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

	output, err := codegen.CompileIR(doc.IR, codegen.Config{
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

	return writeOutput(stdout, *outPath, output)
}

type loadedDocument struct {
	IR *irv1.Document
}

func loadInput(input []byte, formatName string) (*loadedDocument, error) {
	switch formatName {
	case "authoring":
		return loadAuthoring(input)
	case "protobuf":
		doc, err := ir.Unmarshal(input)
		if err != nil {
			return nil, fmt.Errorf("read normalized protobuf: %w", err)
		}
		return &loadedDocument{IR: doc}, nil
	default:
		return nil, fmt.Errorf("unsupported --format %q", formatName)
	}
}

func loadAuthoring(input []byte) (*loadedDocument, error) {
	doc, err := authoring.ParseYAML(input)
	if err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	normalizedDoc, err := normalize.Normalize(doc)
	if err != nil {
		return nil, fmt.Errorf("normalize input: %w", err)
	}
	return &loadedDocument{IR: normalizedDoc}, nil
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
	fmt.Fprintln(w, "  ffcodegen go --in flags.yaml|featureflags.ir.v1.pb [--config ffcodegen.yaml] [--format authoring|protobuf] [--out flags.gen.go] [--dump normalized.yaml]")
}
