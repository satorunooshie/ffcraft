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
    weights: {on: 1, off: 1}
flags:
  - key: surface
    variant_set: values
    default_variant: off
    extensions:
      com.example.metadata.v1:
        owner: platform
        description: surface coverage
        expiry: 2030-01-01
        tags: [critical, rollout]
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
	metadata := flag.Extensions["com.example.metadata.v1"].GetObjectValue().Fields
	if metadata["owner"].GetStringValue() != "platform" || len(metadata["tags"].GetListValue().Values) != 2 {
		t.Fatalf("metadata extension was not preserved: %#v", flag.Extensions)
	}
	if _, ok := flag.Environments["fixed"].GetKind().(*ffv1.Environment_FixedServe); !ok {
		t.Fatalf("fixed environment kind = %T", flag.Environments["fixed"].GetKind())
	}
	dynamic := flag.Environments["dynamic"].GetRuleEvaluation()
	if dynamic == nil || len(dynamic.Rules) != 2 || len(dynamic.ScheduledRollouts) != 1 {
		t.Fatalf("dynamic environment was not fully parsed: %#v", dynamic)
	}
	if dynamic.Rules[0].Action.GetDistribute() == nil || dynamic.Rules[1].Action.GetProgressiveRollout() == nil {
		t.Fatalf("action variants were not preserved: %#v", dynamic.Rules)
	}
}

func TestParseYAMLRejectsFractionalDistributionWeights(t *testing.T) {
	const source = `version: v1
variant_sets:
  values: {on: true, off: false}
distributions:
  rollout:
    stickiness: user.id
    weights: {on: 33.3, off: 66.7}
flags:
  - key: fractional
    variant_set: values
    default_variant: off
    environments:
      prod: {serve: off}
`
	if _, err := ParseYAML([]byte(source)); err == nil {
		t.Fatal("fractional distribution weights must be rejected")
	}
}
