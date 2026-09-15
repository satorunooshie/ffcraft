# Compiler Targets

This document describes how `ffcraft` maps the normalized protobuf IR to each supported target.

## Summary

## flagd

`flagd` output is JSON matching the `https://flagd.dev/schema/v0/flags.json` schema.

### Mapping

- `serve` compiles to a fixed variant result
- `distribute` compiles to a `fractional` operation
- `scheduled_rollouts` compile to nested `if` expressions ordered by descending effective date
- progressive rollout is lowered during normalization into scheduled IR snapshots
- top-level array variant values are not supported by the flagd compiler

### Scheduled Rollout Semantics

`scheduled_rollouts` are treated as full snapshots.

- each enabled step represents the complete effective state from its `date`
- evaluation uses the most recent enabled step whose `date <= now`
- if no step is active yet, evaluation falls back to the base environment rules and `default_action`

### Constraints

- `default_action` is required for rule-evaluation environments
- `default_action.progressive_rollout` is accepted only as an environment `default_action`

Normalized object values use recursive `VariantValue`; target runtime encoding
constraints still apply. Object fields may contain arrays.
For example, `all: [anonymous, google]` is not supported for flagd, while
`all: {providers: [anonymous, google]}` is supported.

## GO Feature Flag

`gofeatureflag` output is YAML matching the GO Feature Flag file format.

### Mapping

- `serve` compiles to `variation`
- `distribute` compiles to `percentage`
- normalized progressive rollout snapshots compile to scheduled `defaultRule` entries
- `scheduled_rollouts` compile to native `scheduledRollout`

### Bucketing

GO Feature Flag scopes bucketing at the flag level through `bucketingKey`.

- `distribute.stickiness` is used to infer `bucketingKey`
- if the same flag uses different `stickiness` values across actions, compilation fails

### Constraints


## Normalized YAML

Normalized YAML is target-neutral. It keeps:

- explicit `default_action`
- explicit `scheduled_rollouts`
- progressive rollout stages lowered to scheduled IR entries

Progressive rollout lowering happens during normalization; target compilers consume the resulting scheduled snapshots.

An IR action-resolution failure is an evaluation error. A target runtime may
return an application fallback value, but must preserve the error in evaluation
metadata when that metadata is exposed.
