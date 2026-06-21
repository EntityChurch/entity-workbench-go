package workbench

import (
	"context"
	"testing"

	"go.entitychurch.org/entity-core-go/core/handler"
)

func TestStripPeerIDPrefix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/peer123/local/files/root/a.md", "local/files/root/a.md"},
		{"/peer123/x", "x"},
		{"no-leading-slash", "no-leading-slash"},
		{"/onlyonesegment", "/onlyonesegment"}, // no second slash → unchanged
		{"/p/", ""},
		{"/p/a/b/c", "a/b/c"},
	}
	for _, c := range cases {
		if got := stripPeerIDPrefix(c.in); got != c.want {
			t.Fatalf("stripPeerIDPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsMarkdownPath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"a.md", true}, {"a.MD", true}, {"a.markdown", true}, {"a.MARKDOWN", true},
		{"dir/sub/file.md", true},
		{"a.txt", false}, {"a", false}, {"a.mdx", false}, {"README", false},
	}
	for _, c := range cases {
		if got := isMarkdownPath(c.in); got != c.want {
			t.Fatalf("isMarkdownPath(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNotificationIngest_Guards(t *testing.T) {
	h := NewNotificationIngestHandler(nil)

	// Unknown operation.
	resp, err := h.Handle(context.Background(), &handler.Request{
		Operation: "frob", Params: notifEntity(t, "/p/local/files/r/a.md"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 400 || errCode(t, resp) != "unknown_operation" {
		t.Fatalf("unknown op: status=%d code=%q", resp.Status, errCode(t, resp))
	}

	// Missing handler context (no store / location index).
	resp, err = h.Handle(context.Background(), &handler.Request{
		Operation: "receive", Params: notifEntity(t, "/p/local/files/r/a.md"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 500 || errCode(t, resp) != "internal_error" {
		t.Fatalf("nil ctx: status=%d code=%q", resp.Status, errCode(t, resp))
	}
}

// RegisterMount normalizes a missing trailing slash and the URI match
// is longest-prefix; UnregisterMount removes it. Observed through the
// handler's own response codes: a matched mount advances past the
// "no_mount_for_uri" guard to "source_not_bound" (the FileData isn't
// in the test store); an unmatched URI stays at "no_mount_for_uri".
func TestNotificationIngest_MountRegistrationRouting(t *testing.T) {
	_, s, li := testPeerContext(t)
	hctx := &handler.HandlerContext{Store: s, LocationIndex: li}
	h := NewNotificationIngestHandler(nil)

	req := func() *handler.Request {
		return &handler.Request{
			Operation: "receive",
			Params:    notifEntity(t, "/peerX/local/files/repo/notes/a.md"),
			Context:   hctx,
		}
	}

	// No mounts yet → 404 no_mount_for_uri.
	resp, _ := h.Handle(context.Background(), req())
	if resp.Status != 404 || errCode(t, resp) != "no_mount_for_uri" {
		t.Fatalf("no mount: status=%d code=%q", resp.Status, errCode(t, resp))
	}

	// Register WITHOUT trailing slashes; handler must normalize and match.
	h.RegisterMount("local/files/repo", "archives")
	resp, _ = h.Handle(context.Background(), req())
	if resp.Status != 404 || errCode(t, resp) != "source_not_bound" {
		t.Fatalf("matched mount should advance to source_not_bound, got status=%d code=%q",
			resp.Status, errCode(t, resp))
	}

	// Unregister → back to no_mount_for_uri.
	h.UnregisterMount("local/files/repo")
	resp, _ = h.Handle(context.Background(), req())
	if resp.Status != 404 || errCode(t, resp) != "no_mount_for_uri" {
		t.Fatalf("after unmount: status=%d code=%q", resp.Status, errCode(t, resp))
	}
}

func TestNotificationIngest_ManifestAndName(t *testing.T) {
	h := NewNotificationIngestHandler(nil)
	if h.Name() != "workbench-notification-ingest" {
		t.Fatalf("Name() = %q", h.Name())
	}
	if mf := h.Manifest(); mf.Pattern != NotificationIngestPattern {
		t.Fatalf("Manifest Pattern = %q, want %q", mf.Pattern, NotificationIngestPattern)
	}
}
