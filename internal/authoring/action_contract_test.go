package authoring

import (
	"strings"
	"testing"
)

func TestParseActionOneofContracts(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
		err    string
	}{
		{"serve", "{serve: on}", "serve", ""},
		{"distribute", "{distribute: rollout}", "distribute", ""},
		{"progressive", "{progressive_rollout: {variant: on, stickiness: user.id, start: '2026-01-01T00:00:00Z', end: '2026-01-02T00:00:00Z', steps: 2}}", "progressive", ""},
		{"missing", "{}", "", "one of serve"},
		{"multiple", "{serve: on, distribute: rollout}", "", "exactly one"},
		{"invalid serve type", "{serve: [on]}", "", "expected string scalar"},
		{"invalid progressive steps", "{progressive_rollout: {variant: on, steps: 1.5}}", "", "expected integer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, err := parseActionNode(yamlNode(t, test.source), "$.action")
			if test.err != "" {
				if err == nil || !strings.Contains(err.Error(), test.err) {
					t.Fatalf("parseActionNode() error = %v, want %q", err, test.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch test.want {
			case "serve":
				if action.GetServe().GetVariant() != "on" {
					t.Fatalf("serve action = %v", action)
				}
			case "distribute":
				if action.GetDistribute().GetDistribution() != "rollout" {
					t.Fatalf("distribute action = %v", action)
				}
			case "progressive":
				rollout := action.GetProgressiveRollout()
				if rollout.GetVariant() != "on" || rollout.GetSteps() != 2 {
					t.Fatalf("progressive action = %v", action)
				}
			}
		})
	}
	if _, err := parseActionNode(yamlNode(t, "{serve: on, unknown: x}"), "$.action"); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown action field error = %v", err)
	}
}
