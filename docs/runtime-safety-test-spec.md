# ffcraft Runtime Safety Test Specification v5

## 0. Guarantee Scope

This specification limits the guaranteed scope of "runtime safety" to the following:

```text
Deterministic Artifact / Representation Compatibility

+
Deterministic Semantic Correctness

+
Explicitly Supported Runtime Lifecycle Scenarios
```

It does not guarantee 100% elimination of arbitrary infrastructure failures such as:

```text
OOM
process kill
host failure
arbitrary network outage
permission change
port collision
filesystem corruption
external service outage
```

However, failure and recovery scenarios explicitly included as supported Lifecycle Scenarios below are within the guarantee scope.

Final domain:

```text
Supported Input Domain
×
Compile Target
×
Official Runtime Profile
×
Compatibility Class
×
Concrete Runtime Artifact Tuple
×
Value / Context Domain
×
Runtime Lifecycle Scenario
```

P0 primarily guarantees deterministic artifact/value compatibility.

Lifecycle Scenarios are added starting in P1.

## Implementation Status (as of 2026-09-13)

This document describes the specification and plan for ultimately verifying runtime safety.
It does not mean that every acceptance criterion described here has been implemented.

The following is implemented at present:

- Integer precision protection at P0 authoring / normalized ingress
- Structural validation of the normalized IR
- Value-shape validation based on target capabilities
- Rejection of top-level list variants for flagd
- Basic flagd / GO Feature Flag / codegen integration tests

The following currently have only their design and verification foundation; runtime compatibility guarantees are not complete:

- Measured numeric transport matrix results for each runtime profile
- Verification of provider-specific numeric coercion differences
- Complete linkage between the lifecycle matrix and concrete artifact evidence
- Ongoing operation of evidence-based Supported decisions

Accordingly, the P0 runtime profile evidence and artifact identity items are partially complete,
while the P1 runtime transport / lifecycle items are incomplete (foundation only).

---

# 1. Boundary Axis Revision

Rename B9.

```text
B0-A Authoring Decode / Lossless Ingress
B0-N Normalized Input Decode / Lossless Ingress

B1   Common Semantic Validation
B2   Normalize
B3   Normalized IR Validation
B4   Compile Target Capability Validation
B5   Compile / Serialization
B6   Native Parser / Loader
B7   Native Runtime / Transport
B8   OpenFeature Provider / SDK
B9   Generated Accessor / Typed API
B10  CLI / Artifact Write
```

The reason for splitting B0 into two ingress boundaries is that both of these paths exist:

```text
authoring
→ normalize
```

and

```text
normalized input
→ direct compile
```

Both paths must be checked before conversion to a lossy representation.

---

# 2. Numeric Precision Is a P0 Concern

In the current IR v1, root integers are preserved as

```text
VariantValue.int_value
→ int64
```

while object values use

```text
VariantValue.object_value
→ google.protobuf.Struct
```

with a potentially lossy numeric representation.

Therefore, the following must not be treated as the same numeric domain.

```text
Root Int

vs

Object
 └ Integer
```

`google.protobuf.Struct` / `google.protobuf.Value.number_value` use a binary64 floating-point representation.

For example,

```text
9007199254740993
= 2^53 + 1
```

the original integer intent may no longer be recoverable after conversion to Struct.

Important:

```text
Loss before B3/B4
cannot be repaired by B3/B4 validation.
```

This must therefore be prevented before conversion into the IR.

---

# 3. Numeric Strategy

Separate the long-term architecture from the P0 mitigation.

## Strategic End State — Option A

In a future IR schema revision, replace the following representation

with a recursive value model that can preserve int64:

```text
google.protobuf.Struct object_value
```


Concept:

```text
Value
 ├ Bool
 ├ String
 ├ Int64
 ├ Double
 ├ Object<string, Value>
 ├ List<Value>
 └ Null
```

Example:

```proto
message Value {
  oneof kind {
    bool bool_value = 1;
    string string_value = 2;
    int64 int_value = 3;
    double double_value = 4;
    ObjectValue object_value = 5;
    ListValue list_value = 6;
    NullValue null_value = 7;
  }
}

message ObjectValue {
  map<string, Value> fields = 1;
}
```

