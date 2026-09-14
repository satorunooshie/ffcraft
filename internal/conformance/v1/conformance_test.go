package v1_test

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"testing"

	"github.com/satorunooshie/ffcraft/internal/capability"
	"github.com/satorunooshie/ffcraft/internal/compiler/flagd"
	"github.com/satorunooshie/ffcraft/internal/compiler/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

//go:embed testdata/*.yaml
var fixtures embed.FS

func TestV1NormalizedFixture(t *testing.T) {
	fixtureNames, err := fs.Glob(fixtures, "testdata/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtureNames {
		t.Run(fixture, func(t *testing.T) {
			data, err := fixtures.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := normalizedyaml.Unmarshal(data)
			if err != nil {
				t.Fatal(err)
			}
			if err := ir.Validate(doc); err != nil {
				t.Fatal(err)
			}
			encoded, err := normalizedyaml.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			roundTrip, err := normalizedyaml.Unmarshal(encoded)
			if err != nil || !proto.Equal(doc, roundTrip) {
				t.Fatalf("normalized YAML round trip mismatch: %v", err)
			}
		})
	}
}

func TestV1CompilerOutputIgnoresExtensions(t *testing.T) {
	fixtureNames, err := fs.Glob(fixtures, "testdata/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtureNames {
		t.Run(fixture, func(t *testing.T) {
			data, err := fixtures.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			withExtensions, err := normalizedyaml.Unmarshal(data)
			if err != nil {
				t.Fatal(err)
			}
			withoutExtensions := proto.Clone(withExtensions).(*irv1.Document)
			withoutExtensions.Extensions = nil

			flagdWith, _, err := flagd.CompileIR(withExtensions, "prod", flagd.CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			flagdWithout, _, err := flagd.CompileIR(withoutExtensions, "prod", flagd.CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(flagdWith, flagdWithout) {
				t.Fatal("flagd output changed after stripping extensions")
			}

			goffWith, _, err := gofeatureflag.CompileIR(withExtensions, "prod", gofeatureflag.CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			goffWithout, _, err := gofeatureflag.CompileIR(withoutExtensions, "prod", gofeatureflag.CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(goffWith, goffWithout) {
				t.Fatal("GO Feature Flag output changed after stripping extensions")
			}
		})
	}
}

func TestV1UnknownCoreFieldFailsCompilation(t *testing.T) {
	data, err := fixtures.ReadFile("testdata/core_conditions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := normalizedyaml.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	unknown := protowire.AppendTag(nil, 99, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	doc.ProtoReflect().SetUnknown(unknown)

	for _, compile := range []struct {
		name string
		call func() error
	}{
		{name: "flagd", call: func() error {
			_, _, err := flagd.CompileIR(doc, "prod", flagd.CompileOptions{})
			return err
		}},
		{name: "gofeatureflag", call: func() error {
			_, _, err := gofeatureflag.CompileIR(doc, "prod", gofeatureflag.CompileOptions{})
			return err
		}},
	} {
		t.Run(compile.name, func(t *testing.T) {
			err := compile.call()
			var diagnostic *ir.CoreValidationError
			if !errors.As(err, &diagnostic) {
				t.Fatalf("error = %v, want CoreValidationError", err)
			}
			if diagnostic.Code != ir.UnknownCoreFieldCode || diagnostic.Path != "$" || diagnostic.FieldNumber != 99 {
				t.Fatalf("diagnostic = %+v, want code=%s path=$ field=99", diagnostic, ir.UnknownCoreFieldCode)
			}
		})
	}
}

func TestV1TargetCompilersFailClosedForUnrepresentablePresence(t *testing.T) {
	data, err := fixtures.ReadFile("testdata/core_conditions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := normalizedyaml.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	flag := doc.Flags["conditions"]
	flag.Environments["prod"].Base.Rules[0].Condition = &irv1.Condition{
		Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{
			Attribute: &irv1.AttributePath{Segments: []string{"user", "id"}},
		}},
	}
	if err := ir.Validate(doc); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		name string
		call func(*irv1.Document) error
	}{
		{
			name: "flagd",
			call: func(doc *irv1.Document) error {
				_, _, err := flagd.CompileIR(doc, "prod", flagd.CompileOptions{})
				return err
			},
		},
		{
			name: "gofeatureflag",
			call: func(doc *irv1.Document) error {
				_, _, err := gofeatureflag.CompileIR(doc, "prod", gofeatureflag.CompileOptions{})
				return err
			},
		},
	} {
		t.Run(target.name, func(t *testing.T) {
			err := target.call(doc)
			var capabilityError *capability.UnsupportedConditionError
			if !errors.As(err, &capabilityError) {
				t.Fatalf("error = %v, want UnsupportedConditionError", err)
			}
			if capabilityError.Code() != capability.UnsupportedConditionCode {
				t.Fatalf("diagnostic code = %q, want %q", capabilityError.Code(), capability.UnsupportedConditionCode)
			}
			if err == nil {
				t.Fatal("expected unsupported presence condition to fail closed")
			}
		})
	}
}

func TestV1ConditionCapabilityMatrixIsConnectedToCompilers(t *testing.T) {
	for _, entry := range capability.ConditionCapabilityMatrix() {
		entry := entry
		t.Run(string(entry.Target)+"/"+string(entry.Condition), func(t *testing.T) {
			doc := conditionCapabilityFixture(entry.Condition)
			var err error
			switch entry.Target {
			case capability.TargetFlagd:
				_, _, err = flagd.CompileIR(doc, "prod", flagd.CompileOptions{})
			case capability.TargetGOFeatureFlag:
				_, _, err = gofeatureflag.CompileIR(doc, "prod", gofeatureflag.CompileOptions{})
			default:
				t.Fatalf("unknown target %q", entry.Target)
			}
			var unsupported *capability.UnsupportedConditionError
			if entry.Supported {
				if err != nil {
					t.Fatalf("compile error = %v", err)
				}
				return
			}
			if !errors.As(err, &unsupported) || unsupported.Code() != capability.UnsupportedConditionCode {
				t.Fatalf("error = %v, want %s", err, capability.UnsupportedConditionCode)
			}
		})
	}
}

func conditionCapabilityFixture(kind capability.ConditionKind) *irv1.Document {
	path := &irv1.AttributePath{Segments: []string{"value"}}
	stringLiteral := func(value string) *irv1.ScalarValue {
		return &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: value}}
	}
	var condition *irv1.Condition
	switch kind {
	case capability.ConditionConstant:
		condition = &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}
	case capability.ConditionEquality:
		condition = &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{
			Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_EQ, Attribute: path, Literal: stringLiteral("on"),
		}}}
	case capability.ConditionNumeric:
		condition = &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{
			Operator: irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GTE, Attribute: path,
			Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}},
		}}}
	case capability.ConditionMembership:
		condition = &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{
			Attribute: path, Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{stringLiteral("on"), stringLiteral("off")}},
		}}}
	case capability.ConditionStringMatch:
		condition = &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{
			Operator: irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS, Attribute: path, Literal: "o",
		}}}
	case capability.ConditionSemver:
		condition = &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{
			Operator: irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GTE, Attribute: path, Semver: "1.0.0",
		}}}
	case capability.ConditionPresence:
		condition = &irv1.Condition{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: path}}}
	case capability.ConditionLogical:
		condition = &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{
			Operator: irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE,
			Conditions: []*irv1.Condition{
				{Kind: &irv1.Condition_Constant{Constant: true}},
				{Kind: &irv1.Condition_Constant{Constant: false}},
			},
		}}}
	case capability.ConditionNegation:
		condition = &irv1.Condition{Kind: &irv1.Condition_Negation{Negation: &irv1.Condition{Kind: &irv1.Condition_Constant{Constant: false}}}}
	}
	return &irv1.Document{Flags: map[string]*irv1.Flag{
		"condition": {
			Variants: map[string]*irv1.VariantValue{
				"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
				"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
			},
			Environments: map[string]*irv1.Environment{
				"prod": {Base: &irv1.Evaluation{
					Rules:         []*irv1.Rule{{Condition: condition, Action: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}}},
					DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}},
				}},
			},
		},
	}}
}
