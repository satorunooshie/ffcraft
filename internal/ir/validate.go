package ir

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"sync"
	"unicode/utf8"

	"buf.build/go/protovalidate"
	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	validatorOnce sync.Once
	validator     protovalidate.Validator
	validatorErr  error
)

// Validate checks only target-independent IR invariants.
func Validate(doc *irv1.Document) error {
	if doc == nil {
		return fmt.Errorf("IR document is nil")
	}
	if err := rejectUnknownCore(doc); err != nil {
		return err
	}
	validatorOnce.Do(func() { validator, validatorErr = protovalidate.New() })
	if validatorErr != nil {
		return validatorErr
	}
	core := proto.Clone(doc).(*irv1.Document)
	core.Extensions = nil
	for _, flag := range core.Flags {
		if flag == nil {
			continue
		}
		flag.Extensions = nil
		for _, environment := range flag.Environments {
			if environment == nil {
				continue
			}
			environment.Extensions = nil
		}
	}
	if err := validator.Validate(core); err != nil {
		return &CoreValidationError{Code: "FFCRAFT_IR_INVALID_CORE", Path: "$", MessageType: string(core.ProtoReflect().Descriptor().FullName()), Cause: err}
	}
	if len(doc.Flags) == 0 {
		return fmt.Errorf("IR document has no flags")
	}
	for key, flag := range doc.Flags {
		if key == "" || flag == nil || len(flag.Variants) == 0 || len(flag.Environments) == 0 {
			return fmt.Errorf("flag %q is incomplete", key)
		}
		if err := validateExtensions(flag.Extensions); err != nil {
			return fmt.Errorf("flag %q extensions: %w", key, err)
		}
		for variant, value := range flag.Variants {
			if err := validateVariantDepth(value, 0); err != nil {
				return fmt.Errorf("flag %q variant %q: %w", key, variant, err)
			}
		}
		var variantKind string
		for variant, value := range flag.Variants {
			kind := fmt.Sprintf("%T", value.GetKind())
			if variantKind == "" {
				variantKind = kind
			} else if kind != variantKind {
				return fmt.Errorf("flag %q variants are not homogeneous: %q has %s, want %s", key, variant, kind, variantKind)
			}
		}
		for name, env := range flag.Environments {
			if err := validateEnvironment(env, flag.Variants); err != nil {
				return fmt.Errorf("flag %q environment %q: %w", key, name, err)
			}
		}
	}
	return validateExtensions(doc.Extensions)
}

func validateEnvironment(env *irv1.Environment, variants map[string]*irv1.VariantValue) error {
	if env == nil || env.Base == nil {
		return fmt.Errorf("base evaluation is required")
	}
	if err := validateEvaluation(env.Base, variants); err != nil {
		return err
	}
	var previous *timestamppb.Timestamp
	for _, scheduled := range env.Schedule {
		if scheduled == nil || scheduled.EffectiveAt == nil || scheduled.Evaluation == nil {
			return fmt.Errorf("schedule entry is incomplete")
		}
		if previous != nil && compareTimestamps(scheduled.EffectiveAt, previous) <= 0 {
			return fmt.Errorf("schedule timestamps must be strictly increasing")
		}
		previous = scheduled.EffectiveAt
		if err := validateEvaluation(scheduled.Evaluation, variants); err != nil {
			return err
		}
	}
	return validateExtensions(env.Extensions)
}

func compareTimestamps(a, b *timestamppb.Timestamp) int {
	if a.Seconds < b.Seconds {
		return -1
	}
	if a.Seconds > b.Seconds {
		return 1
	}
	if a.Nanos < b.Nanos {
		return -1
	}
	if a.Nanos > b.Nanos {
		return 1
	}
	return 0
}

