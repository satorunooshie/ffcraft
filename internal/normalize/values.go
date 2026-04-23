package normalize

import (
	"fmt"
	"maps"

	"google.golang.org/protobuf/types/known/structpb"

	ffv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/v1"
	"github.com/satorunooshie/ffcraft/internal/ast"
)

func normalizeAction(doc *ffv1.FeatureFlagDocument, action *ffv1.Action) (ast.Action, error) {
	switch kind := action.Kind.(type) {
	case *ffv1.Action_Serve:
		return &ast.ServeAction{Variant: kind.Serve.Variant}, nil
	case *ffv1.Action_Distribute:
		dist := doc.Distributions[kind.Distribute.Distribution]
		allocations := make(map[string]float64, len(dist.Allocations))
		maps.Copy(allocations, dist.Allocations)
		return &ast.DistributeAction{
			Stickiness:  dist.Stickiness,
			Allocations: allocations,
		}, nil
	case *ffv1.Action_ProgressiveRollout:
		return &ast.ProgressiveRolloutAction{
			Variant:    kind.ProgressiveRollout.Variant,
			Stickiness: kind.ProgressiveRollout.Stickiness,
			Start:      kind.ProgressiveRollout.Start,
			End:        kind.ProgressiveRollout.End,
			Steps:      kind.ProgressiveRollout.Steps,
		}, nil
	default:
		return nil, fmt.Errorf("action must define serve, distribute, or progressive_rollout")
	}
}

func normalizeExperimentation(exp *ffv1.Experimentation) *ast.Experimentation {
	if exp == nil {
		return nil
	}
	return &ast.Experimentation{Start: exp.Start, End: exp.End}
}

func normalizeBinaryValues(left, right *ffv1.Value) (ast.Value, ast.Value, error) {
	nleft, err := normalizeValue(left)
	if err != nil {
		return nil, nil, err
	}
	nright, err := normalizeValue(right)
	if err != nil {
		return nil, nil, err
	}
	return nleft, nright, nil
}

func normalizeValue(value *ffv1.Value) (ast.Value, error) {
	switch kind := value.Kind.(type) {
	case *ffv1.Value_Var:
		return &ast.Var{Path: kind.Var.Path}, nil
	case *ffv1.Value_Scalar:
		return normalizeScalar(kind.Scalar)
	case *ffv1.Value_StringList:
		items := make([]ast.Value, 0, len(kind.StringList.Values))
		for _, item := range kind.StringList.Values {
			items = append(items, &ast.Scalar{Kind: ast.ScalarKindString, String: item})
		}
		return &ast.List{Values: items}, nil
	case *ffv1.Value_List:
		items := make([]ast.Value, 0, len(kind.List.Values))
		for _, item := range kind.List.Values {
			value, err := normalizeValue(item)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		return &ast.List{Values: items}, nil
	default:
		return nil, fmt.Errorf("value is unset")
	}
}

func normalizeScalar(value *ffv1.Scalar) (ast.Value, error) {
	switch kind := value.Kind.(type) {
	case *ffv1.Scalar_StringValue:
		return &ast.Scalar{Kind: ast.ScalarKindString, String: kind.StringValue}, nil
	case *ffv1.Scalar_BoolValue:
		return &ast.Scalar{Kind: ast.ScalarKindBool, Bool: kind.BoolValue}, nil
	case *ffv1.Scalar_IntValue:
		return &ast.Scalar{Kind: ast.ScalarKindInt, Int: kind.IntValue}, nil
	case *ffv1.Scalar_DoubleValue:
		return &ast.Scalar{Kind: ast.ScalarKindDouble, Double: kind.DoubleValue}, nil
	case *ffv1.Scalar_NullValue:
		return &ast.Scalar{Kind: ast.ScalarKindNull}, nil
	default:
		return nil, fmt.Errorf("unsupported scalar type %T", kind)
	}
}

func normalizeVariantSet(vs *ffv1.VariantSet) map[string]ast.VariantValue {
	out := make(map[string]ast.VariantValue, len(vs.Variants))
	for key, value := range vs.Variants {
		out[key] = normalizeVariantValue(value)
	}
	return out
}

func normalizeVariantValue(value *ffv1.VariantValue) ast.VariantValue {
	switch kind := value.Kind.(type) {
	case *ffv1.VariantValue_BoolValue:
		return ast.VariantValue{Kind: ast.VariantValueKindBool, Bool: kind.BoolValue}
	case *ffv1.VariantValue_StringValue:
		return ast.VariantValue{Kind: ast.VariantValueKindString, String: kind.StringValue}
	case *ffv1.VariantValue_IntValue:
		return ast.VariantValue{Kind: ast.VariantValueKindInt, Int: kind.IntValue}
	case *ffv1.VariantValue_DoubleValue:
		return ast.VariantValue{Kind: ast.VariantValueKindDouble, Double: kind.DoubleValue}
	case *ffv1.VariantValue_ObjectValue:
		return ast.VariantValue{Kind: ast.VariantValueKindObject, Object: structToMap(kind.ObjectValue)}
	case *ffv1.VariantValue_ListValue:
		items := make([]ast.VariantValue, 0, len(kind.ListValue.Values))
		for _, item := range kind.ListValue.Values {
			items = append(items, normalizeVariantValue(item))
		}
		return ast.VariantValue{Kind: ast.VariantValueKindList, List: items}
	case *ffv1.VariantValue_NullValue:
		return ast.VariantValue{Kind: ast.VariantValueKindNull}
	default:
		return ast.VariantValue{Kind: ast.VariantValueKindUnknown}
	}
}

func normalizeMetadata(metadata *ffv1.Metadata) *ast.Metadata {
	if metadata == nil {
		return nil
	}
	return &ast.Metadata{
		Owner:       metadata.Owner,
		Description: metadata.Description,
		Expiry:      metadata.Expiry,
		Tags:        append([]string(nil), metadata.Tags...),
	}
}

func structToMap(value *structpb.Struct) map[string]any {
	out := make(map[string]any, len(value.Fields))
	for key, field := range value.Fields {
		out[key] = field.AsInterface()
	}
	return out
}
