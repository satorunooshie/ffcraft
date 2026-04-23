package testhelper

import (
	"io/fs"
	"os"
	"testing"
)

func MustReadFile(tb testing.TB, fsys fs.ReadFileFS, path string) []byte {
	tb.Helper()
	data, err := fsys.ReadFile(path)
	if err != nil {
		tb.Fatalf("read embedded file %s: %v", path, err)
	}
	return data
}

func ShouldUpdateGolden() bool {
	return os.Getenv("UPDATE_GOLDEN") == "1"
}

func WriteGolden(tb testing.TB, path string, data []byte) {
	tb.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		tb.Fatalf("write golden %s: %v", path, err)
	}
}
