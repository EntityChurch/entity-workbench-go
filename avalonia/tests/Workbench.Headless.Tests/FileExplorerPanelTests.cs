using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Text.Json;
using System.Threading;
using Avalonia.Headless.XUnit;
using EntityAvalonia;
using EntityAvalonia.Panels;
using EntityAvalonia.Tests;
using Xunit;

namespace Workbench.Headless.Tests;

// FileExplorerPanel — envelope contract plus an END-TO-END mount.
//
// The envelope tests deserialize a literal bridge payload through the
// panel's own options and type, which is the only shape that catches
// AP49: System.Text.Json discards an undeclared member in silence, so a
// field the model computes and the bridge sends arrives as false/null
// with every other test still green.
//
// The mount test is the one that matters. It creates a real directory of
// MIXED kinds, mounts it through the shipped bridge export, and asserts
// the panel lists those files with the right statuses — because the
// defect this panel exists to fix was never in any single layer. Every
// layer was green; there was no edge between them.
public class FileExplorerPanelEnvelopeTests
{
    private static readonly JsonSerializerOptions Opts = new() { PropertyNameCaseInsensitive = true };

    [Fact]
    public void The_Render_Envelope_Does_Not_Drop_Any_Entry_Field()
    {
        // Every field avalonia/bridge/file_explorer.go marshals, with a
        // value distinguishable from its zero — a dropped field reads as
        // the zero, which is why the fixture must not contain zeros.
        const string json = """
        {
          "ok": true,
          "error": "",
          "root": "notes",
          "filesystemRoot": "/home/me/notes",
          "sourcePrefix": "local/files/notes/",
          "targetPrefix": "archives/notes/",
          "dir": "src/deep",
          "crumbs": ["src", "deep"],
          "totalFiles": 5,
          "totalBytes": 4096,
          "ingested": 2,
          "notIngested": 3,
          "kindCounts": { "markdown": 1, "image": 3, "code": 1 },
          "note": "a note",
          "entries": [{
            "name": "util.go",
            "relPath": "src/deep/util.go",
            "isDir": false,
            "childFiles": 0,
            "childBytes": 0,
            "size": 1234,
            "modifiedAt": 1700000000,
            "kind": "code",
            "language": "go",
            "mediaType": "text/x-go",
            "sourcePath": "local/files/notes/src/deep/util.go",
            "targetPath": "archives/notes/src/deep/util.go",
            "entityType": "doc/code-file",
            "ingested": true,
            "status": "go"
          }]
        }
        """;

        var dto = JsonSerializer.Deserialize<FileExplorerPanel.RenderDto>(json, Opts)!;
        Assert.True(dto.Ok);
        Assert.Equal("notes", dto.Root);
        Assert.Equal("/home/me/notes", dto.FilesystemRoot);
        Assert.Equal("local/files/notes/", dto.SourcePrefix);
        Assert.Equal("archives/notes/", dto.TargetPrefix);
        Assert.Equal("src/deep", dto.Dir);
        Assert.Equal(new[] { "src", "deep" }, dto.Crumbs!);
        Assert.Equal(5, dto.TotalFiles);
        Assert.Equal(4096, dto.TotalBytes);
        Assert.Equal(2, dto.Ingested);

        // The load-bearing number. If notIngested is dropped it arrives
        // as 0, the summary says nothing, and the panel is back to the
        // single count that could not tell a healthy mount from one with
        // nothing openable in it.
        Assert.Equal(3, dto.NotIngested);
        Assert.Equal(3, dto.KindCounts!["image"]);

        var e = Assert.Single(dto.Entries!);
        Assert.Equal("util.go", e.Name);
        Assert.Equal("src/deep/util.go", e.RelPath);
        Assert.False(e.IsDir);
        Assert.Equal(1234, e.Size);
        Assert.Equal(1700000000, e.ModifiedAt);
        Assert.Equal("code", e.Kind);
        Assert.Equal("go", e.Language);
        Assert.Equal("text/x-go", e.MediaType);
        Assert.Equal("local/files/notes/src/deep/util.go", e.SourcePath);
        Assert.Equal("archives/notes/src/deep/util.go", e.TargetPath);
        Assert.Equal("doc/code-file", e.EntityType);
        Assert.True(e.Ingested);
        Assert.Equal("go", e.Status);
    }

