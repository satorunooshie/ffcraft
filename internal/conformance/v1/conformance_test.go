package v1_test

import (
	"bytes"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
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
	"gopkg.in/yaml.v3"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

//go:embed testdata/*.yaml
var fixtures embed.FS

//go:embed testdata/authoring/*.yaml
var authoringFixtures embed.FS

//go:embed testdata/protobuf/*.hex
var protobufFixtures embed.FS

//go:embed testdata/expected/*
var expectedFixtures embed.FS

//go:embed testdata/invalid/invalid_cases.yaml
var invalidFixtures embed.FS

//go:embed testdata/manifest/conformance_manifest.yaml
var manifestFixture []byte

func TestV1ConformanceManifestIsCompleteAndResolvable(t *testing.T) {
	var manifest struct {
		Version string `yaml:"version"`
		Items   []struct {
			ID       string   `yaml:"id"`
			Fixtures []string `yaml:"fixtures"`
		} `yaml:"items"`
	}
	if err := yaml.Unmarshal(manifestFixture, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "conformance/v1" {
		t.Fatalf("manifest version = %q", manifest.Version)
	}
	seen := make(map[string]struct{}, len(manifest.Items))
	referenced := make(map[string]struct{})
	for _, item := range manifest.Items {
		if item.ID == "" || len(item.Fixtures) == 0 {
			t.Fatalf("invalid manifest item: %+v", item)
		}
		if _, exists := seen[item.ID]; exists {
			t.Fatalf("duplicate manifest item %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		for _, fixture := range item.Fixtures {
			if !conformanceFixtureExists(fixture) {
				t.Fatalf("manifest item %q references missing fixture %q", item.ID, fixture)
			}
			referenced[fixture] = struct{}{}
		}
	}
	expectedIDs := []string{
		"core_only_document", "scoped_extensions", "every_extension_value_kind", "deeply_nested_objects_lists",
		"int64_boundaries", "finite_doubles_and_null", "repeated_namespace_scopes", "unknown_namespaces",
		"empty_maps_and_absent_fields", "invalid_numbers", "empty_namespaces", "oversized_keys",
		"duplicate_yaml_mapping_keys", "custom_yaml_tags", "scalar_typing_edges", "oversized_object_field_names",
		"unknown_core_protobuf_fields", "unknown_extension_namespaces_compile", "unsupported_oneof_variants",
		"valid_protobuf_extensions_round_trip",
		"integer_weights_gcd_canonicalization", "invalid_distribution_shapes", "lossless_numeric_equality",
		"missing_null_presence", "runtime_type_mismatch_invalid_semver", "homogeneous_heterogeneous_membership",
		"string_match_operators", "semver_precedence_invalid_literal", "logical_operators",
		"timestamp_timezone_nanoseconds", "schedule_order_duplicates_redundancy_replacement",
		"multi_environment_target_output_semantics",
	}
	slices.Sort(expectedIDs)
	if !slices.Equal(expectedIDs, sortedKeys(seen)) {
		t.Fatalf("manifest IDs = %v, want %v", sortedKeys(seen), expectedIDs)
	}
	for _, fixture := range allConformanceFixturePaths() {
		if _, ok := referenced[fixture]; !ok {
			t.Fatalf("fixture %q is not referenced by the conformance manifest", fixture)
		}
	}
}

func TestV1ValidProtobufExtensionFixtureRoundTrip(t *testing.T) {
	wireText, err := protobufFixtures.ReadFile("testdata/protobuf/extensions_valid.hex")
	if err != nil {
		t.Fatal(err)
	}
	wire, err := hex.DecodeString(string(bytes.TrimSpace(wireText)))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ir.Unmarshal(wire)
	if err != nil {
		t.Fatalf("valid protobuf fixture rejected: %v", err)
	}
	encoded, err := ir.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.Unmarshal(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(doc, decoded) {
		t.Fatal("valid protobuf fixture changed across round trip")
	}
	if doc.Extensions["conformance"].GetObjectValue().Fields["integer"].GetIntValue() != 9007199254740993 {
		t.Fatal("valid protobuf fixture did not preserve large int64 extension")
	}
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func allConformanceFixturePaths() []string {
	patterns := []struct {
		filesystem fs.FS
		pattern    string
	}{
		{fixtures, "testdata/*.yaml"},
		{authoringFixtures, "testdata/authoring/*.yaml"},
		{protobufFixtures, "testdata/protobuf/*.hex"},
		{invalidFixtures, "testdata/invalid/*.yaml"},
		{expectedFixtures, "testdata/expected/*"},
	}
	var paths []string
	for _, item := range patterns {
		matches, err := fs.Glob(item.filesystem, item.pattern)
		if err != nil {
			panic(err)
		}
		paths = append(paths, matches...)
	}
	slices.Sort(paths)
	return paths
}

func conformanceFixtureExists(path string) bool {
	for _, filesystem := range []fs.FS{fixtures, authoringFixtures, protobufFixtures, invalidFixtures, expectedFixtures} {
		if _, err := fs.Stat(filesystem, path); err == nil {
			return true
		}
	}
	return false
}

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
			wire, err := ir.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := ir.Unmarshal(wire)
			if err != nil || !proto.Equal(doc, decoded) {
				t.Fatalf("protobuf decode/encode/decode semantic mismatch: %v", err)
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
				if environmentHasSemverCondition(withExtensions, environment) {
					assertUnsupportedSemver(t, err)
					assertUnsupportedSemver(t, withoutErr)
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

			generated, err := codegen.CompileIR(withExtensions, codegen.Config{PackageName: "flags", InferSDKFallback: true})
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
	if !errors.Is(err, capability.ErrUnsupportedCondition) || !errors.As(err, &unsupported) {
		t.Fatalf("error = %v, want unsupported presence diagnostic", err)
	}
}

func assertUnsupportedSemver(t *testing.T, err error) {
	t.Helper()
	var unsupported *capability.UnsupportedConditionError
	if !errors.Is(err, capability.ErrUnsupportedCondition) || !errors.As(err, &unsupported) || unsupported.Condition != capability.ConditionSemver {
		t.Fatalf("error = %v, want unsupported semver diagnostic", err)
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

func hasSemverCondition(doc *irv1.Document) bool {
	for _, flag := range doc.Flags {
		for _, environment := range flag.Environments {
			if evaluationHasSemver(environment.Base) {
				return true
			}
			for _, scheduled := range environment.Schedule {
				if evaluationHasSemver(scheduled.Evaluation) {
					return true
				}
			}
		}
	}
	return false
}

func environmentHasSemverCondition(doc *irv1.Document, name string) bool {
	for _, flag := range doc.Flags {
		environment, ok := flag.Environments[name]
		if !ok {
			continue
		}
		if evaluationHasSemver(environment.Base) {
			return true
		}
		for _, scheduled := range environment.Schedule {
			if evaluationHasSemver(scheduled.Evaluation) {
				return true
			}
		}
	}
	return false
}

func evaluationHasSemver(evaluation *irv1.Evaluation) bool {
	if evaluation == nil {
		return false
	}
	for _, rule := range evaluation.Rules {
		if conditionHasSemver(rule.Condition) {
			return true
		}
	}
	return false
}

func conditionHasSemver(condition *irv1.Condition) bool {
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_SemverComparison:
		return true
	case *irv1.Condition_Logical:
		if slices.ContainsFunc(kind.Logical.Conditions, conditionHasSemver) {
			return true
		}
	case *irv1.Condition_Negation:
		return conditionHasSemver(kind.Negation)
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
		if slices.ContainsFunc(kind.Logical.Conditions, conditionHasPresence) {
			return true
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

func TestV1ProtobufFixturesRejectUnknownCoreAndOneofFields(t *testing.T) {
	base := mustFixture(t, "testdata/core_conditions.yaml")
	for _, test := range []struct {
		name   string
		file   string
		mutate func(*irv1.Document, []byte)
	}{
		{name: "unknown core field", file: "testdata/protobuf/unknown_core_field.hex", mutate: func(doc *irv1.Document, wire []byte) { doc.ProtoReflect().SetUnknown(wire) }},
		{name: "unsupported condition oneof variant", file: "testdata/protobuf/unsupported_oneof_variant.hex", mutate: func(doc *irv1.Document, wire []byte) {
			condition := &irv1.Condition{}
			if err := proto.Unmarshal(wire, condition); err != nil {
				panic(err)
			}
			doc.Flags["conditions"].Environments["prod"].Base.Rules[0].Condition = condition
		}},
		{name: "unsupported equality enum value", file: "testdata/protobuf/invalid_enum_value.hex", mutate: func(doc *irv1.Document, wire []byte) {
			condition := &irv1.Condition{}
			if err := proto.Unmarshal(wire, condition); err != nil {
				panic(err)
			}
			doc.Flags["conditions"].Environments["prod"].Base.Rules[0].Condition = condition
		}},
		{name: "invalid oneof payload", file: "testdata/protobuf/invalid_oneof_payload.hex", mutate: func(doc *irv1.Document, wire []byte) {
			condition := &irv1.Condition{}
			if err := proto.Unmarshal(wire, condition); err != nil {
				panic(err)
			}
			doc.Flags["conditions"].Environments["prod"].Base.Rules[0].Condition = condition
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			wireText, err := protobufFixtures.ReadFile(test.file)
			if err != nil {
				t.Fatal(err)
			}
			wire, err := hex.DecodeString(string(bytes.TrimSpace(wireText)))
			if err != nil {
				t.Fatal(err)
			}
			doc := proto.Clone(base).(*irv1.Document)
			test.mutate(doc, wire)
			if err := ir.Validate(doc); err == nil {
				t.Fatal("protobuf fixture was accepted")
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
			if !errors.Is(err, capability.ErrUnsupportedCondition) || capabilityError.Condition != capability.ConditionPresence {
				t.Fatal("expected unsupported presence condition to fail closed")
			}
		})
	}
}

func TestV1ConditionCapabilityMatrixIsConnectedToCompilers(t *testing.T) {
	for _, entry := range capability.ConditionCapabilityMatrix() {
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
			if !errors.Is(err, capability.ErrUnsupportedCondition) || !errors.As(err, &unsupported) {
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
	delete(withoutPresence.Flags, "semver_runtime")
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
		wantFlagd  []string
		wantGOFF   []string
	}{
		{name: "prod base", env: "prod", at: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"user": map[string]any{"segment": "beta"}}, wantRule: "on", wantTarget: "user.segment", wantFlagd: []string{"1767225600", `"user.id"`, `"user.segment"`, `"off",`, `"on",`}, wantGOFF: []string{"2026-01-01T00:00:00.000000001Z", `user.id sw "prod-"`, `user.segment eq "beta"`}},
		{name: "prod scheduled", env: "prod", at: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"user": map[string]any{"id": "prod-1"}}, wantDist: true, wantTarget: "user.id", wantFlagd: []string{"1767225600", `"user.id"`, `"off",`, `"on",`}, wantGOFF: []string{"2026-01-01T00:00:00.000000001Z", `user.id sw "prod-"`, `"off": 1`, `"on": 2`}},
		{name: "staging base", env: "staging", at: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"app": map[string]any{"version": "2.1.0"}}, wantRule: "on", wantTarget: "app.version", wantFlagd: []string{"1767207600", `"app.version"`, `"region"`}, wantGOFF: []string{"2025-12-31T19:00:00Z", `app.version ge 2.0.0`, `region in ["ap-northeast", "us-east"]`}},
		{name: "staging scheduled", env: "staging", at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"region": "ap-northeast"}, wantRule: "on", wantTarget: "region", wantFlagd: []string{"1767207600", `"region"`, `"app.version"`}, wantGOFF: []string{"2025-12-31T19:00:00Z", `region in ["ap-northeast", "us-east"]`, `app.version ge 2.0.0`}},
		{name: "canary replacement", env: "canary", at: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"cohort": "canary"}, wantRule: "on", wantTarget: "cohort", wantFlagd: []string{"1768003200", "1770681600", `"cohort"`, `"on",`, `"off"`}, wantGOFF: []string{"2026-01-10T00:00:00Z", "2026-02-10T00:00:00Z", `cohort eq "canary"`}},
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
				if target.name == "flagd" && test.env == "staging" {
					assertUnsupportedSemver(t, err)
					continue
				}
				if err != nil {
					t.Fatalf("%s compile: %v", target.name, err)
				}
				suffix := ".goff.yaml"
				if target.name == "flagd" {
					suffix = ".flagd.json"
				}
				expectedName := fmt.Sprintf("testdata/expected/multi_env.%s", test.env)
				expectedData, err := expectedFixtures.ReadFile(expectedName + suffix)
				if err != nil {
					t.Fatal(err)
				}
				var actualValue, expectedValue any
				if target.name == "flagd" {
					if err := json.Unmarshal(output, &actualValue); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(expectedData, &expectedValue); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := yaml.Unmarshal(output, &actualValue); err != nil {
						t.Fatal(err)
					}
					if err := yaml.Unmarshal(expectedData, &expectedValue); err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(actualValue, expectedValue) {
					t.Fatalf("%s output semantic mismatch for %s\nactual: %#v\nexpected: %#v", target.name, test.env, actualValue, expectedValue)
				}
				fragments := test.wantGOFF
				if target.name == "flagd" {
					fragments = test.wantFlagd
				}
				for _, fragment := range fragments {
					if !strings.Contains(string(output), fragment) {
						t.Fatalf("%s output missing exact semantic fragment %q for %s", target.name, fragment, test.name)
					}
				}
			}
		})
	}
	if evaluation := evaluationAt(flag.Environments["canary"], time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC)); len(evaluation.Rules) != 0 || evaluation.DefaultAction.GetServe() != "off" {
		t.Fatalf("complete snapshot replacement did not restore off evaluation: %+v", evaluation)
	}
}

func TestV1MultiEnvironmentScheduleRuntimeMatrix(t *testing.T) {
	flag := mustFixture(t, "testdata/multi_environment_semantics.yaml").Flags["multi_env"]
	for _, test := range []struct {
		name    string
		env     string
		at      time.Time
		context runtimeeval.Context
		want    string
	}{
		{name: "prod base match", env: "prod", at: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"user": map[string]any{"segment": "beta"}}, want: "on"},
		{name: "prod base miss", env: "prod", at: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"user": map[string]any{"segment": "free"}}, want: "off"},
		{name: "prod scheduled distribution", env: "prod", at: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"user": map[string]any{"id": "prod-1"}}, want: "distribution"},
		{name: "prod scheduled miss", env: "prod", at: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"user": map[string]any{"id": "test-1"}}, want: "off"},
		{name: "staging base match", env: "staging", at: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"app": map[string]any{"version": "2.1.0"}}, want: "on"},
		{name: "staging base invalid semver", env: "staging", at: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"app": map[string]any{"version": "invalid"}}, want: "off"},
		{name: "staging scheduled match", env: "staging", at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"region": "ap-northeast"}, want: "on"},
		{name: "staging scheduled miss", env: "staging", at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"region": "eu-west"}, want: "off"},
		{name: "canary first snapshot", env: "canary", at: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"cohort": "canary"}, want: "on"},
		{name: "canary replacement", env: "canary", at: time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC), context: runtimeeval.Context{"cohort": "canary"}, want: "off"},
	} {
		t.Run(test.name, func(t *testing.T) {
			evaluation := evaluationAt(flag.Environments[test.env], test.at)
			if got := evaluationOutcome(evaluation, test.context); got != test.want {
				t.Fatalf("evaluation outcome = %q, want %q", got, test.want)
			}
		})
	}
}

