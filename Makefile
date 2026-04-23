GO ?= go
BUF ?= buf
BINARY ?= ffcompile

.PHONY: build
build:
	$(GO) build ./cmd/ffcompile

.PHONY: proto
proto:
	$(BUF) generate

.PHONY: test
test:
	$(GO) test ./...

.PHONY: update-go-golden
update-go-golden:
	UPDATE_GOLDEN=1 $(GO) test ./internal/gogen ./cmd/ffcodegen

.PHONY: update-go-example
update-go-example:
	mkdir -p examples/go-codegen/basic/gen examples/go-codegen/rollout/gen examples/go-codegen/withhooks/gen
	$(GO) run ./cmd/ffcompile build flagd --in examples/go-codegen/basic/ffcompile.yaml --env prod --out examples/go-codegen/basic/gen/prod.flagd.json
	$(GO) run ./cmd/ffcompile build gofeatureflag --in examples/go-codegen/basic/ffcompile.yaml --env prod --out examples/go-codegen/basic/gen/prod.goff.yaml
	$(GO) run ./cmd/ffcodegen go --in examples/go-codegen/basic/ffcompile.yaml --config examples/go-codegen/basic/ffcodegen.yaml --out examples/go-codegen/basic/gen/featureflags_gen.go
	$(GO) run ./cmd/ffcompile build flagd --in examples/go-codegen/rollout/ffcompile.yaml --env prod --out examples/go-codegen/rollout/gen/prod.flagd.json
	$(GO) run ./cmd/ffcompile build gofeatureflag --in examples/go-codegen/rollout/ffcompile.yaml --env prod --out examples/go-codegen/rollout/gen/prod.goff.yaml
	$(GO) run ./cmd/ffcodegen go --in examples/go-codegen/rollout/ffcompile.yaml --config examples/go-codegen/rollout/ffcodegen.yaml --out examples/go-codegen/rollout/gen/featureflags_gen.go
	$(GO) run ./cmd/ffcompile build flagd --in examples/go-codegen/withhooks/ffcompile.yaml --env prod --out examples/go-codegen/withhooks/gen/prod.flagd.json
	$(GO) run ./cmd/ffcompile build gofeatureflag --in examples/go-codegen/withhooks/ffcompile.yaml --env prod --out examples/go-codegen/withhooks/gen/prod.goff.yaml
	$(GO) run ./cmd/ffcodegen go --in examples/go-codegen/withhooks/ffcompile.yaml --config examples/go-codegen/withhooks/ffcodegen.yaml --out examples/go-codegen/withhooks/gen/featureflags_gen.go

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -f $(BINARY)
