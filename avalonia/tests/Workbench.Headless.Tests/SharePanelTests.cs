using System.Text.Json;
using EntityAvalonia.Panels;
using Xunit;

namespace Workbench.Headless.Tests;

// SharePanel's wire contract.
//
// These deserialize LITERAL bridge payloads through the panel's own
// options and types, which is the only shape that catches AP49:
// System.Text.Json discards an undeclared member in total silence, so a
// field the operation computes and the bridge sends can arrive as
// false/null/"" with every other test still green. Every fixture value
// below is deliberately distinguishable from its type's zero — a dropped
// field reads as the zero, so a fixture full of zeros asserts nothing.
//
// Three of these fields are not decoration, and losing any one of them
// silently reproduces a failure mode this repo has already paid for:
//
//   * `reconnected` — the grant set is assembled at HANDSHAKE, so a
//     policy written on a live connection is inert until it is
//     re-established. Dropped, the panel reports a share that does
//     nothing as a success.
//   * `publisherMustDial` — over a dial-by-address connection the
//     kernel's reciprocal grant is one-directional, so after an accept
//     the publisher must dial back or deliveries are refused. Dropped,
//     the folder stays empty and NOTHING on either side says why.
//   * `caveat` — withdrawal stops the next handshake and does not reach
//     into a live connection. Dropped, the UI implies a revocation
//     stronger than the one that happened.
public class SharePanelTests
{
    private static readonly JsonSerializerOptions Opts = new() { PropertyNameCaseInsensitive = true };

    private static T Parse<T>(string json) => JsonSerializer.Deserialize<T>(json, Opts)!;

    [Fact]
    public void The_Render_Envelope_Does_Not_Drop_Any_Field()
    {
        const string json = """
        {
          "ok": true,
          "error": "",
          "localPeerId": "12D3KooWLocalPeerIdentifier",
          "localAlias": "self",
          "listenAddr": "0.0.0.0:9000",
          "advertisedUrl": "tcp://192.168.1.20:9000",
          "peers": [{
            "peerId": "12D3KooWRemotePeerIdentifier",
            "alias": "laptop",
            "address": "192.168.1.31:9000",
            "connected": true,
            "source": "connected"
          }],
          "mounts": [{ "root": "notes", "targetPrefix": "archives/notes/", "sharedWith": 2 }],
          "shares": [{
            "root": "notes",
            "title": "My notes",
            "targetPrefix": "archives/notes/",
            "audience": ["12D3KooWRemotePeerIdentifier"],
            "createdAtMillis": 1756800000000
          }],
          "syncs": [{
            "remotePeerId": "12D3KooWRemotePeerIdentifier",
            "remoteAlias": "laptop",
            "root": "notes",
            "sourcePrefix": "local/files/notes/",
            "targetPrefix": "archives/notes/",
            "live": true
          }],
          "problems": ["one share record would not decode"]
        }
        """;

        var v = Parse<SharePanel.RenderEnvelope>(json);
        Assert.True(v.Ok);
        Assert.Equal("12D3KooWLocalPeerIdentifier", v.LocalPeerId);
        Assert.Equal("self", v.LocalAlias);
        Assert.Equal("0.0.0.0:9000", v.ListenAddr);
        Assert.Equal("tcp://192.168.1.20:9000", v.AdvertisedUrl);

        var p = Assert.Single(v.Peers!);
        Assert.Equal("12D3KooWRemotePeerIdentifier", p.PeerId);
        Assert.Equal("laptop", p.Alias);
        Assert.Equal("192.168.1.31:9000", p.Address);
        Assert.True(p.Connected);
        // "connected" is a fact and "discovery" is an advertisement. The
        // picker renders the difference, so the field has to survive.
        Assert.Equal("connected", p.Source);

        var m = Assert.Single(v.Mounts!);
        Assert.Equal("notes", m.Root);
        Assert.Equal("archives/notes/", m.TargetPrefix);
        Assert.Equal(2, m.SharedWith);

        var s = Assert.Single(v.Shares!);
        Assert.Equal("notes", s.Root);
        Assert.Equal("My notes", s.Title);
        Assert.Equal("archives/notes/", s.TargetPrefix);
        Assert.Equal(new[] { "12D3KooWRemotePeerIdentifier" }, s.Audience!);
        Assert.Equal(1756800000000UL, s.CreatedAtMillis);

        var sy = Assert.Single(v.Syncs!);
        Assert.Equal("12D3KooWRemotePeerIdentifier", sy.RemotePeerId);
        Assert.Equal("laptop", sy.RemoteAlias);
        Assert.Equal("notes", sy.Root);
        Assert.Equal("local/files/notes/", sy.SourcePrefix);
        Assert.Equal("archives/notes/", sy.TargetPrefix);
        Assert.True(sy.Live);

        // A share record that will not decode is a thing the operator
        // has. Dropping it renders a broken share as no share at all.
        Assert.Equal(new[] { "one share record would not decode" }, v.Problems!);
    }