This would allow the following path to

```text
author intent
→ Common IR
```

preserve numbers losslessly,

```text
Common IR
→ target/runtime
```

and determine target-specific numeric capabilities at the appropriate boundary.

However, because this would have a substantial compatibility impact on the existing IR/protobuf schema, it is not required in P0.

---

# 4. P0 Numeric Policy — Option B + Explicit v1 Domain

P0 retains the current IR v1.

Instead of converting

```text
YAML / textual normalized input
→ google.protobuf.Struct
```

check the integer domain before conversion.

## Current v1 Object Integer Policy

For values authored as integers inside objects,

```text
-9007199254740991
<= value <=
 9007199254740991
```

only the following domain is allowed.

In other words, the conservative JSON-safe integer domain is:

```text
[-(2^53 - 1), +(2^53 - 1)]
```

This domain is adopted.

The following are rejected:

```text
2^53
2^53 + 1
int64 max
-2^53
-(2^53 + 1)
int64 min
```

Note:

Although `2^53` itself is exactly representable in binary64, not every
integer immediately above it is. To simplify the guarantee, v1 adopts a domain
in which all integers in the selected range are exactly representable.

This is not a value-specific policy that treats only selected even numbers as safe.

---

# 5. Numeric Rule Applies Recursively

The P0 safe-integer guard applies recursively to every integer token stored in
`google.protobuf.Struct` / `google.protobuf.Value`, including direct object fields.

Reject:

```yaml
v:
  id: 9007199254740993
```

Reject:

```yaml
v:
  users:
    - id: 9007199254740993
```

Reject:

```yaml
v:
  a:
    b:
      c:
        value: 9007199254740993
```

Reject:

```yaml
v:
  values:
    - 9007199254740993
```

Root typed integers, on the other hand, are a separate domain.

```yaml
v: 9007199254740993
```

If preserved as `VariantValue.int_value`, it is valid in the Common IR.

Similarly,

```text
Root List
 └ Int VariantValue
```

if preserved as custom `ListValue.values -> VariantValue`, int64 can be retained as long as it does not pass through Struct.

---

# 6. B0 Lossless Ingress Invariant

The following is the new core invariant.

```text
I0 — Lossless Ingress

ffcraft MUST NOT convert an input representation into its
normalized IR if that conversion changes a semantically
significant value without reporting an error.
```

Specifically,

```text
input integer X
→ Struct conversion
→ integer-equivalent Y
where X != Y
```

is prohibited.

Silent rounding is prohibited.

---

# 7. B0-A Authoring Numeric Tests — P0

Required:

```text
B0A-OBJECT-INT-SAFE-MAX-001

object:
  id: 9007199254740991

→ PASS
```

```text
B0A-OBJECT-INT-UNSAFE-POSITIVE-001

object:
  id: 9007199254740992

→ REJECT
```

```text
B0A-OBJECT-INT-LOSSY-POSITIVE-001

object:
  id: 9007199254740993

→ REJECT
```

```text
B0A-OBJECT-INT-SAFE-MIN-001

object:
  id: -9007199254740991

→ PASS
```

```text
B0A-OBJECT-INT-UNSAFE-NEGATIVE-001

object:
  id: -9007199254740992

→ REJECT
```

These IDs complete the required recursive coverage:

```text
B0A-OBJECT-LIST-INT-LOSSY-001
B0A-DEEP-OBJECT-INT-LOSSY-001
```

The root-integer cases are specified below.

---

# 8. Root Integer Regression — P0

The object safety check must not be incorrectly applied to root integers.

Required:

```text
B0A-ROOT-INT-2P53P1-001

root variant:
9007199254740993

→ decoded as int64
→ PASS
→ exact value preserved
```

Also verify that int64 max/min values are lossless when preserved in the Common IR.

In other words, P0 numeric tests must verify the difference between

```text
Object nested int
→ safe domain restriction

Root typed int
→ int64 domain
```

the two domains above. This distinction is intentional.

---

# 9. B0-N Direct Normalized Input

The same lossless guard applies to a direct normalized compile path if ffcraft decodes text input and constructs Struct.

