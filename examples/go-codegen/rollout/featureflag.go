package rollout

//go:generate -command ffcompile go run ../../../cmd/ffcompile
//go:generate -command ffcodegen go run ../../../cmd/ffcodegen

//go:generate ffcompile build flagd --in ./ffcompile.yaml --env prod --out ./gen/prod.flagd.json
//go:generate ffcodegen go --in ./ffcompile.yaml --config ./ffcodegen.yaml --out ./gen/featureflags_gen.go
