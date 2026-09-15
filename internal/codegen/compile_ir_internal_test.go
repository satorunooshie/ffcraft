package codegen

import (
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ast"
)

func TestVariantKindAndLiteralTables(t *testing.T) {
	tests := []struct {
		name        string
		value       *irv1.VariantValue
		wantKind    flagKind
		wantLiteral string
	}{
		{"bool", &irv1.VariantValue{Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, flagKindBool, "true"},
		{"string", &irv1.VariantValue{Kind: &irv1.VariantValue_StringValue{StringValue: "on"}}, flagKindString, `"on"`},
		{"int", &irv1.VariantValue{Kind: &irv1.VariantValue_IntValue{IntValue: 7}}, flagKindInt, "7"},
		{"double", &irv1.VariantValue{Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 1.5}}, flagKindFloat, "1.5"},
		{"object", &irv1.VariantValue{Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"b": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}, "a": {Kind: &irv1.VariantValue_StringValue{StringValue: "x"}}}}}}, flagKindObject, `map[string]any{"a": "x", "b": false}`},
		{"list", &irv1.VariantValue{Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_IntValue{IntValue: 1}}, {Kind: &irv1.VariantValue_NullValue{NullValue: &irv1.VariantNull{}}}}}}}, flagKindList, "[]any{1, nil}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, ok := variantKindOfIR(test.value)
			if !ok || kind != test.wantKind || goLiteralIR(test.value) != test.wantLiteral {
				t.Fatalf("variant = %v, %v, %q; want %v, true, %q", kind, ok, goLiteralIR(test.value), test.wantKind, test.wantLiteral)
			}
			got, err := variantKindIR(map[string]*irv1.VariantValue{"x": test.value})
			if err != nil || got != test.wantKind {
				t.Fatalf("variantKindIR() = %v, %v", got, err)
			}
		})
	}
	if _, err := variantKindIR(map[string]*irv1.VariantValue{"x": {}}); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("unsupported variant error = %v", err)
	}
	if _, err := variantKindIR(map[string]*irv1.VariantValue{
		"a": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
		"b": {Kind: &irv1.VariantValue_StringValue{StringValue: "x"}},
	}); err == nil || !strings.Contains(err.Error(), "homogeneous") {
		t.Fatalf("heterogeneous variant error = %v", err)
	}
}

func TestCollectIRContextFieldsTraversesConditions(t *testing.T) {
	attr := func(path []string) *irv1.AttributePath { return &irv1.AttributePath{Segments: path} }
	condition := &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{
		Conditions: []*irv1.Condition{
			{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Attribute: attr([]string{"user", "name"}), Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}}}},
			{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: attr([]string{"device", "id"})}}},
		},
	}}}
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{
		"f": {
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
	fields, err := collectIRContextFields(doc, ContextDefaultsConfig{}, nil)
	if err != nil || len(fields) != 2 || fields[0].Path != "device.id" || fields[1].Path != "user.name" {
		t.Fatalf("collectIRContextFields() = %#v, %v", fields, err)
	}
}

func TestCollectIRContextFieldsInfersAndOverridesTypes(t *testing.T) {
	attr := func(path ...string) *irv1.AttributePath { return &irv1.AttributePath{Segments: path} }
	serve := &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}
	rules := []*irv1.Rule{
		{Condition: &irv1.Condition{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Attribute: attr("score"), Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}}}}, Action: serve},
		{Condition: &irv1.Condition{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: attr("region"), Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{{Kind: &irv1.ScalarValue_StringValue{StringValue: "jp"}}}}}}}, Action: serve},
		{Condition: &irv1.Condition{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Attribute: attr("user", "id"), Literal: "test-"}}}, Action: serve},
		{Condition: &irv1.Condition{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Attribute: attr("version"), Semver: "1.2.3"}}}, Action: serve},
		{Condition: &irv1.Condition{Kind: &irv1.Condition_Negation{Negation: &irv1.Condition{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: attr("optional")}}}}}, Action: serve},
	}
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{Rules: rules, DefaultAction: serve}}}}}}
	fields, err := collectIRContextFields(doc, ContextDefaultsConfig{}, []ContextFieldConfig{{Path: "score", Name: "ScoreValue", Type: "int64"}, {Path: "new.path", Type: "[]string"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		name     string
		typeName string
	}{
		"new.path": {"NewPath", "[]string"},
		"optional": {"Optional", "string"},
		"region":   {"Region", "string"},
		"score":    {"ScoreValue", "int64"},
		"user.id":  {"UserID", "string"},
		"version":  {"Version", "string"},
	}
	if len(fields) != len(want) {
		t.Fatalf("context fields = %#v, want %d fields", fields, len(want))
	}
	for _, field := range fields {
		expected, ok := want[field.Path]
		if !ok || field.FieldName != expected.name || field.FieldType != expected.typeName {
			t.Fatalf("context field = %#v, want %#v", field, expected)
		}
	}
	if _, err := collectIRContextFields(doc, ContextDefaultsConfig{}, []ContextFieldConfig{{Path: "score", Type: "time.Time"}}); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("unsupported context override = %v", err)
	}
}

