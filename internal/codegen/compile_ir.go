package codegen

import (
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"
	"text/template"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/ir"
)

func compileIRDocument(doc *irv1.Document, cfg Config) ([]byte, error) {
	if err := ir.Validate(doc); err != nil {
		return nil, err
	}
	if cfg.PackageName == "" {
		return nil, fmt.Errorf("package name is required")
	}
	if cfg.ContextType == "" {
		cfg.ContextType = "EvalContext"
	}
	if cfg.ClientType == "" {
		cfg.ClientType = "Client"
	}
	if cfg.EvaluatorType == "" {
		cfg.EvaluatorType = "Evaluator"
	}
	contextFields, err := collectIRContextFields(doc, cfg.ContextDefaults, cfg.ContextFields)
	if err != nil {
		return nil, err
	}
	flags := make([]compiledFlag, 0, len(doc.Flags))
	for key, source := range doc.Flags {
		compiled, err := compileIRFlag(key, source, cfg.Accessors[key])
		if err != nil {
			return nil, fmt.Errorf("flag %q: %w", key, err)
		}
		flags = append(flags, compiled)
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
	data := templateData{
		PackageName:             cfg.PackageName,
		ContextType:             cfg.ContextType,
		ClientType:              cfg.ClientType,
		EvaluatorType:           cfg.EvaluatorType,
		ContextFields:           contextFields,
		HasAttributePaths:       len(contextFields) > 0 || hasTargetingKeyPaths(flags),
		HasContextFields:        len(contextFields) > 0,
		HasErrors:               hasErrors(flags),
		HasCollectionFlags:      hasCollectionFlags(flags),
		HasTargetingKeyPaths:    hasTargetingKeyPaths(flags),
		HasRequiredTargetingKey: hasRequiredTargetingKey(flags),
		Flags:                   flags,
	}
	return renderTemplate(data)
}

func renderTemplate(data templateData) ([]byte, error) {
	tmpl, err := templateForCodegen()
	if err != nil {
		return nil, err
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute generated Go template: %w", err)
	}
	return formatGeneratedGo([]byte(buf.String()))
}

func templateForCodegen() (*template.Template, error) {
	return template.New("go").Funcs(template.FuncMap{
		"isStringFlag": func(kind flagKind) bool { return kind == flagKindString },
		"isBoolFlag":   func(kind flagKind) bool { return kind == flagKindBool },
		"isIntFlag":    func(kind flagKind) bool { return kind == flagKindInt },
		"isFloatFlag":  func(kind flagKind) bool { return kind == flagKindFloat },
		"isObjectFlag": func(kind flagKind) bool { return kind == flagKindObject },
		"isListFlag":   func(kind flagKind) bool { return kind == flagKindList },
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict requires an even number of values")
			}
			out := make(map[string]any, len(values)/2)
			for index := 0; index < len(values); index += 2 {
				key, ok := values[index].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				out[key] = values[index+1]
			}
			return out, nil
		},
	}).Parse(fileTemplate)
}

func formatGeneratedGo(source []byte) ([]byte, error) {
	formatted, err := format.Source(source)
	if err != nil {
		return nil, fmt.Errorf("format generated Go: %w", err)
	}
	return formatted, nil
}

func compileIRFlag(key string, source *irv1.Flag, accessor AccessorConfig) (compiledFlag, error) {
	if source == nil || len(source.Variants) == 0 {
		return compiledFlag{}, fmt.Errorf("variants are required")
	}
	if len(source.Environments) == 0 {
		return compiledFlag{}, fmt.Errorf("environments are required")
	}
	variantKind, err := variantKindIR(source.Variants)
	if err != nil {
		return compiledFlag{}, err
	}
	variantNames := make([]string, 0, len(source.Variants))
	for name := range source.Variants {
		variantNames = append(variantNames, name)
	}
	sort.Strings(variantNames)
	baseEnvironment := firstIREnvironment(source.Environments)
	serve, ok := baseEnvironment.Base.DefaultAction.GetKind().(*irv1.Action_Serve)
	if !ok {
		return compiledFlag{}, fmt.Errorf("codegen requires a base default_action.serve")
	}
	for environmentName, environment := range source.Environments {
		other, ok := environment.Base.DefaultAction.GetKind().(*irv1.Action_Serve)
		if !ok || other.Serve != serve.Serve {
			return compiledFlag{}, fmt.Errorf("codegen cannot represent environment-specific default variants for flag %q: environment %q serves %q, want %q", key, environmentName, other.Serve, serve.Serve)
		}
	}
	defaultValue, ok := source.Variants[serve.Serve]
	if !ok {
		return compiledFlag{}, fmt.Errorf("default variant %q not found", serve.Serve)
	}
	accessorName := accessor.Name
	if accessorName == "" {
		accessorName = toExportedName(key)
	}
	compiled := compiledFlag{
		Key:                  key,
		AccessorName:         accessorName,
		ConstName:            "Flag" + accessorName,
		DefaultVariant:       serve.Serve,
		DefaultLiteral:       goLiteralIR(defaultValue),
		Kind:                 variantKind,
		UsesContext:          flagIRUsesContext(source),
		RequiresTargetingKey: flagIRRequiresTargetingKey(source),
		TargetingKeyPaths:    flagIRTargetingKeyPaths(source),
	}
	if variantKind == flagKindString {
		variantType := accessor.VariantType
		if variantType == "" {
			variantType = accessorName + "Variant"
		}
		compiled.VariantType = variantType
		for _, name := range variantNames {
			compiled.Variants = append(compiled.Variants, compiledVariant{Name: name, ConstName: variantType + toExportedName(name)})
		}
	}
	return compiled, nil
}

