# Schema

This directory contains JSON Schema for the `ffcraft` authoring format.

Files:

- [developer-flags.schema.json](developer-flags.schema.json): JSON Schema for the current `v1` authoring YAML

Notes:

- the canonical source of truth for the data model is [proto/ffcraft/v1/ffcraft.proto](../proto/ffcraft/v1/ffcraft.proto)
- this JSON Schema is intended for editor integration, linting, and non-Go tooling
- some semantic validations still live in application code, such as cross-reference checks, distribution totals, rollout ordering, and target-specific compile constraints
