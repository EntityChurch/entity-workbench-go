// Package shellpanel adapts shellcmd to the workbench panel surface:
// a renderer-neutral ShellModel that wraps shellcmd.Shell, dispatches
// commands through shellcmd.Registry, and produces []OutputLine for
// canvas + console panel renderers. It is the Stage 2 deliverable of
// PHASE-G-SHELL-CENTRIC-UI-PLAN.md.
//
// Two halves:
//   - render.go: RenderResult — a panel-side equivalent of
//     shell/format.go::FormatText, emitting structured OutputLines
//     instead of writing to a stdout-shaped io.Writer.
//   - model.go: ShellModel — the per-panel state (history, output,
//     submitter). Per-panel because each shell panel has its own
//     scrollback + WD; the underlying ShellWorkspace is shared.
package shellpanel

import (
	"fmt"
	"strings"

	"go.entitychurch.org/entity-core-go/core/entity"

	"entity-workbench-go/shellcmd"
	wb "entity-workbench-go/workbench"
)

// RenderResult converts a shellcmd.Result into structured OutputLines
// suitable for canvas + console panel rendering. The mapping is a
// faithful sibling of shell/format.go::FormatText — same logical
// output, structured for ValueKind-aware renderers instead of stdout.
//
// Empty results return nil (matches shell convention: cd succeeds
// silently, the prompt advances and nothing is printed).
func RenderResult(r shellcmd.Result) []wb.OutputLine {
	switch r.Kind {
	case shellcmd.KindNone:
		return nil
	case shellcmd.KindMessage:
		return []wb.OutputLine{{Text: r.Message, Kind: wb.KindString}}
	case shellcmd.KindPath:
		return []wb.OutputLine{{Text: r.Path.String(), Kind: wb.KindPath}}
	case shellcmd.KindLines:
		return renderLines(r.Lines)
	case shellcmd.KindListing:
		return renderListing(r.Listing)
	case shellcmd.KindEntity:
		return renderEntity(r.Entity)
	case shellcmd.KindTree:
		return renderTree(r.Tree)
	case shellcmd.KindDispatch:
		return renderDispatch(r.Dispatch)
	case shellcmd.KindInfo:
		return renderInfo(r.Info)
	}
	return nil
}

// RenderError formats an error from Registry.Dispatch as a single
// kind-Error OutputLine. Mirrors the REPL's stderr behavior.
func RenderError(err error) wb.OutputLine {
	return wb.OutputLine{Text: "error: " + err.Error(), Kind: wb.KindError}
}

func renderLines(lines []string) []wb.OutputLine {
	out := make([]wb.OutputLine, len(lines))
	for i, l := range lines {
		out[i] = wb.OutputLine{Text: l, Kind: wb.KindUnknown}
	}
	return out
}

func renderListing(rows []shellcmd.ListingRow) []wb.OutputLine {
	if len(rows) == 0 {
		return []wb.OutputLine{{Text: "  (empty)", Kind: wb.KindNull}}
	}
	out := make([]wb.OutputLine, 0, len(rows))
	for _, row := range rows {
		switch row.Kind {
		case "connection":
			out = append(out, wb.OutputLine{
				Text: fmt.Sprintf("  %-12s %s", row.Name, row.Detail),
				Kind: wb.KindKey,
			})
		default:
			suffix := ""
			if row.HasChildren {
				suffix = "/"
			}
			out = append(out, wb.OutputLine{
				Text: fmt.Sprintf("  %-30s %s", row.Name+suffix, row.Kind),
				Kind: wb.KindPath,
			})
		}
	}
	return out
}

func renderEntity(p *shellcmd.EntityPayload) []wb.OutputLine {
	if p == nil {
		return nil
	}
	if p.Diag {
		return []wb.OutputLine{{Text: p.Entity.DiagnoseHash(), Kind: wb.KindHash}}
	}
	out := []wb.OutputLine{
		{Text: "Type:  " + p.Entity.Type, Kind: wb.KindKey},
		{Text: "Hash:  " + p.Entity.ContentHash.String(), Kind: wb.KindHash},
	}
	if p.Decoded == nil {
		out = append(out, wb.OutputLine{
			Text: fmt.Sprintf("Data:  (raw, %d bytes)", len(p.Entity.Data)),
			Kind: wb.KindNull,
		})
		return out
	}
	out = append(out, wb.OutputLine{Text: "Data:", Kind: wb.KindKey})
	for _, line := range wb.FormatCBOR(p.Decoded) {
		out = append(out, wb.FlattenFormattedLine(line, 1))
	}
	return out
}

func renderTree(rows []shellcmd.TreeRow) []wb.OutputLine {
	out := make([]wb.OutputLine, 0, len(rows))
	for _, row := range rows {
		indent := strings.Repeat("  ", row.Depth)
		suffix := ""
		if row.HasChildren {
			suffix = "/"
		}
		if row.Entity != nil {
			out = append(out, wb.OutputLine{
				Text: fmt.Sprintf("%s%s%s  [%s] %s",
					indent, row.Name, suffix, row.Entity.Type, row.Entity.ContentHash),
				Kind: wb.KindPath,
			})
			if row.Decoded != nil {
				for _, line := range wb.FormatCBOR(row.Decoded) {
					out = append(out, wb.FlattenFormattedLine(line, row.Depth+2))
				}
			}
		} else {
			out = append(out, wb.OutputLine{
				Text: fmt.Sprintf("%s%s%s", indent, row.Name, suffix),
				Kind: wb.KindPath,
			})
		}
	}
	return out
}

func renderDispatch(d *shellcmd.DispatchResp) []wb.OutputLine {
	if d == nil {
		return nil
	}
	out := []wb.OutputLine{
		{Text: fmt.Sprintf("Status: %d", d.Status), Kind: wb.KindNumber},
		{Text: "Type:   " + d.Result.Type, Kind: wb.KindKey},
		{Text: "Hash:   " + d.Result.ContentHash.String(), Kind: wb.KindHash},
	}
	if d.Decoded != nil {
		out = append(out, wb.OutputLine{Text: "Data:", Kind: wb.KindKey})
		for _, line := range wb.FormatCBOR(d.Decoded) {
			out = append(out, wb.FlattenFormattedLine(line, 1))
		}
	}
	if d.Included > 0 {
		out = append(out, wb.OutputLine{
			Text: fmt.Sprintf("Included: %d entities", d.Included),
			Kind: wb.KindNull,
		})
	}
	return out
}

func renderInfo(info *shellcmd.PeerInfo) []wb.OutputLine {
	if info == nil {
		return nil
	}
	out := []wb.OutputLine{
		{Text: "Alias:   " + info.Alias, Kind: wb.KindKey},
		{Text: "Address: " + info.Address, Kind: wb.KindKey},
		{Text: "PeerID:  " + info.PeerID, Kind: wb.KindHash},
	}
	if len(info.Grants) > 0 {
		out = append(out, wb.OutputLine{
			Text: fmt.Sprintf("Grants:  %d", len(info.Grants)),
			Kind: wb.KindNull,
		})
	}
	return out
}

// Compile-time guard: a non-empty entity.Entity has a Type field. If
// the upstream entity package ever renames or removes Type, this fails
// at build, signalling the renderers above need updating.
var _ = entity.Entity{}.Type
