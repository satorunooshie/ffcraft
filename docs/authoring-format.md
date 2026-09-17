# Authoring Format v1

This document describes the authoring YAML accepted by `ffcraft`.

The canonical type definition is [proto/ffcraft/v1/ffcraft.proto](../proto/ffcraft/v1/ffcraft.proto). A JSON Schema for editor integration is available at [schema/developer-flags.schema.json](../schema/developer-flags.schema.json).

## Top Level

```yaml
version: v1
variant_sets: {}
rules: {}
distributions: {}
flags: []
```

- `version`: required. Currently `v1`
- `variant_sets`: optional. Reusable variant maps
- `rules`: optional. Reusable conditions
- `distributions`: optional. Reusable positive integer relative weights
- `flags`: required. The list of flags

## variant_sets

```yaml
variant_sets:
  boolean:
    on: true
    off: false

  checkout_mode:
    control: control
    treatment_a: treatment_a
    treatment_b: treatment_b
```

- each flag references one `variant_set`
- `default_variant`, `serve`, `default_action.serve`, and distribution allocation keys must exist in that set
- values may be scalar, object, list, or `null`

## rules

```yaml
rules:
  internal_user:
    eq:
      - { var: user.type }
      - internal

  ios_user:
    eq:
      - { var: device.platform }
      - ios

  internal_ios:
    all_of:
      - rule: internal_user
      - rule: ios_user
```

### Operators

Comparison:

- `eq`
- `ne`
- `gt`
- `gte`
- `lt`
- `lte`

Collection and string:

- `in`
- `contains` (array element membership)
- `string_contains` (case-sensitive substring matching)
- `starts_with`
- `ends_with`

Semver:

- `semver_gt`
- `semver_gte`
- `semver_lt`
- `semver_lte`

Logical:

- `all_of`
- `any_of`
- `one_of`
- `not`
- `literal_bool`
- `rule`

### Values

Variable reference:

```yaml
{ var: user.country }
```

Literal values:

```yaml
JP
123
true
null
```

List:

```yaml
[JP, US]
```

For the `flagd` target, a variant value whose top-level value is a list is not
supported by the target representation. Use an object when the value contains arrays:

```yaml
all:
  providers:
    - anonymous
    - google
```

## distributions

```yaml
distributions:
  checkout_ab:
    stickiness: user.id
    weights:
      treatment_a: 1
      treatment_b: 1
      control: 8
```

- `weights` are positive integer relative weights; they must contain at least two variants and do not need to sum to `100`
- allocation keys must exist in the target `variant_set`
- `stickiness` is the stable bucketing key for weighted rollout

## flags

```yaml
flags:
  - key: enable-new-home
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        rules:
          - if:
              rule: internal_ios
            serve: on
        default_action:
          serve: off
```

- `key`: required and unique
- `variant_set`: required
- `default_variant`: required
- `environments`: required

## Environment Forms

### Fixed Serve

```yaml
environments:
  prod:
    serve: on
```

This is the shortest form. The environment always returns one variant.

### Rule Evaluation

```yaml
environments:
  prod:
    rules:
      - if:
          eq:
            - { var: user.country }
            - JP
        serve: on
    default_action:
      serve: off
```

- `rules` are evaluated from top to bottom
- the first matching rule wins
- `default_action` is used when no rule matches
- if `default_action` is omitted, evaluation falls back to the flag `default_variant`

## Actions

### Fixed Variant

```yaml
serve: on
```

### Distribution Reference

```yaml
distribute: checkout_ab
```

### Progressive Rollout

```yaml
default_action:
  progressive_rollout:
    variant: on
    stickiness: user.id
    start: "2026-05-01T09:00:00Z"
    end: "2026-05-22T09:00:00Z"
    steps: 4
```

- `progressive_rollout` is supported only in environment `default_action`
- `variant` is the target variant
- `stickiness` is the stable bucketing key
- `stickiness` must be a regular attribute path such as `user.id`; avoid `targetingKey` because providers do not treat it like a normal attribute path consistently
- `start` and `end` must be RFC3339 timestamps
- `steps` is the total number of rollout stages, including the final 100% stage

Normalization lowers `progressive_rollout` to scheduled IR snapshots. The final snapshot is effective at `end` and serves the target variant at 100%:

- both target compilers consume the resulting scheduled snapshots

## scheduled_rollouts

```yaml
environments:
  prod:
    default_action:
      serve: off
    scheduled_rollouts:
      - name: internal launch
        description: enable for employees only
        date: "2026-05-03T00:00:00Z"
        disabled: false
        rules:
          - if:
              rule: internal_user
            serve: on
        default_action:
          serve: off

      - name: broad launch
        date: "2026-05-10T00:00:00Z"
        default_action:
          serve: on
```

Each step is a complete snapshot of the effective environment state from its `date`.

- `scheduled_rollouts[].default_action` is required
- `scheduled_rollouts[].rules` are optional
- `scheduled_rollouts[].name`, `description`, and `disabled` are optional
- `scheduled_rollouts[].date` must be RFC3339
- `scheduled_rollouts` must be sorted in ascending date order
- `scheduled_rollouts[].date` values must be unique
- `disabled: true` excludes the step from evaluation without deleting it from the rollout plan

Evaluation is fixed:

- enabled steps with `date <= now` are eligible
- the newest eligible step is used
- if no step is active yet, evaluation uses the base environment `rules` and `default_action`

## Validation

At minimum, `ffcompile` validates:

- uniqueness of `flags[].key`
- existence of referenced `variant_set`, `rule`, and `distribution`
- variant consistency for `default_variant`, `serve`, and action/default-action references
- distribution weights are positive integer relative weights with at least two variants
- distribution allocation keys exist in the target `variant_set`
- rule cycle detection
- `scheduled_rollouts` are ascending by date with no duplicates
- `scheduled_rollouts[].default_action` is present
- `progressive_rollout.steps > 0`
- `progressive_rollout.start < progressive_rollout.end`

## Unsupported / Not Yet Compiled

- YAML aliases and anchors

## Contains semantics and target support

`contains: [{var: user.tags}, beta]` tests for the exact `beta` element in an
array; `in: [beta, {var: user.tags}]` is equivalent. Go code generation preserves
`UserTags []string`. Numeric and boolean elements infer their corresponding
slice types. IR evaluation rejects non-array containers and does not coerce
strings to numbers or interpret nested arrays as ranges.

Use `string_contains: [{var: user.name}, beta]` for a case-sensitive substring
match and a Go `string` field. Non-string and missing values evaluate to false.

| Operation | Go generation / IR | flagd | GO Feature Flag |
| --- | --- | --- | --- |
| Array `contains` / reversed `in` | Supported | Explicit error | Explicit error |
| `string_contains` | Supported | String type guard | Explicit error |

Unsupported provider output returns `FFCRAFT_TARGET_CONDITION_UNSUPPORTED`
without emitting configuration, including for nested and scheduled conditions.
The native array operators do not preserve exact element equality. GO Feature
Flag's native substring operation is case-insensitive, so it cannot preserve
this condition's semantics. Existing array authoring remains usable for Go
code generation; compiling that input to either provider now fails explicitly.

flagd uses `if(starts_with(attribute, ""), in(literal, attribute), false)`.
The string guard follows the
[official string comparison specification](https://flagd.dev/reference/specifications/custom-operations/string-comparison-operation-spec/).
Generated output is tested with flagd core v0.15.0 for valid strings, wrong
container types, missing attributes, empty strings, and negation.

Runtime-specific array support, if added later, must be explicitly selected
and tested against a named implementation and version. Nonstandard operators
such as `contains_any` are not enabled in portable output.
