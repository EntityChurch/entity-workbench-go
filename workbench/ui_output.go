package workbench

import (
	"fmt"
	"strings"
)

// OutputLine is a renderer-neutral line of text output from content models.
// Renderers map Kind to their own color/style system.
type OutputLine struct {
	Text string
	Kind ValueKind
}

// FlattenFormattedLine converts a FormattedLine (from FormatCBOR) into an
// OutputLine with plain text and a kind tag. baseIndent is added to the
// line's own indent level.
func FlattenFormattedLine(line FormattedLine, baseIndent int) OutputLine {
	indent := strings.Repeat("  ", line.Indent+baseIndent)
	switch {
	case line.Key != nil && line.Value != nil:
		return OutputLine{
			Text: fmt.Sprintf("%s%s  %s", indent, line.Key.Text, line.Value.Text),
			Kind: line.Value.Kind,
		}
	case line.Key != nil:
		return OutputLine{
			Text: fmt.Sprintf("%s%s", indent, line.Key.Text),
			Kind: KindKey,
		}
	case line.Index >= 0 && line.Value != nil:
		return OutputLine{
			Text: fmt.Sprintf("%s[%d] %s", indent, line.Index, line.Value.Text),
			Kind: line.Value.Kind,
		}
	case line.Index >= 0:
		return OutputLine{
			Text: fmt.Sprintf("%s[%d]", indent, line.Index),
			Kind: KindNull,
		}
	case line.Value != nil:
		return OutputLine{
			Text: fmt.Sprintf("%s%s", indent, line.Value.Text),
			Kind: line.Value.Kind,
		}
	default:
		return OutputLine{Text: indent, Kind: KindNull}
	}
}

// FlattenFormattedLines converts a slice of FormattedLines to OutputLines.
func FlattenFormattedLines(lines []FormattedLine, baseIndent int) []OutputLine {
	out := make([]OutputLine, len(lines))
	for i, line := range lines {
		out[i] = FlattenFormattedLine(line, baseIndent)
	}
	return out
}

// LevelName returns a display name for a LogLevel.
func LevelName(l LogLevel) string {
	switch l {
	case LogVerbose:
		return "verbose"
	case LogDebug:
		return "debug"
	default:
		return "info"
	}
}

// ParseLevelName parses a level display name back to a LogLevel.
func ParseLevelName(s string) LogLevel {
	switch s {
	case "verbose":
		return LogVerbose
	case "debug":
		return LogDebug
	default:
		return LogInfo
	}
}