    // A row with no document must arrive carrying its REASON. Dropping
    // `status` gives an empty string, which the panel would render as a
    // blank column — indistinguishable from a healthy file, which is the
    // exact failure mode the panel exists to end.
    [Fact]
    public void A_Row_With_No_Document_Arrives_With_Its_Reason()
    {
        const string json = """
        {
          "ok": true, "root": "notes", "entries": [{
            "name": "photo.png", "relPath": "photo.png", "isDir": false,
            "size": 9001, "kind": "image", "ingested": false,
            "sourcePath": "local/files/notes/photo.png",
            "targetPath": "", "entityType": "",
            "status": "no document yet — ingest pending or failed"
          }]
        }
        """;
        var dto = JsonSerializer.Deserialize<FileExplorerPanel.RenderDto>(json, Opts)!;
        var e = Assert.Single(dto.Entries!);
        Assert.False(e.Ingested);
        Assert.Equal("", e.TargetPath);
        Assert.Contains("no document", e.Status);
    }

    [Fact]
    public void The_Preview_Envelope_Does_Not_Drop_Its_Fields()
    {
        const string json = """
        {
          "ok": true, "error": "", "relPath": "a.go", "kind": "code",
          "language": "go", "text": "package main", "textual": true,
          "truncated": true, "size": 999999
        }
        """;
        var dto = JsonSerializer.Deserialize<FileExplorerPanel.PreviewDto>(json, Opts)!;
        Assert.Equal("a.go", dto.RelPath);
        Assert.Equal("code", dto.Kind);
        Assert.Equal("go", dto.Language);
        Assert.Equal("package main", dto.Text);
        Assert.True(dto.Textual);
        // Truncation must survive the wire: a preview silently cut at the
        // cap and presented as the whole file is a wrong answer about
        // what is on disk.
        Assert.True(dto.Truncated);
        Assert.Equal(999999, dto.Size);
    }
}

[Collection(nameof(BridgeCollection))]
public class FileExplorerPanelMountTests
{
    private readonly BridgeFixture _bridge;

    public FileExplorerPanelMountTests(BridgeFixture bridge) => _bridge = bridge;

