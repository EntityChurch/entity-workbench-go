module entity-workbench-go/console

go 1.24

require (
	go.entitychurch.org/entity-core-go/core v0.8.0
	entity-workbench-go/entitysdk v0.0.0
	entity-workbench-go/shellboot v0.0.0
	entity-workbench-go/shellcmd v0.0.0
	entity-workbench-go/shellpanel v0.0.0
	entity-workbench-go/workbench v0.0.0
	github.com/gdamore/tcell/v2 v2.8.1
	github.com/rivo/tview v0.42.0
)

require (
	go.entitychurch.org/entity-core-go/ext v0.8.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/term v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
)

replace (
	go.entitychurch.org/entity-core-go/core => ../../entity-core-go/core
	go.entitychurch.org/entity-core-go/ext => ../../entity-core-go/ext
	entity-workbench-go/entitysdk => ../entitysdk
	entity-workbench-go/shellboot => ../shellboot
	entity-workbench-go/shellcmd => ../shellcmd
	entity-workbench-go/shellpanel => ../shellpanel
	entity-workbench-go/workbench => ../workbench
)
