package fetch_test

import (
	"strings"
	"testing"

	"go.entitychurch.org/entity-core-go/core/types"

	"entity-workbench-go/fetch"
)

// An ALREADY-ABSOLUTE tree path names its own peer, and no layer may
// re-qualify it — `ENTITY-CORE-PROTOCOL` §1.4, which calls this *"the single
// most-recurring cross-impl bug class"* and names `/{local}//{other}/…` as its
// signature.
//
// The static road is where a republished view is literally a directory, so
// `{prefix}/{author}/system/signature/{hex}` is a file the origin serves under
// the AUTHOR while the layout belongs to the REPUBLISHER. Those are different
// peers, and every URL this package builds joined the layout's own peer-id.
//
// ⚠ **The old failure was silent, which is why this is a gate and not a
// comment.** `types.BuildTreeLeafURL` does a `TrimLeft(treePath, "/")`, so an
// absolute path lost its leading slash and was appended to a base that already
// ended in the serving peer — producing a syntactically perfect URL, under two
// peer-ids, for an object nobody publishes. A 404 from that reads as a
// withholding origin: an accusation against the publisher, caused by the
// reader.
func TestTreeLeafURL_AnAbsolutePathResolvesUnderThePeerItNames(t *testing.T) {
	const origin = "https://example.test"
	const serving = "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS"
	const author = "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sT"

	for _, tc := range []struct {
		name   string
		prefix string
		want   string
	}{
		{
			name:   "origin-rooted prefix — the named peer is appended",
			prefix: origin,
			want:   origin + "/" + author + "/system/signature/00ab.bin",
		},
		{
			name:   "peer-rooted prefix — the SERVING peer is stripped before the named one is joined",
			prefix: origin + "/" + serving,
			want:   origin + "/" + author + "/system/signature/00ab.bin",
		},
		{
			name:   "origin-rooted under a path — the path survives, the peer is still the named one",
			prefix: origin + "/mirrors/entity",
			want:   origin + "/mirrors/entity/" + author + "/system/signature/00ab.bin",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, err := fetch.LayoutFromProfile(origin, types.HTTPPollProfileData{
				PeerID: serving,
				Endpoint: types.TransportEndpoint{
					TreeURLPrefix:    tc.prefix,
					ContentURLPrefix: "/content",
					ContentLayout:    types.ContentLayoutSharded24,
				},
			})
			if err != nil {
				t.Fatalf("LayoutFromProfile: %v", err)
			}
			got := l.TreeLeafURL("/" + author + "/system/signature/00ab")
			if got != tc.want {
				t.Errorf("\n  got  %s\n  want %s", got, tc.want)
			}
			// The signature of the bug this replaces, asserted in its own
			// right: a URL naming BOTH peers is §1.4's prepend-local
			// double-qualification, and it is worth catching by its shape as
			// well as by inequality — the shape is what identifies the class.
			if strings.Contains(got, serving) && strings.Contains(got, author) {
				t.Errorf("the URL names BOTH the serving peer (%s) and the author (%s): %s",
					serving, author, got)
			}
		})
	}
}

// The control arm. A PEER-RELATIVE path still belongs to this layout's
// publisher, and must keep being joined to it — otherwise the fix above is not
// a fix, it is a change of meaning for every path this package already
// resolves. The two forms are not a mode: one names a path in the publisher's
// own namespace, the other names a path in somebody else's.
func TestTreeLeafURL_APeerRelativePathStillBelongsToThePublisher(t *testing.T) {
	const origin = "https://example.test"
	const serving = "2KFRBJ9feEPCZZiNaCEKsCAVkGkp1htWZk9a8jz5n5D2sS"

	l, err := fetch.LayoutFromProfile(origin, types.HTTPPollProfileData{
		PeerID: serving,
		Endpoint: types.TransportEndpoint{
			TreeURLPrefix:    origin,
			ContentURLPrefix: "/content",
			ContentLayout:    types.ContentLayoutSharded24,
		},
	})
	if err != nil {
		t.Fatalf("LayoutFromProfile: %v", err)
	}
	want := origin + "/" + serving + "/app/feed/index.bin"
	if got := l.TreeLeafURL("app/feed/index"); got != want {
		t.Errorf("a peer-relative path stopped resolving under its publisher:\n  got  %s\n  want %s",
			got, want)
	}
}