```text
normalized YAML / JSON text
→ preflight numeric inspection
→ protobuf conversion
```

in this order.

Prohibited:

```text
protobuf conversion
→ rounded value
→ ValidateNormalizedIR
```

because it cannot be detected after rounding has already occurred.

If an interface accepts an already materialized

```text
google.protobuf.Struct
```

as input, the number inside Struct is the binary64 value itself and must be treated as authoritative.

The original integer token must not be inferred from it.

---

# 10. Long-term IR Migration Requirement

Treat migration to a custom recursive Value as a separate IR schema migration,
PR, and major compatibility decision.

Downstream transport constraints such as flagd RPC remain even after the migration.

In other words,

```text
Lossless IR
```

does not mean

```text
all targets support full int64 nested values
```

Correct architecture:

```text
Authoring
→ lossless IR
→ target capability validation
→ possibly reject for specific runtime transport
```

---

# 11. Compile Capability Must Be Predicate-based

Production capability must not be represented only by a simple bitset.

Prohibited end state:

```go
RootValueKinds
NestedValueKinds
```

Representing all compatibility using only these two sets is prohibited.

Even in this case alone,

```text
Root List
Object → List
Object → List → Object
Object → Empty List
```

capabilities differ by these dimensions.

---

# 12. Value Shape Model

During target validation, obtain at least the following facts for each value node.

Concept:

```go
type ValueFacts struct {
    Kind ValueKind

    RootKind ValueKind
    ParentKind *ValueKind

    Depth int

    Position ValuePosition

    NumericClass NumericClass

    Path []ValueKind
}
```

`Position` examples:

```text
Root
ObjectField
ListElement
ObjectInsideList
ListInsideObject
EvaluationContext
```

`NumericClass` examples:

```text
NotNumeric
Int64
SafeJSONInteger
UnsafeJSONInteger
FiniteDouble
NonFiniteDouble
```

---

# 13. Capability Predicate

Production capability is expressed as predicates / constraints.

Concept:

```go
type CapabilityDecision int

const (
    Supported CapabilityDecision = iota
    Unsupported
)

type ValueCapabilityRule struct {
    Match    ValuePredicate
    Decision CapabilityDecision

    ErrorCode string

    EvidenceIDs []string
}
```

Or equivalently,

```go
SupportsValue(profile, valueFacts) Decision
```

is also acceptable.

The important point is that capability decisions can use a combination of facts,
rather than being limited to

```text
Kind only
```
or to

```text
Kind
× Position
× Parent Shape
× Numeric Domain
× relevant semantic context
```

as decision inputs.

---

# 14. Capability Rule Completeness

Have meta-tests that verify the following properties of policy rules.

```text
Every supported-domain ValueShape
must resolve to exactly one effective decision.
```

Neither of the following may be silently accepted:

```text
no matching capability rule
two conflicting rules match
```

If rule precedence is used, it must be explicit and deterministic.

---

# 15. Example Capability Rules

Conceptual examples:

```text
flagd:

MATCH
  Position = Root
  Kind     = List
DECISION
  Unsupported
```

```text
flagd:

MATCH
  Position = ObjectField
  Kind     = List
DECISION
  Supported
```

```text
flagd:

MATCH
  Position     = ObjectField
  Kind         = Int
  NumericClass = UnsafeJSONInteger
DECISION
  Unsupported
```

However, if the value is already rejected at B0 by the current IR v1, the last rule is treated as defense-in-depth / evidence documentation.

---

# 16. RuntimeVersionTuple Becomes RuntimeArtifactTuple

Record what was executed as identity, not just its version number.

Recommended type:

```go
type ModuleArtifact struct {
    Module  string
    Version string
    Sum     string
}

type ContainerArtifact struct {
    Image  string
    Tag    string
    Digest string
}

type ProtocolIdentity struct {
    ID string

    ProtoFile   string
    ProtoPackage string
    Service     string

    ProtocolVersion string
}

type RuntimeArtifactTuple struct {
    ID string

    RuntimeProfile RuntimeProfile

    RuntimeModule *ModuleArtifact
    ProviderModule *ModuleArtifact
    SDKModule *ModuleArtifact
    CoreModule *ModuleArtifact

    RuntimeContainer *ContainerArtifact

    Protocol ProtocolIdentity
}
```

