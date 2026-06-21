module entity-workbench-go/inspect

go 1.25.0

require (
	go.entitychurch.org/entity-core-go/core v0.8.0
	go.entitychurch.org/entity-core-go/ext v0.8.0
	entity-workbench-go/entitysdk v0.0.0
	github.com/fxamacker/cbor/v2 v2.9.0
)

replace (
	go.entitychurch.org/entity-core-go/core => ../../entity-core-go/core
	go.entitychurch.org/entity-core-go/ext => ../../entity-core-go/ext
	entity-workbench-go/entitysdk => ../entitysdk
)
