using System.Text.Json;
using EntityAvalonia.Panels;
using Xunit;

namespace Workbench.Headless.Tests;

// The envelope tests below deserialize a LITERAL bridge payload through
// the panel's own options and type. That is the only shape that catches
// the AP49 defect: `System.Text.Json` discards an undeclared member in
// total silence, so a field the model computes and the bridge sends can
// arrive as `false`/`null` with every other test still green. Asserting
// on a value the panel would have had to declare to receive is the point.
public class LocalFilesPanelTests
{
    private static readonly JsonSerializerOptions Opts = new() { PropertyNameCaseInsensitive = true };

    private static LocalFilesPanel.RenderEnvelope Parse(string json) =>
        JsonSerializer.Deserialize<LocalFilesPanel.RenderEnvelope>(json, Opts)!;

    [Fact]
    public void The_Render_Envelope_Does_Not_Drop_Any_Mount_Field()
    {
        // Every field avalonia/bridge/local_files.go marshals, with a
        // value distinguishable from its zero — a field dropped by the
        // deserializer reads as the zero, which is exactly why the
        // fixture must not use zeros.
        const string json = """
        {
          "ok": true,
          "note": "",
          "mounts": [{
            "root": "notes",
            "filesystemRoot": "/home/me/notes",
            "prefix": "local/files/notes/",
            "readOnly": true,
            "include": ["*.md"],
            "exclude": ["vendor", "*.log"],
            "publishDescriptors": true,
            "configPath": "system/config/local/files/notes",
            "fileCount": 42,
            "watcherObservable": false,
            "err": ""
          }]
        }
        """;

        var view = Parse(json);
        Assert.True(view.Ok);
        var m = Assert.Single(view.Mounts!);

        Assert.Equal("notes", m.Root);
        Assert.Equal("/home/me/notes", m.FilesystemRoot);
        Assert.Equal("local/files/notes/", m.Prefix);
        Assert.True(m.ReadOnly);
        Assert.Equal(new[] { "*.md" }, m.Include!);
        Assert.Equal(new[] { "vendor", "*.log" }, m.Exclude!);
        Assert.True(m.PublishDescriptors);
        Assert.Equal("system/config/local/files/notes", m.ConfigPath);
        Assert.Equal(42, m.FileCount);
        Assert.Equal("", m.Err);
    }

    // A read-only mount rendered as read-write is a lie about what the
    // peer will do to the operator's disk, and `false` is the value a
    // dropped bool arrives as. Pinned separately so the failure names
    // the consequence.
    [Fact]
    public void ReadOnly_Survives_The_Boundary()
    {
        var m = Assert.Single(Parse("""
        {"ok":true,"mounts":[{"root":"r","readOnly":true}]}
        """).Mounts!);
        Assert.True(m.ReadOnly);
    }

    // The panel must be able to distinguish "we know the watcher is
    // reported" from "we cannot see it". Today the bridge always sends
    // false; if that ever becomes true the panel already renders it, and
    // this asserts the field is carried rather than defaulted.
    [Fact]
    public void WatcherObservable_Is_Carried_Not_Defaulted()
    {
        Assert.True(Assert.Single(Parse("""
        {"ok":true,"mounts":[{"root":"r","watcherObservable":true}]}
        """).Mounts!).WatcherObservable);

        Assert.False(Assert.Single(Parse("""
        {"ok":true,"mounts":[{"root":"r","watcherObservable":false}]}
        """).Mounts!).WatcherObservable);
    }

    // An empty mount set must arrive as an empty list, never null. The
    // bridge sends `[]` deliberately: a null List<T> makes the panel's
    // first foreach a NullReferenceException, which per AP46 surfaces as
    // a process abort rather than a catchable exception.
    [Fact]
    public void An_Empty_Mount_Set_Is_A_List_Not_A_Null()
    {
        var view = Parse("""{"ok":true,"mounts":[],"note":"no filesystem mounts on this peer"}""");
        Assert.NotNull(view.Mounts);
        Assert.Empty(view.Mounts!);
        Assert.False(string.IsNullOrEmpty(view.Note));
    }