    // The end-to-end one: a real directory of mixed kinds, mounted
    // through the shipped export, browsed through the shipped panel.
    //
    // Tier: real-session (TESTING-STRATEGY §4). It is slow and it is the
    // only test in the tree that crosses mount → watcher → subscription →
    // ingest → model → bridge → panel, which is precisely where the
    // defect lived: every layer green, no edge between them.
    //
    // [AvaloniaFact], not [Fact]: the panel constructs real controls, and
    // a control built outside the headless app session throws in its
    // constructor. The envelope tests above are plain [Fact] because they
    // only deserialize — they never touch a control.
    [AvaloniaFact]
    public void A_Mounted_Directory_Of_Mixed_Kinds_Becomes_Browsable_Files()
    {
        var dir = Path.Combine(Path.GetTempPath(), "wb-explorer-" + Guid.NewGuid().ToString("N")[..8]);
        Directory.CreateDirectory(Path.Combine(dir, "src"));
        try
        {
            File.WriteAllText(Path.Combine(dir, "readme.md"), "# Readme\n\nhello\n");
            File.WriteAllText(Path.Combine(dir, "notes.txt"), "plain text here\n");
            File.WriteAllText(Path.Combine(dir, "src", "main.go"), "package main\n\nfunc main() {}\n");
            File.WriteAllBytes(Path.Combine(dir, "blob.bin"), new byte[] { 0, 1, 2, 3, 4, 5 });

            var mountReply = Bridge.TakeString(Bridge.LocalFilesMount(
                _bridge.DefaultPeer, dir, "archives/explorer-test/", "", "",
                excludeSet: 1, force: 0, readOnly: 0));
            using var mountDoc = JsonDocument.Parse(mountReply);
            Assert.True(mountDoc.RootElement.GetProperty("ok").GetBoolean(),
                $"mount failed: {mountReply}");
            var rootName = mountDoc.RootElement.GetProperty("rootName").GetString()!;

            using var panel = new FileExplorerPanel(_bridge.DefaultPeer, null);
            Assert.Contains(rootName, panel.MountRoots);

            // The watcher's initial scan and the ingest chain are
            // asynchronous, so poll with a deadline rather than assert
            // once. A fixed sleep would either be flaky or slow, and
            // AP32's rule applies: wait on the thing that actually
            // changes.
            var rows = AwaitRows(panel, r => r.Count >= 4, "four entries at the mount root");

            var byName = new Dictionary<string, FileExplorerPanel.EntryVm>();
            foreach (var r in rows) byName[r.Name] = r;

            // A folder is inferred from the paths beneath it.
            Assert.True(byName.ContainsKey("src"), $"no src folder in [{string.Join(",", byName.Keys)}]");
            Assert.True(byName["src"].IsDir);

            // Every kind classifies, including the one that used to be
            // dropped on the floor entirely.
            Assert.Equal("markdown", byName["readme.md"].Kind);
            Assert.Equal("text", byName["notes.txt"].Kind);
            Assert.Equal("binary", byName["blob.bin"].Kind);

            // Sizes come from the source FileData, so a real file has a
            // real size — a zero here means the join never happened.
            Assert.True(byName["readme.md"].Size > 0, "readme.md has no size");

            // Descend, and the code file is there and classified.
            panel.Navigate("src");
            var inSrc = AwaitRows(panel, r => r.Count >= 1, "one file under src");
            Assert.Equal("main.go", inSrc[0].Name);
            Assert.Equal("code", inSrc[0].Kind);
            Assert.Equal("go", inSrc[0].Language);

            // **The assertion this test exists for.** Everything above is
            // derivable from the filename and the source layer — the
            // layer that never fails — so all of it would still pass with
            // the ingest chain completely dead. Before the type registry
            // it WAS effectively dead for three of these four files:
            // .txt, .go and .bin returned `type_not_handled` and no
            // document was ever written.
            //
            // A test that stops at "the files are listed" is the same
            // mistake the old mount row made, at a different layer.
            panel.Navigate("");
            var settled = AwaitRows(panel,
                r => CountIngested(r) >= 3,
                "three ingested documents at the mount root");

            foreach (var name in new[] { "readme.md", "notes.txt", "blob.bin" })
            {
                var row = settled.Find(r => r.Name == name);
                Assert.True(row is { Ingested: true },
                    $"{name} has no document — status: {row?.Status ?? "(row missing)"}");
                Assert.NotEqual("", row!.TargetPath);
                Assert.StartsWith("doc/", row.EntityType);
            }

            // Each kind lands as its OWN entity type, which is what makes
            // a viewer able to dispatch on it.
            Assert.Equal("doc/markdown-file", settled.Find(r => r.Name == "readme.md")!.EntityType);
            Assert.Equal("doc/text-file", settled.Find(r => r.Name == "notes.txt")!.EntityType);
            Assert.Equal("doc/binary-file", settled.Find(r => r.Name == "blob.bin")!.EntityType);

            // And a textual file previews through the source layer.
            var preview = Bridge.TakeString(Bridge.FileExplorerPreview(
                panel.ExplorerHandleForTests, "notes.txt"));
            Assert.Contains("plain text here", preview);
        }
        finally
        {
            try { Directory.Delete(dir, recursive: true); } catch { }
        }
    }

    // AwaitRows polls Refresh until a predicate holds, then returns the
    // rows. Bounded, so a broken chain fails with a message rather than
    // hanging — and the predicate is a parameter because "the rows
    // arrived" and "the ingest finished" are different waits and only
    // the second one measures this session's work.
    private static List<FileExplorerPanel.EntryVm> AwaitRows(
        FileExplorerPanel panel,
        Func<List<FileExplorerPanel.EntryVm>, bool> settled,
        string what)
    {
        var sw = Stopwatch.StartNew();
        List<FileExplorerPanel.EntryVm> rows = new();
        while (sw.Elapsed < TimeSpan.FromSeconds(20))
        {
            panel.Refresh();
            rows = panel.VisibleRowsForTests;
            if (settled(rows)) return rows;
            Thread.Sleep(50);
        }
        var seen = string.Join(", ", rows.ConvertAll(r =>
            $"{r.Name}({r.Kind},{(r.Ingested ? "doc" : "no-doc")})"));
        Assert.Fail($"waited 20s for {what}; saw [{seen}] — summary: {panel.SummaryText}");
        return rows;
    }

    private static int CountIngested(List<FileExplorerPanel.EntryVm> rows)
    {
        var n = 0;
        foreach (var r in rows) if (!r.IsDir && r.Ingested) n++;
        return n;
    }
}
