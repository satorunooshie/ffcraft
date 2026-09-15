package codegen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
)

func TestIRVariantKindAndLiteralTables(t *testing.T) {
	tests := []struct {
		name  string
		value *irv1.VariantValue
		kind  flagKind
		want  string
	}{
		{"bool", &irv1.VariantValue{Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, flagKindBool, "true"},
		{"string", &irv1.VariantValue{Kind: &irv1.VariantValue_StringValue{StringValue: "on"}}, flagKindString, `"on"`},
		{"int", &irv1.VariantValue{Kind: &irv1.VariantValue_IntValue{IntValue: 7}}, flagKindInt, "7"},
		{"double", &irv1.VariantValue{Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 1.5}}, flagKindFloat, "1.5"},
		{"object", &irv1.VariantValue{Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"x": {Kind: &irv1.VariantValue_StringValue{StringValue: "y"}}}}}}, flagKindObject, `map[string]any{"x": "y"}`},
		{"list", &irv1.VariantValue{Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_IntValue{IntValue: 1}}}}}}, flagKindList, "[]any{1}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, ok := variantKindOfIR(test.value)
			if !ok || kind != test.kind || goLiteralIR(test.value) != test.want {
				t.Fatalf("variantKindOfIR() = %v, %v; literal = %q", kind, ok, goLiteralIR(test.value))
			}
		})
	}
	if _, err := variantKindIR(map[string]*irv1.VariantValue{"x": {}}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unset variant error = %v", err)
	}
	if _, err := variantKindIR(map[string]*irv1.VariantValue{"a": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, "b": {Kind: &irv1.VariantValue_StringValue{StringValue: "x"}}}); err == nil || !strings.Contains(err.Error(), "homogeneous") {
		t.Fatalf("heterogeneous variant error = %v", err)
	}
}

func TestCompileIRRejectsUnrepresentableEnvironmentAction(t *testing.T) {
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{"f": {
		Variants: map[string]*irv1.VariantValue{
			"on":  {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
			"off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}},
		},
		Environments: map[string]*irv1.Environment{
			"a": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}}},
			"b": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{
				AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}},
				Weights:       map[string]uint32{"on": 1, "off": 1},
			}}}}},
		},
	}}}
	if _, err := CompileIR(doc, Config{PackageName: "generated"}); err == nil || !strings.Contains(err.Error(), "requires sdk_fallback_variant") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIRCompileFlagContracts(t *testing.T) {
	base := func(defaultVariant string) *irv1.Flag {
		return &irv1.Flag{
			Variants:     map[string]*irv1.VariantValue{"off": {Kind: &irv1.VariantValue_StringValue{StringValue: "off"}}, "on": {Kind: &irv1.VariantValue_StringValue{StringValue: "on"}}},
			Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: defaultVariant}}}}},
		}
	}
	for _, test := range []struct {
		name   string
		source *irv1.Flag
		want   string
	}{
		{"nil", nil, "variants are required"}, {"no variants", &irv1.Flag{Environments: map[string]*irv1.Environment{}}, "variants are required"}, {"no environments", &irv1.Flag{Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_StringValue{StringValue: "on"}}}}, "environments are required"}, {"missing fallback", base("on"), "requires sdk_fallback_variant"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := compileIRFlag("f", test.source, AccessorConfig{}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compileIRFlag() = %v, want %q", err, test.want)
			}
		})
	}
	compiled, err := compileIRFlag("checkout", base("on"), AccessorConfig{Name: "CheckoutMode", VariantType: "CheckoutVariant", SDKFallbackVariant: "on"})
	if err != nil || compiled.AccessorName != "CheckoutMode" || compiled.VariantType != "CheckoutVariant" || len(compiled.Variants) != 2 {
		t.Fatalf("compiled flag = %#v, %v", compiled, err)
	}
	base("on").Environments["staging"] = &irv1.Environment{Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}}}
}

