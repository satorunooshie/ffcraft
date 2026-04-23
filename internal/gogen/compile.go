package gogen

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/satorunooshie/ffcraft/internal/ast"
)

type Config struct {
	PackageName   string
	ContextType   string
	ClientType    string
	EvaluatorType string
	ContextFields []ContextFieldConfig
	Accessors     map[string]AccessorConfig
}

type CompileOptions struct {
	AllowMissingEnvironment bool
	PackageName             string
}

type AccessorConfig struct {
	Name        string `yaml:"name"`
	VariantType string `yaml:"variant_type"`
}

type ContextFieldConfig struct {
	Path string `yaml:"path"`
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type flagKind int

const (
	flagKindBool flagKind = iota
	flagKindString
	flagKindInt
	flagKindFloat
	flagKindObject
	flagKindList
)

type compiledVariant struct {
	Name      string
	ConstName string
}

type compiledFlag struct {
	Key                  string
	AccessorName         string
	ConstName            string
	DefaultVariant       string
	DefaultValue         ast.VariantValue
	Kind                 flagKind
	VariantType          string
	Variants             []compiledVariant
	UsesContext          bool
	RequiresTargetingKey bool
	TargetingKeyPaths    []string
}

type compiledContextField struct {
	Path      string
	FieldName string
	FieldType string
}

type templateData struct {
	PackageName             string
	ContextType             string
	ClientType              string
	EvaluatorType           string
	ContextFields           []compiledContextField
	HasAttributePaths       bool
	HasContextFields        bool
	HasErrors               bool
	HasCollectionFlags      bool
	HasTargetingKeyPaths    bool
	HasRequiredTargetingKey bool
	Flags                   []compiledFlag
}

func Compile(doc *ast.Document, cfg Config) ([]byte, error) {
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

	contextFields, err := collectContextFields(doc, cfg.ContextFields)
	if err != nil {
		return nil, err
	}
	flags, err := collectFlags(doc, cfg.Accessors)
	if err != nil {
		return nil, err
	}

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

	tmpl, err := template.New("go").Funcs(template.FuncMap{
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
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				out[key] = values[i+1]
			}
			return out, nil
		},
		"goLiteral": goLiteral,
	}).Parse(fileTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated Go: %w", err)
	}
	return formatted, nil
}

func CompileWithOptions(doc *ast.Document, environment string, opts CompileOptions) ([]byte, []string, error) {
	filtered, warnings, err := filterEnvironment(doc, environment, opts.AllowMissingEnvironment)
	if err != nil {
		return nil, nil, err
	}
	output, err := Compile(filtered, Config{PackageName: opts.PackageName})
	if err != nil {
		return nil, nil, err
	}
	return output, warnings, nil
}