func firstIREnvironment(environments map[string]*irv1.Environment) *irv1.Environment {
	keys := make([]string, 0, len(environments))
	for key := range environments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return environments[keys[0]]
}

func variantKindIR(variants map[string]*irv1.VariantValue) (flagKind, error) {
	var kind flagKind
	first := true
	for name, value := range variants {
		current, ok := variantKindOfIR(value)
		if !ok {
			return 0, fmt.Errorf("variant %q has unsupported type", name)
		}
		if first {
			kind, first = current, false
		} else if kind != current {
			return 0, fmt.Errorf("variants are not homogeneous")
		}
	}
	return kind, nil
}

func variantKindOfIR(value *irv1.VariantValue) (flagKind, bool) {
	switch value.GetKind().(type) {
	case *irv1.VariantValue_BoolValue:
		return flagKindBool, true
	case *irv1.VariantValue_StringValue:
		return flagKindString, true
	case *irv1.VariantValue_IntValue:
		return flagKindInt, true
	case *irv1.VariantValue_DoubleValue:
		return flagKindFloat, true
	case *irv1.VariantValue_ObjectValue:
		return flagKindObject, true
	case *irv1.VariantValue_ListValue:
		return flagKindList, true
	default:
		return 0, false
	}
}

func goLiteralIR(value *irv1.VariantValue) string {
	switch kind := value.GetKind().(type) {
	case *irv1.VariantValue_BoolValue:
		return strconv.FormatBool(kind.BoolValue)
	case *irv1.VariantValue_StringValue:
		return strconv.Quote(kind.StringValue)
	case *irv1.VariantValue_IntValue:
		return strconv.FormatInt(kind.IntValue, 10)
	case *irv1.VariantValue_DoubleValue:
		return strconv.FormatFloat(kind.DoubleValue, 'g', -1, 64)
	case *irv1.VariantValue_NullValue:
		return "nil"
	case *irv1.VariantValue_ObjectValue:
		keys := make([]string, 0, len(kind.ObjectValue.Fields))
		for key := range kind.ObjectValue.Fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		items := make([]string, 0, len(keys))
		for _, key := range keys {
			items = append(items, strconv.Quote(key)+": "+goLiteralIR(kind.ObjectValue.Fields[key]))
		}
		return "map[string]any{" + strings.Join(items, ", ") + "}"
	case *irv1.VariantValue_ListValue:
		items := make([]string, 0, len(kind.ListValue.Values))
		for _, item := range kind.ListValue.Values {
			items = append(items, goLiteralIR(item))
		}
		return "[]any{" + strings.Join(items, ", ") + "}"
	default:
		return "nil"
	}
}

