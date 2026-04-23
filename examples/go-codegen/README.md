# go-codegen examples

- `basic`: minimal generated accessors plus an OpenFeature runtime example
- `rollout`: generated rollout accessors with targeting key handling
- `withhooks`: generated accessors plus OpenFeature hook wiring
- `adapter`: reusable OpenFeature adapter for generated `EvaluationContext` types
- `hooks`: shared hook implementations for the examples

Generate artifacts:

```bash
make update-go-example
```

Run the examples:

```bash
cd examples/go-codegen
go run ./basic/flagd
go run ./basic/gofeatureflag
go run ./rollout/flagd
go run ./rollout/gofeatureflag
go run ./withhooks/flagd
go run ./withhooks/gofeatureflag
```
