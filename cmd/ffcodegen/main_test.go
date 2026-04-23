package main

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/satorunooshie/ffcraft/internal/testhelper"
)

//go:embed testdata/*.yaml testdata/*.go
var testdataFS embed.FS

func TestRun(t *testing.T) {
	t.Parallel()

	codegenDir := t.TempDir()
	authoringPath := writeFixtureToDir(t, codegenDir, "example.yaml")
	normalizedPath := writeFixtureToDir(t, codegenDir, "normalized.golden.yaml")
	configPath := writeFixtureToDir(t, codegenDir, "ffcodegen.yaml")
	badConfigPath := writeFixtureToDir(t, codegenDir, "ffcodegen_missing_target.yaml")

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
			name: "generate from authoring to stdout with dump",
			args: func(_ string) []string {
				return []string{"go", "--in", authoringPath, "--config", configPath, "--dump", "-"}
			},
			wantStdoutFile: "go.golden.go",
			wantStderrFile: "normalized.golden.yaml",
		},
		{
			name: "generate from authoring to file",
			args: func(outPath string) []string {
				return []string{"go", "--in", authoringPath, "--config", configPath, "--out", outPath}
			},
			wantOutputFile: "go.golden.go",
		},
		{
			name: "generate from authoring without config",
			args: func(_ string) []string {
				return []string{"go", "--in", authoringPath}
			},
			wantStdoutFile: "go.default.golden.go",
		},
		{
			name: "generate from normalized auto-detect",
			args: func(_ string) []string {
				return []string{"go", "--in", normalizedPath, "--config", configPath}
			},
			wantStdoutFile: "go.golden.go",
		},
		{
			name: "generate from normalized explicit format",
			args: func(_ string) []string {
				return []string{"go", "--in", normalizedPath, "--config", configPath, "--format", "normalized"}
			},
			wantStdoutFile: "go.golden.go",
		},
		{
			name: "errors on missing required in",
			args: func(_ string) []string {
				return []string{"go"}
			},
			wantErr: "--in is required",
		},
		{
			name: "errors on unsupported format",
			args: func(_ string) []string {
				return []string{"go", "--in", authoringPath, "--format", "unknown"}
			},
			wantErr: `unsupported --format "unknown"`,
		},
		{
			name: "errors on missing go target in config",
			args: func(_ string) []string {
				return []string{"go", "--in", authoringPath, "--config", badConfigPath}
			},
			wantErr: "targets.go is required",
		},
		{
			name: "errors on unknown target",
			args: func(_ string) []string {
				return []string{"unknown"}
			},
			wantErr: `unknown target "unknown"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outPath := filepath.Join(t.TempDir(), "output.go")
			dumpPath := filepath.Join(filepath.Dir(outPath), "normalized.yaml")
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
			assertDumpFileMatchesFixture(t, dumpPath, tt.wantDumpFile)
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
	if testhelper.ShouldUpdateGolden() {
		testhelper.WriteGolden(t, filepath.Join("testdata", fixture), got)
		return
	}
	want := testhelper.MustReadFile(t, testdataFS, filepath.Join("testdata", fixture))
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
		t.Fatalf("read output file: %v", err)
	}
	if testhelper.ShouldUpdateGolden() {
		testhelper.WriteGolden(t, filepath.Join("testdata", fixture), got)
		return
	}
	want := testhelper.MustReadFile(t, testdataFS, filepath.Join("testdata", fixture))
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
		t.Fatalf("read dump file: %v", err)
	}
	want := testhelper.MustReadFile(t, testdataFS, filepath.Join("testdata", fixture))
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("dump mismatch (-want +got)\n%s", cmp.Diff(string(bytes.TrimSpace(want)), string(bytes.TrimSpace(got))))
	}
}