    // A mount whose config did not decode still arrives as a row, with
    // the reason attached. Dropping it would report a broken mount as no
    // mount at all.
    [Fact]
    public void A_Broken_Mount_Arrives_As_A_Row_With_Its_Reason()
    {
        var m = Assert.Single(Parse("""
        {"ok":true,"mounts":[{"root":"broken","err":"config did not decode: cbor: bad type"}]}
        """).Mounts!);
        Assert.Equal("broken", m.Root);
        Assert.Contains("did not decode", m.Err);
    }
}

// --- The mutating surface -------------------------------------------
//
// Same discipline as the render envelope above, aimed at the three
// replies the panel gained when it stopped being read-only: a field the
// bridge marshals and the panel does not declare arrives as its zero,
// silently. For these three that is not cosmetic — a dropped
// `watcherStillRunning` turns an honest warning into a clean-teardown
// claim, and a dropped `conflict` turns a refusal-with-a-reason into a
// bare failure with a "Mount anyway" button that never appears.
public class LocalFilesMutationEnvelopeTests
{
    private static readonly JsonSerializerOptions Opts = new() { PropertyNameCaseInsensitive = true };

    private static T Parse<T>(string json) => JsonSerializer.Deserialize<T>(json, Opts)!;

    [Fact]
    public void The_Mount_Result_Does_Not_Drop_Any_Field()
    {
        // Every value distinguishable from its zero — a dropped field
        // reads as the zero, which is why the fixture must not use them.
        const string json = """
        {
          "ok": true,
          "error": "",
          "rootName": "notes",
          "filesystemRoot": "/home/me/notes",
          "sourcePrefix": "local/files/notes/",
          "targetPrefix": "archives/notes/",
          "include": ["*.md"],
          "exclude": ["vendor"],
          "capabilityPath": "system/capability/grants/chain/local-files/notes",
          "handlerPattern": "workbench/ingest-from-notification",
          "subscriptionId": "sub-7",
          "conflict": null
        }
        """;

        var r = Parse<LocalFilesPanel.MountResult>(json);
        Assert.True(r.Ok);
        Assert.Equal("notes", r.RootName);
        Assert.Equal("/home/me/notes", r.FilesystemRoot);
        Assert.Equal("local/files/notes/", r.SourcePrefix);
        Assert.Equal("archives/notes/", r.TargetPrefix);
        Assert.Equal(new[] { "*.md" }, r.Include!);
        Assert.Equal(new[] { "vendor" }, r.Exclude!);
        Assert.Equal("system/capability/grants/chain/local-files/notes", r.CapabilityPath);
        Assert.Equal("workbench/ingest-from-notification", r.HandlerPattern);
        Assert.Equal("sub-7", r.SubscriptionId);
        Assert.Null(r.Conflict);
    }

    // A target-prefix conflict is STRUCTURE, not a sentence. The panel
    // lists the offending types and offers to proceed; if this DTO drops
    // the conflict the panel shows a bare failure and the operator has no
    // way to see what is in the way or to override it.
    [Fact]
    public void A_Mount_Conflict_Arrives_With_Its_Offending_Types()
    {
        const string json = """
        {
          "ok": false,
          "error": "mount aborted: 3 existing binding(s) ...",
          "conflict": {
            "targetPrefix": "archives/notes/",
            "sourcePrefix": "local/files/notes/",
            "expectedTypes": ["doc/markdown-file", "doc/code-file"],
            "targetTotal": 3,
            "targetExpected": 1,
            "sourceTotal": 2,
            "foreignOrder": ["app/note", "doc/other"],
            "foreign": { "app/note": 2, "doc/other": 1 }
          }
        }
        """;

        var r = Parse<LocalFilesPanel.MountResult>(json);
        Assert.False(r.Ok);
        var c = r.Conflict;
        Assert.NotNull(c);
        Assert.Equal("archives/notes/", c!.TargetPrefix);
        Assert.Equal("local/files/notes/", c.SourcePrefix);
        // Plural since the ingest registry — a mount owns every doc/*
        // type it can write, and the panel renders the SET.
        Assert.Equal(new[] { "doc/markdown-file", "doc/code-file" }, c.ExpectedTypes!);
        Assert.Equal(3, c.TargetTotal);
        Assert.Equal(1, c.TargetExpected);
        Assert.Equal(2, c.SourceTotal);
        // Order is the model's, not the map's — a dictionary does not
        // have one, which is why the bridge sends both.
        Assert.Equal(new[] { "app/note", "doc/other" }, c.ForeignOrder!);
        Assert.Equal(2, c.Foreign!["app/note"]);
        Assert.Equal(1, c.Foreign["doc/other"]);
    }

