# ffcraft Public Extension and Normalized IR Specification

## Status

This document defines the public `v1` contract for ffcraft authoring,
normalized IR, extensions, and compiler input.

The authoring YAML format is the source of truth. The normalized protobuf IR is
the only public normalized representation and the only public compiler input.
Normalized YAML is an output-only, deterministic, human-readable view of the
protobuf IR.

## 1. Public pipeline

```text
authoring YAML (source of truth)
    -> parse
    -> validate
    -> normalize
    -> ffcraft.ir.v1 protobuf
          -> compiler or generator
          -> deterministic normalized YAML view
```

Compilers and generators MUST consume `ffcraft.ir.v1`. They MUST NOT require
the original authoring YAML after normalization and MUST NOT interpret
authoring-only syntax.

The public command roles are:

- `build`: authoring YAML to a target runtime format
- `normalize`: authoring YAML to protobuf IR or normalized YAML output
- `compile`: protobuf IR to a target runtime format

Normalized YAML MUST NOT be accepted as compiler or generator input.

## 2. Authoring format

Authoring YAML contains core feature-flag data and optional opaque extensions.
Supported extension locations are:

- document root
- each flag
- each environment

```yaml
version: v1

variant_sets:
  boolean:
    on: true
    off: false

flags:
  - key: checkout
    variant_set: boolean
    default_variant: off
    extensions:
      com.example.analytics.v1:
        event: checkout_evaluated
    environments:
      prod:
        default_action:
          serve: on
        extensions:
          com.example.client.v1:
            enabled: true

extensions:
  com.example.owner.v1:
    team: platform
```

`extensions` is a mapping from a namespace name to an arbitrary value.
Namespace names have no ffcraft-defined meaning. ffcraft MUST NOT reserve or
interpret names such as `metadata`, `analytics`, or `client`.

Extension values may be mappings, sequences, strings, booleans, signed 64-bit
integers, finite doubles, or null, including nested combinations of these
values. Mapping keys are strings. Extension values at different scopes are
independent; ffcraft does not merge, inherit, or override them.

Core mappings are strict. Unknown core fields MUST be rejected. Unknown fields
inside an extension value MUST be accepted and preserved as extension data.

`metadata` is not a core field. Metadata, ownership, analytics, and client
configuration MUST be represented by an extension namespace.

## 3. Normalized IR

The normative normalized IR is the protobuf package `ffcraft.ir.v1`, defined by
`proto/ffcraft/ir/v1/normalized.proto`.

The IR MUST be self-contained and contain normalized core semantics, including:

- resolved variants
- environments keyed by name
- normalized conditions and rules
- normalized actions
- distributions and scheduled evaluations
- document, flag, and environment extensions

Authoring-only variant sets, named rules, named distributions, shorthand,
unresolved references, comments, YAML styles, anchors, and aliases MUST NOT be
required by an IR consumer.

The extension wire model preserves numeric kinds:

```proto
message ExtensionValue {
  oneof kind {
    bool bool_value = 1;
    string string_value = 2;
    int64 int_value = 3;
    double double_value = 4;
    ExtensionObject object_value = 5;
    ExtensionList list_value = 6;
    ExtensionNull null_value = 7;
  }
}

message ExtensionObject {
  map<string, ExtensionValue> fields = 1;
}

message ExtensionList {
  repeated ExtensionValue values = 1;
}

message ExtensionNull {}
```

The actual protobuf schema is normative. The illustration above does not assign
field numbers to document, flag, or environment extensions.

## 4. Normalized YAML view

Normalized YAML uses version `normalized/v1` and is a deterministic,
human-readable serialization of the protobuf IR.

The emitter MUST:

- emit lexical map-key order
- preserve sequence order
- omit absent optional fields
- omit an empty top-level `extensions` map
- emit an existing empty extension object as `{}`
- emit null extension values as `null`
- emit integers in decimal notation
- preserve the distinction between integer and double values
- omit comments, anchors, aliases, tags, and YAML styles

Normalized YAML is not a second semantic model and is not a public input
format. If the protobuf IR contains unknown wire data that cannot be represented
in YAML, the emitter MUST fail explicitly rather than silently dropping it.