func validateEvaluation(eval *irv1.Evaluation, variants map[string]*irv1.VariantValue) error {
	if eval == nil {
		return fmt.Errorf("evaluation is nil")
	}
	if err := validateAction(eval.DefaultAction, variants); err != nil {
		return fmt.Errorf("default action: %w", err)
	}
	for index, rule := range eval.Rules {
		if rule == nil {
			return fmt.Errorf("rule[%d] is nil", index)
		}
		if err := validateConditionDepth(rule.Condition, 0); err != nil {
			return fmt.Errorf("rule[%d] condition: %w", index, err)
		}
		if err := validateAction(rule.Action, variants); err != nil {
			return fmt.Errorf("rule[%d] action: %w", index, err)
		}
	}
	return nil
}

func validateAction(action *irv1.Action, variants map[string]*irv1.VariantValue) error {
	switch kind := action.GetKind().(type) {
	case *irv1.Action_Serve:
		if _, ok := variants[kind.Serve]; !ok {
			return fmt.Errorf("unknown serve variant %q", kind.Serve)
		}
	case *irv1.Action_Distribute:
		if kind.Distribute == nil || len(kind.Distribute.Weights) < 2 || kind.Distribute.AllocationKey == nil {
			return fmt.Errorf("invalid distribution")
		}
		var divisor uint32
		for name, weight := range kind.Distribute.Weights {
			if weight == 0 {
				return fmt.Errorf("distribution weight %q is zero", name)
			}
			if _, ok := variants[name]; !ok {
				return fmt.Errorf("unknown distribution variant %q", name)
			}
			if divisor == 0 {
				divisor = weight
			} else {
				divisor = gcd(divisor, weight)
			}
		}
		if divisor != 1 {
			return fmt.Errorf("distribution weights must be in GCD=1 form")
		}
	default:
		return fmt.Errorf("action kind is required")
	}
	return nil
}

func gcd(a, b uint32) uint32 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func validateCondition(condition *irv1.Condition) error { return validateConditionDepth(condition, 0) }
func validateConditionDepth(condition *irv1.Condition, depth int) error {
	if depth > 64 {
		return fmt.Errorf("condition nesting depth exceeds 64")
	}
	switch kind := condition.GetKind().(type) {
	case *irv1.Condition_Constant:
		return nil
	case *irv1.Condition_Equality:
		if kind.Equality == nil {
			return fmt.Errorf("equality condition is nil")
		}
		if !validEqualityOperator(kind.Equality.Operator) {
			return fmt.Errorf("unsupported equality operator %d", kind.Equality.Operator)
		}
		return validateAttributeLiteral(kind.Equality.Attribute, kind.Equality.Literal)
	case *irv1.Condition_NumericComparison:
		if kind.NumericComparison == nil {
			return fmt.Errorf("numeric comparison condition is nil")
		}
		if !validNumericComparisonOperator(kind.NumericComparison.Operator) {
			return fmt.Errorf("unsupported numeric comparison operator %d", kind.NumericComparison.Operator)
		}
		if kind.NumericComparison.Attribute == nil || kind.NumericComparison.Literal == nil {
			return fmt.Errorf("numeric comparison is incomplete")
		}
		if literal, ok := kind.NumericComparison.Literal.GetKind().(*irv1.NumericValue_DoubleValue); ok && (math.IsNaN(literal.DoubleValue) || math.IsInf(literal.DoubleValue, 0)) {
			return fmt.Errorf("numeric comparison literal is not finite")
		}
	case *irv1.Condition_Membership:
		if kind.Membership == nil {
			return fmt.Errorf("membership condition is nil")
		}
		if kind.Membership.Attribute == nil || kind.Membership.Literals == nil || len(kind.Membership.Literals.Values) == 0 {
			return fmt.Errorf("membership is incomplete")
		}
		if err := validateScalarHomogeneity(kind.Membership.Literals.Values); err != nil {
			return err
		}
	case *irv1.Condition_StringMatch:
		if kind.StringMatch == nil {
			return fmt.Errorf("string match condition is nil")
		}
		if !validStringMatchOperator(kind.StringMatch.Operator) {
			return fmt.Errorf("unsupported string match operator %d", kind.StringMatch.Operator)
		}
		if kind.StringMatch.Attribute == nil {
			return fmt.Errorf("string match attribute is required")
		}
	case *irv1.Condition_SemverComparison:
		if kind.SemverComparison == nil {
			return fmt.Errorf("semver comparison condition is nil")
		}
		if !validSemverComparisonOperator(kind.SemverComparison.Operator) {
			return fmt.Errorf("unsupported semver comparison operator %d", kind.SemverComparison.Operator)
		}
		if kind.SemverComparison.Attribute == nil || kind.SemverComparison.Semver == "" {
			return fmt.Errorf("semver comparison is incomplete")
		}
		if !semverPattern.MatchString(kind.SemverComparison.Semver) {
			return fmt.Errorf("invalid SemVer literal %q", kind.SemverComparison.Semver)
		}
	case *irv1.Condition_Presence:
		if kind.Presence == nil {
			return fmt.Errorf("presence condition is nil")
		}
		if kind.Presence.Attribute == nil {
			return fmt.Errorf("presence attribute is required")
		}
	case *irv1.Condition_Logical:
		if kind.Logical == nil {
			return fmt.Errorf("logical condition is nil")
		}
		if !validLogicalOperator(kind.Logical.Operator) {
			return fmt.Errorf("unsupported logical operator %d", kind.Logical.Operator)
		}
		if len(kind.Logical.Conditions) < 2 {
			return fmt.Errorf("logical condition needs at least two children")
		}
		for _, child := range kind.Logical.Conditions {
			if err := validateConditionDepth(child, depth+1); err != nil {
				return err
			}
		}
	case *irv1.Condition_Negation:
		return validateConditionDepth(kind.Negation, depth+1)
	default:
		return fmt.Errorf("condition kind is required")
	}
	return nil
}