Not every field needs to be required for every profile.

Populate only the fields applicable to the artifact.

---

# 17. Artifact Identity Rule

Runtime evidence containing only a version number is not reproducible evidence:

```text
version = X
```

At minimum, record the following:

For a Go module:

```text
module path
module version
```

must be recorded.

If possible:

```text
go.sum hash / immutable module checksum
```

also record the checksum.

For a container:

```text
image
tag
digest
```

must be recorded.

For CI evidence, prefer the digest over the tag as the identity.

---

# 18. flagd RPC ProtocolIdentity

Do not leave `flagd/rpc-v2` as an internal ffcraft nickname only.

Fix it to at least the following.

```text
ProtocolID:
  flagd-evaluation-v2

ProtoFile:
  flagd/evaluation/v2/evaluation.proto

ProtoPackage:
  flagd.evaluation.v2

Service:
  flagd.evaluation.v2.Service
```

Methods used by the typed contract:

```text
ResolveBoolean
ResolveString
ResolveFloat
ResolveInt
ResolveObject
```

Raw RPC contract tests must use this exact protocol identity.

Do not substitute another schema or service without authorization.

Items belonging to another API generation, such as `ResolveAll`, must be a separate suite from the Core typed-v2 contract.

---

# 19. GOFF Provider Identity

Do not infer the provider artifact from the RuntimeProfile name alone.

For example:

```text
gofeatureflag/in-process
```

is a semantic runtime profile, while

```text
the Go provider module actually used
```

is separate metadata.

RuntimeArtifactTuple must retain the following:

```text
ProviderModule
ProviderVersion
```

If the GOFF core dependency also affects runtime behavior,

```text
CoreModule
CoreVersion
```

fix it as a separate identity.

---

# 20. Compatibility Evidence

CompatibilityClass:

```text
the set of artifacts/versions that ffcraft declares to have the same capability
```

RuntimeArtifactTuple:

```text
the concrete set of artifacts actually tested
```

Flow:

```text
SupportedRuntimePolicy
    ↓
CompatibilityClass
    ↓
Representative RuntimeArtifactTuple
    ↓
Runtime Contract Evidence
    ↓
Effective Compile Capability
```

---

# 21. B9 Rename

Old:

```text
B9 Generated Adapter / Codegen
```

is prohibited.

New:

```text
B9 Generated Accessor / Typed API
```

Reason:

ffcodegen does not generate an OpenFeature adapter as production code.

The generated artifacts are SDK-agnostic typed accessors and a Client abstraction.

---

# 22. B8 → B9 Integration Shape

Actual chain:

```text
B8
OpenFeature Provider / SDK

    ↓

External Application Adapter

    ↓

B9
ffcraft Generated Accessor / Typed API
```

The External Application Adapter is not an ffcraft-generated artifact.

---

# 23. Reference Adapter for Tests

To verify B8→B9 E2E in P1,

```text
test-only reference OpenFeature adapter
```

it may be included in the repository.

Example:

```text
internal/integration/referenceadapter
```

or a similar location.

However, explicitly state that it is:

```text
- not production generated code
- not a public ffcraft API
- a compatibility test harness
```

This adapter

```text
generated Client interface
↔ OpenFeature Client
```

connects the following.

---

# 24. B9 Contract

What B9 guarantees:

```text
generated accessor signature is correct

generated context type is correct

generated Client contract is correct

runtime result can be decoded into expected generated Go type

list/object/scalar accessor does not lose intended type
```

For a GOFF root list, steady-state:

```text
GOFF runtime
→ OpenFeature structured result
→ reference adapter
→ generated Client interface
→ generated []T accessor
```

is verified in P1.

---

# 25. Runtime Lifecycle Scenario Dimension

Add this to the Input Dimension starting in P1.

```text
RuntimeLifecycleScenario
```

Minimum scenarios:

```text
ColdStart
InitialLoad

HotConfigUpdate

Disconnect
Reconnect

InitialConnectionFailure
RecoveryAfterInitialFailure

InvalidConfigUpdate
RecoveryAfterInvalidUpdate

Shutdown
```