## 5. Compiler behavior

Core compilers MUST ignore extension namespaces and extension contents when
producing target runtime configuration.

For every deterministic target:

```text
compile(IR) == compile(IR with extensions removed)
```

A compiler MUST fail closed when it encounters an unknown core semantic field,
unknown enum value, unknown oneof variant, or any core construct it cannot
represent faithfully. Unknown extension namespaces MUST NOT cause core
compilation to fail.

Environment selection is performed from normalized IR:

```text
flag + environment name -> selected environment semantics
```

Environment names are opaque, case-sensitive identifiers. They have no
standard meanings such as `dev`, `staging`, or `prod`.

## 6. Unknown protobuf fields

Protobuf runtimes may preserve fields that a consumer does not recognize as raw
unknown fields. Protobuf-to-protobuf transformers and codecs MUST preserve
those fields when the runtime supports preservation.

Unknown fields in core semantic messages MUST cause a compile or validation
error. Unknown fields inside an extension value are opaque and MUST be retained
by protobuf transformations when supported. Since normalized YAML has no
portable representation for arbitrary protobuf wire fields, its emitter MUST
reject such data instead of dropping it.

## 7. Extension validation

Extension schemas and validation belong to namespace owners, not ffcraft core.
An extension validator may select a document, flag, or environment scope and a
namespace. It receives the normalized IR and returns either:

- valid
- not applicable
- invalid, with a diagnostic code, severity, message, and scope-relative path

Core validation MUST NOT require a standard namespace list or inspect extension
contents beyond transport and representability constraints.

## 8. Extension anti-patterns

Extensions are an escape hatch for independently owned, non-core data. They
MUST NOT become an alternative representation of core feature-flag semantics.

The following patterns are discouraged or invalid:

- placing variants, targeting rules, actions, environments, or schedules in an
  extension namespace;
- making core compilation depend on a particular extension namespace;
- using an extension to change evaluation behavior without representing that
  behavior in the normalized IR;
- treating extension values at document, flag, and environment scope as
  implicitly inherited or merged;
- using one unversioned namespace for data whose schema evolves incompatibly;
- putting secrets, credentials, executable code, or deployment commands in
  extensions;
- relying on core to validate namespace-specific fields or to discover a
  namespace schema automatically.

If data is required for every conforming compiler to preserve or execute the
same runtime semantics, it belongs in the core IR and requires a core schema
change. If data is owned by a particular consumer or namespace owner and can
remain opaque to ffcraft, it belongs in an extension namespace.

## 9. Compatibility and versioning

Authoring and normalized IR versions are independent. A change to authoring
syntax that preserves normalized semantics does not require an IR version
change. An incompatible normalized semantic or wire change requires a new IR
major version, such as `ffcraft.ir.v2`.

Within an IR major version, field numbers, field types, existing semantics, and
existing oneof meanings MUST NOT change incompatibly. Removed protobuf field
numbers MUST NOT be reused after the schema is published.

Unknown core constructs MUST fail closed. Unknown extension namespaces remain
forward-compatible because core compilers do not interpret them.

## 10. Conformance requirements

An implementation claiming conformance MUST test:

- document, flag, and environment extensions
- every extension value kind
- nested objects and lists
- int64 boundary values, finite doubles, and null
- empty objects and absent extension maps
- unknown namespaces
- arbitrary extension fields
- rejection of unknown core YAML fields
- rejection of unknown core protobuf fields and oneof variants
- protobuf encode/decode semantic equality
- normalized YAML output fidelity
- unchanged compiler output with and without extensions
- environment selection
- duplicate YAML keys, custom tags, anchors, aliases, and invalid numbers

Comparisons use semantic equality. Protobuf map order and field order are not
semantic; list order is semantic.

## 11. Normative artifacts

The following artifacts define the public contract:

- this specification
- `proto/ffcraft/v1/ffcraft.proto` for authoring input
- `proto/ffcraft/ir/v1/normalized.proto` for normalized IR
- versioned conformance fixtures and expected outputs

Generated language bindings are public artifacts for the normalized protobuf
package. Internal Go ASTs and implementation packages are not public semantic
contracts.