func collectFlags(doc *ast.Document, accessors map[string]AccessorConfig) ([]compiledFlag, error) {
	flags := make([]compiledFlag, 0, len(doc.Flags))
	for _, src := range doc.Flags {
		compiled, err := compileFlag(src, accessors[src.Key])
		if err != nil {
			return nil, fmt.Errorf("flag %q: %w", src.Key, err)
		}
		flags = append(flags, compiled)
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
	return flags, nil
}

func collectContextFields(doc *ast.Document, overrides []ContextFieldConfig) ([]compiledContextField, error) {
	byPath := map[string]compiledContextField{}
	order := make([]string, 0)

	add := func(path, inferredType string) {
		if path == "" {
			return
		}
		if _, ok := byPath[path]; ok {
			return
		}
		byPath[path] = compiledContextField{
			Path:      path,
			FieldName: toExportedName(strings.ReplaceAll(path, ".", "_")),
			FieldType: normalizeFieldType(inferredType),
		}
		order = append(order, path)
	}

	for _, flag := range doc.Flags {
		for _, env := range flag.Environments {
			collectContextFieldsFromEnvironment(env, add)
		}
	}

	for _, override := range overrides {
		if override.Path == "" {
			return nil, fmt.Errorf("context.fields.path is required")
		}
		field, ok := byPath[override.Path]
		if !ok {
			field = compiledContextField{
				Path:      override.Path,
				FieldName: toExportedName(strings.ReplaceAll(override.Path, ".", "_")),
				FieldType: "string",
			}
			order = append(order, override.Path)
		}
		if override.Name != "" {
			field.FieldName = override.Name
		}
		if override.Type != "" {
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

func collectContextFieldsFromEnvironment(env *ast.Environment, add func(path, inferredType string)) {
	for _, rule := range env.Rules {
		collectContextFieldsFromCondition(rule.Condition, add)
	}
	for _, step := range env.ScheduledRollouts {
		for _, rule := range step.Rules {
			collectContextFieldsFromCondition(rule.Condition, add)
		}
	}
}

func collectContextFieldsFromCondition(cond ast.Condition, add func(path, inferredType string)) {
	switch c := cond.(type) {
	case *ast.Eq:
		collectContextFieldsFromBinary(c.Left, c.Right, add)
	case *ast.Ne:
		collectContextFieldsFromBinary(c.Left, c.Right, add)
	case *ast.Gt:
		collectContextFieldsFromBinary(c.Left, c.Right, add)
	case *ast.Gte:
		collectContextFieldsFromBinary(c.Left, c.Right, add)
	case *ast.Lt:
		collectContextFieldsFromBinary(c.Left, c.Right, add)
	case *ast.Lte:
		collectContextFieldsFromBinary(c.Left, c.Right, add)
	case *ast.In:
		collectContextFieldsFromBinary(c.Target, c.Candidate, add)
	case *ast.Contains:
		collectContextFieldsFromBinary(c.Container, c.Value, add)
	case *ast.StartsWith:
		if v, ok := c.Target.(*ast.Var); ok {
			add(v.Path, "string")
		}
	case *ast.EndsWith:
		if v, ok := c.Target.(*ast.Var); ok {
			add(v.Path, "string")
		}
	case *ast.Matches:
		if v, ok := c.Target.(*ast.Var); ok {
			add(v.Path, "string")
		}
	case *ast.SemverGt:
		if v, ok := c.Left.(*ast.Var); ok {
			add(v.Path, "string")
		}
	case *ast.SemverGte:
		if v, ok := c.Left.(*ast.Var); ok {
			add(v.Path, "string")
		}
	case *ast.SemverLt:
		if v, ok := c.Left.(*ast.Var); ok {
			add(v.Path, "string")
		}
	case *ast.SemverLte:
		if v, ok := c.Left.(*ast.Var); ok {
			add(v.Path, "string")
		}
	case *ast.AllOf:
		for _, next := range c.Conditions {
			collectContextFieldsFromCondition(next, add)
		}
	case *ast.AnyOf:
		for _, next := range c.Conditions {
			collectContextFieldsFromCondition(next, add)
		}
	case *ast.OneOf:
		for _, next := range c.Conditions {
			collectContextFieldsFromCondition(next, add)
		}
	case *ast.Not:
		collectContextFieldsFromCondition(c.Condition, add)
	}
}

func collectContextFieldsFromBinary(left, right ast.Value, add func(path, inferredType string)) {
	if v, ok := left.(*ast.Var); ok {
		add(v.Path, inferValueType(right, v.Path))
	}
	if v, ok := right.(*ast.Var); ok {
		add(v.Path, inferValueType(left, v.Path))
	}
}

func inferValueType(value ast.Value, path string) string {
	switch v := value.(type) {
	case *ast.Scalar:
		switch v.Kind {
		case ast.ScalarKindBool:
			return "bool"
		case ast.ScalarKindInt:
			return "int64"
		case ast.ScalarKindDouble:
			return "float64"
		default:
			return "string"
		}
	case *ast.List:
		for _, item := range v.Values {
			t := inferValueType(item, path)
			if t != "string" {
				return t
			}
		}
	}
	if strings.HasSuffix(path, ".id") {
		return "int64"
	}
	return "string"
}

func normalizeFieldType(value string) string {
	switch value {
	case "string", "bool", "int64", "float64":
		return value
	default:
		return "string"
	}
}

func filterEnvironment(doc *ast.Document, environment string, allowMissing bool) (*ast.Document, []string, error) {
	filtered := &ast.Document{Flags: make([]*ast.Flag, 0, len(doc.Flags))}
	warnings := make([]string, 0)
	for _, src := range doc.Flags {
		env, ok := src.Environments[environment]
		if !ok {
			if allowMissing {
				warnings = append(warnings, fmt.Sprintf("warning: skipping flag %q because environment %q is not defined", src.Key, environment))
				continue
			}
			return nil, nil, fmt.Errorf("flag %q: environment %q not found", src.Key, environment)
		}
		filtered.Flags = append(filtered.Flags, &ast.Flag{
			Key:            src.Key,
			Variants:       src.Variants,
			DefaultVariant: src.DefaultVariant,
			Metadata:       src.Metadata,
			Environments:   map[string]*ast.Environment{environment: env},
		})
	}
	return filtered, warnings, nil
}

func compileFlag(src *ast.Flag, cfg AccessorConfig) (compiledFlag, error) {
	if len(src.Variants) == 0 {
		return compiledFlag{}, fmt.Errorf("variants are required")
	}
	defaultValue, ok := src.Variants[src.DefaultVariant]
	if !ok {
		return compiledFlag{}, fmt.Errorf("default variant %q not found", src.DefaultVariant)
	}
	accessorName := cfg.Name
	if accessorName == "" {
		accessorName = toExportedName(src.Key)
	}
	kind, err := variantSetKind(src)
	if err != nil {
		return compiledFlag{}, err
	}
	base := compiledFlag{
		Key:                  src.Key,
		AccessorName:         accessorName,
		ConstName:            "Flag" + accessorName,
		DefaultVariant:       src.DefaultVariant,
		DefaultValue:         defaultValue,
		Kind:                 kind,
		UsesContext:          flagUsesContext(src),
		RequiresTargetingKey: flagRequiresTargetingKey(src),
		TargetingKeyPaths:    flagTargetingKeyPaths(src),
	}
	switch kind {
	case flagKindBool:
		return base, nil
	case flagKindInt:
		return base, nil
	case flagKindFloat:
		return base, nil
	case flagKindObject:
		return base, nil
	case flagKindList:
		return base, nil
	case flagKindString:
		variantType := cfg.VariantType
		if variantType == "" {
			variantType = accessorName + "Variant"
		}
		variantNames := make([]string, 0, len(src.Variants))
		for name := range src.Variants {
			variantNames = append(variantNames, name)
		}
		sort.Strings(variantNames)
		variants := make([]compiledVariant, 0, len(variantNames))
		for _, name := range variantNames {
			variants = append(variants, compiledVariant{Name: name, ConstName: variantType + toExportedName(name)})
		}
		base.VariantType = variantType
		base.Variants = variants
		return base, nil
	default:
		return compiledFlag{}, fmt.Errorf("unsupported flag kind")
	}
}

func variantSetKind(src *ast.Flag) (flagKind, error) {
	if isBoolVariantSet(src) {
		return flagKindBool, nil
	}
	if isIntVariantSet(src) {
		return flagKindInt, nil
	}
	if isFloatVariantSet(src) {
		return flagKindFloat, nil
	}
	if isObjectVariantSet(src) {
		return flagKindObject, nil
	}
	if isListVariantSet(src) {
		return flagKindList, nil
	}
	if isStringVariantSet(src) {
		return flagKindString, nil
	}
	return 0, fmt.Errorf("go target supports only boolean, string, int, float, object, and list variant sets")
}

func hasRequiredTargetingKey(flags []compiledFlag) bool {
	for _, flag := range flags {
		if flag.RequiresTargetingKey {
			return true
		}
	}
	return false
}

func hasTargetingKeyPaths(flags []compiledFlag) bool {
	for _, flag := range flags {
		if len(flag.TargetingKeyPaths) > 0 {
			return true
		}
	}
	return false
}

func hasErrors(flags []compiledFlag) bool {
	for _, flag := range flags {
		if flag.RequiresTargetingKey || flag.Kind == flagKindObject || flag.Kind == flagKindList {
			return true
		}
	}
	return false
}

func hasCollectionFlags(flags []compiledFlag) bool {
	for _, flag := range flags {
		if flag.Kind == flagKindObject || flag.Kind == flagKindList {
			return true
		}
	}
	return false
}

func flagUsesContext(flag *ast.Flag) bool {
	for _, env := range flag.Environments {
		if environmentUsesContext(env) {
			return true
		}
	}
	return false
}

func environmentUsesContext(env *ast.Environment) bool {
	for _, rule := range env.Rules {
		if conditionUsesContext(rule.Condition) {
			return true
		}
	}
	for _, step := range env.ScheduledRollouts {
		for _, rule := range step.Rules {
			if conditionUsesContext(rule.Condition) {
				return true
			}
		}
	}
	return false
}

func conditionUsesContext(cond ast.Condition) bool {
	switch c := cond.(type) {
	case *ast.Eq:
		return valueUsesContext(c.Left) || valueUsesContext(c.Right)
	case *ast.Ne:
		return valueUsesContext(c.Left) || valueUsesContext(c.Right)
	case *ast.Gt:
		return valueUsesContext(c.Left) || valueUsesContext(c.Right)
	case *ast.Gte:
		return valueUsesContext(c.Left) || valueUsesContext(c.Right)
	case *ast.Lt:
		return valueUsesContext(c.Left) || valueUsesContext(c.Right)
	case *ast.Lte:
		return valueUsesContext(c.Left) || valueUsesContext(c.Right)
	case *ast.In:
		return valueUsesContext(c.Target) || valueUsesContext(c.Candidate)
	case *ast.Contains:
		return valueUsesContext(c.Container) || valueUsesContext(c.Value)
	case *ast.StartsWith:
		return valueUsesContext(c.Target)
	case *ast.EndsWith:
		return valueUsesContext(c.Target)
	case *ast.Matches:
		return valueUsesContext(c.Target)
	case *ast.SemverGt:
		return valueUsesContext(c.Left)
	case *ast.SemverGte:
		return valueUsesContext(c.Left)
	case *ast.SemverLt:
		return valueUsesContext(c.Left)
	case *ast.SemverLte:
		return valueUsesContext(c.Left)
	case *ast.AllOf:
		return slices.ContainsFunc(c.Conditions, conditionUsesContext)
	case *ast.AnyOf:
		return slices.ContainsFunc(c.Conditions, conditionUsesContext)
	case *ast.OneOf:
		return slices.ContainsFunc(c.Conditions, conditionUsesContext)
	case *ast.Not:
		return conditionUsesContext(c.Condition)
	}
	return false
}

func valueUsesContext(value ast.Value) bool {
	switch v := value.(type) {
	case *ast.Var:
		return v.Path != ""
	case *ast.List:
		return slices.ContainsFunc(v.Values, valueUsesContext)
	}
	return false
}

func flagRequiresTargetingKey(flag *ast.Flag) bool {
	for _, env := range flag.Environments {
		if environmentRequiresTargetingKey(env) {
			return true
		}
	}
	return false
}

func environmentRequiresTargetingKey(env *ast.Environment) bool {
	if actionRequiresTargetingKey(env.DefaultAction) {
		return true
	}
	for _, rule := range env.Rules {
		if actionRequiresTargetingKey(rule.Action) {
			return true
		}
	}
	for _, step := range env.ScheduledRollouts {
		if actionRequiresTargetingKey(step.DefaultAction) {
			return true
		}
		for _, rule := range step.Rules {
			if actionRequiresTargetingKey(rule.Action) {
				return true
			}
		}
	}
	return false
}

func actionRequiresTargetingKey(action ast.Action) bool {
	switch action.(type) {
	case *ast.DistributeAction, *ast.ProgressiveRolloutAction:
		return true
	default:
		return false
	}
}

func flagTargetingKeyPaths(flag *ast.Flag) []string {
	paths := map[string]struct{}{}
	for _, env := range flag.Environments {
		collectTargetingKeyPathsFromEnvironment(env, paths)
	}
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func collectTargetingKeyPathsFromEnvironment(env *ast.Environment, paths map[string]struct{}) {
	collectTargetingKeyPathFromAction(env.DefaultAction, paths)
	for _, rule := range env.Rules {
		collectTargetingKeyPathFromAction(rule.Action, paths)
	}
	for _, step := range env.ScheduledRollouts {
		collectTargetingKeyPathFromAction(step.DefaultAction, paths)
		for _, rule := range step.Rules {
			collectTargetingKeyPathFromAction(rule.Action, paths)
		}
	}
}

func collectTargetingKeyPathFromAction(action ast.Action, paths map[string]struct{}) {
	switch v := action.(type) {
	case *ast.DistributeAction:
		addTargetingKeyPath(v.Stickiness, paths)
	case *ast.ProgressiveRolloutAction:
		addTargetingKeyPath(v.Stickiness, paths)
	}
}

func addTargetingKeyPath(path string, paths map[string]struct{}) {
	if path == "" || path == "targetingKey" {
		return
	}
	paths[path] = struct{}{}
}

func isBoolVariantSet(src *ast.Flag) bool {
	if len(src.Variants) == 0 {
		return false
	}
	for _, value := range src.Variants {
		if value.Kind != ast.VariantValueKindBool {
			return false
		}
	}
	return true
}

func isStringVariantSet(src *ast.Flag) bool {
	if len(src.Variants) == 0 {
		return false
	}
	for _, value := range src.Variants {
		if value.Kind != ast.VariantValueKindString {
			return false
		}
	}
	return true
}

func isIntVariantSet(src *ast.Flag) bool {
	if len(src.Variants) == 0 {
		return false
	}
	for _, value := range src.Variants {
		if value.Kind != ast.VariantValueKindInt {
			return false
		}
	}
	return true
}

func isFloatVariantSet(src *ast.Flag) bool {
	if len(src.Variants) == 0 {
		return false
	}
	for _, value := range src.Variants {
		if value.Kind != ast.VariantValueKindDouble {
			return false
		}
	}
	return true
}

func isObjectVariantSet(src *ast.Flag) bool {
	if len(src.Variants) == 0 {
		return false
	}
	for _, value := range src.Variants {
		if value.Kind != ast.VariantValueKindObject {
			return false
		}
	}
	return true
}

func isListVariantSet(src *ast.Flag) bool {
	if len(src.Variants) == 0 {
		return false
	}
	for _, value := range src.Variants {
		if value.Kind != ast.VariantValueKindList {
			return false
		}
	}
	return true
}

func boolDefault(defaultVariant string) string {
	switch strings.ToLower(defaultVariant) {
	case "on", "true":
		return "true"
	default:
		return "false"
	}
}

func goLiteral(value ast.VariantValue) string {
	switch value.Kind {
	case ast.VariantValueKindBool:
		if value.Bool {
			return "true"
		}
		return "false"
	case ast.VariantValueKindString:
		return strconv.Quote(value.String)
	case ast.VariantValueKindInt:
		return strconv.FormatInt(value.Int, 10)
	case ast.VariantValueKindDouble:
		return strconv.FormatFloat(value.Double, 'g', -1, 64)
	case ast.VariantValueKindObject:
		return goMapLiteral(value.Object)
	case ast.VariantValueKindList:
		return goSliceLiteral(value.List)
	case ast.VariantValueKindNull:
		return "nil"
	default:
		return "nil"
	}
}

func goMapLiteral(value map[string]any) string {
	if len(value) == 0 {
		return "map[string]any{}"
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		items = append(items, strconv.Quote(key)+": "+goAnyLiteral(value[key]))
	}
	return "map[string]any{" + strings.Join(items, ", ") + "}"
}

func goSliceLiteral(values []ast.VariantValue) string {
	if len(values) == 0 {
		return "[]any{}"
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, goLiteral(value))
	}
	return "[]any{" + strings.Join(items, ", ") + "}"
}

func goAnyLiteral(value any) string {
	switch v := value.(type) {
	case nil:
		return "nil"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case string:
		return strconv.Quote(v)
	case int:
		return strconv.FormatInt(int64(v), 10)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case map[string]any:
		return goMapLiteral(v)
	case []any:
		if len(v) == 0 {
			return "[]any{}"
		}
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, goAnyLiteral(item))
		}
		return "[]any{" + strings.Join(items, ", ") + "}"
	default:
		return "nil"
	}
}

func toExportedName(value string) string {
	parts := splitName(value)
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		upper := strings.ToUpper(part)
		switch upper {
		case "ID":
			b.WriteString(upper)
		default:
			b.WriteString(strings.ToUpper(part[:1]))
			if len(part) > 1 {
				b.WriteString(part[1:])
			}
		}
	}
	if b.Len() == 0 {
		return "Generated"
	}
	return b.String()
}

func splitName(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ' '
	})
}

//go:embed templates/go.tmpl
var fileTemplate string
