package shellcmd

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// NormalizeAlias validates a user-supplied alias name and returns its
// NFC-normalized form. Per GUIDE-SHELL-FRAMING.md §6.1, alias names
// are Unicode-permissive: any codepoint sequence is accepted except
// those that would create structural ambiguity with the shell's
// path/sigil grammar.
//
// Rejected:
//   - empty string
//   - "@" (alias sigil + federation separator)
//   - "/" (path separator)
//   - ":" (protocol op-namespace separator)
//   - whitespace (token boundary)
//   - control characters (U+0000-U+001F, U+007F)
//   - shell quote/escape characters: ', ", \
//
// Beyond these structural restrictions, any Unicode codepoint is
// permitted — including non-Latin scripts, emoji, combining marks.
// NFC normalization is applied so visually-identical-but-byte-
// different inputs collapse to one stored form.
//
// Callers MUST apply this at alias-table write boundaries (connect
// <alias>, peer rename) so the stored alias is the canonical form
// that lookups will match against.
func NormalizeAlias(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("alias name must not be empty")
	}
	for _, r := range name {
		if err := checkAliasRune(r); err != nil {
			return "", err
		}
	}
	return norm.NFC.String(name), nil
}

func checkAliasRune(r rune) error {
	switch r {
	case '@':
		return fmt.Errorf("alias name must not contain '@' (reserved as alias sigil)")
	case '/':
		return fmt.Errorf("alias name must not contain '/' (path separator)")
	case ':':
		return fmt.Errorf("alias name must not contain ':' (reserved for protocol op-namespace)")
	case '\'', '"', '\\':
		return fmt.Errorf("alias name must not contain shell-escape characters (' \" \\)")
	}
	if unicode.IsSpace(r) {
		return fmt.Errorf("alias name must not contain whitespace")
	}
	if unicode.IsControl(r) {
		return fmt.Errorf("alias name must not contain control characters")
	}
	return nil
}

// IsReservedAlias reports whether the alias name is one of the
// built-in pronouns that the resolver special-cases (currently only
// "self"). Reserved names cannot be bound to a connection via
// `connect`; the caller surfaces an error before reaching the table.
func IsReservedAlias(name string) bool {
	return name == "self" || strings.EqualFold(name, "primary") || strings.EqualFold(name, "system")
}
