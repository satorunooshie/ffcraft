package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/authoring"
	"github.com/satorunooshie/ffcraft/internal/compiler/flagd"
	"github.com/satorunooshie/ffcraft/internal/compiler/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "ffcompile: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("subcommand is required")
	}

	switch args[0] {
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "normalize":
		return runNormalize(args[1:], stdout)
	case "compile":
		return runCompile(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

type compileCommandOptions struct {
	inPath          string
	environment     string
	outPath         string
	dumpPath        string
	allowMissingEnv bool
}

func parseCompileOptions(name string, args []string, withDump bool) (compileCommandOptions, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	inPath := fs.String("in", "", "normalized protobuf input path")
	inputAlias := fs.String("input", "", "input path")
	env := fs.String("env", "", "environment name")
	outPath := fs.String("out", "", "output path; stdout when omitted or '-' else")
	allowMissingEnv := fs.Bool("allow-missing-env", false, "skip flags that do not define the requested environment and emit warnings")
	dumpPath := new(string)
	if withDump {
		dumpPath = fs.String("dump", "", "write normalized YAML to this path; use '-' for stderr")
	}

	if err := fs.Parse(args); err != nil {
		return compileCommandOptions{}, err
	}
	if *inputAlias != "" {
		*inPath = *inputAlias
	}
	if *inPath == "" {
		return compileCommandOptions{}, errors.New("--in is required")
	}
	if *env == "" {
		return compileCommandOptions{}, errors.New("--env is required")
	}
	if fs.NArg() != 0 {
		return compileCommandOptions{}, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	return compileCommandOptions{
		inPath:          *inPath,
		environment:     *env,
		outPath:         *outPath,
		dumpPath:        *dumpPath,
		allowMissingEnv: *allowMissingEnv,
	}, nil
}

func runNormalize(args []string, stdout io.Writer) error {
	positionalInput := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalInput, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("normalize", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	inPath := fs.String("in", "", "input YAML path")
	format := fs.String("format", "yaml", "output format: yaml or protobuf")
	inputAlias := fs.String("input", "", "input YAML path")
	outPath := fs.String("out", "", "output YAML path; stdout when omitted or '-'")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inputAlias != "" {
		*inPath = *inputAlias
	}
	if *inPath == "" {
		*inPath = positionalInput
	}
	if *inPath == "" && fs.NArg() == 1 {
		*inPath = fs.Arg(0)
	}
	if *inPath == "" {
		return errors.New("--in is required")
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if *format != "yaml" && *format != "protobuf" {
		return fmt.Errorf("unsupported normalize format %q", *format)
	}

	normalizedDoc, err := loadAuthoring(*inPath)
	if err != nil {
		return fmt.Errorf("normalize input: %w", err)
	}

	var output []byte
	if *format == "protobuf" {
		output, err = ir.Marshal(normalizedDoc)
		if err != nil {
			return fmt.Errorf("marshal normalized protobuf: %w", err)
		}
	} else {
		output, err = normalizedyaml.Marshal(normalizedDoc)
		if err != nil {
			return fmt.Errorf("marshal normalized yaml: %w", err)
		}
		output = append(output, '\n')
	}

	if *outPath == "" || *outPath == "-" {
		_, err = stdout.Write(output)
		return err
	}
	if err := os.WriteFile(*outPath, output, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func runBuild(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("build target is required")
	}

	switch args[0] {
	case "flagd":
		return runBuildFlagd(args[1:], stdout, stderr)
	case "gofeatureflag":
		return runBuildGOFeatureFlag(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown build target %q", args[0])
	}
}

func runCompile(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("compile target is required")
	}

	switch args[0] {
	case "flagd":
		return runCompileFlagd(args[1:], stdout, stderr)
	case "gofeatureflag":
		return runCompileGOFeatureFlag(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown compile target %q", args[0])
	}
}

func runBuildFlagd(args []string, stdout, stderr io.Writer) error {
	opts, err := parseCompileOptions("build flagd", args, true)
	if err != nil {
		return err
	}

	normalizedDoc, err := loadAuthoring(opts.inPath)
	if err != nil {
		return fmt.Errorf("normalize input: %w", err)
	}

	if err := writeNormalizedDump(stderr, opts.dumpPath, normalizedDoc); err != nil {
		return err
	}

	output, warnings, err := flagd.CompileIR(normalizedDoc, opts.environment, flagd.CompileOptions{
		AllowMissingEnvironment: opts.allowMissingEnv,
	})
	if err != nil {
		return fmt.Errorf("compile flagd json: %w", err)
	}
	if err := writeWarnings(stderr, warnings); err != nil {
		return err
	}
	output = append(output, '\n')
	return writeOutput(stdout, opts.outPath, output)
}

func runCompileFlagd(args []string, stdout, stderr io.Writer) error {
	opts, err := parseCompileOptions("compile flagd", args, false)
	if err != nil {
		return err
	}

	doc, err := loadProtobuf(opts.inPath)
	if err != nil {
		return err
	}

	output, warnings, err := flagd.CompileIR(doc, opts.environment, flagd.CompileOptions{
		AllowMissingEnvironment: opts.allowMissingEnv,
	})
	if err != nil {
		return fmt.Errorf("compile flagd json: %w", err)
	}
	if err := writeWarnings(stderr, warnings); err != nil {
		return err
	}
	output = append(output, '\n')
	return writeOutput(stdout, opts.outPath, output)
}

func runBuildGOFeatureFlag(args []string, stdout, stderr io.Writer) error {
	opts, err := parseCompileOptions("build gofeatureflag", args, true)
	if err != nil {
		return err
	}

	normalizedDoc, err := loadAuthoring(opts.inPath)
	if err != nil {
		return fmt.Errorf("normalize input: %w", err)
	}

	if err := writeNormalizedDump(stderr, opts.dumpPath, normalizedDoc); err != nil {
		return err
	}

	output, warnings, err := gofeatureflag.CompileIR(normalizedDoc, opts.environment, gofeatureflag.CompileOptions{
		AllowMissingEnvironment: opts.allowMissingEnv,
	})
	if err != nil {
		return fmt.Errorf("compile gofeatureflag yaml: %w", err)
	}
	if err := writeWarnings(stderr, warnings); err != nil {
		return err
	}
	output = append(output, '\n')
	return writeOutput(stdout, opts.outPath, output)
}

func runCompileGOFeatureFlag(args []string, stdout, stderr io.Writer) error {
	opts, err := parseCompileOptions("compile gofeatureflag", args, false)
	if err != nil {
		return err
	}
	doc, err := loadProtobuf(opts.inPath)
	if err != nil {
		return err
	}

	output, warnings, err := gofeatureflag.CompileIR(doc, opts.environment, gofeatureflag.CompileOptions{
		AllowMissingEnvironment: opts.allowMissingEnv,
	})
	if err != nil {
		return fmt.Errorf("compile gofeatureflag yaml: %w", err)
	}
	if err := writeWarnings(stderr, warnings); err != nil {
		return err
	}
	output = append(output, '\n')
	return writeOutput(stdout, opts.outPath, output)
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

func loadAuthoring(path string) (*irv1.Document, error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	doc, err := authoring.ParseYAML(input)
	if err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	normalized, err := normalize.Normalize(doc)
	if err != nil {
		return nil, fmt.Errorf("normalize input: %w", err)
	}
	return normalized, nil
}

func loadProtobuf(path string) (*irv1.Document, error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	doc, err := ir.Unmarshal(input)
	if err != nil {
		return nil, fmt.Errorf("read normalized protobuf: %w", err)
	}
	return doc, nil
}

func writeNormalizedDump(stderr io.Writer, path string, doc *irv1.Document) error {
	if path == "" {
		return nil
	}
	dump, err := normalizedyaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal normalized yaml: %w", err)
	}
	dump = append(dump, '\n')
	if path == "-" {
		_, err = stderr.Write(dump)
		return err
	}
	if err := os.WriteFile(path, dump, 0o644); err != nil {
		return fmt.Errorf("write dump: %w", err)
	}
	return nil
}

func writeWarnings(w io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintln(w, warning); err != nil {
			return err
		}
	}
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  ffcompile build flagd --in flags.yaml --env prod [--out flagd.json] [--dump normalized.yaml]")
	fmt.Fprintln(w, "  ffcompile build gofeatureflag --in flags.yaml --env prod [--out flags.goff.yaml] [--dump normalized.yaml]")
	fmt.Fprintln(w, "  ffcompile normalize flags.yaml [--format yaml|protobuf] [--out normalized.yaml]")
	fmt.Fprintln(w, "  ffcompile compile flagd --in featureflags.ir.v1.pb --env prod [--out flagd.json]")
	fmt.Fprintln(w, "  ffcompile compile gofeatureflag --in featureflags.ir.v1.pb --env prod [--out flags.goff.yaml]")
}