func validEqualityOperator(operator irv1.EqualityOperator) bool {
	return operator == irv1.EqualityOperator_EQUALITY_OPERATOR_EQ || operator == irv1.EqualityOperator_EQUALITY_OPERATOR_NE
}

func validNumericComparisonOperator(operator irv1.NumericComparisonOperator) bool {
	return operator >= irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_GT && operator <= irv1.NumericComparisonOperator_NUMERIC_COMPARISON_OPERATOR_LTE
}

func validStringMatchOperator(operator irv1.StringMatchOperator) bool {
	return operator >= irv1.StringMatchOperator_STRING_MATCH_OPERATOR_CONTAINS && operator <= irv1.StringMatchOperator_STRING_MATCH_OPERATOR_ENDS_WITH
}

func validSemverComparisonOperator(operator irv1.SemVerComparisonOperator) bool {
	return operator >= irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_GT && operator <= irv1.SemVerComparisonOperator_SEM_VER_COMPARISON_OPERATOR_LTE
}

func validLogicalOperator(operator irv1.LogicalOperator) bool {
	return operator >= irv1.LogicalOperator_LOGICAL_OPERATOR_ALL && operator <= irv1.LogicalOperator_LOGICAL_OPERATOR_EXACTLY_ONE
}

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func validateScalarHomogeneity(values []*irv1.ScalarValue) error {
	domain := scalarDomain(values[0])
	for _, value := range values[1:] {
		if scalarDomain(value) != domain {
			return fmt.Errorf("membership literals must be homogeneous")
		}
	}
	return nil
}
func scalarDomain(value *irv1.ScalarValue) string {
	switch value.GetKind().(type) {
	case *irv1.ScalarValue_IntValue, *irv1.ScalarValue_DoubleValue:
		return "number"
	case *irv1.ScalarValue_StringValue:
		return "string"
	case *irv1.ScalarValue_BoolValue:
		return "bool"
	case *irv1.ScalarValue_NullValue:
		return "null"
	default:
		return "unknown"
	}
}