func TestIRContextConditionTraversal(t *testing.T) {
	attr := func(parts ...string) *irv1.AttributePath { return &irv1.AttributePath{Segments: parts} }
	stringValue := &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}
	conditions := []*irv1.Condition{
		{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Attribute: attr("user", "name"), Literal: stringValue}}},
		{Kind: &irv1.Condition_NumericComparison{NumericComparison: &irv1.NumericComparisonCondition{Attribute: attr("score"), Literal: &irv1.NumericValue{Kind: &irv1.NumericValue_IntValue{IntValue: 1}}}}},
		{Kind: &irv1.Condition_Membership{Membership: &irv1.MembershipCondition{Attribute: attr("region"), Literals: &irv1.ScalarList{Values: []*irv1.ScalarValue{stringValue}}}}},
		{Kind: &irv1.Condition_StringMatch{StringMatch: &irv1.StringMatchCondition{Attribute: attr("prefix"), Literal: "x"}}},
		{Kind: &irv1.Condition_SemverComparison{SemverComparison: &irv1.SemVerComparisonCondition{Attribute: attr("version"), Semver: "1.2.3"}}},
		{Kind: &irv1.Condition_Presence{Presence: &irv1.PresenceCondition{Attribute: attr("optional")}}},
	}
	fields := map[string]string{}
	for _, condition := range conditions {
		collectIRContextFieldsFromCondition(condition, func(path, kind string) { fields[path] = kind })
	}
	for path, want := range map[string]string{"user.name": "string", "score": "int64", "region": "string", "prefix": "string", "version": "string", "optional": "any"} {
		if fields[path] != want {
			t.Errorf("context field %q = %q, want %q", path, fields[path], want)
		}
	}
	if !irConditionUsesContext(conditions[0]) || irConditionUsesContext(&irv1.Condition{Kind: &irv1.Condition_Constant{Constant: true}}) || irConditionUsesContext(nil) {
		t.Fatal("IR context detection contract violated")
	}
	logical := &irv1.Condition{Kind: &irv1.Condition_Logical{Logical: &irv1.LogicalCondition{Conditions: []*irv1.Condition{conditions[0]}}}}
	if !irConditionUsesContext(logical) || !irConditionUsesContext(&irv1.Condition{Kind: &irv1.Condition_Negation{Negation: conditions[0]}}) {
		t.Fatal("nested IR context detection failed")
	}
}

func TestIRTargetingAndConfigContracts(t *testing.T) {
	distribute := &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: &irv1.AttributePath{Segments: []string{"user", "id"}}, Weights: map[string]uint32{"on": 1}}}}
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: distribute}}}}}}
	flag := doc.Flags["f"]
	if flagIRUsesContext(flag) || !flagIRRequiresTargetingKey(flag) || !strings.EqualFold(strings.Join(flagIRTargetingKeyPaths(flag), ","), "user.id") {
		t.Fatalf("IR targeting helpers failed: context=%v key=%v paths=%v", flagIRUsesContext(flag), flagIRRequiresTargetingKey(flag), flagIRTargetingKeyPaths(flag))
	}
	for _, test := range []struct {
		name  string
		value string
		want  string
	}{
		{"trim", " string ", "string"}, {"unknown", "time.Time", "string"}, {"slice", "[]int64", "[]int64"},
	} {
		if got := normalizeFieldType(test.value); got != test.want {
			t.Errorf("normalizeFieldType(%q) = %q", test.value, got)
		}
	}
	if err := validateContextDefaults(ContextDefaultsConfig{ScalarTypes: map[string]string{"bad": "string"}}); err == nil {
		t.Fatal("invalid context default accepted")
	}
	if got := applyContextDefaults("int64", ContextDefaultsConfig{ScalarTypes: map[string]string{"int": "string"}}); got != "string" {
		t.Fatalf("applyContextDefaults() = %q", got)
	}
}

func TestIRGeneratedSourceIsParseable(t *testing.T) {
	source, err := CompileIR(&irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}, "off": {Kind: &irv1.VariantValue_BoolValue{BoolValue: false}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "off"}}}}}}}}, Config{PackageName: "generated", InferSDKFallback: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
		t.Fatalf("generated source is not parseable: %v", err)
	}
	if _, err := CompileIR(&irv1.Document{}, Config{}); err == nil || !strings.Contains(err.Error(), "FFCRAFT_IR_INVALID_CORE") {
		t.Fatalf("invalid IR error = %v", err)
	}
}