    [Fact]
    public void A_Share_Carries_Per_Peer_State_For_The_Reciprocal_Dial()
    {
        // audienceState is what turns the last step of a share from an
        // instruction into a button. Without it the panel cannot tell
        // "we can dial them right now" from "we have no idea where they
        // listen", and the only honest thing left to render is a shell
        // command — which is what it did, and what got it thrown back.
        const string json = """
        {
          "ok": true,
          "shares": [{
            "root": "downloads",
            "title": "Downloads",
            "targetPrefix": "local/files/downloads/",
            "audience": ["12D3KooWKnown", "12D3KooWUnknown"],
            "createdAtMillis": 1756800000000,
            "audienceState": [
              { "peerId": "12D3KooWKnown", "alias": "laptop", "connected": true,
                "address": "192.168.1.31:9000", "addressSource": "discovery" },
              { "peerId": "12D3KooWUnknown", "alias": "", "connected": false,
                "address": "", "addressSource": "" }
            ]
          }]
        }
        """;

        var v = Parse<SharePanel.RenderEnvelope>(json);
        var s = Assert.Single(v.Shares!);
        Assert.Equal(2, s.AudienceState!.Count);

        var known = s.AudienceState[0];
        Assert.Equal("laptop", known.Alias);
        Assert.True(known.Connected);
        Assert.Equal("192.168.1.31:9000", known.Address);
        // The source is rendered because the claims differ in strength:
        // an address we dialled before worked once; an mDNS announcement
        // is what the peer says about itself.
        Assert.Equal("discovery", known.AddressSource);

        // The whole point of the second entry: an empty address is a
        // real, expected state, and the panel turns it into a field
        // rather than an error.
        Assert.Empty(s.AudienceState[1].Address);
    }

    [Fact]
    public void Needing_An_Address_Is_Not_An_Error()
    {
        // ok=true AND connected=false AND needsAddress=true. If the panel
        // read this as a failure it would tell the operator something
        // broke, when the truth is that we have never dialled this peer
        // and they are not announcing — a question, not a fault.
        const string json = """
        {
          "ok": true,
          "error": "",
          "peerId": "12D3KooWRemotePeerIdentifier",
          "peerAlias": "",
          "address": "",
          "addressSource": "",
          "connected": false,
          "needsAddress": true,
          "note": "no address is known for this peer"
        }
        """;

        var v = Parse<SharePanel.CompleteReply>(json);
        Assert.True(v.Ok);
        Assert.False(v.Connected);
        Assert.True(v.NeedsAddress);
        Assert.StartsWith("no address is known", v.Note);
    }

    [Fact]
    public void A_Completed_Dial_Reports_Which_Address_It_Used()
    {
        const string json = """
        {
          "ok": true,
          "error": "",
          "peerId": "12D3KooWRemotePeerIdentifier",
          "peerAlias": "laptop",
          "address": "192.168.1.31:9000",
          "addressSource": "connection-table",
          "connected": true,
          "needsAddress": false,
          "note": ""
        }
        """;

        var v = Parse<SharePanel.CompleteReply>(json);
        Assert.True(v.Ok);
        Assert.True(v.Connected);
        Assert.False(v.NeedsAddress);
        Assert.Equal("192.168.1.31:9000", v.Address);
        Assert.Equal("connection-table", v.AddressSource);
    }

