package normalize

import (
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/authoring"
	"github.com/satorunooshie/ffcraft/internal/ir"
)

func TestNormalizeAuthoringSemanticsTable(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		check func(t *testing.T, document *irv1.Document)
	}{
		{name: "all conditions actions and schedule sugar", yaml: comprehensiveAuthoringYAML(), check: func(t *testing.T, document *irv1.Document) {
			t.Helper()
			if len(document.Flags) != 1 {
				t.Fatalf("flag count = %d, want 1", len(document.Flags))
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authoringDocument, err := authoring.ParseYAML([]byte(tt.yaml))
			if err != nil {
				t.Fatal(err)
			}
			document, err := Normalize(authoringDocument)
			if err != nil {
				t.Fatal(err)
			}
			if err := ir.Validate(document); err != nil {
				t.Fatal(err)
			}
			if len(document.Flags) != 1 || len(document.Flags["all-conditions"].Environments["prod"].Schedule) == 0 {
				t.Fatalf("normalized document did not preserve schedule semantics: %s", document)
			}
			tt.check(t, document)
		})
	}
}

func TestNormalizeRejectsUnresolvedReferences(t *testing.T) {
	const source = `version: v1
variant_sets:
  boolean:
    on: true
    off: false
flags:
  - key: unresolved
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        rules:
          - if:
              rule: missing
            serve: on
        default_action:
          serve: off
`
	doc, err := authoring.ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(doc); err == nil || !strings.Contains(err.Error(), "referenced rule") {
		t.Fatalf("Normalize() error = %v, want referenced rule", err)
	}
}

func TestNormalizeASTRetainsMatchesForTargetCapabilityValidation(t *testing.T) {
	const source = `version: v1
variant_sets:
  boolean:
    on: true
    off: false
flags:
  - key: matches
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        rules:
          - if: {matches: [{var: user.id}, '^user-']}
            serve: on
        default_action:
          serve: off
`
	doc, err := authoring.ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeAST(doc); err != nil {
		t.Fatal(err)
	}
}

func comprehensiveAuthoringYAML() string {
	return `version: v1
variant_sets:
  boolean:
    on: true
    off: false
rules:
  named_eq:
    eq: [{var: user.id}, user-1]
flags:
  - key: all-conditions
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        rules:
          - if: {literal_bool: false}
            serve: on
          - if: {ne: [{var: user.id}, user-2]}
            serve: on
          - if: {gt: [{var: score}, 1]}
            serve: on
          - if: {gte: [{var: score}, 1]}
            serve: on
          - if: {lt: [{var: score}, 10]}
            serve: on
          - if: {lte: [{var: score}, 10]}
            serve: on
          - if: {in: [{var: region}, [jp, us]]}
            serve: on
          - if: {contains: [{var: user.tags}, beta]}
            serve: on
          - if: {starts_with: [{var: user.id}, user-]}
            serve: on
          - if: {ends_with: [{var: user.id}, -1]}
            serve: on
          - if: {semver_gt: [{var: version}, 1.0.0]}
            serve: on
          - if: {semver_gte: [{var: version}, 1.0.0]}
            serve: on
          - if: {semver_lt: [{var: version}, 9.0.0]}
            serve: on
          - if: {semver_lte: [{var: version}, 9.0.0]}
            serve: on
          - if:
              all_of:
                - {literal_bool: true}
                - {rule: named_eq}
            serve: on
          - if:
              any_of:
                - {literal_bool: false}
                - {rule: named_eq}
            serve: on
          - if:
              one_of:
                - {literal_bool: true}
                - {literal_bool: false}
            serve: on
          - if:
              not: {literal_bool: false}
            serve: on
        default_action:
          progressive_rollout:
            variant: on
            stickiness: user.id
            start: "2026-01-01T00:00:00Z"
            end: "2026-01-10T00:00:00Z"
            steps: 2
        experimentation:
          start: "2026-02-01T00:00:00Z"
          end: "2026-02-10T00:00:00Z"
`
}
