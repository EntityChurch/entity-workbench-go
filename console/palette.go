package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	wb "entity-workbench-go/workbench"
)

type commandPalette struct {
	*tview.Flex
	ws       *workspace
	input    *tview.InputField
	list     *tview.List
	filtered []int
}

func newCommandPalette(ws *workspace) *commandPalette {
	cp := &commandPalette{
		Flex: tview.NewFlex().SetDirection(tview.FlexRow),
		ws:   ws,
	}

	cp.input = tview.NewInputField().
		SetLabel("> ").
		SetFieldBackgroundColor(tcell.ColorBlack).
		SetLabelColor(tcell.ColorSkyblue)

	cp.list = tview.NewList().
		ShowSecondaryText(true).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorSkyblue).
		SetSecondaryTextColor(tcell.ColorGray).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)

	cp.filter("")

	cp.input.SetChangedFunc(func(text string) {
		cp.filter(text)
	})

	cp.input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			ws.closePalette()
			return nil
		case tcell.KeyEnter:
			idx := cp.list.GetCurrentItem()
			if idx >= 0 && idx < len(cp.filtered) {
				cmd := wb.Registry[cp.filtered[idx]]
				ws.closePalette()
				ws.executeAction(cmd.Action)
			} else {
				ws.closePalette()
			}
			return nil
		case tcell.KeyDown, tcell.KeyTab:
			current := cp.list.GetCurrentItem()
			if current < cp.list.GetItemCount()-1 {
				cp.list.SetCurrentItem(current + 1)
			}
			return nil
		case tcell.KeyUp, tcell.KeyBacktab:
			current := cp.list.GetCurrentItem()
			if current > 0 {
				cp.list.SetCurrentItem(current - 1)
			}
			return nil
		}
		return event
	})

	// Center the palette on screen
	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cp.input, 1, 0, true).
		AddItem(cp.list, 0, 1, false)
	inner.SetBorder(true).
		SetTitle(" Commands ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorSkyblue)

	cp.AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(inner, 50, 0, true).
			AddItem(nil, 0, 1, false),
			10, 0, true).
		AddItem(nil, 0, 1, false)

	return cp
}

func (cp *commandPalette) filter(query string) {
	cp.filtered = wb.FilterCommands(query)
	cp.list.Clear()
	for _, idx := range cp.filtered {
		cmd := wb.Registry[idx]
		cp.list.AddItem(
			fmt.Sprintf(" %s", cmd.Name),
			fmt.Sprintf("   %s", cmd.Desc),
			0, nil,
		)
	}
}
