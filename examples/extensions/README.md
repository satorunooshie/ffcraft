# Extension ownership example

This example shows one flag definition shared by several consumers. The
feature-flag behavior is core ffcraft data; consumer-owned information is kept
in independently named extension namespaces.

```text
checkout
  ├─ core semantics       -> every compiler
  ├─ com.example.client.v1 -> client tooling
  ├─ com.example.backend.v1 -> backend tooling
  └─ com.example.team.v1   -> team tooling
```

The same namespace can appear at document, flag, and environment scope. These
values are independent. ffcraft does not merge or inherit them.

## 1. Authoring YAML to public protobuf IR

`ffcompile.yaml` is the source-of-truth authoring format. The compiler input
after normalization is only `ffcraft.ir.v1` protobuf:

```bash
go run ./cmd/ffcompile normalize examples/extensions/ffcompile.yaml \
  --format protobuf --out /tmp/featureflags.ir.v1.pb

go run ./cmd/ffcompile compile flagd \
  --in /tmp/featureflags.ir.v1.pb --env prod

go run ./cmd/ffcompile compile gofeatureflag \
  --in /tmp/featureflags.ir.v1.pb --env prod
```

The deterministic human-readable view is output-only. It is useful for code
review and debugging, but it is not accepted by `ffcompile compile`:

```bash
go run ./cmd/ffcompile normalize examples/extensions/ffcompile.yaml \
  --format yaml --out /tmp/extensions.normalized.yaml
```

The checked-in [normalized.yaml](normalized.yaml) is an example of this view.
For example, the extension data is visible under the flag and environment
scope:

```yaml
flags:
    checkout:
        extensions:
            com.example.client.v1:
                object_value:
                    fields:
                        analytics_event:
                            string_value: checkout_evaluated
```

## 2. External protobuf consumer

`consumer/main.go` intentionally imports only the public generated protobuf
package and `google.golang.org/protobuf`. It demonstrates separate ownership:

```bash
go run ./examples/extensions/consumer \
  --in /tmp/featureflags.ir.v1.pb
```

Example output:

```text
client exposure: ...
backend audit: ...
team owner: growth-platform
client cache_ttl_seconds: 60
backend rollout_ticket: ROLLOUT-123
team oncall: growth-platform
```

The consumer reads `com.example.client.v1`, `com.example.backend.v1`, and
`com.example.team.v1` independently. It does not need to understand or
validate another team's namespace.

## 3. Core output comparison

The [comparison test](extensions_test.go) compiles the example twice: once
with all extensions and once after removing every extension value. It asserts
byte-for-byte equality for both target outputs:

```bash
go test ./examples/extensions/...
```

This is the concrete guarantee that extensions are metadata for consumers, not
core flag behavior. The same property is visible from the CLI:

```bash
go run ./cmd/ffcompile compile flagd \
  --in /tmp/featureflags.ir.v1.pb --env prod --out /tmp/with-extensions.flagd.json
go run ./cmd/ffcompile compile gofeatureflag \
  --in /tmp/featureflags.ir.v1.pb --env prod --out /tmp/with-extensions.goff.yaml
```

Neither output contains `com.example.client.v1`, `com.example.backend.v1`, or
`com.example.team.v1`; those namespaces remain in the protobuf IR for the
consumers that own them.

The relevant target output is deliberately just core behavior:

```json
{
  "flags": {
    "checkout": {
      "state": "ENABLED",
      "variants": {"off": false, "on": true},
      "defaultVariant": "off"
    }
  }
}
```

The GO Feature Flag output has the same boundary:

```yaml
checkout:
    variations:
        "off": false
        "on": true
    defaultRule:
        variation: "off"
```

For a human-readable dump alongside a target build:

```bash
go run ./cmd/ffcompile build flagd \
  --in examples/extensions/ffcompile.yaml --env prod --dump -
```