    [Fact]
    public void The_Accept_Reply_Keeps_The_Step_Neither_Side_Can_Perform_Alone()
    {
        const string json = """
        {
          "ok": true,
          "error": "",
          "peerId": "12D3KooWRemotePeerIdentifier",
          "peerAlias": "laptop",
          "root": "notes",
          "policyPath": "system/capability/policy/12D3KooWRemotePeerIdentifier",
          "grantSummary": "workbench/blob-resolve:*",
          "sourcePrefix": "local/files/notes/",
          "targetPrefix": "archives/notes/",
          "targetRoot": "notes",
          "subscriptionId": "sub-7f21",
          "reconnected": true,
          "reconnectNote": "re-dialled 192.168.1.31:9000",
          "publisherMustDial": "connect 12D3KooWLocalPeerIdentifier <this-peer's host:port>"
        }
        """;

        var v = Parse<SharePanel.AcceptReply>(json);
        Assert.True(v.Ok);
        Assert.Equal("notes", v.Root);
        Assert.Equal("archives/notes/", v.TargetPrefix);
        Assert.Equal("sub-7f21", v.SubscriptionId);
        Assert.True(v.Reconnected);
        Assert.Equal("re-dialled 192.168.1.31:9000", v.ReconnectNote);
        // The whole reason the panel exists rather than a doc paragraph.
        Assert.Equal("connect 12D3KooWLocalPeerIdentifier <this-peer's host:port>",
            v.PublisherMustDial);
    }

    // AP49 for the field that answers the operator's actual question.
    //
    // `backfill` carries how many of the files ALREADY in the remote
    // folder came across. An undeclared field is dropped by
    // System.Text.Json in silence, and a drop here reproduces exactly
    // the symptom the backfill was written to fix: an empty folder and a
    // surface that says nothing about it. Asserted on arrival, not
    // implied by the panel rendering something.
    [Fact]
    public void The_Accept_Reply_Keeps_The_Count_Of_Files_That_Came_Across()
    {
        const string json = """
        {
          "ok": true,
          "root": "notes",
          "publisherMustDial": "connect 12D3KooWLocalPeerIdentifier <host:port>",
          "backfill": {
            "scanned": 12,
            "materialized": 11,
            "alreadyCurrent": 0,
            "skipped": 0,
            "failed": 1,
            "truncated": false,
            "errors": ["local/files/notes/big.bin: blob closure fetch from 12D3Koo…: 403"],
            "summary": "12 file(s) found, 11 transferred, 1 FAILED",
            "ran": true
          }
        }
        """;

        var v = Parse<SharePanel.AcceptReply>(json);
        Assert.NotNull(v.Backfill);
        Assert.True(v.Backfill!.Ran);
        Assert.Equal(12, v.Backfill.Scanned);
        Assert.Equal(11, v.Backfill.Materialized);
        // The failure count must survive separately from the summary
        // string. The panel colours on it, and a surface that reports
        // only the summary cannot tell a partial transfer from a whole
        // one — which is the class of quiet-success bug this whole area
        // was built out of.
        Assert.Equal(1, v.Backfill.Failed);
        Assert.Single(v.Backfill.Errors!);
    }

    // A skipped backfill and an empty one are DIFFERENT, and a surface
    // that renders them the same is back to "we did not look" reading as
    // "there was nothing to bring".
    [Fact]
    public void A_Skipped_Backfill_Is_Distinguishable_From_An_Empty_One()
    {
        var skipped = Parse<SharePanel.ResyncReply>("""
        {"ok":true,"backfill":{"scanned":0,"summary":"no files in the remote folder yet","ran":false}}
        """);
        var empty = Parse<SharePanel.ResyncReply>("""
        {"ok":true,"backfill":{"scanned":0,"summary":"no files in the remote folder yet","ran":true}}
        """);

        Assert.False(skipped.Backfill!.Ran);
        Assert.True(empty.Backfill!.Ran);
    }

    // The forget reply's caveat is the same class of field as `caveat`
    // on revoke: both halves of it look like a bug to an operator who
    // just asked for a clean slate, and left unsaid the next "clean"
    // test is silently dirty on one side.
    [Fact]
    public void The_Forget_Reply_Keeps_The_Caveat_And_The_Per_Peer_Problems()
    {
        const string json = """
        {
          "ok": true,
          "allMode": true,
          "caveat": "Files already received are untouched. The other peer still remembers YOU.",
          "peers": [
            {
              "peerId": "12D3KooWRemotePeerIdentifier",
              "peerAlias": "laptop",
              "syncsStopped": ["notes"],
              "offersWithdrawn": [],
              "policyRemoved": true,
              "disconnected": true,
              "problems": ["unsync notes: subscription close: context deadline exceeded"],
              "summary": "1 sync(s) stopped, authorization removed, disconnected, 1 PROBLEM(S)"
            }
          ]
        }
        """;

        var v = Parse<SharePanel.ForgetReply>(json);
        Assert.True(v.AllMode);
        Assert.Contains("still remembers YOU", v.Caveat);
        var one = Assert.Single(v.Peers!);
        Assert.True(one.PolicyRemoved);
        Assert.Equal(new[] { "notes" }, one.SyncsStopped);
        // A partly-forgotten peer must not read as a clean one.
        Assert.Single(one.Problems!);
    }