    // The load-bearing one. `ext/localfiles` exposes StartWatching and no
    // StopWatching, so an unmount leaves the fsnotify watcher running.
    // If this field is dropped it arrives as false and the panel reports
    // a clean teardown — telling the operator something untrue about
    // where their edits will go.
    [Fact]
    public void Unmount_Carries_The_Watcher_Still_Running_Warning()
    {
        var r = Parse<LocalFilesPanel.UnmountResult>("""
        {"ok":true,"rootName":"notes","subscriptionCloseError":"","watcherStillRunning":true}
        """);
        Assert.True(r.Ok);
        Assert.Equal("notes", r.RootName);
        Assert.True(r.WatcherStillRunning);
    }

    // A subscription that would not close is reported, not swallowed:
    // the unmount still succeeded in the sense that matters (the ingest
    // mapping is gone), so it is a note on a success rather than a
    // failure, and the panel says both.
    [Fact]
    public void Unmount_Reports_A_Subscription_Close_Failure_Without_Claiming_Total_Failure()
    {
        var r = Parse<LocalFilesPanel.UnmountResult>("""
        {"ok":true,"rootName":"notes","subscriptionCloseError":"connection reset","watcherStillRunning":true}
        """);
        Assert.True(r.Ok);
        Assert.Equal("connection reset", r.SubscriptionCloseError);
    }

    // A sweep DELETES bindings, so the removed paths travel in full. A
    // count alone would ask the operator to trust a destructive
    // operation they cannot inspect.
    [Fact]
    public void Sweep_Carries_The_Removed_Paths_Not_Just_Counts()
    {
        var r = Parse<LocalFilesPanel.SweepResult>("""
        {
          "ok": true,
          "rootName": "notes",
          "filesystemFiles": 9,
          "sourcePresent": 11,
          "sourceRemoved": ["gone-a.md", "gone-b.md"],
          "targetRemoved": ["archives/notes/gone-a.md"],
          "added": 2,
          "addErrors": ["skipped weird.md: permission denied"]
        }
        """);
        Assert.True(r.Ok);
        Assert.Equal(9, r.FilesystemFiles);
        Assert.Equal(11, r.SourcePresent);
        Assert.Equal(new[] { "gone-a.md", "gone-b.md" }, r.SourceRemoved!);
        Assert.Equal(new[] { "archives/notes/gone-a.md" }, r.TargetRemoved!);
        Assert.Equal(2, r.Added);
        Assert.Single(r.AddErrors!);
    }

    // Empty collections must be lists, never null. The bridge sends `[]`
    // deliberately: a null List<T> makes the panel's first foreach a
    // NullReferenceException, which per AP46 surfaces as a process abort
    // rather than a catchable exception.
    [Fact]
    public void Sweep_With_Nothing_Removed_Is_Empty_Lists_Not_Nulls()
    {
        var r = Parse<LocalFilesPanel.SweepResult>("""
        {"ok":true,"rootName":"n","filesystemFiles":0,"sourcePresent":0,
         "sourceRemoved":[],"targetRemoved":[],"added":0,"addErrors":[]}
        """);
        Assert.NotNull(r.SourceRemoved);
        Assert.Empty(r.SourceRemoved!);
        Assert.NotNull(r.TargetRemoved);
        Assert.NotNull(r.AddErrors);
    }
}
