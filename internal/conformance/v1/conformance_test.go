package v1_test

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/satorunooshie/ffcraft/internal/authoring"
	"github.com/satorunooshie/ffcraft/internal/capability"
	"github.com/satorunooshie/ffcraft/internal/codegen"
	"github.com/satorunooshie/ffcraft/internal/compiler/flagd"
	"github.com/satorunooshie/ffcraft/internal/compiler/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/ir"
	"github.com/satorunooshie/ffcraft/internal/normalize"
	"github.com/satorunooshie/ffcraft/internal/normalizedyaml"
	"github.com/satorunooshie/ffcraft/internal/runtimeeval"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

//go:embed testdata/*.yaml
var fixtures embed.FS

//go:embed testdata/authoring/*.yaml
var authoringFixtures embed.FS

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

			for _, environment := range fixtureEnvironments(withExtensions) {
				flagdWith, _, err := flagd.CompileIR(withExtensions, environment, flagd.CompileOptions{})
				flagdWithout, _, withoutErr := flagd.CompileIR(withoutExtensions, environment, flagd.CompileOptions{})
				if hasPresenceCondition(withExtensions) {
					assertUnsupportedPresence(t, err)
					assertUnsupportedPresence(t, withoutErr)
					continue
				}
				if err != nil || withoutErr != nil {
					t.Fatalf("flagd environment %q: with extensions: %v, without extensions: %v", environment, err, withoutErr)
				}
				if !bytes.Equal(flagdWith, flagdWithout) {
					t.Fatalf("flagd output changed after stripping extensions for environment %q", environment)
				}

				goffWith, _, err := gofeatureflag.CompileIR(withExtensions, environment, gofeatureflag.CompileOptions{})
				goffWithout, _, withoutErr := gofeatureflag.CompileIR(withoutExtensions, environment, gofeatureflag.CompileOptions{})
				if err != nil || withoutErr != nil {
					t.Fatalf("GO Feature Flag environment %q: with extensions: %v, without extensions: %v", environment, err, withoutErr)
				}
				if !bytes.Equal(goffWith, goffWithout) {
					t.Fatalf("GO Feature Flag output changed after stripping extensions for environment %q", environment)
				}
			}

			generated, err := codegen.CompileIR(withExtensions, codegen.Config{PackageName: "flags"})
			if err != nil {
				t.Fatal(err)
			}
			if fixture == "testdata/multi_environment_semantics.yaml" {
				for _, fragment := range []string{"UserSegment", "UserID", "AppVersion", "Region", "Cohort"} {
					if !bytes.Contains(generated, []byte(fragment)) {
						t.Fatalf("multi-environment codegen output missing %q", fragment)
					}
				}
			}
		})
	}
}