func validateAttributeLiteral(attribute *irv1.AttributePath, literal *irv1.ScalarValue) error {
	if attribute == nil || len(attribute.Segments) == 0 || literal == nil {
		return fmt.Errorf("attribute equality is incomplete")
	}
	return nil
}
func validateVariant(value *irv1.VariantValue) error { return validateVariantDepth(value, 0) }
func validateVariantDepth(value *irv1.VariantValue, depth int) error {
	if value == nil {
		return fmt.Errorf("value is nil")
	}
	if depth > 64 {
		return fmt.Errorf("variant nesting depth exceeds 64")
	}
	switch kind := value.GetKind().(type) {
	case *irv1.VariantValue_DoubleValue:
		if math.IsNaN(kind.DoubleValue) || math.IsInf(kind.DoubleValue, 0) {
			return fmt.Errorf("double is not finite")
		}
	case *irv1.VariantValue_ObjectValue:
		if kind.ObjectValue == nil {
			return fmt.Errorf("object value is nil")
		}
		for name, child := range kind.ObjectValue.Fields {
			if name == "" {
				return fmt.Errorf("object key is empty")
			}
			if err := validateVariantDepth(child, depth+1); err != nil {
				return err
			}
		}
	case *irv1.VariantValue_ListValue:
		if kind.ListValue == nil {
			return fmt.Errorf("list value is nil")
		}
		for _, child := range kind.ListValue.Values {
			if err := validateVariantDepth(child, depth+1); err != nil {
				return err
			}
		}
	case nil:
		return fmt.Errorf("value kind is required")
	}
	return nil
}
func validateExtensions(values map[string]*irv1.ExtensionValue) error {
	if len(values) > 256 {
		return fmt.Errorf("extension namespace count exceeds 256")
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "" || len(name) > 128 || !utf8.ValidString(name) || values[name] == nil {
			return fmt.Errorf("invalid namespace %q", name)
		}
		if err := validateExtensionDepth(values[name], 0); err != nil {
			return fmt.Errorf("namespace %q: %w", name, err)
		}
		if size := proto.Size(values[name]); size > 1<<20 {
			return fmt.Errorf("namespace %q payload exceeds 1048576 protobuf bytes", name)
		}
	}
	return nil
}
func validateExtensionDepth(value *irv1.ExtensionValue, depth int) error {
	if depth > 64 {
		return fmt.Errorf("extension nesting depth exceeds 64")
	}
	if value == nil {
		return fmt.Errorf("extension value is nil")
	}
	if value.GetKind() == nil && len(value.ProtoReflect().GetUnknown()) != 0 {
		return nil
	}
	switch kind := value.GetKind().(type) {
	case *irv1.ExtensionValue_StringValue:
		if len(kind.StringValue) > 256 || !utf8.ValidString(kind.StringValue) {
			return fmt.Errorf("string exceeds 256 bytes or is invalid UTF-8")
		}
	case *irv1.ExtensionValue_DoubleValue:
		if math.IsNaN(kind.DoubleValue) || math.IsInf(kind.DoubleValue, 0) {
			return fmt.Errorf("double is not finite")
		}
	case *irv1.ExtensionValue_ObjectValue:
		if kind.ObjectValue == nil {
			return fmt.Errorf("object value is nil")
		}
		if len(kind.ObjectValue.Fields) > 256 {
			return fmt.Errorf("object field count exceeds 256")
		}
		for name, child := range kind.ObjectValue.Fields {
			if name == "" || len(name) > 256 || !utf8.ValidString(name) {
				return fmt.Errorf("object key is empty")
			}
			if err := validateExtensionDepth(child, depth+1); err != nil {
				return err
			}
		}
	case *irv1.ExtensionValue_ListValue:
		if kind.ListValue == nil {
			return fmt.Errorf("list value is nil")
		}
		if len(kind.ListValue.Values) > 256 {
			return fmt.Errorf("extension list length exceeds 256")
		}
		for _, child := range kind.ListValue.Values {
			if err := validateExtensionDepth(child, depth+1); err != nil {
				return err
			}
		}
	case nil:
		return fmt.Errorf("value kind is required")
	}
	return nil
}
