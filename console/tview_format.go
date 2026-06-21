package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/rivo/tview"

	wb "entity-workbench-go/workbench"
)

// writeFormattedLine writes a single wb.FormattedLine to w with tview
// color tags. baseIndent is added to the line's own indent level.
func writeFormattedLine(w io.Writer, line wb.FormattedLine, baseIndent int) {
	prefix := strings.Repeat("  ", line.Indent+baseIndent)
	switch {
	case line.Key != nil && line.Value != nil:
		fmt.Fprintf(w, "%s[yellow]%s[-]  %s\n", prefix, tview.Escape(line.Key.Text), tviewColor(line.Value))
	case line.Key != nil:
		fmt.Fprintf(w, "%s[yellow]%s[-]\n", prefix, tview.Escape(line.Key.Text))
	case line.Index >= 0 && line.Value != nil:
		fmt.Fprintf(w, "%s[gray][%d][-] %s\n", prefix, line.Index, tviewColor(line.Value))
	case line.Index >= 0:
		fmt.Fprintf(w, "%s[gray][%d][-]\n", prefix, line.Index)
	case line.Value != nil:
		fmt.Fprintf(w, "%s%s\n", prefix, tviewColor(line.Value))
	}
}

// tviewColor wraps a FormattedValue with tview color tags based on kind.
func tviewColor(fv *wb.FormattedValue) string {
	switch fv.Kind {
	case wb.KindNull:
		return fmt.Sprintf("[gray]%s[-]", tview.Escape(fv.Text))
	case wb.KindBool:
		return fmt.Sprintf("[purple]%s[-]", fv.Text)
	case wb.KindString:
		return fmt.Sprintf("[green]%s[-]", tview.Escape(fv.Text))
	case wb.KindNumber:
		return fmt.Sprintf("[purple]%s[-]", fv.Text)
	case wb.KindBytes:
		return fmt.Sprintf("[teal]%s[-]", fv.Text)
	case wb.KindHash:
		return fmt.Sprintf("[blue]%s[-]", fv.Text)
	default:
		return fmt.Sprintf("[white]%s[-]", tview.Escape(fv.Text))
	}
}
