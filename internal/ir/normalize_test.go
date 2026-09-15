package ir_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ast"
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

func TestProgressiveRolloutSingleStepEndsWithServe(t *testing.T) {
	doc, err := authoring.ParseYAML([]byte(`version: v1
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
            steps: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalize.Normalize(doc)
	if err != nil {
		t.Fatal(err)
	}
	schedule := normalized.Flags["rollout"].Environments["prod"].Schedule
	if len(schedule) != 1 || !schedule[0].EffectiveAt.AsTime().Equal(time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)) || schedule[0].Evaluation.DefaultAction.GetServe() != "on" {
		t.Fatalf("single-step rollout = %#v, want end-time serve(on)", schedule)
	}
}

func TestNormalizePreservesExtensionsAtEveryAuthoringScope(t *testing.T) {
	doc, err := authoring.ParseYAML([]byte(`version: v1
variant_sets:
  values:
    on: true
flags:
  - key: example
    variant_set: values
    default_variant: on
    extensions:
      flag_meta:
        enabled: true
        nested: [1, 2.5, null]
    environments:
      prod:
        default_action: {serve: on}
        extensions:
          environment_meta: {region: jp}
extensions:
  document_meta: {owner: platform}
`))
	if err != nil {
		t.Fatal(err)
	}

	normalized, err := normalize.Normalize(doc)
	if err != nil {
		t.Fatal(err)
	}
	flag := normalized.Flags["example"]
	environment := flag.Environments["prod"]
	if normalized.Extensions["document_meta"].GetObjectValue().Fields["owner"].GetStringValue() != "platform" {
		t.Fatal("document extension was not preserved")
	}
	if !normalized.Flags["example"].Extensions["flag_meta"].GetObjectValue().Fields["enabled"].GetBoolValue() {
		t.Fatal("flag extension was not preserved")
	}
	if environment.Extensions["environment_meta"].GetObjectValue().Fields["region"].GetStringValue() != "jp" {
		t.Fatal("environment extension was not preserved")
	}
	items := flag.Extensions["flag_meta"].GetObjectValue().Fields["nested"].GetListValue().Values
	if len(items) != 3 || items[0].GetIntValue() != 1 || items[1].GetDoubleValue() != 2.5 || items[2].GetNullValue() == nil {
		t.Fatalf("nested extension value was not preserved: %#v", items)
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
            steps: 4
        scheduled_rollouts:
          - date: "2026-02-01T00:00:00Z"
            default_action:
              serve: on
`, check: func(t *testing.T, document *irv1.Document) {
			schedule := document.Flags["rollout"].Environments["prod"].Schedule
			if len(schedule) != 4 {
				t.Fatalf("schedule length = %d, want 4", len(schedule))
			}
			for index := 1; index < len(schedule); index++ {
				if !schedule[index-1].EffectiveAt.AsTime().Before(schedule[index].EffectiveAt.AsTime()) {
					t.Fatal("schedule is not strictly increasing")
				}
			}
			if !schedule[0].EffectiveAt.AsTime().Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("first rollout timestamp = %v, want start", schedule[0].EffectiveAt.AsTime())
			}
			if !schedule[3].EffectiveAt.AsTime().Equal(time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("last rollout timestamp = %v, want end", schedule[3].EffectiveAt.AsTime())
			}
			if got := schedule[3].Evaluation.DefaultAction.GetServe(); got != "on" {
				t.Fatalf("last rollout action = %q, want serve(on)", got)
			}
			wantTimes := []time.Time{
				time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			}
			for index, want := range wantTimes {
				if got := schedule[index].EffectiveAt.AsTime(); !got.Equal(want) {
					t.Fatalf("schedule[%d] timestamp = %v, want %v", index, got, want)
				}
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

func TestNormalizeCanonicalizesMaxUint32DistributionWeights(t *testing.T) {
	document, err := ir.FromAST(&ast.Document{Flags: []*ast.Flag{{
		Key: "max-weight", DefaultVariant: "off",
		Variants: map[string]ast.VariantValue{
			"on":  {Kind: ast.VariantValueKindBool, Bool: true},
			"off": {Kind: ast.VariantValueKindBool},
		},
		Environments: map[string]*ast.Environment{"prod": {
			DefaultAction: &ast.DistributeAction{Stickiness: "user.id", Allocations: map[string]float64{
				"on":  4294967295,
				"off": 4294967295,
			}},
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	weights := document.Flags["max-weight"].Environments["prod"].Base.DefaultAction.GetDistribute().Weights
	if weights["on"] != 1 || weights["off"] != 1 {
		t.Fatalf("canonical max weights = %#v, want 1:1", weights)
	}
}

func TestFromASTRejectsDistributionWeightOverflow(t *testing.T) {
	_, err := ir.FromAST(&ast.Document{Flags: []*ast.Flag{{
		Key: "overflow", DefaultVariant: "off",
		Variants: map[string]ast.VariantValue{
			"on":  {Kind: ast.VariantValueKindBool, Bool: true},
			"off": {Kind: ast.VariantValueKindBool},
		},
		Environments: map[string]*ast.Environment{"prod": {
			DefaultAction: &ast.DistributeAction{Stickiness: "user.id", Allocations: map[string]float64{
				"on":  4294967296,
				"off": 1,
			}},
		}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "normalized uint32 range") {
		t.Fatalf("FromAST() error = %v, want uint32 overflow rejection", err)
	}
}

func TestFromASTRejectsProgressiveRolloutBeyondIRScheduleLimit(t *testing.T) {
	_, err := ir.FromAST(&ast.Document{Flags: []*ast.Flag{{
		Key: "too-many-steps", DefaultVariant: "off",
		Variants: map[string]ast.VariantValue{
			"on":  {Kind: ast.VariantValueKindBool, Bool: true},
			"off": {Kind: ast.VariantValueKindBool},
		},
		Environments: map[string]*ast.Environment{"prod": {
			DefaultAction: &ast.ProgressiveRolloutAction{
				Variant: "on", Stickiness: "user.id", Start: "2026-01-01T00:00:00Z", End: "2026-01-02T00:00:00Z", Steps: 1025,
			},
		}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "exceeds IR schedule limit 1024") {
		t.Fatalf("FromAST() error = %v, want schedule limit rejection", err)
	}
}

func TestFromASTLowersSameVariantProgressiveRolloutToNoOp(t *testing.T) {
	document, err := ir.FromAST(&ast.Document{Flags: []*ast.Flag{{
		Key: "same-variant", DefaultVariant: "on",
		Variants: map[string]ast.VariantValue{
			"on": {Kind: ast.VariantValueKindBool, Bool: true},
		},
		Environments: map[string]*ast.Environment{"prod": {
			DefaultAction: &ast.ProgressiveRolloutAction{
				Variant: "on", Stickiness: "user.id", Start: "2026-01-01T00:00:00Z", End: "2026-01-02T00:00:00Z", Steps: 2,
			},
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	environment := document.Flags["same-variant"].Environments["prod"]
	if len(environment.Schedule) != 0 || environment.Base.DefaultAction.GetServe() != "on" {
		t.Fatalf("same-variant progressive rollout = %s, want no-op base serve on", environment)
	}
}

func TestFromASTRejectsNilContainers(t *testing.T) {
	tests := []struct {
		name string
		doc  *ast.Document
		want string
	}{
		{"nil flag", &ast.Document{Flags: []*ast.Flag{nil}}, "flag[0] is nil"},
		{"nil environment", &ast.Document{Flags: []*ast.Flag{{
			Key: "flag", DefaultVariant: "on",
			Variants:     map[string]ast.VariantValue{"on": {Kind: ast.VariantValueKindBool, Bool: true}},
			Environments: map[string]*ast.Environment{"prod": nil},
		}}}, "environment is nil"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ir.FromAST(test.doc); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("FromAST() error = %v, want %q", err, test.want)
			}
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