func TestIRGeneratedVariantKinds(t *testing.T) {
	tests := []struct {
		name     string
		variants map[string]*irv1.VariantValue
		defaultV string
	}{
		{"string", map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_StringValue{StringValue: "on"}}, "off": {Kind: &irv1.VariantValue_StringValue{StringValue: "off"}}}, "off"},
		{"int", map[string]*irv1.VariantValue{"one": {Kind: &irv1.VariantValue_IntValue{IntValue: 1}}, "zero": {Kind: &irv1.VariantValue_IntValue{IntValue: 0}}}, "zero"},
		{"float", map[string]*irv1.VariantValue{"one": {Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 1.5}}, "zero": {Kind: &irv1.VariantValue_DoubleValue{DoubleValue: 0}}}, "zero"},
		{"object", map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{"enabled": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}}}}}, "off": {Kind: &irv1.VariantValue_ObjectValue{ObjectValue: &irv1.VariantObject{Fields: map[string]*irv1.VariantValue{}}}}}, "off"},
		{"list", map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{Values: []*irv1.VariantValue{{Kind: &irv1.VariantValue_StringValue{StringValue: "x"}}}}}}, "off": {Kind: &irv1.VariantValue_ListValue{ListValue: &irv1.VariantList{}}}}, "off"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: test.variants, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: test.defaultV}}}}}}}}
			if _, err := CompileIR(doc, Config{PackageName: "generated", InferSDKFallback: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestIRSupportTables(t *testing.T) {
	for _, value := range []string{"string", "bool", "int", "int64", "float64", "[]string", "[]bool", "[]int", "[]int64", "[]float64", "[]any", "map[string]any"} {
		if !isSupportedFieldType(value) {
			t.Errorf("supported field type rejected: %q", value)
		}
	}
	if isSupportedFieldType("unknown") {
		t.Fatal("unknown field type accepted")
	}
	for _, value := range []string{"string", "bool", "int", "int64", "float64", "unknown"} {
		_ = inferredScalarTypeKey(value)
	}
	for _, value := range []string{"[]string", "[]bool", "[]int", "[]int64", "[]float64", "[]any", "unknown"} {
		_ = inferredCollectionTypeKey(value)
	}
	flags := []compiledFlag{{Kind: flagKindObject, RequiresTargetingKey: true, TargetingKeyPaths: []string{"user.id"}}}
	if !hasErrors(flags) || !hasCollectionFlags(flags) || !hasRequiredTargetingKey(flags) || !hasTargetingKeyPaths(flags) {
		t.Fatal("compiled flag aggregate helpers failed")
	}
	if got := toExportedName("user.ids"); got != "UserIDs" {
		t.Fatalf("toExportedName() = %q", got)
	}
	if err := validateContextDefaults(ContextDefaultsConfig{CollectionTypes: map[string]string{"any": "[]any"}}); err != nil {
		t.Fatal(err)
	}
}

func TestIRContextFieldsAndScheduledTargeting(t *testing.T) {
	attr := func(path ...string) *irv1.AttributePath { return &irv1.AttributePath{Segments: path} }
	serve := &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}
	condition := &irv1.Condition{Kind: &irv1.Condition_Equality{Equality: &irv1.EqualityCondition{Attribute: attr("user", "name"), Literal: &irv1.ScalarValue{Kind: &irv1.ScalarValue_StringValue{StringValue: "x"}}}}}
	distribute := &irv1.Action{Kind: &irv1.Action_Distribute{Distribute: &irv1.Distribution{AllocationKey: attr("device", "id"), Weights: map[string]uint32{"on": 1}}}}
	doc := &irv1.Document{Flags: map[string]*irv1.Flag{"f": {Variants: map[string]*irv1.VariantValue{"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}}}, Environments: map[string]*irv1.Environment{"prod": {Base: &irv1.Evaluation{Rules: []*irv1.Rule{{Condition: condition, Action: serve}}, DefaultAction: serve}, Schedule: []*irv1.ScheduledEvaluation{{Evaluation: &irv1.Evaluation{Rules: []*irv1.Rule{{Condition: condition, Action: distribute}}, DefaultAction: distribute}}}}}}}}
	fields, err := collectIRContextFields(doc, ContextDefaultsConfig{ScalarTypes: map[string]string{"string": "[]string"}}, []ContextFieldConfig{{Path: "extra", Name: "Extra", Type: "bool"}})
	if err != nil || len(fields) != 2 {
		t.Fatalf("collectIRContextFields() = %#v, %v", fields, err)
	}
	if !flagIRUsesContext(doc.Flags["f"]) || !flagIRRequiresTargetingKey(doc.Flags["f"]) {
		t.Fatal("scheduled IR context/targeting detection failed")
	}
	if !strings.Contains(strings.Join(flagIRTargetingKeyPaths(doc.Flags["f"]), ","), "device.id") {
		t.Fatalf("scheduled targeting paths = %#v", flagIRTargetingKeyPaths(doc.Flags["f"]))
	}
	if got := irScalarType(&irv1.ScalarValue{Kind: &irv1.ScalarValue_BoolValue{BoolValue: true}}); got != "bool" {
		t.Fatalf("irScalarType() = %q", got)
	}
}

func TestIRBoundaryDiagnostics(t *testing.T) {
	if _, err := collectIRContextFields(&irv1.Document{}, ContextDefaultsConfig{ScalarTypes: map[string]string{"string": "unsupported"}}, nil); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("invalid defaults error = %v", err)
	}
	if _, err := collectIRContextFields(&irv1.Document{}, ContextDefaultsConfig{}, []ContextFieldConfig{{Path: "x", Type: "unsupported"}}); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("invalid override error = %v", err)
	}
	if irEvaluationUsesContext(nil) || irConditionUsesContext(nil) {
		t.Fatal("nil IR context unexpectedly detected")
	}
	if got := flagIRTargetingKeyPaths(&irv1.Flag{}); len(got) != 0 {
		t.Fatalf("empty targeting paths = %#v", got)
	}
	collectIRContextFieldsFromCondition(nil, func(string, string) { t.Fatal("nil condition emitted context") })
	if _, err := formatGeneratedGo([]byte("package broken\nfunc {")); err == nil || !strings.Contains(err.Error(), "format generated Go") {
		t.Fatalf("invalid generated source error = %v", err)
	}
}