func TestContextTypeContractTables(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{" string ", "string"},
		{"int", "int"},
		{"unknown", "string"},
		{"[]string", "[]string"},
		{"map[string]any", "map[string]any"},
		{"[]unknown", "string"},
	} {
		if got := normalizeFieldType(test.input); got != test.want {
			t.Errorf("normalizeFieldType(%q) = %q, want %q", test.input, got, test.want)
		}
	}
	if !isSupportedFieldType("[]int64") || isSupportedFieldType("custom") {
		t.Fatal("isSupportedFieldType() contract violated")
	}
	if got := applyContextDefaults("int64", ContextDefaultsConfig{ScalarTypes: map[string]string{"int": "string"}}); got != "string" {
		t.Fatalf("applyContextDefaults() = %q", got)
	}
	tests := []struct {
		name     string
		defaults ContextDefaultsConfig
		want     string
	}{
		{"scalar key", ContextDefaultsConfig{ScalarTypes: map[string]string{"date": "string"}}, "unsupported key"},
		{"scalar type", ContextDefaultsConfig{ScalarTypes: map[string]string{"string": "date"}}, "unsupported type"},
		{"collection key", ContextDefaultsConfig{CollectionTypes: map[string]string{"date": "[]string"}}, "unsupported key"},
		{"collection type", ContextDefaultsConfig{CollectionTypes: map[string]string{"any": "date"}}, "unsupported type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateContextDefaults(test.defaults); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateContextDefaults() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCompileIRFlagContractTable(t *testing.T) {
	base := func(defaultVariant string) *irv1.Flag {
		return &irv1.Flag{
			Variants: map[string]*irv1.VariantValue{
				"off": {Kind: &irv1.VariantValue_StringValue{StringValue: "off"}},
				"on":  {Kind: &irv1.VariantValue_StringValue{StringValue: "on"}},
			},
			Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: defaultVariant}}}}},
		}
	}
	tests := []struct {
		name     string
		key      string
		source   *irv1.Flag
		accessor AccessorConfig
		want     string
	}{
		{"nil source", "f", nil, AccessorConfig{}, "variants are required"},
		{"no environments", "f", &irv1.Flag{Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}}}, AccessorConfig{}, "environments are required"},
		{"missing default variant", "f", base("missing"), AccessorConfig{}, "default variant"},
		{"custom accessor", "checkout", base("on"), AccessorConfig{Name: "CheckoutMode", VariantType: "CheckoutVariant"}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := compileIRFlag(test.key, test.source, test.accessor)
			if test.want != "" {
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("compileIRFlag() = %#v, %v, want %q", compiled, err, test.want)
				}
				return
			}
			if err != nil || compiled.AccessorName != "CheckoutMode" || compiled.VariantType != "CheckoutVariant" || len(compiled.Variants) != 2 {
				t.Fatalf("compileIRFlag() = %#v, %v", compiled, err)
			}
		})
	}
}

