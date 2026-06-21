module entity-workbench-go/workbench

go 1.24

require (
	go.entitychurch.org/entity-core-go/core v0.8.0
	go.entitychurch.org/entity-core-go/ext v0.8.0
	entity-workbench-go/entitysdk v0.0.0
	github.com/fxamacker/cbor/v2 v2.9.0
)

require (
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
)

replace (
	go.entitychurch.org/entity-core-go/core => ../../entity-core-go/core
	go.entitychurch.org/entity-core-go/ext => ../../entity-core-go/ext
	entity-workbench-go/entitysdk => ../entitysdk
)
