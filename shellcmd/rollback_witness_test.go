package shellcmd

import (
	"strings"
	"testing"
)

// A-33's report, at the layer where the question has exactly one answer.
//
// The panel-side test asserts the sentence reaches a pixel; it cannot
// assert the NEGATIVE, because the headless fixture peer is shared and
// another test accepts a real folder into it, so *"a send-only folder
// says nothing"* is unprovable over that panel (AP70). Here there is no
// shared anything.
//
// The negative is the arm that matters: a line drawn unconditionally
// would claim an undefended incoming leg on a peer that only publishes,
// which is a confident answer about a leg that does not exist.
func TestRollbackWitness_OnlyWhereThereIsAnIncomingLeg(t *testing.T) {
	for _, c := range []struct {
		name    string
		fs      FolderStatus
		wantSay bool
	}{
		{"a received folder has an incoming leg",
			FolderStatus{Local: false, Accepted: true, RollbackWitness: WitnessNotSupported}, true},
		{"a folder we own that also pulls has one",
			FolderStatus{Local: true, ReceiveFrom: []string{"peer"}, RollbackWitness: WitnessNotSupported}, true},
		{"a send-only folder has none",
			FolderStatus{Local: true}, false},
	} {
		note := c.fs.RollbackWitnessNote()
		if (note != "") != c.wantSay {
			t.Errorf("%s: note=%q, want spoken=%v", c.name, note, c.wantSay)
		}
		if c.wantSay {
			// The sentence has to carry both halves: WHAT the state is
			// (`not_supported`, the ruling's own spelling, so it is
			// greppable against the text) and WHAT IT MEANS for the
			// operator. A field name alone is not a report.
			for _, want := range []string{"not_supported", "NOT rollback-protected"} {
				if !strings.Contains(note, want) {
					t.Errorf("%s: note does not mention %q: %q", c.name, want, note)
				}
			}
		}
	}
}
