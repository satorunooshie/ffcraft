# ffcodegen

`ffcodegen` is the code generation companion to `ffcraft`.
It reads authoring YAML or normalized YAML and emits application-facing generated code.

Role split:

- `ffcompile`: produces runtime configuration such as `flagd` JSON or `GO Feature Flag` YAML
- `ffcodegen`: produces application-facing typed APIs from the same source definition

Today the supported target is `go`.

## Commands

Generate Go accessors from authoring YAML:

```bash
go run ./cmd/ffcodegen go --in ffcompile.yaml --config ffcodegen.yaml --out featureflags_gen.go
```

Generate Go accessors with defaults:

```bash
go run ./cmd/ffcodegen go --in ffcompile.yaml
```

Supported flags:

- `--in`: required input path
- `--config`: optional `ffcodegen.yaml`
- `--out`: optional output path, stdout when omitted or `-`
- `--format`: `auto`, `authoring`, or `normalized`
- `--dump`: when reading authoring YAML, also write normalized YAML

## Defaults

When `--config` is omitted, `ffcodegen go` uses:

- package: `featureflags`
- context fields: auto-extracted from `var` references
- accessor names: auto-generated from flag keys
- output: stdout unless `--out` is given

Default target values:

| Field | Default |
| --- | --- |
| `targets.go.package` | `featureflags` |
| `targets.go.context_type` | `EvalContext` |
| `targets.go.client_type` | `Client` |
| `targets.go.evaluator_type` | `Evaluator` |
| `targets.go.context.fields` | auto-extracted from `var` references |
| `targets.go.accessors` | auto-generated from flag keys |

## ffcodegen.yaml

Minimal example:

```yaml
version: v1
source: ./ffcompile.yaml

targets:
  go:
    package: featureflag
    output: ./internal/featureflag/featureflags_gen.go
    context_type: EvalContext
    client_type: Client
    evaluator_type: Evaluator
    context:
      fields:
        - path: user.id
          name: UserID
          type: int64
        - path: device.platform
          name: Platform
          type: string
    accessors:
      enable-new-home:
        name: EnableNewHome
      checkout-mode:
        name: CheckoutMode
        variant_type: CheckoutModeVariant
```

Top-level fields:

- `version`: must be `v1`
- `source`: source authoring file path
- `targets`: target-specific generator settings

Go target fields:

- `targets.go.package`: generated package name
- `targets.go.output`: suggested output path
- `targets.go.context_type`: generated context struct name
- `targets.go.client_type`: generated client interface name
- `targets.go.evaluator_type`: generated evaluator type name
- `targets.go.context.fields`: optional context field overrides
- `targets.go.accessors`: optional accessor and variant type overrides

### Context Fields

`context.fields` overrides the inferred Go field name or type for a path used in conditions.

Each field supports:

- `path`: attribute path such as `user.id`
- `name`: generated Go field name
- `type`: generated Go field type

Supported field types:

- `string`
- `bool`
- `int64`
- `float64`

If a path is referenced in authoring YAML but not listed in `context.fields`, `ffcodegen` infers it automatically.

### Accessors

`accessors` lets you override generated names per flag.

Supported fields:

- `name`: accessor name
- `variant_type`: variant enum type name for string-valued flags

## Generated API

The generated Go package is designed around usage, not raw evaluation payloads.

- boolean, string, int64, float64, object, and list variant flags become typed accessor methods
- referenced attributes become a typed context struct
- runtime SDK integration stays outside the generated package

API patterns:

| Flag shape | Generated signature |
| --- | --- |
| context used, no rollout | `Flag(ctx context.Context, ec EvalContext)` |
| no context, no rollout | `Flag(ctx context.Context)` |
| rollout or stickiness required | `Flag(ctx context.Context, targetingKey string)` |

Typical usage:

```go
evaluator := featureflag.New(client)

enabled, err := evaluator.EnableNewHome(ctx, featureflag.EvalContext{
	UserID: userID,
})

variant, err := evaluator.CheckoutMode(ctx, "user-123")
```

For rollout or stickiness based flags, generated accessors take an explicit `targetingKey string`.

```go
variant, err := evaluator.ExperimentRollout(ctx, "user-123")
```

If a rollout uses a stickiness path such as `user.id`, the generated evaluator also mirrors `targetingKey` into that attribute path so providers that bucket on nested attributes can resolve the same identifier.

## Runtime Contract

The generated Go code expects a small client interface implemented by your runtime evaluator.
`ffcodegen` intentionally keeps this interface SDK-agnostic, so consumer projects can adapt OpenFeature or another runtime without leaking third-party SDK types into generated application code.

Typical setup:

1. Generate typed accessors with `ffcodegen`
2. Implement the generated `Client` interface in your infra layer
3. Convert the generated `EvaluationContext` into your runtime SDK's context type inside that adapter

For OpenFeature, the intended integration is a thin adapter over `*openfeature.Client`, not direct OpenFeature types in generated code.