func TestV1EveryEnvironmentScheduleBoundaryUsesExpectedSnapshot(t *testing.T) {
	flag := mustFixture(t, "testdata/multi_environment_semantics.yaml").Flags["multi_env"]
	contexts := map[string]runtimeeval.Context{
		"prod":    {"user": map[string]any{"segment": "beta", "id": "prod-1"}},
		"staging": {"app": map[string]any{"version": "2.1.0"}, "region": "ap-northeast"},
		"canary":  {"cohort": "canary"},
	}
	for environmentName, environment := range flag.Environments {
		t.Run(environmentName, func(t *testing.T) {
			context := contexts[environmentName]
			boundaries := make([]struct {
				name string
				at   time.Time
				want *irv1.Evaluation
			}, 0, len(environment.Schedule)*2+1)
			for index, scheduled := range environment.Schedule {
				at := scheduled.EffectiveAt.AsTime()
				previous := environment.Base
				if index > 0 {
					previous = environment.Schedule[index-1].Evaluation
				}
				boundaries = append(boundaries,
					struct {
						name string
						at   time.Time
						want *irv1.Evaluation
					}{name: fmt.Sprintf("before[%d]", index), at: at.Add(-time.Nanosecond), want: previous},
					struct {
						name string
						at   time.Time
						want *irv1.Evaluation
					}{name: fmt.Sprintf("at[%d]", index), at: at, want: scheduled.Evaluation},
				)
			}
			if len(boundaries) == 0 {
				boundaries = append(boundaries, struct {
					name string
					at   time.Time
					want *irv1.Evaluation
				}{name: "base", at: time.Unix(0, 0).UTC(), want: environment.Base})
			}
			for _, boundary := range boundaries {
				t.Run(boundary.name, func(t *testing.T) {
					got := evaluationAt(environment, boundary.at)
					if got != boundary.want {
						t.Fatalf("evaluation at %s selected %p, want %p", boundary.at.Format(time.RFC3339Nano), got, boundary.want)
					}
					for index, rule := range got.Rules {
						if !runtimeeval.Evaluate(rule.Condition, context) {
							continue
						}
						if rule.Action.GetServe() == "" && rule.Action.GetDistribute() == nil {
							t.Fatalf("rule[%d] has no executable action", index)
						}
					}
				})
			}
		})
	}
}

