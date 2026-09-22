#!/usr/bin/env python3
"""no-control-bytes.py — refuse a raw C0 control byte in a text source.

Why this is a script and not a grep
-----------------------------------
`BrowserPanel.cs` grew seventeen raw control bytes — NUL, SOH and STX
typed *literally* into C# string and char literals, as separators for the
list signatures that gate a redraw. They are semantically fine and the
compiler is happy. But **git calls any file with a NUL in its first 8000
bytes binary**, and a binary file has no diff, no blame, and no merge
resolution. The 1513-line centrepiece of a session's work rendered as
`Bin 36537 -> 64640 bytes` and reviewed as nothing at all.

The obvious guard is `grep -P '[\\x00-\\x1f]'` and **it does not work**.
GNU grep cannot match a NUL in a pattern at all — its matcher runs on
NUL-terminated C strings, so the byte can never appear in one. Run it
over a file `od -c` proves contains `\\0` and grep reports the file
clean, exits 1, and looks exactly like a pass. That false negative is
worse than no check: it is a sweep that certifies the one thing it is
structurally unable to see. (`grep -I` *classifies* rather than matches
and does see it, but it cannot say which byte or where.)

So: read the bytes and look at them.

What is refused, and what is not
--------------------------------
The C0 range except tab (0x09), LF (0x0a) and CR (0x0d), plus DEL (0x7f).
NUL is called out separately in the report because it is the byte with
the git consequence; the rest are refused as the same authoring mistake
caught one step earlier.

Escapes are the fix, never a `.gitattributes` override: `"\\u0000"` and
`'\\u0002'` compile to the identical characters and leave the file
textual. Marking the file binary instead would keep the bytes and throw
away the diff, which is the cost rather than the cause.

Usage: no-control-bytes.py FILE...   (exit 1 and a report if any is dirty)
"""

import sys

# Tab, LF and CR are the three C0 bytes that legitimately appear in a
# text source. Everything else in the range is an authoring accident.
ALLOWED = {0x09, 0x0A, 0x0D}


def offenders(path):
    """Return {byte: first offset} for each disallowed byte in `path`."""
    try:
        with open(path, "rb") as fh:
            blob = fh.read()
    except (OSError, IsADirectoryError):
        return {}

    found = {}
    for offset, byte in enumerate(blob):
        if (byte < 0x20 or byte == 0x7F) and byte not in ALLOWED:
            found.setdefault(byte, offset)
    return found


def main(argv):
    dirty = []
    for path in argv:
        found = offenders(path)
        if found:
            dirty.append((path, found))

    if not dirty:
        return 0

    print("  raw control bytes in text sources:")
    for path, found in dirty:
        detail = ", ".join(
            "0x%02x at byte %d" % (byte, offset)
            for byte, offset in sorted(found.items())
        )
        note = "  <-- NUL: git treats this file as BINARY" if 0 in found else ""
        print("    %s: %s%s" % (path, detail, note))
    print("  -> write them as escapes (\"\\u0000\", '\\u0002'); the compiler")
    print("     sees the same characters and the file keeps its diff.")
    return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
