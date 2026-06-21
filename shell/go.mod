module entity-workbench-go/shell

go 1.24

require (
	go.entitychurch.org/entity-core-go/core v0.8.0
	entity-workbench-go/entitysdk v0.0.0
	entity-workbench-go/shellboot v0.0.0
	entity-workbench-go/shellcmd v0.0.0
)

require (
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/mattn/go-runewidth v0.0.3 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/peterh/liner v1.2.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/sys v0.0.0-20211117180635-dee7805ff2e1 // indirect
)

replace (
	go.entitychurch.org/entity-core-go/core => ../../entity-core-go/core
	entity-workbench-go/entitysdk => ../entitysdk
	entity-workbench-go/shellboot => ../shellboot
	entity-workbench-go/shellcmd => ../shellcmd
)