func evaluationOutcome(evaluation *irv1.Evaluation, context runtimeeval.Context) string {
	for _, rule := range evaluation.Rules {
		if !runtimeeval.Evaluate(rule.Condition, context) {
			continue
		}
		switch action := rule.Action.GetKind().(type) {
		case *irv1.Action_Serve:
			return action.Serve
		case *irv1.Action_Distribute:
			return "distribution"
		}
	}
	return evaluation.DefaultAction.GetServe()
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
	flagdOutput, _, err := flagd.CompileIR(doc, "prod", flagd.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"1769904000", "1775001600"} {
		if !bytes.Contains(flagdOutput, []byte(marker)) {
			t.Fatalf("flagd output missing non-redundant snapshot %s", marker)
		}
	}
	for _, marker := range []string{"1767225600", "1772323200"} {
		if bytes.Contains(flagdOutput, []byte(marker)) {
			t.Fatalf("flagd output contains redundant snapshot %s", marker)
		}
	}
	goffOutput, _, err := gofeatureflag.CompileIR(doc, "prod", gofeatureflag.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"2026-02-01T00:00:00Z", "2026-04-01T00:00:00Z"} {
		if !bytes.Contains(goffOutput, []byte(marker)) {
			t.Fatalf("GO Feature Flag output missing non-redundant snapshot %s", marker)
		}
	}
	for _, marker := range []string{"2026-01-01T00:00:00Z", "2026-03-01T00:00:00Z"} {
		if bytes.Contains(goffOutput, []byte(marker)) {
			t.Fatalf("GO Feature Flag output contains redundant snapshot %s", marker)
		}
	}
}