N/A is allowed depending on the profile.

Silent omission of untested cases is prohibited.

---

# 26. Lifecycle Capability Is Separate from Compile Capability

Important:

```text
CompileTargetCapability
```

and

```text
RuntimeLifecycleCapability
```

must not be confused.

For example,

```text
flagd in-process:
object/list representation works
but reconnect lifecycle has upstream bug
```

In this case,

```text
Object/List compile capability = supported
```

may be recorded as Supported.

At the same time,

```text
InitialFailure → Reconnect → ContinuousUpdate
```

A lifecycle capability may be recorded as unsupported / known-broken.

A provider lifecycle bug alone does not require disabling compilation for all flag values.

---

# 27. Runtime Lifecycle Support Model

Concept:

```go
type RuntimeLifecycleCapability struct {
    Profile RuntimeProfile
    CompatibilityClass string

    Scenario RuntimeLifecycleScenario

    Status SupportStatus

    EvidenceIDs []string
    KnownIssueIDs []string
}
```

Status examples:

```text
Supported
Unsupported
KnownBroken
NotApplicable
NotIndependentlyObservable
```

---

# 28. Lifecycle Evidence Gate

When declaring a Lifecycle Scenario officially supported,
require positive contract evidence for

```text
every representative RuntimeArtifactTuple
```

If this conflicts with a known upstream bug,

```text
weakening the test expectation
```

is prohibited.

Do one of the following:

```text
Supported → KnownBroken

narrow the supported version range

update to a fixed provider version
```

---

# 29. P1 flagd Lifecycle Contracts

At minimum:

```text
B8-FLAGD-INPROCESS-HOTRELOAD-001
```

```text
B8-FLAGD-INPROCESS-INITFAIL-RECOVER-001
```

```text
B8-FLAGD-INPROCESS-INITFAIL-RECOVER-UPDATE-001
```

```text
B8-FLAGD-INPROCESS-SHUTDOWN-AFTER-RECOVERY-001
```

```text
B8-FLAGD-FILE-HOTRELOAD-001
```

For RPC as well, according to the support policy,

```text
B8-FLAGD-RPC-DISCONNECT-RECONNECT-001
```

must be added.

---

# 30. Lifecycle Assertions

It is insufficient for an evaluation to eventually return any result:

```text
evaluation eventually returns something
```

For a Reconnect scenario, at minimum:

```text
provider transitions to expected state

evaluation recovers

first recovered flag set is visible

subsequent config update is also visible

provider does not remain permanently frozen

shutdown terminates
```

must be verified.

Use bounded timeouts; infinite waits are prohibited.

---

# 31. GOFF Lifecycle Contracts

P1:

```text
B8-GOFF-INPROCESS-HOTRELOAD-001
B8-GOFF-INPROCESS-RECOVERY-001

B8-GOFF-REMOTE-DISCONNECT-RECONNECT-001
B8-GOFF-REMOTE-RELOAD-001

B8-GOFF-SHUTDOWN-001
```

Include only scenarios that are actually officially guaranteed in the support matrix.

---

# 32. P0 Scope Revision

Add numeric ingress safety to P0.

P0 requirements:

The following are design and implementation requirements for P0. For runtime profile evidence
and artifact identity, the verification foundation is considered complete for now; complete
registration of concrete artifact evidence is deferred to P1.

```text
A. Root List bug

flagd Root List reject
flagd Object/List pass
GOFF Root List runtime evidence foundation
```

```text
B. Lossless IR construction

Object nested unsafe integer
must be rejected before Struct conversion
```

```text
C. Runtime profile evidence foundation

flagd rpc/in-process/file
GOFF in-process/remote
```

```text
D. Artifact identity foundation

module/container/protocol identity model and validation
```

```text
E. Predicate-capable validation model

do not lock production design into
RootValueKinds/NestedValueKinds-only bitsets
```

The lifecycle scenarios themselves are P1.

---

# 33. P0 Numeric Acceptance Criteria

P0 completion requires the following.

```text
1.
9007199254740991 inside object
is preserved exactly
```

