package entitysdk

import "testing"

// TestDefaultPropagation pins the guide §5.3 defaults so a future
// change to the propagation map surfaces in CI rather than silently
// shifting cross-panel behavior.
func TestDefaultPropagation(t *testing.T) {
	cases := []struct {
		event ActionEvent
		want  Propagation
	}{
		{EventNavigate, PropContext},
		{EventSelect, PropContext},
		{EventSubmit, PropPanel},
		{EventClear, PropPanel},
		{EventSetFilter, PropPanel},
		{EventToggleRaw, PropPanel},
		// Unknown events fall back to panel (safe default).
		{ActionEvent("not-a-real-event"), PropPanel},
	}
	for _, c := range cases {
		if got := DefaultPropagation(c.event); got != c.want {
			t.Errorf("DefaultPropagation(%q) = %v, want %v", c.event, got, c.want)
		}
	}
}