func fixtureEnvironments(doc *irv1.Document) []string {
	seen := make(map[string]struct{})
	for _, flag := range doc.Flags {
		for environment := range flag.Environments {
			seen[environment] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for environment := range seen {
		result = append(result, environment)
	}
	slices.Sort(result)
	return result
}

func assertUnsupportedPresence(t *testing.T, err error) {
	t.Helper()
	var unsupported *capability.UnsupportedConditionError
	if !errors.As(err, &unsupported) || unsupported.Code() != capability.UnsupportedConditionCode {
		t.Fatalf("error = %v, want unsupported presence diagnostic", err)
	}
}

func hasPresenceCondition(doc *irv1.Document) bool {
	for _, flag := range doc.Flags {
		for _, environment := range flag.Environments {
			if evaluationHasPresence(environment.Base) {
				return true
			}
			for _, scheduled := range environment.Schedule {
				if evaluationHasPresence(scheduled.Evaluation) {
					return true
				}
			}
		}
	}
	return false
}

func evaluationHasPresence(evaluation *irv1.Evaluation) bool {
	if evaluation == nil {
		return false
	}
	for _, rule := range evaluation.Rules {
		if conditionHasPresence(rule.Condition) {
			return true
		}
	}
	return false
}

func conditionHasPresence(condition *irv1.Condition) bool {
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_Presence:
		return true
	case *irv1.Condition_Logical:
		for _, child := range kind.Logical.Conditions {
			if conditionHasPresence(child) {
				return true
			}
		}
	case *irv1.Condition_Negation:
		return conditionHasPresence(kind.Negation)
	}
	return false
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

func TestV1RuntimeSemanticFixtures(t *testing.T) {
	doc := mustFixture(t, "testdata/runtime_semantics.yaml")
	numeric := doc.Flags["numeric_equality"].Environments["prod"].Base.Rules[0].Condition
	semver := doc.Flags["semver_runtime"].Environments["prod"].Base.Rules[0].Condition
	null := doc.Flags["null_and_presence"].Environments["prod"].Base.Rules[0].Condition
	presence := doc.Flags["null_and_presence"].Environments["prod"].Base.Rules[1].Condition

	for _, test := range []struct {
		name      string
		condition *irv1.Condition
		context   runtimeeval.Context
		want      bool
	}{
		{name: "lossless int64", condition: numeric, context: runtimeeval.Context{"value": int64(9007199254740993)}, want: true},
		{name: "lossy float64 does not equal int64", condition: numeric, context: runtimeeval.Context{"value": float64(9007199254740993)}, want: false},
		{name: "explicit null", condition: null, context: runtimeeval.Context{"value": nil}, want: true},
		{name: "missing is not null", condition: null, context: runtimeeval.Context{}, want: false},
		{name: "PRESENT includes null", condition: presence, context: runtimeeval.Context{"value": nil}, want: true},
		{name: "PRESENT rejects missing", condition: presence, context: runtimeeval.Context{}, want: false},
		{name: "SemVer precedence", condition: semver, context: runtimeeval.Context{"version": "2.0.1"}, want: true},
		{name: "SemVer prerelease precedes release", condition: semver, context: runtimeeval.Context{"version": "2.0.0-rc.1"}, want: false},
		{name: "invalid runtime SemVer", condition: semver, context: runtimeeval.Context{"version": "not-semver"}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeeval.Evaluate(test.condition, test.context); got != test.want {
				t.Fatalf("Evaluate() = %v, want %v", got, test.want)
			}
		})
	}

	withoutPresence := proto.Clone(doc).(*irv1.Document)
	delete(withoutPresence.Flags, "null_and_presence")
	for _, target := range []struct {
		name string
		call func(*irv1.Document) ([]byte, error)
	}{
		{name: "flagd", call: func(document *irv1.Document) ([]byte, error) {
			output, _, err := flagd.CompileIR(document, "prod", flagd.CompileOptions{})
			return output, err
		}},
		{name: "gofeatureflag", call: func(document *irv1.Document) ([]byte, error) {
			output, _, err := gofeatureflag.CompileIR(document, "prod", gofeatureflag.CompileOptions{})
			return output, err
		}},
	} {
		t.Run(target.name, func(t *testing.T) {
			withExtensions, err := target.call(withoutPresence)
			if err != nil {
				t.Fatal(err)
			}
			withoutExtensionsDoc := proto.Clone(withoutPresence).(*irv1.Document)
			withoutExtensionsDoc.Extensions = nil
			withoutExtensions, err := target.call(withoutExtensionsDoc)
			if err != nil || !bytes.Equal(withExtensions, withoutExtensions) {
				t.Fatalf("runtime semantic target output changed with extensions: %v", err)
			}
		})
	}
}

func TestV1MultiEnvironmentScheduleSemantics(t *testing.T) {
	doc := mustFixture(t, "testdata/multi_environment_semantics.yaml")
	flag := doc.Flags["multi_env"]
	for _, test := range []struct {
		name       string
		env        string
		at         time.Time
		context    runtimeeval.Context
		wantRule   string
		wantDist   bool
		wantTarget string
	}{
		{name: "prod base", env: "prod", at: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"user": map[string]any{"segment": "beta"}}, wantRule: "on", wantTarget: "user.segment"},
		{name: "prod scheduled", env: "prod", at: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"user": map[string]any{"id": "prod-1"}}, wantDist: true, wantTarget: "user.id"},
		{name: "staging base", env: "staging", at: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"app": map[string]any{"version": "2.1.0"}}, wantRule: "on", wantTarget: "app.version"},
		{name: "staging scheduled", env: "staging", at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"region": "ap-northeast"}, wantRule: "on", wantTarget: "region"},
		{name: "canary replacement", env: "canary", at: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"cohort": "canary"}, wantRule: "on", wantTarget: "cohort"},
	} {
		t.Run(test.name, func(t *testing.T) {
			environment := flag.Environments[test.env]
			evaluation := evaluationAt(environment, test.at)
			if evaluation.DefaultAction.GetServe() != "off" {
				t.Fatalf("default action = %q, want off", evaluation.DefaultAction.GetServe())
			}
			if len(evaluation.Rules) == 0 || !runtimeeval.Evaluate(evaluation.Rules[0].Condition, test.context) {
				t.Fatalf("schedule condition did not match for %s", test.wantTarget)
			}
			if test.wantDist {
				if evaluation.Rules[0].Action.GetDistribute() == nil {
					t.Fatal("expected a distribution action")
				}
			} else if evaluation.Rules[0].Action.GetServe() != test.wantRule {
				t.Fatalf("rule action = %q, want %q", evaluation.Rules[0].Action.GetServe(), test.wantRule)
			}
			for _, target := range []struct {
				name string
				call func(*irv1.Document, string) ([]byte, error)
			}{
				{name: "flagd", call: func(document *irv1.Document, env string) ([]byte, error) {
					output, _, err := flagd.CompileIR(document, env, flagd.CompileOptions{})
					return output, err
				}},
				{name: "gofeatureflag", call: func(document *irv1.Document, env string) ([]byte, error) {
					output, _, err := gofeatureflag.CompileIR(document, env, gofeatureflag.CompileOptions{})
					return output, err
				}},
			} {
				output, err := target.call(doc, test.env)
				if err != nil {
					t.Fatalf("%s compile: %v", target.name, err)
				}
				if !strings.Contains(string(output), test.wantTarget) {
					t.Fatalf("%s output missing targeting path %q", target.name, test.wantTarget)
				}
			}
		})
	}
	if evaluation := evaluationAt(flag.Environments["canary"], time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC)); len(evaluation.Rules) != 0 || evaluation.DefaultAction.GetServe() != "off" {
		t.Fatalf("complete snapshot replacement did not restore off evaluation: %+v", evaluation)
	}
}

