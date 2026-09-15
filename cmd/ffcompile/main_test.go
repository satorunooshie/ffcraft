package main

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/testhelper"
)

//go:embed testdata/*.yaml testdata/*.json testdata/*.txt
var testdataFS embed.FS

func TestRun(t *testing.T) {
	t.Parallel()

	authoringPath := writeFixture(t, "example.yaml")
	normalizedPath := writeNormalizedProtobufFixture(t, authoringPath)
	missingEnvAuthoringPath := writeFixture(t, "missing_env_authoring.yaml")
	missingEnvNormalizedPath := writeNormalizedProtobufFixture(t, missingEnvAuthoringPath)
	tests := []struct {
		name           string
		args           func(outPath string) []string
		wantErr        string
		wantStdoutFile string
		wantStderrFile string
		wantOutputFile string
		wantDumpFile   string
	}{
		{
			name: "normalize to stdout",
			args: func(_ string) []string {
				return []string{"normalize", "--in", authoringPath}
			},
			wantStdoutFile: "normalized.golden.yaml",
		},
		{
			name: "normalize to file",
			args: func(outPath string) []string {
				return []string{"normalize", "--in", authoringPath, "--out", outPath}
			},
			wantOutputFile: "normalized.golden.yaml",
		},
		{
			name: "build flagd to stdout with dump on stderr",
			args: func(_ string) []string {
				return []string{"build", "flagd", "--in", authoringPath, "--env", "prod", "--dump", "-"}
			},
			wantStdoutFile: "flagd.golden.json",
			wantStderrFile: "normalized.golden.yaml",
		},
		{
			name: "build flagd with dump file",
			args: func(outPath string) []string {
				dumpPath := filepath.Join(filepath.Dir(outPath), "normalized.yaml")
				return []string{"build", "flagd", "--in", authoringPath, "--env", "prod", "--out", outPath, "--dump", dumpPath}
			},
			wantOutputFile: "flagd.golden.json",
			wantDumpFile:   "normalized.golden.yaml",
		},
		{
			name: "build gofeatureflag to stdout with dump on stderr",
			args: func(_ string) []string {
				return []string{"build", "gofeatureflag", "--in", authoringPath, "--env", "prod", "--dump", "-"}
			},
			wantStdoutFile: "gofeatureflag.golden.yaml",
			wantStderrFile: "normalized.golden.yaml",
		},
		{
			name: "build gofeatureflag with dump file",
			args: func(outPath string) []string {
				dumpPath := filepath.Join(filepath.Dir(outPath), "normalized.yaml")
				return []string{"build", "gofeatureflag", "--in", authoringPath, "--env", "prod", "--out", outPath, "--dump", dumpPath}
			},
			wantOutputFile: "gofeatureflag.golden.yaml",
			wantDumpFile:   "normalized.golden.yaml",
		},
		{
			name: "compile flagd to stdout",
			args: func(_ string) []string {
				return []string{"compile", "flagd", "--in", normalizedPath, "--env", "prod"}
			},
			wantStdoutFile: "flagd.golden.json",
		},
		{
			name: "compile gofeatureflag to stdout",
			args: func(_ string) []string {
				return []string{"compile", "gofeatureflag", "--in", normalizedPath, "--env", "prod"}
			},
			wantStdoutFile: "gofeatureflag.golden.yaml",
		},
		{
			name: "compile flagd to file",
			args: func(outPath string) []string {
				return []string{"compile", "flagd", "--in", normalizedPath, "--env", "prod", "--out", outPath}
			},
			wantOutputFile: "flagd.golden.json",
		},
		{
			name: "compile gofeatureflag to file",
			args: func(outPath string) []string {
				return []string{"compile", "gofeatureflag", "--in", normalizedPath, "--env", "prod", "--out", outPath}
			},
			wantOutputFile: "gofeatureflag.golden.yaml",
		},
		{
			name: "build flagd allows missing env with warning",
			args: func(_ string) []string {
				return []string{"build", "flagd", "--in", missingEnvAuthoringPath, "--env", "prod", "--allow-missing-env"}
			},
			wantStdoutFile: "missing_env_flagd.golden.json",
			wantStderrFile: "missing_env.warning.txt",
		},
		{
			name: "compile flagd allows missing env with warning",
			args: func(_ string) []string {
				return []string{"compile", "flagd", "--in", missingEnvNormalizedPath, "--env", "prod", "--allow-missing-env"}
			},
			wantStdoutFile: "missing_env_flagd.golden.json",
			wantStderrFile: "missing_env.warning.txt",
		},
		{
			name: "build gofeatureflag allows missing env with warning",
			args: func(_ string) []string {
				return []string{"build", "gofeatureflag", "--in", missingEnvAuthoringPath, "--env", "prod", "--allow-missing-env"}
			},
			wantStdoutFile: "missing_env_gofeatureflag.golden.yaml",
			wantStderrFile: "missing_env.warning.txt",
		},
		{
			name: "compile gofeatureflag allows missing env with warning",
			args: func(_ string) []string {
				return []string{"compile", "gofeatureflag", "--in", missingEnvNormalizedPath, "--env", "prod", "--allow-missing-env"}
			},
			wantStdoutFile: "missing_env_gofeatureflag.golden.yaml",
			wantStderrFile: "missing_env.warning.txt",
		},
		{
			name: "build flagd errors on missing env by default",
			args: func(_ string) []string {
				return []string{"build", "flagd", "--in", missingEnvAuthoringPath, "--env", "prod"}
			},
			wantErr: `environment "prod" not found`,
		},
		{
			name: "compile flagd errors on missing env by default",
			args: func(_ string) []string {
				return []string{"compile", "flagd", "--in", missingEnvNormalizedPath, "--env", "prod"}
			},
			wantErr: `environment "prod" not found`,
		},
		{
			name: "build gofeatureflag errors on missing env by default",
			args: func(_ string) []string {
				return []string{"build", "gofeatureflag", "--in", missingEnvAuthoringPath, "--env", "prod"}
			},
			wantErr: `environment "prod" not found`,
		},
		{
			name: "compile gofeatureflag errors on missing env by default",
			args: func(_ string) []string {
				return []string{"compile", "gofeatureflag", "--in", missingEnvNormalizedPath, "--env", "prod"}
			},
			wantErr: `environment "prod" not found`,
		},
		{
			name:    "normalize missing required flags",
			args:    func(_ string) []string { return []string{"normalize"} },
			wantErr: "--in is required",
		},
		{
			name:    "build missing target",
			args:    func(_ string) []string { return []string{"build"} },
			wantErr: "build target is required",
		},
		{
			name:    "build unknown target",
			args:    func(_ string) []string { return []string{"build", "unknown"} },
			wantErr: `unknown build target "unknown"`,
		},
		{
			name:    "compile missing target",
			args:    func(_ string) []string { return []string{"compile"} },
			wantErr: "compile target is required",
		},
		{
			name:    "compile unknown target",
			args:    func(_ string) []string { return []string{"compile", "unknown"} },
			wantErr: `unknown compile target "unknown"`,
		},
		{
			name:    "unknown subcommand",
			args:    func(_ string) []string { return []string{"unknown"} },
			wantErr: `unknown subcommand "unknown"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outPath := filepath.Join(t.TempDir(), "output.txt")
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := run(tt.args(outPath), &stdout, &stderr)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			assertBufferMatchesFixture(t, stdout.Bytes(), tt.wantStdoutFile, "stdout")
			assertBufferMatchesFixture(t, stderr.Bytes(), tt.wantStderrFile, "stderr")
			assertOutputFileMatchesFixture(t, outPath, tt.wantOutputFile)
			assertDumpFileMatchesFixture(t, filepath.Join(filepath.Dir(outPath), "normalized.yaml"), tt.wantDumpFile)
		})
	}
}

func TestPublicProtobufNormalizeAndCompilePipeline(t *testing.T) {
	authoringPath := writeFixture(t, "example.yaml")
	var protobuf bytes.Buffer
	var diagnostics bytes.Buffer
	if err := run([]string{"normalize", authoringPath, "--format", "protobuf"}, &protobuf, &diagnostics); err != nil {
		t.Fatal(err)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("normalize protobuf wrote diagnostics to stderr: %q", diagnostics.String())
	}
	doc, err := ir.Unmarshal(protobuf.Bytes())
	if err != nil {
		t.Fatalf("normalize protobuf output is not valid IR: %v", err)
	}
	if len(doc.Flags) == 0 {
		t.Fatal("normalize protobuf output contains no flags")
	}

	protobufPath := filepath.Join(t.TempDir(), "featureflags.ir.v1.pb")
	if err := os.WriteFile(protobufPath, protobuf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var protobufOutput bytes.Buffer
	if err := run([]string{"compile", "flagd", "--in", protobufPath, "--env", "prod"}, &protobufOutput, &diagnostics); err != nil {
		t.Fatalf("compile protobuf = %v", err)
	}
	var yamlOutput bytes.Buffer
	if err := run([]string{"build", "flagd", "--in", authoringPath, "--env", "prod"}, &yamlOutput, &diagnostics); err != nil {
		t.Fatalf("compile authoring = %v", err)
	}
	if !bytes.Equal(protobufOutput.Bytes(), yamlOutput.Bytes()) {
		t.Fatalf("protobuf compile differs from native pipeline:\nprotobuf=%s\nnative=%s", protobufOutput.Bytes(), yamlOutput.Bytes())
	}
}

func TestNormalizeProtobufFailureDoesNotEmitPartialIR(t *testing.T) {
	invalidPath := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte("flags: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"normalize", "--in", invalidPath, "--format", "protobuf"}, &stdout, &stderr)
	if err == nil || stdout.Len() != 0 {
		t.Fatalf("normalize failure = %v, stdout bytes = %d", err, stdout.Len())
	}
	if err := run([]string{"normalize", "--in", invalidPath, "--format", "unsupported"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "unsupported normalize format") {
		t.Fatalf("unsupported format error = %v", err)
	}
}

func TestNormalizeRejectsUnexpectedPositionalArguments(t *testing.T) {
	authoringPath := writeFixture(t, "example.yaml")

	for _, args := range [][]string{
		{"normalize", authoringPath, "extra"},
		{"normalize", "--in", authoringPath, "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "unexpected positional arguments") {
			t.Fatalf("run(%v) error = %v, want unexpected positional arguments", args, err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("run(%v) wrote %d bytes after argument error", args, stdout.Len())
		}
	}
}

func TestCompileRejectsNormalizedYAMLInput(t *testing.T) {
	normalizedPath := writeFixture(t, "normalized.golden.yaml")
	var stdout, stderr bytes.Buffer
	err := run([]string{"compile", "flagd", "--in", normalizedPath, "--env", "prod"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "read normalized protobuf") {
		t.Fatalf("run() error = %v, want normalized protobuf input error", err)
	}
}

func TestInputLoaderBoundaryContracts(t *testing.T) {
	authoringPath := filepath.Join(t.TempDir(), "authoring.yaml")
	if err := os.WriteFile(authoringPath, []byte("version: v1\nvariant_sets: {}\nflags: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	normalizedPath := filepath.Join(t.TempDir(), "normalized.yaml")
	normalized, err := testdataFS.ReadFile("testdata/normalized.golden.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(normalizedPath, normalized, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		load func(string) (*irv1.Document, error)
		path string
		want string
	}{
		{"authoring missing file", loadAuthoring, filepath.Join(t.TempDir(), "missing.yaml"), "read input"},
		{"normalized protobuf missing file", loadProtobuf, filepath.Join(t.TempDir(), "missing.pb"), "read input"},
		{"authoring invalid document", loadAuthoring, normalizedPath, "parse input"},
		{"normalized protobuf invalid document", loadProtobuf, authoringPath, "read normalized protobuf"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.load(test.path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loader error = %v, want %q", err, test.want)
			}
		})
	}
}

func writeFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := testdataFS.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func writeNormalizedProtobufFixture(t *testing.T, authoringPath string) string {
	t.Helper()
	doc, err := loadAuthoring(authoringPath)
	if err != nil {
		t.Fatalf("load authoring fixture: %v", err)
	}
	data, err := ir.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal protobuf fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "featureflags.ir.v1.pb")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write protobuf fixture: %v", err)
	}
	return path
}

func writeFixtureToDir(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := testdataFS.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func assertBufferMatchesFixture(t *testing.T, got []byte, fixture, stream string) {
	t.Helper()
	if fixture == "" {
		if len(got) != 0 {
			t.Fatalf("unexpected %s:\n%s", stream, got)
		}
		return
	}
	want := testhelper.MustReadFile(t, testdataFS, "testdata/"+fixture)
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch (-want +got)\n%s", stream, cmp.Diff(string(bytes.TrimSpace(want)), string(bytes.TrimSpace(got))))
	}
}

func assertOutputFileMatchesFixture(t *testing.T, path, fixture string) {
	t.Helper()
	if fixture == "" {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("unexpected output file %s", path)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output %s: %v", path, err)
	}
	want := testhelper.MustReadFile(t, testdataFS, "testdata/"+fixture)
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("output mismatch (-want +got)\n%s", cmp.Diff(string(bytes.TrimSpace(want)), string(bytes.TrimSpace(got))))
	}
}

func assertDumpFileMatchesFixture(t *testing.T, path, fixture string) {
	t.Helper()
	if fixture == "" {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("unexpected dump file %s", path)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dump %s: %v", path, err)
	}
	want := testhelper.MustReadFile(t, testdataFS, "testdata/"+fixture)
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("dump mismatch (-want +got)\n%s", cmp.Diff(string(bytes.TrimSpace(want)), string(bytes.TrimSpace(got))))
	}
}
