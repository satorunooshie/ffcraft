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
- `distributions`: optional. Reusable percentage allocations
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
- `contains`
- `starts_with`
- `ends_with`
- `matches`

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

## distributions

```yaml
distributions:
  checkout_ab:
    stickiness: user.id
    allocations:
      treatment_a: 10
      treatment_b: 10
      control: 80
```

- `allocations` must sum to `100`
- allocation keys must exist in the target `variant_set`
- `stickiness` is the stable bucketing key for percentage-based rollout

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

Normalized YAML keeps `progressive_rollout` as-is. Expansion is compiler-specific:

- `flagd` expands it into generated time-based snapshots
- `gofeatureflag` compiles it to native `progressiveRollout`

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

## experimentation

`experimentation` can appear at the environment level and on `scheduled_rollouts` steps.

Environment-level experimentation:

```yaml
environments:
  prod:
    default_action:
      distribute: checkout_ab
    experimentation:
      start: "2026-05-01T00:00:00Z"
      end: "2026-05-20T00:00:00Z"
```

Scheduled-step experimentation:

```yaml
scheduled_rollouts:
  - date: "2026-05-08T09:00:00Z"
    rules:
      - if:
          rule: is_beta_user
        serve: on
    default_action:
      distribute: ten_percent_on
    experimentation:
      start: "2026-05-08T09:00:00Z"
      end: "2026-05-15T09:00:00Z"
```

- `start` and `end` must be RFC3339 timestamps
- `start` must be before `end`

Semantics differ by compiler. See [docs/compiler-targets.md](compiler-targets.md).

## Validation

At minimum, `ffcompile` validates:

- uniqueness of `flags[].key`
- existence of referenced `variant_set`, `rule`, and `distribution`
- variant consistency for `default_variant`, `serve`, and action/default-action references
- distribution totals equal `100`
- distribution allocation keys exist in the target `variant_set`
- rule cycle detection
- `metadata.expiry` matches `YYYY-MM-DD`
- `scheduled_rollouts` are ascending by date with no duplicates
- `scheduled_rollouts[].default_action` is present
- `progressive_rollout.steps > 0`
- `progressive_rollout.start < progressive_rollout.end`
- `experimentation.start < experimentation.end`

## Unsupported / Not Yet Compiled

- YAML aliases and anchors
- `matches` compilation for both `flagd` and `gofeatureflag`
- top-level environment `experimentation` compilation for `flagd`

`matches` is accepted by parse, validate, and normalize, but compilation currently fails.
