package authoring

import (
	"strings"
	"testing"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

func TestParseYAMLCoreScalars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		yaml    string
		want    func(*testing.T, *ffv1.FeatureFlagDocument)
		wantErr string
	}{
		{name: "plain edge cases are strings", yaml: "yes", want: assertExtensionString("yes")},
		{name: "leading zero is string", yaml: "01", want: assertExtensionString("01")},
		{name: "hexadecimal is string", yaml: "0x10", want: assertExtensionString("0x10")},
		{name: "decimal is double", yaml: "1.0", want: assertExtensionDouble(1)},
		{name: "exponent is double", yaml: "1e3", want: assertExtensionDouble(1000)},
		{name: "integer overflow is rejected", yaml: "9223372036854775808", wantErr: "cannot be represented as int64"},
		{name: "nonfinite is rejected", yaml: ".inf", wantErr: "finite JSON number"},
		{name: "custom tags are rejected", yaml: "!custom value", wantErr: "custom YAML tags"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := ParseYAML([]byte("version: v1\nflags: []\nextensions:\n  x: " + test.yaml + "\n"))
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("ParseYAML() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			test.want(t, doc)
		})
	}
}

func assertExtensionString(want string) func(*testing.T, *ffv1.FeatureFlagDocument) {
	return func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
		if got := doc.Extensions["x"].GetStringValue(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func assertExtensionDouble(want float64) func(*testing.T, *ffv1.FeatureFlagDocument) {
	return func(t *testing.T, doc *ffv1.FeatureFlagDocument) {
		if got := doc.Extensions["x"].GetDoubleValue(); got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