    [Fact]
    public void The_Share_Reply_Keeps_Whether_The_Grant_Is_Actually_In_Force()
    {
        const string json = """
        {
          "ok": true,
          "error": "",
          "root": "notes",
          "targetPrefix": "archives/notes/",
          "peerId": "12D3KooWRemotePeerIdentifier",
          "peerAlias": "laptop",
          "policyPath": "system/capability/policy/12D3KooWRemotePeerIdentifier",
          "grantSummary": "system/tree:get, system/subscription:*",
          "audience": ["12D3KooWRemotePeerIdentifier"],
          "reconnected": false,
          "reconnectNote": "no dialable address on record for this peer"
        }
        """;

        var v = Parse<SharePanel.ShareCreateReply>(json);
        Assert.True(v.Ok);
        Assert.Equal("notes", v.Root);
        Assert.Equal("system/tree:get, system/subscription:*", v.GrantSummary);
        Assert.Equal(new[] { "12D3KooWRemotePeerIdentifier" }, v.Audience!);
        // False here is the interesting case: the share is written and
        // NOT in force. A panel that cannot see this reports success for
        // an operation that will do nothing until an unrelated restart.
        Assert.False(v.Reconnected);
        Assert.Equal("no dialable address on record for this peer", v.ReconnectNote);
    }

    [Fact]
    public void The_Revoke_Reply_Keeps_The_Caveat_A_Surface_Must_Print()
    {
        const string json = """
        {
          "ok": true,
          "error": "",
          "root": "notes",
          "peerId": "12D3KooWRemotePeerIdentifier",
          "policyRemoved": true,
          "offerRemoved": false,
          "stillOffered": ["12D3KooWAnotherPeerIdentifier"],
          "caveat": "the grant is removed from the policy table, so the NEXT handshake will not carry it"
        }
        """;

        var v = Parse<SharePanel.ShareRevokeReply>(json);
        Assert.True(v.Ok);
        Assert.True(v.PolicyRemoved);
        Assert.False(v.OfferRemoved);
        // "withdrawn from A, still shared with B" — not "now private".
        Assert.Equal(new[] { "12D3KooWAnotherPeerIdentifier" }, v.StillOffered!);
        Assert.StartsWith("the grant is removed", v.Caveat);
    }

    [Fact]
    public void The_Unsync_Reply_Keeps_The_Note_That_Files_Are_Not_Deleted()
    {
        const string json = """
        {
          "ok": true,
          "error": "",
          "peerId": "12D3KooWRemotePeerIdentifier",
          "root": "notes",
          "found": true,
          "subscriptionCloseError": "",
          "note": "the local mount and every document already received are left in place"
        }
        """;

        var v = Parse<SharePanel.UnsyncReply>(json);
        Assert.True(v.Ok);
        Assert.True(v.Found);
        // Silence here reads as "the files are gone", which is the
        // opposite of true.
        Assert.StartsWith("the local mount and every document", v.Note);
    }

    [Fact]
    public void An_Offers_Refusal_Decodes_As_A_Reason_Not_As_An_Empty_List()
    {
        // The common case, and it is not a bug: an offer record is only
        // readable once they have shared with us, so a 403 here is the
        // permission stage working. An empty list would render as "they
        // are offering nothing", which is a different and wrong claim.
        const string json = """
        {
          "ok": false,
          "error": "read 12D3KooWRemote's offers: 403 permission denied",
          "peer": "12D3KooWRemotePeerIdentifier",
          "offers": []
        }
        """;

        var v = Parse<SharePanel.OffersReply>(json);
        Assert.False(v.Ok);
        Assert.Contains("403", v.Error);
        Assert.Empty(v.Offers!);
    }
}
