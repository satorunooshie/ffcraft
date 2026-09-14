package authoring

import (
	"testing"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
)

func TestParseYAMLSurfaceContract(t *testing.T) {
	const source = `version: v1
variant_sets:
  values:
    on: true
    off: false
    object: {nested: [1, 2.5, null]}
distributions:
  half:
    stickiness: user.id
    allocations: {on: 0.5, off: 0.5}
flags:
  - key: surface
    variant_set: values
    default_variant: off
    metadata:
      owner: platform
      description: surface coverage
      expiry: 2030-01-01
      tags: [critical, rollout]
    extensions:
      source: {team: platform, enabled: true, limit: 42}
    environments:
      fixed:
        serve: on
        extensions: {region: us}
      dynamic:
        rules:
          - if: {eq: [{var: user.id}, alice]}
            distribute: half
          - if: {literal_bool: true}
            progressive_rollout:
              variant: on
              stickiness: user.id
              start: 2028-01-01T00:00:00Z
              end: 2028-01-02T00:00:00Z
              steps: 4
        default_action: {serve: off}
        experimentation: {start: 2028-02-01T00:00:00Z, end: 2028-02-02T00:00:00Z}
        scheduled_rollouts:
          - name: snapshot
            description: first
            disabled: true
            date: 2028-03-01T00:00:00Z
            default_action: {serve: on}
            rules:
              - if: {literal_bool: false}
                serve: off
`
	doc, err := ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	flag := doc.Flags[0]
	if flag.Metadata == nil || flag.Metadata.Owner != "platform" || len(flag.Metadata.Tags) != 2 {
		t.Fatalf("metadata was not preserved: %#v", flag.Metadata)
	}
	if _, ok := flag.Environments["fixed"].GetKind().(*ffv1.Environment_FixedServe); !ok {
		t.Fatalf("fixed environment kind = %T", flag.Environments["fixed"].GetKind())
	}
	dynamic := flag.Environments["dynamic"].GetRuleEvaluation()
	if dynamic == nil || len(dynamic.Rules) != 2 || dynamic.Experimentation == nil || len(dynamic.ScheduledRollouts) != 1 {
		t.Fatalf("dynamic environment was not fully parsed: %#v", dynamic)
	}
	if dynamic.Rules[0].Action.GetDistribute() == nil || dynamic.Rules[1].Action.GetProgressiveRollout() == nil {
		t.Fatalf("action variants were not preserved: %#v", dynamic.Rules)
	}
}