func collectIRContextFields(doc *irv1.Document, defaults ContextDefaultsConfig, overrides []ContextFieldConfig) ([]compiledContextField, error) {
	if err := validateContextDefaults(defaults); err != nil {
		return nil, err
	}
	byPath := make(map[string]compiledContextField)
	order := make([]string, 0)
	add := func(path, inferredType string) {
		if path == "" {
			return
		}
		if _, exists := byPath[path]; exists {
			return
		}
		byPath[path] = compiledContextField{
			Path:      path,
			FieldName: toExportedName(strings.ReplaceAll(path, ".", "_")),
			FieldType: normalizeFieldType(applyContextDefaults(inferredType, defaults)),
		}
		order = append(order, path)
	}
	for _, source := range doc.Flags {
		for _, environment := range source.Environments {
			collectIRContextFieldsFromEvaluation(environment.Base, add)
			for _, scheduled := range environment.Schedule {
				collectIRContextFieldsFromEvaluation(scheduled.Evaluation, add)
			}
		}
	}
	for _, override := range overrides {
		if override.Path == "" {
			return nil, fmt.Errorf("context.fields.path is required")
		}
		field := byPath[override.Path]
		if field.Path == "" {
			field = compiledContextField{Path: override.Path, FieldName: toExportedName(strings.ReplaceAll(override.Path, ".", "_")), FieldType: "string"}
			order = append(order, override.Path)
		}
		if override.Name != "" {
			field.FieldName = override.Name
		}
		if override.Type != "" {
			if !isSupportedFieldType(override.Type) {
				return nil, fmt.Errorf("context.fields[%q].type: unsupported type %q", override.Path, override.Type)
			}
			field.FieldType = normalizeFieldType(override.Type)
		}
		byPath[override.Path] = field
	}
	fields := make([]compiledContextField, 0, len(order))
	for _, path := range order {
		fields = append(fields, byPath[path])
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
	return fields, nil
}

func collectIRContextFieldsFromEvaluation(evaluation *irv1.Evaluation, add func(string, string)) {
	if evaluation == nil {
		return
	}
	for _, rule := range evaluation.Rules {
		collectIRContextFieldsFromCondition(rule.Condition, add)
	}
}

func collectIRContextFieldsFromCondition(condition *irv1.Condition, add func(string, string)) {
	if condition == nil {
		return
	}
	path := func(value *irv1.AttributePath) string {
		if value == nil {
			return ""
		}
		return strings.Join(value.Segments, ".")
	}
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_Equality:
		add(path(kind.Equality.Attribute), irScalarType(kind.Equality.Literal))
	case *irv1.Condition_NumericComparison:
		if kind.NumericComparison.Literal != nil {
			if _, ok := kind.NumericComparison.Literal.GetKind().(*irv1.NumericValue_IntValue); ok {
				add(path(kind.NumericComparison.Attribute), "int64")
			} else {
				add(path(kind.NumericComparison.Attribute), "float")
			}
		}
	case *irv1.Condition_Membership:
		if kind.Membership.Literals != nil && len(kind.Membership.Literals.Values) > 0 {
			add(path(kind.Membership.Attribute), irScalarType(kind.Membership.Literals.Values[0]))
		}
	case *irv1.Condition_StringMatch:
		add(path(kind.StringMatch.Attribute), "string")
	case *irv1.Condition_SemverComparison:
		add(path(kind.SemverComparison.Attribute), "string")
	case *irv1.Condition_Presence:
		add(path(kind.Presence.Attribute), "any")
	case *irv1.Condition_Logical:
		for _, child := range kind.Logical.Conditions {
			collectIRContextFieldsFromCondition(child, add)
		}
	case *irv1.Condition_Negation:
		collectIRContextFieldsFromCondition(kind.Negation, add)
	}
}

func irScalarType(value *irv1.ScalarValue) string {
	switch value.GetKind().(type) {
	case *irv1.ScalarValue_BoolValue:
		return "bool"
	case *irv1.ScalarValue_IntValue:
		return "int64"
	case *irv1.ScalarValue_DoubleValue:
		return "float"
	case *irv1.ScalarValue_StringValue:
		return "string"
	default:
		return "any"
	}
}

func flagIRUsesContext(source *irv1.Flag) bool {
	for _, environment := range source.Environments {
		if irEvaluationUsesContext(environment.Base) {
			return true
		}
		for _, scheduled := range environment.Schedule {
			if irEvaluationUsesContext(scheduled.Evaluation) {
				return true
			}
		}
	}
	return false
}

func irEvaluationUsesContext(evaluation *irv1.Evaluation) bool {
	if evaluation == nil {
		return false
	}
	for _, rule := range evaluation.Rules {
		if rule != nil && irConditionUsesContext(rule.Condition) {
			return true
		}
	}
	return false
}

func irConditionUsesContext(condition *irv1.Condition) bool {
	if condition == nil {
		return false
	}
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_Constant:
		return false
	case *irv1.Condition_Logical:
		for _, child := range kind.Logical.Conditions {
			if irConditionUsesContext(child) {
				return true
			}
		}
		return false
	case *irv1.Condition_Negation:
		return irConditionUsesContext(kind.Negation)
	default:
		return true
	}
}

func flagIRTargetingKeyPaths(source *irv1.Flag) []string {
	paths := make(map[string]struct{})
	for _, environment := range source.Environments {
		collectIRActionKey(environment.Base.DefaultAction, paths)
		for _, rule := range environment.Base.Rules {
			collectIRActionKey(rule.Action, paths)
		}
		for _, scheduled := range environment.Schedule {
			collectIRActionKey(scheduled.Evaluation.DefaultAction, paths)
			for _, rule := range scheduled.Evaluation.Rules {
				collectIRActionKey(rule.Action, paths)
			}
		}
	}
	out := make([]string, 0, len(paths))
	for path := range paths {
		if path != "" && path != "targetingKey" {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func collectIRActionKey(action *irv1.Action, paths map[string]struct{}) {
	if action == nil {
		return
	}
	if distribute, ok := action.GetKind().(*irv1.Action_Distribute); ok && distribute.Distribute != nil && distribute.Distribute.AllocationKey != nil {
		paths[strings.Join(distribute.Distribute.AllocationKey.Segments, ".")] = struct{}{}
	}
}

func flagIRRequiresTargetingKey(source *irv1.Flag) bool {
	return len(flagIRTargetingKeyPaths(source)) > 0
}
