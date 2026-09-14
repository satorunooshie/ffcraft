package ir_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/authoring"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestNormalizeAndProtoRoundTrip(t *testing.T) {
	t.Parallel()
	doc, err := authoring.ParseYAML([]byte(`version: v1
variant_sets:
  boolean:
    on: true
    off: false
flags:
  - key: example
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        default_action:
          serve: on
`))
	if err != nil {
		t.Fatal(err)
	}
	want, err := normalize.Normalize(doc)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := ir.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ir.Unmarshal(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("IR changed across protobuf round trip (-want +got):\n%s", diff)
	}
}

func TestUnknownCoreFieldDiagnostic(t *testing.T) {
	doc := minimalIRForUnknownField()
	unknown := protowire.AppendTag(nil, 99, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	doc.ProtoReflect().SetUnknown(unknown)
	if err := ir.Validate(doc); err == nil {
		t.Fatal("Validate() accepted an unknown core field")
	} else {
		var diagnostic *ir.CoreValidationError
		if !errors.As(err, &diagnostic) || diagnostic.Code != ir.UnknownCoreFieldCode || diagnostic.FieldNumber != 99 || diagnostic.Path != "$" {
			t.Fatalf("unexpected diagnostic: %#v", err)
		}
	}
}

func TestAuthoringExperimentationIsConsumedBeforeIR(t *testing.T) {
	doc, err := authoring.ParseYAML([]byte(`version: v1
variant_sets:
  values:
    on: true
flags:
  - key: experiment
    variant_set: values
    default_variant: on
    environments:
      prod:
        experimentation:
          start: 2026-01-01T00:00:00Z
          end: 2026-01-02T00:00:00Z
        default_action:
          serve: on
`))
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalize.Normalize(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := normalized.Flags["experiment"].Environments["prod"].Base.DefaultAction.GetKind().(*irv1.Action_Serve); !ok {
		t.Fatal("experiment authoring sugar changed the semantic default action")
	}
}

func TestAuthoringToIRLoweringTable(t *testing.T) {
	tests := []struct {
		name   string
		source string
		check  func(t *testing.T, document *irv1.Document)
	}{
		{name: "serve and all scalar condition domains", source: `version: v1
variant_sets:
  values:
    on: true
    off: false
flags:
  - key: scalar
    variant_set: values
    default_variant: off
    environments:
      prod:
        rules:
          - if: {eq: [{var: user.id}, user-1]}
            serve: on
        default_action:
          serve: off
`, check: func(t *testing.T, document *irv1.Document) {
			if _, ok := document.Flags["scalar"].Environments["prod"].Base.Rules[0].Condition.GetKind().(*irv1.Condition_Equality); !ok {
				t.Fatal("expected equality condition")
			}
		}},
		{name: "distribution canonicalization", source: `version: v1
variant_sets:
  values:
    on: true
    off: false
distributions:
  rollout:
    stickiness: user.id
    allocations:
      on: 10
      off: 90
flags:
  - key: distribution
    variant_set: values
    default_variant: off
    environments:
      prod:
        rules:
          - if: {literal_bool: true}
            distribute: rollout
        default_action:
          serve: off
`, check: func(t *testing.T, document *irv1.Document) {
			distribution := document.Flags["distribution"].Environments["prod"].Base.Rules[0].Action.GetDistribute()
			if distribution == nil || distribution.Weights["on"] != 1 || distribution.Weights["off"] != 9 {
				t.Fatalf("unexpected canonical distribution: %v", distribution)
			}
		}},
		{name: "progressive and scheduled snapshots", source: `version: v1
variant_sets:
  values:
    on: true
    off: false
flags:
  - key: rollout
    variant_set: values
    default_variant: off
    environments:
      prod:
        default_action:
          progressive_rollout:
            variant: on
            stickiness: user.id
            start: "2026-01-01T00:00:00Z"
            end: "2026-01-10T00:00:00Z"
            steps: 2
        scheduled_rollouts:
          - date: "2026-02-01T00:00:00Z"
            default_action:
              serve: on
`, check: func(t *testing.T, document *irv1.Document) {
			schedule := document.Flags["rollout"].Environments["prod"].Schedule
			if len(schedule) != 3 {
				t.Fatalf("schedule length = %d, want 3", len(schedule))
			}
			if !schedule[0].EffectiveAt.AsTime().Before(schedule[1].EffectiveAt.AsTime()) || !schedule[1].EffectiveAt.AsTime().Before(schedule[2].EffectiveAt.AsTime()) {
				t.Fatal("schedule is not strictly increasing")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authoringDocument, err := authoring.ParseYAML([]byte(tt.source))
			if err != nil {
				t.Fatal(err)
			}
			document, err := normalize.Normalize(authoringDocument)
			if err != nil {
				t.Fatal(err)
			}
			if err := ir.Validate(document); err != nil {
				t.Fatal(err)
			}
			tt.check(t, document)
		})
	}
}

func minimalIRForUnknownField() *irv1.Document {
	return &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}}}}}}
}

func TestUnknownExtensionFieldsRemainOpaque(t *testing.T) {
	t.Parallel()
	doc := &irv1.Document{
		Flags: map[string]*irv1.Flag{
			"example": {
				Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}},
				Environments: map[string]*irv1.Environment{
					"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}},
				},
			},
		},
		Extensions: map[string]*irv1.ExtensionValue{"vendor": {}},
	}
	doc.Extensions["vendor"].ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	encoded, err := ir.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.Unmarshal(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(doc.Extensions, decoded.Extensions, protocmp.Transform()); diff != "" {
		t.Fatalf("opaque extension changed (-want +got):\n%s", diff)
	}
}