func TestV1AuthoringScheduleNormalizationRemovesRedundantSnapshots(t *testing.T) {
	data, err := authoringFixtures.ReadFile("testdata/authoring/schedule_normalization.yaml")
	if err != nil {
		t.Fatal(err)
	}
	authoringDocument, err := authoring.ParseYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	astDocument, err := normalize.NormalizeAST(authoringDocument)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ir.FromAST(astDocument)
	if err != nil {
		t.Fatal(err)
	}
	schedule := doc.Flags["schedule-normalization"].Environments["prod"].Schedule
	if len(schedule) != 2 {
		t.Fatalf("normalized schedule length = %d, want 2", len(schedule))
	}
	if got := schedule[0].Evaluation.DefaultAction.GetServe(); got != "on" {
		t.Fatalf("first normalized snapshot serves %q, want on", got)
	}
	if got := schedule[1].Evaluation.DefaultAction.GetServe(); got != "off" {
		t.Fatalf("second normalized snapshot serves %q, want off", got)
	}
}

func mustFixture(t *testing.T, name string) *irv1.Document {
	t.Helper()
	data, err := fixtures.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := normalizedyaml.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func evaluationAt(environment *irv1.Environment, at time.Time) *irv1.Evaluation {
	evaluation := environment.Base
	for _, scheduled := range environment.Schedule {
		if scheduled.EffectiveAt.AsTime().After(at) {
			break
		}
		evaluation = scheduled.Evaluation
	}
	return evaluation
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