func TestV1AuthoringDistributionCanonicalizationReachesTargets(t *testing.T) {
	data, err := authoringFixtures.ReadFile("testdata/authoring/gcd_distribution.yaml")
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
	distribution := doc.Flags["gcd-rollout"].Environments["prod"].Base.Rules[0].Action.GetDistribute()
	weights := distribution.Weights
	if !proto.Equal(distribution, &irv1.Distribution{
		AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}},
		Weights:       map[string]uint32{"on": 1, "off": 9},
	}) {
		t.Fatalf("canonical weights = %#v, want on=1/off=9", weights)
	}
	flagdOutput, _, err := flagd.CompileIR(doc, "prod", flagd.CompileOptions{})
	if err != nil {
		t.Fatalf("flagd compile: %v", err)
	}
	var flagdDocument struct {
		Flags map[string]struct {
			Targeting map[string]any `json:"targeting"`
		} `json:"flags"`
	}
	if err := json.Unmarshal(flagdOutput, &flagdDocument); err != nil {
		t.Fatal(err)
	}
	fractional := flagdDocument.Flags["gcd-rollout"].Targeting["if"].([]any)[1].(map[string]any)["fractional"].([]any)
	bucketExpression := fractional[0].(map[string]any)
	if got := bucketExpression["var"]; got != "user.id" {
		t.Fatalf("flagd allocation key = %#v, want user.id", got)
	}
	if got := fractional[1].([]any); got[0] != "off" || got[1] != float64(9) {
		t.Fatalf("flagd off allocation = %#v, want [off 9]", got)
	}
	if got := fractional[2].([]any); got[0] != "on" || got[1] != float64(1) {
		t.Fatalf("flagd on allocation = %#v, want [on 1]", got)
	}

	goffOutput, _, err := gofeatureflag.CompileIR(doc, "prod", gofeatureflag.CompileOptions{})
	if err != nil {
		t.Fatalf("GO Feature Flag compile: %v", err)
	}
	var goffDocument map[string]struct {
		Targeting []struct {
			Percentage map[string]uint32 `yaml:"percentage"`
		} `yaml:"targeting"`
		BucketingKey string `yaml:"bucketingKey"`
	}
	if err := yaml.Unmarshal(goffOutput, &goffDocument); err != nil {
		t.Fatal(err)
	}
	rollout := goffDocument["gcd-rollout"]
	if rollout.BucketingKey != "user.id" || len(rollout.Targeting) != 1 || rollout.Targeting[0].Percentage["off"] != 9 || rollout.Targeting[0].Percentage["on"] != 1 {
		t.Fatalf("GO Feature Flag rollout = %#v, want off=9/on=1 and user.id", rollout)
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
	case capability.ConditionInequality:
		condition = &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{
			Operator: irv1.EqualityOperator_EQUALITY_OPERATOR_NE, Attribute: path, Literal: stringLiteral("on"),
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
