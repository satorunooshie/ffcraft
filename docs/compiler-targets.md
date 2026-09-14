# Compiler Targets

This document describes how `ffcraft` maps the normalized authoring model to each supported target.

## Summary

## flagd

`flagd` output is JSON matching the `https://flagd.dev/schema/v0/flags.json` schema.

### Mapping

- `serve` compiles to a fixed variant result
- `distribute` compiles to a `fractional` operation
- `scheduled_rollouts` compile to nested `if` expressions ordered by descending effective date
- `progressive_rollout` is expanded during compilation into synthetic scheduled steps
- authoring-only `experimentation` is consumed before semantic IR normalization
- top-level array variant values are not supported by the flagd compiler

### Scheduled Rollout Semantics

`scheduled_rollouts` are treated as full snapshots.

- each enabled step represents the complete effective state from its `date`
- evaluation uses the most recent enabled step whose `date <= now`
- if no step is active yet, evaluation falls back to the base environment rules and `default_action`

### Constraints

- `default_action` is required for rule-evaluation environments
- `default_action.progressive_rollout` is accepted only as an environment `default_action`
- `matches` currently returns a compile error

`flagd` object values are backed by `google.protobuf.Struct` at runtime and
therefore must be top-level objects. Object fields may still contain arrays.
For example, `all: [anonymous, google]` is not supported for flagd, while
`all: {providers: [anonymous, google]}` is supported.

## GO Feature Flag

`gofeatureflag` output is YAML matching the GO Feature Flag file format.

### Mapping

- `serve` compiles to `variation`
- `distribute` compiles to `percentage`
- environment `default_action.progressive_rollout` compiles to native `defaultRule.progressiveRollout`
- authoring-only `experimentation` is absent from normalized IR and output
- `scheduled_rollouts` compile to native `scheduledRollout`

### Bucketing

GO Feature Flag scopes bucketing at the flag level through `bucketingKey`.

- `distribute.stickiness` is used to infer `bucketingKey`
- if the same flag uses different `stickiness` values across actions, compilation fails

### Constraints

- `matches` currently returns a compile error

## Normalized YAML

Normalized YAML is target-neutral. It keeps:

- explicit `default_action`
- explicit `scheduled_rollouts`
- explicit `progressive_rollout`
- authoring-only `experimentation` is not part of normalized YAML

Target-specific expansion happens in the compiler, not in the normalized representation.