func TestCodegenTemplateAndFormattingContracts(t *testing.T) {
	if _, err := CompileIR(&irv1.Document{}, Config{}); err == nil || !strings.Contains(err.Error(), "FFCRAFT_IR_INVALID_CORE") {
		t.Fatalf("CompileIR(empty) = %v", err)
	}
	template, err := templateForCodegen()
	if err != nil {
		t.Fatal(err)
	}
	_ = template
	if _, err := formatGeneratedGo([]byte("package broken\nfunc {")); err == nil || !strings.Contains(err.Error(), "format generated Go") {
		t.Fatalf("formatGeneratedGo(invalid) = %v", err)
	}
	source, err := CompileIR(minimalCodegenDocument(), Config{PackageName: "generated"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
		t.Fatalf("generated Go is not parseable: %v", err)
	}
}

func TestCompileIRDocumentConfigContracts(t *testing.T) {
	doc := minimalCodegenDocument()
	if _, err := CompileIR(doc, Config{}); err == nil || !strings.Contains(err.Error(), "package name is required") {
		t.Fatalf("CompileIR(missing package) = %v", err)
	}
	source, err := CompileIR(doc, Config{PackageName: "generated"})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"type Client interface", "type Evaluator struct"} {
		if !strings.Contains(string(source), fragment) {
			t.Fatalf("default generated type missing %q", fragment)
		}
	}
	source, err = CompileIR(doc, Config{
		PackageName:   "generated",
		ContextType:   "RequestContext",
		ClientType:    "FeatureClient",
		EvaluatorType: "FeatureEvaluator",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"type FeatureClient interface", "type FeatureEvaluator struct"} {
		if !strings.Contains(string(source), fragment) {
			t.Fatalf("custom generated type missing %q", fragment)
		}
	}
}

func TestFilterEnvironmentContracts(t *testing.T) {
	doc := &ast.Document{Flags: []*ast.Flag{{
		Key:            "checkout",
		DefaultVariant: "off",
		Variants:       map[string]ast.VariantValue{"on": {Kind: ast.VariantValueKindBool, Bool: true}, "off": {Kind: ast.VariantValueKindBool}},
		Environments: map[string]*ast.Environment{
			"prod":    {DefaultAction: &ast.ServeAction{Variant: "on"}},
			"staging": {DefaultAction: &ast.ServeAction{Variant: "off"}},
		},
	}}}
	filtered, warnings, err := filterEnvironment(doc, "prod", false)
	if err != nil || len(warnings) != 0 || len(filtered.Flags) != 1 || len(filtered.Flags[0].Environments) != 1 || filtered.Flags[0].Environments["prod"].DefaultAction.(*ast.ServeAction).Variant != "on" {
		t.Fatalf("filterEnvironment(prod) = %#v, %#v, %v", filtered, warnings, err)
	}
	if _, _, err := filterEnvironment(doc, "canary", false); err == nil || !strings.Contains(err.Error(), `environment "canary" not found`) {
		t.Fatalf("missing environment error = %v", err)
	}
	filtered, warnings, err = filterEnvironment(doc, "canary", true)
	if err != nil || len(filtered.Flags) != 0 || len(warnings) != 1 || !strings.Contains(warnings[0], "skipping flag") {
		t.Fatalf("missing environment warning mode = %#v, %#v, %v", filtered, warnings, err)
	}
}

func minimalCodegenDocument() *irv1.Document {
	return &irv1.Document{Flags: map[string]*irv1.Flag{"flag": {
		Variants:     map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, "off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}},
		Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}}}},
	}}}
}

func TestFlagIRTargetingKeyPathsIncludesScheduledActions(t *testing.T) {
	distribution := func(path ...string) *irv1.Action {
		return &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: path}, Weights: map[string]uint32{"on": 1, "off": 1}}}}
	}
	docFlag := &irv1.Flag{
		Variants:     map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, "off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}},
		Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: distribution("targetingKey"), Rules: []*irv1.Rule{{Action: distribution("user", "id")}}}, Schedule: []*irv1.ScheduledEvaluation{{Evaluation: &irv1.Evaluation{DefaultAction: distribution("device", "id")}}}}},
	}
	paths := flagIRTargetingKeyPaths(docFlag)
	if len(paths) != 2 || paths[0] != "device.id" || paths[1] != "user.id" || flagIRRequiresTargetingKey(docFlag) != true {
		t.Fatalf("flagIRTargetingKeyPaths() = %#v", paths)
	}
}

