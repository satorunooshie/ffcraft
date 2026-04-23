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
- `scheduled_rollouts[].experimentation` is compiled as an additional time-window guard

### Scheduled Rollout Semantics

`scheduled_rollouts` are treated as full snapshots.

- each enabled step represents the complete effective state from its `date`
- evaluation uses the most recent enabled step whose `date <= now`
- if no step is active yet, evaluation falls back to the base environment rules and `default_action`

### Experimentation Semantics

`flagd` does not have a native experimentation rollout construct in this project. Current behavior is:

- top-level environment `experimentation` is rejected
- `scheduled_rollouts[].experimentation` acts as a temporary overlay
- a step with experimentation applies only when `date <= now`, `start <= now`, and `now < end`
- after `end`, evaluation always returns to the outer normal scheduled or base evaluation

This is intentionally different from a persistent snapshot.

### Constraints

- `default_action` is required for rule-evaluation environments
- `default_action.progressive_rollout` is accepted only as an environment `default_action`
- `matches` currently returns a compile error

## GO Feature Flag

`gofeatureflag` output is YAML matching the GO Feature Flag file format.

### Mapping

- `serve` compiles to `variation`
- `distribute` compiles to `percentage`
- environment `default_action.progressive_rollout` compiles to native `defaultRule.progressiveRollout`
- top-level environment `experimentation` compiles to native `experimentation`
- `scheduled_rollouts` compile to native `scheduledRollout`
- step-level `experimentation` compiles to native scheduled rollout experimentation fields

### Bucketing

GO Feature Flag scopes bucketing at the flag level through `bucketingKey`.

- `distribute.stickiness` is used to infer `bucketingKey`
- if the same flag uses different `stickiness` values across actions, compilation fails

### Constraints

- `matches` currently returns a compile error
- semantics of step-level experimentation are not guaranteed to be identical to `flagd`

## Normalized YAML

Normalized YAML is target-neutral. It keeps:

- explicit `default_action`
- explicit `scheduled_rollouts`
- explicit `progressive_rollout`
- explicit `experimentation`

Target-specific expansion happens in the compiler, not in the normalized representation.