```text
2.
9007199254740992 inside object
is rejected before Struct construction
```

```text
3.
9007199254740993 inside object
is rejected before Struct construction
```

```text
4.
deeply nested unsafe integer
is also rejected
```

```text
5.
unsafe integer inside list inside object
is also rejected
```

```text
6.
root int64 values are not accidentally restricted
to JSON-safe integer range
```

```text
7.
no test detects the problem only after
the value has already been rounded
```

---

# 34. New Critical P0 Test IDs

Add:

```text
B0A-OBJECT-INT-SAFE-MAX-001
B0A-OBJECT-INT-UNSAFE-2P53-001
B0A-OBJECT-INT-LOSSY-2P53P1-001

B0A-DEEP-OBJECT-INT-LOSSY-001
B0A-OBJECT-LIST-INT-LOSSY-001

B0A-ROOT-INT-2P53P1-001
B0A-ROOT-INT64-MAX-001
B0A-ROOT-INT64-MIN-001
```

If direct normalized textual ingress exists:

```text
B0N-OBJECT-INT-LOSSY-001
```

this is also required.

---

# 35. Numeric P1 Scope

Deferring all numeric work to P1 is prohibited.

P0:

```text
precision loss during Common IR construction
```

must be prevented.

P1:

```text
target transport-specific numeric compatibility
```

must be expanded.

P1 scope:

```text
ROOT_INT transport

OBJECT_NUMBER transport

OBJECT_LIST_NUMBER transport

EVALUATION_CONTEXT_NUMBER

2^53 boundaries across each runtime profile

provider numeric coercion differences
```

In other words:

```text
IR precision safety = P0

full runtime numeric compatibility matrix = P1
```

At present, this P1 item provides the matrix types, boundaries, and evidence gate only.
Measured results for each runtime profile, provider coercion differences, and continuous artifact
evidence registration will be added when runtime compatibility is officially guaranteed.

---

# 36. Revised Core Invariants

## I0 — Lossless Ingress

```text
ffcraft-controlled decode/normalize must not silently
change a semantically significant source value.
```

## I1 — Normalized IR Safety

```text
ValidateNormalizedIR(doc) == nil
```

then the IR is internally consistent.

## I2 — Shape-aware Compile Capability

```text
ValidateCompileTarget(target, doc) == nil
```

then every emitted value shape satisfies

```text
Kind
× Position
× Parent
× Numeric Domain
× relevant semantics
```

the capability constraints applicable to

## I3 — Evidence-backed Support

Supported capabilities have positive evidence for the entire official support domain.

## I4 — Native Loadability

Supported artifacts can be loaded natively by the target at B6.

## I5 — Runtime Representation Safety

There is no representation/type failure at B7/B8 or in the combined contract.

## I6 — Provider Semantic Safety

The provider/SDK preserves the semantic result without hidden fallback.

## I7 — Generated Accessor Safety

The ffcraft generated accessor safely returns values obtained through the reference/runtime adapter as the expected type.

## I8 — Cross-target Semantic Safety

The common capability subset does not produce successfully-wrong results.

## I9 — Supported Lifecycle Safety

Lifecycle Scenarios declared officially supported have contract evidence from representative RuntimeArtifactTuples.

---

# 37. Final Concept Separation

Always distinguish the following.

```text
CompileTarget
≠ RuntimeProfile

RuntimeProfile
≠ Provider Artifact

Version
≠ Artifact Identity

CompatibilityClass
≠ Concrete RuntimeArtifactTuple

ValueKind
≠ ValueShape

Nested Value
≠ all nested shapes

Common IR Capability
≠ Target Capability

Compile Capability
≠ Lifecycle Capability

Generated Accessor
≠ OpenFeature Adapter

Lossless IR
≠ Lossless Runtime Transport

Runtime Success
≠ Semantic Correctness
```

The final goal is to ensure that the following path

```text
source value
→ IR
→ target artifact
→ target runtime
→ provider
→ application typed accessor
```

does not produce any of the following at any boundary:

```text
silent information loss
silent type coercion
unsupported representation
hidden fallback
successfully-wrong semantics
```
