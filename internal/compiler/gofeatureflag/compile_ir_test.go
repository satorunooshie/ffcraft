package gofeatureflag

import (
	"strings"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
)

func TestCompileIRDirectSemanticSurface(t *testing.T) {
	doc, err := normalizedyaml.Unmarshal([]byte(`version: normalized/v1
flags:
  direct:
    variants:
      off: {bool_value: false}
      on: {bool_value: true}
    environments:
      prod:
        base:
          rules:
            - condition:
                equality:
                  operator: EQUALITY_OPERATOR_EQ
                  attribute: {segments: [user, segment]}
                  literal: {string_value: beta}
              action: {serve: on}
          default_action: {serve: off}
        schedule:
          - effective_at: "2026-01-01T00:00:00.000000001Z"
            evaluation:
              default_action:
                distribute:
                  allocation_key: {segments: [user, id]}
                  weights: {off: 1, on: 2}
`))
	if err != nil {
		t.Fatal(err)
	}
	output, _, err := CompileIR(doc, "prod", CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"user.segment", "user.id", "scheduledRollout", "percentage", "\"off\": 1", "\"on\": 2"} {
		if !strings.Contains(string(output), fragment) {
			t.Fatalf("GO Feature Flag output missing %q: %s", fragment, output)
		}
	}
}