func TestLegacyContextInferenceTables(t *testing.T) {
	tests := []struct {
		name  string
		value ast.Value
		path  string
		want  string
	}{
		{"bool", &ast.Scalar{Kind: ast.ScalarKindBool}, "value", "bool"},
		{"int", &ast.Scalar{Kind: ast.ScalarKindInt}, "value", "int64"},
		{"double", &ast.Scalar{Kind: ast.ScalarKindDouble}, "value", "float64"},
		{"string", &ast.Scalar{Kind: ast.ScalarKindString}, "value", "string"},
		{"id heuristic", &ast.Var{Path: "user.id"}, "user.id", "int64"},
		{"default variable", &ast.Var{Path: "user.name"}, "user.name", "string"},
		{"empty list", &ast.List{}, "values", "string"},
		{"int list", &ast.List{Values: []ast.Value{&ast.Scalar{Kind: ast.ScalarKindInt}, &ast.Scalar{Kind: ast.ScalarKindInt}}}, "values", "int64"},
		{"mixed list", &ast.List{Values: []ast.Value{&ast.Scalar{Kind: ast.ScalarKindInt}, &ast.Scalar{Kind: ast.ScalarKindString}}}, "values", "any"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := inferValueType(test.value, test.path); got != test.want {
				t.Fatalf("inferValueType() = %q, want %q", got, test.want)
			}
		})
	}
	for _, test := range []struct {
		name  string
		value ast.Value
		want  string
	}{
		{"scalar int", &ast.Scalar{Kind: ast.ScalarKindInt}, "[]int64"},
		{"scalar string", &ast.Scalar{Kind: ast.ScalarKindString}, "[]string"},
		{"list bool", &ast.List{Values: []ast.Value{&ast.Scalar{Kind: ast.ScalarKindBool}}}, "[]bool"},
		{"list mixed", &ast.List{Values: []ast.Value{&ast.Scalar{Kind: ast.ScalarKindBool}, &ast.Scalar{Kind: ast.ScalarKindString}}}, "[]any"},
		{"unknown", &ast.Var{Path: "value"}, "[]string"},
	} {
		t.Run("collection/"+test.name, func(t *testing.T) {
			if got := inferCollectionType(test.value); got != test.want {
				t.Fatalf("inferCollectionType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLegacyConditionContextFieldContracts(t *testing.T) {
	variable := &ast.Var{Path: "user.value"}
	scalar := &ast.Scalar{Kind: ast.ScalarKindString, String: "x"}
	list := &ast.List{Values: []ast.Value{scalar}}
	tests := []struct {
		name      string
		condition ast.Condition
		wantPath  string
		wantType  string
	}{
		{"binary left", &ast.Eq{Left: variable, Right: scalar}, "user.value", "string"},
		{"binary right", &ast.Ne{Left: scalar, Right: variable}, "user.value", "string"},
		{"membership target", &ast.In{Target: variable, Candidate: list}, "user.value", "string"},
		{"membership candidate", &ast.In{Target: scalar, Candidate: variable}, "user.value", "[]string"},
		{"contains container", &ast.Contains{Container: variable, Value: scalar}, "user.value", "[]string"},
		{"contains value", &ast.Contains{Container: list, Value: variable}, "user.value", "string"},
		{"starts with", &ast.StartsWith{Target: variable}, "user.value", "string"},
		{"ends with", &ast.EndsWith{Target: variable}, "user.value", "string"},
		{"matches", &ast.Matches{Target: variable}, "user.value", "string"},
		{"semver gt", &ast.SemverGt{Left: variable}, "user.value", "string"},
		{"semver gte", &ast.SemverGte{Left: variable}, "user.value", "string"},
		{"semver lt", &ast.SemverLt{Left: variable}, "user.value", "string"},
		{"semver lte", &ast.SemverLte{Left: variable}, "user.value", "string"},
		{"nested all", &ast.AllOf{Conditions: []ast.Condition{&ast.Eq{Left: variable, Right: scalar}}}, "user.value", "string"},
		{"nested any", &ast.AnyOf{Conditions: []ast.Condition{&ast.Eq{Left: variable, Right: scalar}}}, "user.value", "string"},
		{"nested one", &ast.OneOf{Conditions: []ast.Condition{&ast.Eq{Left: variable, Right: scalar}}}, "user.value", "string"},
		{"nested not", &ast.Not{Condition: &ast.Eq{Left: variable, Right: scalar}}, "user.value", "string"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotPath, gotType string
			collectContextFieldsFromCondition(test.condition, func(path, inferredType string) {
				gotPath, gotType = path, inferredType
			})
			if gotPath != test.wantPath || gotType != test.wantType {
				t.Fatalf("collectContextFieldsFromCondition() = %q, %q; want %q, %q", gotPath, gotType, test.wantPath, test.wantType)
			}
		})
	}
	var path string
	collectContextFieldsFromCondition(nil, func(value, _ string) { path = value })
	if path != "" {
		t.Fatalf("nil condition emitted path %q", path)
	}
}

func TestContextTypeKeyAndTargetingPathContracts(t *testing.T) {
	for _, test := range []struct {
		value string
		want  string
	}{
		{"string", "string"}, {"bool", "bool"}, {"int", "int"}, {"int64", "int"}, {"float64", "float"}, {"unknown", ""},
	} {
		if got := inferredScalarTypeKey(test.value); got != test.want {
			t.Errorf("inferredScalarTypeKey(%q) = %q, want %q", test.value, got, test.want)
		}
	}
	for _, test := range []struct {
		value string
		want  string
	}{
		{"[]string", "string"}, {"[]bool", "bool"}, {"[]int", "int"}, {"[]int64", "int"}, {"[]float64", "float"}, {"[]any", "any"}, {"unknown", ""},
	} {
		if got := inferredCollectionTypeKey(test.value); got != test.want {
			t.Errorf("inferredCollectionTypeKey(%q) = %q, want %q", test.value, got, test.want)
		}
	}
	paths := map[string]struct{}{}
	for _, value := range []string{"", "targetingKey", "user.id"} {
		addTargetingKeyPath(value, paths)
	}
	if !slices.Equal([]string{"user.id"}, func() []string {
		out := make([]string, 0, len(paths))
		for path := range paths {
			out = append(out, path)
		}
		return out
	}()) {
		t.Fatalf("addTargetingKeyPath() = %#v", paths)
	}
}

func TestLegacyGoLiteralTables(t *testing.T) {
	tests := []struct {
		name  string
		value ast.VariantValue
		want  string
	}{
		{"bool", ast.VariantValue{Kind: ast.VariantValueKindBool, Bool: true}, "true"},
		{"string", ast.VariantValue{Kind: ast.VariantValueKindString, String: "x"}, `"x"`},
		{"int", ast.VariantValue{Kind: ast.VariantValueKindInt, Int: 7}, "7"},
		{"double", ast.VariantValue{Kind: ast.VariantValueKindDouble, Double: 1.5}, "1.5"},
		{"null", ast.VariantValue{Kind: ast.VariantValueKindNull}, "nil"},
		{"object", ast.VariantValue{Kind: ast.VariantValueKindObject, Object: map[string]any{"b": false, "a": "x"}}, `map[string]any{"a": "x", "b": false}`},
		{"list", ast.VariantValue{Kind: ast.VariantValueKindList, List: []ast.VariantValue{{Kind: ast.VariantValueKindInt, Int: 1}, {Kind: ast.VariantValueKindNull}}}, "[]any{1, nil}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := goLiteral(test.value); got != test.want {
				t.Fatalf("goLiteral() = %q, want %q", got, test.want)
			}
		})
	}
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, "nil"}, {"bool", true, "true"}, {"string", "x", `"x"`},
		{"int", int(2), "2"}, {"int8", int8(2), "2"}, {"int16", int16(2), "2"}, {"int32", int32(2), "2"}, {"int64", int64(2), "2"},
		{"uint", uint(3), "3"}, {"uint8", uint8(3), "3"}, {"uint16", uint16(3), "3"}, {"uint32", uint32(3), "3"}, {"uint64", uint64(3), "3"},
		{"float32", float32(1.25), "1.25"}, {"float64", float64(1.25), "1.25"},
		{"empty map", map[string]any{}, "map[string]any{}"}, {"map", map[string]any{"x": int64(1)}, `map[string]any{"x": 1}`},
		{"empty list", []any{}, "[]any{}"}, {"list", []any{true, "x"}, `[]any{true, "x"}`},
	} {
		t.Run("any/"+test.name, func(t *testing.T) {
			if got := goAnyLiteral(test.value); got != test.want {
				t.Fatalf("goAnyLiteral() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLegacyBooleanDefaultTable(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"on", "true"}, {"ON", "true"}, {"true", "true"}, {"off", "false"}, {"false", "false"}, {"", "false"},
	} {
		t.Run(test.input, func(t *testing.T) {
			if got := boolDefault(test.input); got != test.want {
				t.Fatalf("boolDefault(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestLegacyContextAndTargetingHelperContracts(t *testing.T) {
	contextRule := &ast.Rule{Condition: &ast.Eq{Left: &ast.Var{Path: "user.id"}, Right: &ast.Scalar{Kind: ast.ScalarKindString, String: "x"}}, Action: &ast.ServeAction{Variant: "on"}}
	flag := &ast.Flag{
		Environments: map[string]*ast.Environment{"prod": {
			Rules:             []*ast.Rule{contextRule},
			DefaultAction:     &ast.DistributeAction{Stickiness: "user.id", Allocations: map[string]float64{"on": 1, "off": 1}},
			ScheduledRollouts: []*ast.ScheduledStep{{DefaultAction: &ast.ProgressiveRolloutAction{Stickiness: "device.id", Variant: "on"}}},
		}},
	}
	if !flagUsesContext(flag) || !environmentUsesContext(flag.Environments["prod"]) {
		t.Fatal("context helper failed to detect targeting condition")
	}
	if !flagRequiresTargetingKey(flag) || !environmentRequiresTargetingKey(flag.Environments["prod"]) {
		t.Fatal("targeting key helper failed to detect distribution")
	}
	if got := flagTargetingKeyPaths(flag); !slices.Equal(got, []string{"device.id", "user.id"}) {
		t.Fatalf("flagTargetingKeyPaths() = %#v", got)
	}
	if !valueUsesContext(&ast.List{Values: []ast.Value{&ast.Var{Path: "items"}}}) || valueUsesContext(&ast.Var{}) {
		t.Fatal("valueUsesContext() contract violated")
	}
	if !actionRequiresTargetingKey(&ast.ProgressiveRolloutAction{}) || actionRequiresTargetingKey(&ast.ServeAction{}) {
		t.Fatal("actionRequiresTargetingKey() contract violated")
	}
	if flagUsesContext(&ast.Flag{Environments: map[string]*ast.Environment{"prod": {}}}) {
		t.Fatal("empty environment unexpectedly uses context")
	}
}

func TestIRContextInferenceTables(t *testing.T) {
	attribute := &irv1.AttributePath{Segments: []string{"value"}}
	literal := &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}
	condition := func(kind any) *irv1.Condition {
		result := &irv1.Condition{}
		switch kind := kind.(type) {
		case *irv1.Condition_Constant:
			result.Kind = kind
		case *irv1.Condition_Equality:
			result.Kind = kind
		case *irv1.Condition_Logical:
			result.Kind = kind
		case *irv1.Condition_Negation:
			result.Kind = kind
		case *irv1.Condition_Presence:
			result.Kind = kind
		default:
			t.Fatalf("unsupported test condition kind %T", kind)
		}
		return result
	}
	tests := []struct {
		name      string
		condition *irv1.Condition
		want      bool
	}{
		{"nil", nil, false},
		{"constant", condition(&irv1.Condition_Constant{Constant: true}), false},
		{"equality", condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Attribute: attribute, Literal: literal}}), true},
		{"logical constant", condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Conditions: []*irv1.Condition{condition(&irv1.Condition_Constant{Constant: true})}}}), false},
		{"logical context", condition(&irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Conditions: []*irv1.Condition{condition(&irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Attribute: attribute, Literal: literal}})}}}), true},
		{"negation constant", condition(&irv1.Condition_Negation{Negation: condition(&irv1.Condition_Constant{Constant: false})}), false},
		{"negation context", condition(&irv1.Condition_Negation{Negation: condition(&irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: attribute}})}), true},
	}
	for _, test := range tests {
		t.Run("condition/"+test.name, func(t *testing.T) {
			if got := irConditionUsesContext(test.condition); got != test.want {
				t.Fatalf("irConditionUsesContext() = %v, want %v", got, test.want)
			}
		})
	}
	for _, test := range []struct {
		name    string
		literal *irv1.ScalarValue
		want    string
	}{
		{"bool", &irv1.ScalarValue{Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}, "bool"},
		{"int", &irv1.ScalarValue{Kind: &irv1.ScalarValue_IntValue{IntValue: 1}}, "int64"},
		{"double", &irv1.ScalarValue{Kind: &irv1.ScalarValue_DoubleValue{DoubleValue: 1}}, "float"},
		{"string", literal, "string"},
		{"null", &irv1.ScalarValue{Kind: &irv1.ScalarValue_NullValue{NullValue: &irv1.ScalarNull{}}}, "any"},
		{"nil", nil, "any"},
	} {
		t.Run("scalar/"+test.name, func(t *testing.T) {
			if got := irScalarType(test.literal); got != test.want {
				t.Fatalf("irScalarType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLegacyConditionContextDetectionTable(t *testing.T) {
	variable := &ast.Var{Path: "user.value"}
	scalar := &ast.Scalar{Kind: ast.ScalarKindString, String: "x"}
	tests := []struct {
		name      string
		condition ast.Condition
		want      bool
	}{
		{"eq", &ast.Eq{Left: variable, Right: scalar}, true},
		{"ne", &ast.Ne{Left: scalar, Right: variable}, true},
		{"gt", &ast.Gt{Left: variable, Right: scalar}, true},
		{"gte", &ast.Gte{Left: scalar, Right: variable}, true},
		{"lt", &ast.Lt{Left: variable, Right: scalar}, true},
		{"lte", &ast.Lte{Left: scalar, Right: variable}, true},
		{"in", &ast.In{Target: variable, Candidate: &ast.List{Values: []ast.Value{scalar}}}, true},
		{"contains", &ast.Contains{Container: variable, Value: scalar}, true},
		{"starts with", &ast.StartsWith{Target: variable, Prefix: "x"}, true},
		{"ends with", &ast.EndsWith{Target: variable, Suffix: "x"}, true},
		{"matches", &ast.Matches{Target: variable, Pattern: "x"}, true},
		{"semver gt", &ast.SemverGt{Left: variable, Right: "1.2.3"}, true},
		{"semver gte", &ast.SemverGte{Left: variable, Right: "1.2.3"}, true},
		{"semver lt", &ast.SemverLt{Left: variable, Right: "1.2.3"}, true},
		{"semver lte", &ast.SemverLte{Left: variable, Right: "1.2.3"}, true},
		{"all", &ast.AllOf{Conditions: []ast.Condition{&ast.LiteralBool{Value: false}, &ast.Eq{Left: variable, Right: scalar}}}, true},
		{"any", &ast.AnyOf{Conditions: []ast.Condition{&ast.LiteralBool{Value: false}, &ast.Eq{Left: variable, Right: scalar}}}, true},
		{"one", &ast.OneOf{Conditions: []ast.Condition{&ast.LiteralBool{Value: false}, &ast.Eq{Left: variable, Right: scalar}}}, true},
		{"not", &ast.Not{Condition: &ast.Eq{Left: variable, Right: scalar}}, true},
		{"literal", &ast.LiteralBool{Value: true}, false},
		{"constant comparison", &ast.Eq{Left: scalar, Right: scalar}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := conditionUsesContext(test.condition); got != test.want {
				t.Fatalf("conditionUsesContext() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLegacyVariantSetKindTable(t *testing.T) {
	tests := []struct {
		name string
		kind ast.VariantValueKind
		want flagKind
	}{
		{"bool", ast.VariantValueKindBool, flagKindBool},
		{"string", ast.VariantValueKindString, flagKindString},
		{"int", ast.VariantValueKindInt, flagKindInt},
		{"float", ast.VariantValueKindDouble, flagKindFloat},
		{"object", ast.VariantValueKindObject, flagKindObject},
		{"list", ast.VariantValueKindList, flagKindList},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, err := variantSetKind(&ast.Flag{Variants: map[string]ast.VariantValue{"value": {Kind: test.kind}}})
			if err != nil || kind != test.want {
				t.Fatalf("variantSetKind() = %v, %v; want %v", kind, err, test.want)
			}
		})
	}
	if _, err := variantSetKind(&ast.Flag{}); err == nil || !strings.Contains(err.Error(), "supports only") {
		t.Fatalf("variantSetKind(empty) = %v, want unsupported error", err)
	}
	if _, err := variantSetKind(&ast.Flag{Variants: map[string]ast.VariantValue{
		"bool":   {Kind: ast.VariantValueKindBool},
		"string": {Kind: ast.VariantValueKindString},
	}}); err == nil || !strings.Contains(err.Error(), "supports only") {
		t.Fatalf("variantSetKind(mixed) = %v, want unsupported error", err)
	}
}
