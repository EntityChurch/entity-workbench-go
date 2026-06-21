package shellcmd

import "strings"

// SplitArgs splits a shell input line into tokens, respecting single-
// and double-quoted strings. Quotes are stripped from the output —
// the contents become a single token regardless of internal
// whitespace. This is the canonical input tokenizer for shellcmd;
// the standalone REPL and any embedded shell panel must use it so
// users get consistent quoting behavior across surfaces.
//
// Notes:
//   - Unbalanced quotes are tolerated: the open-quote runs to end of
//     line and the partial token is emitted (matches earlier
//     standalone-REPL behavior).
//   - Backslash escapes are NOT supported; this is a shell-input
//     tokenizer, not a full shell parser. Use single quotes around
//     literal text containing double quotes (or vice versa).
//   - Empty input produces a nil slice.
func SplitArgs(line string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(line); i++ {
		c := line[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' || c == '\t' {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}
