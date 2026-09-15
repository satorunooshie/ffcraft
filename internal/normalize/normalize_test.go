package normalize

import (
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ast"
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

func TestNormalizeScheduleSnapshotsPreserveReplacementSemantics(t *testing.T) {
	const source = `version: v1
variant_sets:
  boolean:
    on: true
    off: false
flags:
  - key: scheduled
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        default_action:
          serve: off
        scheduled_rollouts:
          - date: "2026-01-01T00:00:00Z"
            default_action:
              serve: off
          - date: "2026-02-01T00:00:00Z"
            default_action:
              serve: on
          - date: "2026-03-01T00:00:00Z"
            rules:
              - if: {eq: [{var: user.country}, JP]}
                serve: off
            default_action:
              serve: on
          - date: "2026-04-01T00:00:00Z"
            default_action:
              serve: on
          - date: "2026-05-01T00:00:00Z"
            disabled: true
            default_action:
              serve: off
`
	authoringDocument, err := authoring.ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	document, err := Normalize(authoringDocument)
	if err != nil {
		t.Fatal(err)
	}
	environment := document.Flags["scheduled"].Environments["prod"]
	if len(environment.Schedule) != 3 {
		t.Fatalf("normalized schedule length = %d, want 3", len(environment.Schedule))
	}
	if got := environment.Schedule[0].EffectiveAt.AsTime().Format("2006-01-02"); got != "2026-02-01" {
		t.Fatalf("first effective date = %q, want 2026-02-01", got)
	}
	if got := environment.Schedule[0].Evaluation.DefaultAction.GetServe(); got != "on" {
		t.Fatalf("first snapshot default = %q, want on", got)
	}
	if len(environment.Schedule[1].Evaluation.Rules) != 1 || environment.Schedule[1].Evaluation.DefaultAction.GetServe() != "on" {
		t.Fatalf("rule snapshot = %s, want inherited on fallback and one rule", environment.Schedule[1])
	}
	if len(environment.Schedule[2].Evaluation.Rules) != 0 || environment.Schedule[2].Evaluation.DefaultAction.GetServe() != "on" {
		t.Fatalf("complete replacement snapshot = %s, want serve on", environment.Schedule[2])
	}
}

func TestNormalizeRejectsDuplicateScheduleTimestampEvenForRedundantSnapshot(t *testing.T) {
	const source = `version: v1
variant_sets:
  boolean:
    on: true
    off: false
flags:
  - key: duplicate-time
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        default_action: {serve: off}
        scheduled_rollouts:
          - date: "2026-01-01T00:00:00Z"
            default_action: {serve: on}
          - date: "2026-01-01T00:00:00Z"
            default_action: {serve: on}
`
	authoringDocument, err := authoring.ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(authoringDocument); err == nil || !strings.Contains(err.Error(), "dates must be unique") {
		t.Fatalf("Normalize() error = %v, want duplicate timestamp rejection", err)
	}
}

func TestNormalizeRejectsInvalidDistributionTable(t *testing.T) {
	tests := []struct {
		name        string
		variants    string
		allocations string
		want        string
	}{
		{"floating point weight", "on: true\n    off: false", "on: 50.5\n      off: 49.5", "positive integer"},
		{"zero weight", "on: true\n    off: false", "on: 0\n      off: 100", "positive integer"},
		{"one entry", "on: true", "on: 100", "map must be at least 2 entries"},
		{"unknown variant", "on: true\n    off: false", "on: 50\n      off: 25\n      missing: 25", "unknown variant"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := "version: v1\nvariant_sets:\n  values:\n    " + test.variants + "\ndistributions:\n  rollout:\n    stickiness: user.id\n    allocations:\n      " + test.allocations + "\nflags:\n  - key: invalid\n    variant_set: values\n    default_variant: on\n    environments:\n      prod:\n        default_action:\n          distribute: rollout\n"
			doc, err := authoring.ParseYAML([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Normalize(doc); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Normalize() error = %v, want %q", err, test.want)
			}
		})
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

func TestNormalizeASTPreservesNestedVariantsAndExtensions(t *testing.T) {
	const source = `version: v1
variant_sets:
  objects:
    a:
      enabled: true
      nested: [7, null]
    b:
      enabled: false
      nested: [8, null]
flags:
  - key: config
    variant_set: objects
    default_variant: a
    extensions:
      com.example.metadata.v1:
        owner: platform
        description: nested config
        expiry: "2027-01-01"
        tags: [config, nested]
    environments:
      prod:
        default_action:
          serve: a
`
	doc, err := authoring.ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := NormalizeAST(doc)
	if err != nil {
		t.Fatal(err)
	}
	flag := normalized.Flags[0]
	value := flag.Variants["a"]
	if value.Kind != ast.VariantValueKindObject || value.Object["enabled"] != true {
		t.Fatalf("object variant = %#v", value)
	}
	nested, ok := value.Object["nested"].([]any)
	if !ok || len(nested) != 2 || nested[0] != int64(7) || nested[1] != nil {
		t.Fatalf("nested variant values = %#v", value.Object["nested"])
	}
	metadata := flag.Extensions["com.example.metadata.v1"].GetObjectValue().Fields
	if metadata["owner"].GetStringValue() != "platform" || metadata["description"].GetStringValue() != "nested config" || len(metadata["tags"].GetListValue().Values) != 2 {
		t.Fatalf("metadata extension = %#v", flag.Extensions)
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
