using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Layout;
using Avalonia.Media;
using Avalonia.Threading;

namespace EntityAvalonia.Panels;

// FeedPanel — who this peer follows, what they posted, and what one
// reference resolves to.
//
// # Why it exists
//
// `follow` / `unfollow` / `follows` / `timeline` and `ref` shipped as
// shell verbs with no pixel behind them. That is D23 — a model with no
// shipped surface is not shipped — and for `ref` it is sharper than the
// usual case: `FEED-R7` is a **[MUST]** that a reader be able to tell
// §2.2.2's four outcomes apart, so a resolver whose outcome no renderer
// shows satisfies the MUST nowhere.
//
// The panel is a thin envelope over `avalonia/bridge/feed.go`, which is a
// thin envelope over `shellcmd/follow_op.go`. **No stage is reimplemented
// here** (AP57): the facts about what a follow does and does not
// establish would then live in two places, and they are the ones nobody
// rediscovers by reading code.
//
// # Three actions, three different things they cost
//
// **Follow** writes one local declaration and contacts nobody —
// `APP-CONVENTION-FEED` §2.4 makes a feed-follow a follow of a NAMESPACE:
// public, pull-only, no grant, no permission, *and the publisher does not
// know the follower exists*. The panel says so on the button's own row,
// because "follow" means the opposite on every other system a person has
// used, and an operator is entitled to know which one this is.
//
// **Read** dials every followed publisher. It is a BUTTON and is never on
// a wake or a timer — an open panel that contacted everyone you follow on
// every window focus is a thing somebody leaves running overnight.
//
// **Catch up** dials *and* moves durable read positions. Separate control,
// separate export, and the result line says whether a position actually
// MOVED rather than that the button was pressed.
//
// Only the follows LIST is wake-driven, and that is the right split: it
// is tree data under `app/workbench/feed/`, and a refresh button on tree
// data is a bug report about a missing subscription (AP73).
public sealed class FeedPanel : UserControl, IDisposable, IPanelPreferredHeight
{
    // Chrome floor: the follow form, the action row, the reference form,
    // and room for a few entries. Declared because the 200px stack default
    // clips any panel that forgets (AP64) — and a clipped panel with an
    // unreachable button is what an operator meets, not a test.
    public double PreferredSlotMinHeight => 460;

    // P4 (bounded list). A timeline is unbounded — a followed publisher
    // can have posted any number of entries — and an unbounded list in a
    // docked region is AP64's other half. Bounded, and it SAYS it is
    // bounded: a truncated list that does not announce itself reads as a
    // complete one, which on this panel would be a false claim about what
    // somebody published.
    private const int MaxEntriesShown = 300;

    // What one Read asks each publisher for. Bounded for the same reason
    // and passed to the model rather than applied to the result, so the
    // cost is not paid and then discarded.
    private const int ReadLimit = 200;

    private readonly long _peerHandle;

    private readonly TextBox _subjectBox;
    private readonly TextBox _labelBox;
    private readonly Button _followButton;
    private readonly Button _readButton;
    private readonly Button _catchUpButton;
    private readonly TextBlock _statusLine;

    private readonly ItemsControl _followList;
    private readonly ObservableCollection<FollowRow> _follows = new();

    private readonly ItemsControl _sourceList;
    private readonly ObservableCollection<SourceRow> _sources = new();

    private readonly ItemsControl _entryList;
    private readonly ObservableCollection<EntryRow> _entries = new();

    private readonly TextBox _refBox;
    private readonly Button _refButton;
    private readonly StackPanel _refResult;

    private Bridge.TreeWakeCallback? _wakeCallback;
    private GCHandle _wakeCallbackHandle;
    private long _wakeRegistration = -1;
    private bool _disposed;

    private static readonly JsonSerializerOptions Json = new()
    {
        PropertyNameCaseInsensitive = true,
        NumberHandling = JsonNumberHandling.AllowReadingFromString,
    };

    public FeedPanel(long peerHandle, IPanelHost? host = null)
    {
        _peerHandle = peerHandle;

        // Every control is constructed before anything can fail. An early
        // return leaving half the fields null turns a later refresh into a
        // NullReference on the UI thread, which in this runtime is a
        // process death rather than an exception.
        _subjectBox = new TextBox
        {
            Watermark = "peer-id to follow",
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
        };
        _labelBox = new TextBox
        {
            Watermark = "your name for them (optional, local, never sent)",
            FontSize = 12,
        };

        _followButton = new Button { Content = "Follow", FontSize = 13, Padding = new Thickness(14, 4) };
        _followButton.Click += (_, _) => DoFollow();

        _readButton = new Button { Content = "Read", FontSize = 13, Padding = new Thickness(14, 4) };
        _readButton.Click += (_, _) => DoTimeline(advance: false);

        _catchUpButton = new Button
        {
            Content = "Catch up",
            FontSize = 13,
            Padding = new Thickness(14, 4),
        };
        _catchUpButton.Click += (_, _) => DoTimeline(advance: true);

        _statusLine = new TextBlock
        {
            Text = "",
            FontSize = 11,
            Opacity = 0.8,
            TextWrapping = TextWrapping.Wrap,
            Margin = new Thickness(0, 4, 0, 0),
        };

        _followList = new ItemsControl
        {
            ItemsSource = _follows,
            ItemTemplate = Rows.Of<FollowRow>((row, _) => BuildFollowView(row), supportsRecycling: false),
        };
        _sourceList = new ItemsControl
        {
            ItemsSource = _sources,
            ItemTemplate = Rows.Of<SourceRow>((row, _) => BuildSourceView(row), supportsRecycling: false),
        };
        _entryList = new ItemsControl
        {
            ItemsSource = _entries,
            ItemTemplate = Rows.Of<EntryRow>((row, _) => BuildEntryView(row), supportsRecycling: false),
        };

        _refBox = new TextBox
        {
            Watermark = "entity+ref://<peer>/<path>  — resolve one reference",
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
        };
        _refButton = new Button { Content = "Resolve", FontSize = 12, Padding = new Thickness(12, 3) };
        _refButton.Click += (_, _) => DoResolve();
        _refResult = new StackPanel { Spacing = 2, Margin = new Thickness(0, 4, 0, 0) };

        var controls = new StackPanel { Spacing = 4, Margin = new Thickness(0, 0, 0, 6) };
        controls.Children.Add(new TextBlock
        {
            Text = "Feeds — who you follow, and what they posted",
            FontWeight = FontWeight.SemiBold,
            FontSize = 14,
            Opacity = 0.85,
        });
        controls.Children.Add(_subjectBox);
        controls.Children.Add(_labelBox);

        var actions = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 10 };
        actions.Children.Add(_followButton);
        actions.Children.Add(_readButton);
        actions.Children.Add(_catchUpButton);
        controls.Children.Add(actions);

        // §2.4's sentence, on screen and not in a tooltip. "Follow" means
        // the opposite on every other system a person has used, so a
        // caption that only appears on hover is a caption nobody reads.
        controls.Children.Add(new TextBlock
        {
            Text = "Following is pull-only: nothing is asked of them and they are never told. "
                 + "Read fetches; Catch up fetches and moves your saved position.",
            FontSize = 11,
            Opacity = 0.62,
            TextWrapping = TextWrapping.Wrap,
        });
        controls.Children.Add(_statusLine);

        var body = new StackPanel { Spacing = 6 };
        body.Children.Add(SectionLabel("following"));
        body.Children.Add(_followList);
        // FEED-R23: a view of more than one publisher MUST declare what
        // produced it. Rendered above the entries, always, including when
        // every source is fine — a provenance block that only appears on
        // failure teaches an operator that its absence means one source.
        body.Children.Add(SectionLabel("sources this view was assembled from"));
        body.Children.Add(_sourceList);
        body.Children.Add(SectionLabel("entries"));
        body.Children.Add(_entryList);
        body.Children.Add(SectionLabel("resolve a reference"));
        body.Children.Add(_refBox);
        body.Children.Add(_refButton);
        body.Children.Add(_refResult);

        var dock = new DockPanel { LastChildFill = true, Margin = new Thickness(8) };
        DockPanel.SetDock(controls, Dock.Top);
        dock.Children.Add(controls);
        dock.Children.Add(new ScrollViewer { Content = body });
        Content = dock;

        // Only the follows list is wake-driven. See the class note.
        _wakeCallback = OnWakeFromGo;
        _wakeCallbackHandle = GCHandle.Alloc(_wakeCallback);
        var cbPtr = Marshal.GetFunctionPointerForDelegate(_wakeCallback);
        var reply = Bridge.TakeString(Bridge.SharingRegisterWake(_peerHandle, cbPtr));
        _wakeRegistration = ParseRegistration(reply);

        RefreshFollows();
        PanelLog.Write("feed", $"Mount peer={_peerHandle}");
    }

    private static TextBlock SectionLabel(string text) => new()
    {
        Text = text,
        FontSize = 11,
        Opacity = 0.55,
        Margin = new Thickness(0, 8, 0, 2),
    };

    // --- actions ---------------------------------------------------------

    private void DoFollow()
    {
        var subject = (_subjectBox.Text ?? "").Trim();
        if (subject.Length == 0)
        {
            _statusLine.Text = "enter a peer-id to follow";
            return;
        }
        var label = (_labelBox.Text ?? "").Trim();
        // Synchronous and deliberately so: a follow writes one local entity
        // and dials nobody, so there is nothing to wait on and nothing to
        // move off the UI thread.
        var reply = Bridge.TakeString(Bridge.FeedFollowPeer(_peerHandle, subject, label));
        ApplyFollows(reply, $"following {Short(subject)} — they were not told");
        _subjectBox.Text = "";
        _labelBox.Text = "";
    }

    private void DoUnfollow(string subject)
    {
        var reply = Bridge.TakeString(Bridge.FeedUnfollowPeer(_peerHandle, subject));
        ApplyFollows(reply, $"stopped following {Short(subject)} — your read position went with it");
    }

    private void RefreshFollows()
    {
        var reply = Bridge.TakeString(Bridge.FeedFollowsRender(_peerHandle));
        ApplyFollows(reply, null);
    }

    // DoTimeline dials. Off the UI thread, and the buttons are disabled
    // for the duration — a second press while one read is in flight would
    // dial every publisher twice.
    private async void DoTimeline(bool advance)
    {
        var subject = (_subjectBox.Text ?? "").Trim();
        SetBusy(true);
        _statusLine.Text = advance ? "catching up…" : "reading…";
        try
        {
            var reply = await Task.Run(() => Bridge.TakeString(
                advance
                    ? Bridge.FeedTimelineCatchUp(_peerHandle, subject, ReadLimit)
                    : Bridge.FeedTimelineRead(_peerHandle, subject, ReadLimit)));
            ApplyTimeline(reply, advance);
        }
        catch (Exception ex)
        {
            _statusLine.Text = "read failed: " + ex.Message;
        }
        finally
        {
            SetBusy(false);
        }
    }

    private async void DoResolve()
    {
        var reference = (_refBox.Text ?? "").Trim();
        if (reference.Length == 0)
        {
            return;
        }
        _refButton.IsEnabled = false;
        try
        {
            var reply = await Task.Run(() =>
                Bridge.TakeString(Bridge.FeedResolveRef(_peerHandle, reference)));
            ApplyRef(reply);
        }
        catch (Exception ex)
        {
            ShowRefError(ex.Message);
        }
        finally
        {
            _refButton.IsEnabled = true;
        }
    }

    private void SetBusy(bool busy)
    {
        _readButton.IsEnabled = !busy;
        _catchUpButton.IsEnabled = !busy;
        _followButton.IsEnabled = !busy;
    }

    // --- applying replies ------------------------------------------------

    private void ApplyFollows(string reply, string? note)
    {
        FollowsDto? dto;
        try
        {
            dto = JsonSerializer.Deserialize<FollowsDto>(reply, Json);
        }
        catch (Exception ex)
        {
            _statusLine.Text = "follows: " + ex.Message;
            return;
        }
        if (dto is null)
        {
            _statusLine.Text = "follows: empty reply";
            return;
        }
        if (!string.IsNullOrEmpty(dto.Error))
        {
            _statusLine.Text = dto.Error;
            return;
        }

        _follows.Clear();
        foreach (var r in dto.Rows ?? new List<FollowRowDto>())
        {
            _follows.Add(new FollowRow(r.Subject ?? "", r.Label ?? "", r.Reachable, r.Why ?? "", DoUnfollow));
        }

        // Problems are the peer-level facts — §2.4's privacy sentence most
        // of all. Kept beside the rows and not merged into them: a
        // sentence about where this peer publishes its follow records
        // belongs to no row.
        var lines = new List<string>();
        if (note is not null)
        {
            lines.Add(note);
        }
        foreach (var p in dto.Problems ?? new List<string>())
        {
            lines.Add("⚠ " + p);
        }
        _statusLine.Text = string.Join("\n", lines);
    }

    private void ApplyTimeline(string reply, bool advance)
    {
        TimelineDto? dto;
        try
        {
            dto = JsonSerializer.Deserialize<TimelineDto>(reply, Json);
        }
        catch (Exception ex)
        {
            _statusLine.Text = "timeline: " + ex.Message;
            return;
        }
        if (dto is null)
        {
            _statusLine.Text = "timeline: empty reply";
            return;
        }
        if (!string.IsNullOrEmpty(dto.Error))
        {
            _statusLine.Text = dto.Error;
            return;
        }

        _sources.Clear();
        foreach (var s in dto.Sources ?? new List<TimelineSourceDto>())
        {
            _sources.Add(new SourceRow(s));
        }

        _entries.Clear();
        var all = dto.Entries ?? new List<TimelineEntryDto>();
        var shown = Math.Min(all.Count, MaxEntriesShown);
        for (var i = 0; i < shown; i++)
        {
            _entries.Add(new EntryRow(all[i]));
        }

        var status = $"{all.Count} entries from {_sources.Count} source(s)";
        if (all.Count > shown)
        {
            status += $" — showing the first {shown}";
        }
        if (advance)
        {
            // "advanced" means a position MOVED. A catch-up that found
            // nothing new and reported "positions were advanced" is a
            // surface describing its own mode rather than what happened —
            // and this one is about durable state.
            status += dto.Advanced
                ? " — read positions moved"
                : " — nothing new, so no position moved";
        }
        _statusLine.Text = status;
    }

    private void ApplyRef(string reply)
    {
        RefDto? dto;
        try
        {
            dto = JsonSerializer.Deserialize<RefDto>(reply, Json);
        }
        catch (Exception ex)
        {
            ShowRefError(ex.Message);
            return;
        }
        if (dto is null)
        {
            ShowRefError("empty reply");
            return;
        }

        _refResult.Children.Clear();
        if (!string.IsNullOrEmpty(dto.Error))
        {
            // A fault about the PUBLISHER — unreachable, published
            // nothing, root does not verify, bytes withheld. Kept visually
            // distinct from every outcome that is about the REFERENCE,
            // because collapsing the two is exactly what the typed outcome
            // exists to prevent.
            _refResult.Children.Add(new SelectableTextBlock
            {
                Text = "could not ask the publisher: " + dto.Error,
                Foreground = Brushes.IndianRed,
                FontSize = 12,
                TextWrapping = TextWrapping.Wrap,
            });
            return;
        }

        // FEED-R7 is about the ABILITY TO TELL, so the row name is
        // rendered verbatim and not translated into a tick or a cross.
        // Three of the six outcomes have bytes and two of those are
        // perfectly healthy; a colour alone cannot carry that.
        _refResult.Children.Add(new SelectableTextBlock
        {
            Text = dto.Row ?? "(no outcome)",
            FontWeight = FontWeight.SemiBold,
            FontSize = 13,
            Foreground = RowBrush(dto.Row),
        });
        if (dto.Moved)
        {
            // The MUST's own field. Separate from the row because a
            // caller switching on an enum can forget a case and a
            // provenance line reads one boolean.
            _refResult.Children.Add(new SelectableTextBlock
            {
                Text = "the path resolved to something other than what the link recorded",
                FontSize = 12,
                Foreground = Brushes.Goldenrod,
                TextWrapping = TextWrapping.Wrap,
            });
        }
        AddRefLine(dto.Note, 12, 0.95);
        AddRefLine(dto.Provenance, 11, 0.7);
        if (!string.IsNullOrEmpty(dto.EntityType))
        {
            AddRefLine("type " + dto.EntityType, 11, 0.7);
        }
        if (!string.IsNullOrEmpty(dto.Prefix))
        {
            AddRefLine("their root commits to " + dto.Prefix, 11, 0.7);
        }
    }

    private void AddRefLine(string? text, double size, double opacity)
    {
        if (string.IsNullOrEmpty(text))
        {
            return;
        }
        _refResult.Children.Add(new SelectableTextBlock
        {
            Text = text,
            FontSize = size,
            Opacity = opacity,
            TextWrapping = TextWrapping.Wrap,
        });
    }

    private void ShowRefError(string message)
    {
        _refResult.Children.Clear();
        _refResult.Children.Add(new SelectableTextBlock
        {
            Text = message,
            Foreground = Brushes.IndianRed,
            FontSize = 12,
            TextWrapping = TextWrapping.Wrap,
        });
    }

    // RowBrush colours the outcome without replacing it. `moved` is not a
    // fault — a document that evolved since somebody linked to it is the
    // ordinary case — so it is Goldenrod rather than red, and `current`
    // and `pinned` are the two that are simply fine.
    private static IBrush RowBrush(string? row) => row switch
    {
        "pinned" or "current" => Brushes.DarkSeaGreen,
        "moved" or "fell-back-to-seen" => Brushes.Goldenrod,
        _ => Brushes.IndianRed,
    };

    // --- row views -------------------------------------------------------

    private static Control BuildFollowView(FollowRow row)
    {
        var line = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 8, Margin = new Thickness(0, 2, 0, 2) };
        line.Children.Add(new SelectableTextBlock
        {
            Text = row.Display,
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
            Width = 340,
            TextWrapping = TextWrapping.NoWrap,
        });
        // Reachability is an OBSERVATION and never a health verdict on the
        // follow: §2.4 requires nothing of the far end, so a follow of a
        // peer who is asleep is a perfectly good follow. Rendered as a
        // sentence about reading, in Goldenrod, never in red.
        line.Children.Add(new TextBlock
        {
            Text = row.Reachable ? "readable" : "not readable yet",
            FontSize = 11,
            Opacity = 0.8,
            Foreground = row.Reachable ? Brushes.DarkSeaGreen : Brushes.Goldenrod,
            VerticalAlignment = VerticalAlignment.Center,
        });
        var drop = new Button { Content = "Unfollow", FontSize = 11, Padding = new Thickness(8, 1) };
        drop.Click += (_, _) => row.Unfollow(row.Subject);
        line.Children.Add(drop);

        if (!row.Reachable && row.Why.Length > 0)
        {
            var stack = new StackPanel();
            stack.Children.Add(line);
            stack.Children.Add(new TextBlock
            {
                Text = row.Why,
                FontSize = 11,
                Opacity = 0.6,
                TextWrapping = TextWrapping.Wrap,
                Margin = new Thickness(4, 0, 0, 4),
            });
            return stack;
        }
        return line;
    }

    private static Control BuildSourceView(SourceRow row)
    {
        var stack = new StackPanel { Margin = new Thickness(0, 2, 0, 2) };
        var head = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 8 };
        head.Children.Add(new SelectableTextBlock
        {
            Text = row.Display,
            FontFamily = new FontFamily("monospace"),
            FontSize = 12,
            Width = 340,
        });
        head.Children.Add(new TextBlock
        {
            Text = row.Summary,
            FontSize = 11,
            Opacity = 0.8,
            Foreground = row.HasError ? Brushes.IndianRed : Brushes.Gray,
            VerticalAlignment = VerticalAlignment.Center,
            TextWrapping = TextWrapping.Wrap,
        });
        stack.Children.Add(head);
        foreach (var n in row.Notes)
        {
            stack.Children.Add(new TextBlock
            {
                Text = n,
                FontSize = 11,
                Opacity = 0.6,
                TextWrapping = TextWrapping.Wrap,
                Margin = new Thickness(4, 0, 0, 0),
            });
        }
        return stack;
    }

    private static Control BuildEntryView(EntryRow row)
    {
        var stack = new StackPanel { Margin = new Thickness(0, 3, 0, 5) };
        var head = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 8 };
        head.Children.Add(new TextBlock
        {
            Text = row.Who,
            FontSize = 11,
            Opacity = 0.75,
            FontWeight = FontWeight.SemiBold,
        });
        // FEED-R4. "unattributed" alone reads as a defect in the reader,
        // so the reason travels with it.
        head.Children.Add(new TextBlock
        {
            Text = row.AttributionMark,
            FontSize = 11,
            Foreground = row.Attributed ? Brushes.DarkSeaGreen : Brushes.Goldenrod,
        });
        if (!row.Listed)
        {
            // AP106: a conformance fallback will happily hide a broken
            // index path, and the count alone cannot tell the two apart.
            // Which road found this entry is a fact about the READ, so it
            // belongs on the row.
            head.Children.Add(new TextBlock
            {
                Text = "found by enumeration",
                FontSize = 11,
                Opacity = 0.6,
            });
        }
        stack.Children.Add(head);

        // FEED-R1: a rejected entry is KEPT and its body is NOT rendered.
        // Dropping the row would make this list disagree with the index it
        // came from, and the rejected row is the interesting one.
        if (row.Rejected)
        {
            stack.Children.Add(new SelectableTextBlock
            {
                Text = "this entry names an author other than the peer it was published under — not rendered",
                FontSize = 12,
                Foreground = Brushes.IndianRed,
                TextWrapping = TextWrapping.Wrap,
            });
        }
        else
        {
            stack.Children.Add(new SelectableTextBlock
            {
                Text = row.Text,
                FontSize = 13,
                TextWrapping = TextWrapping.Wrap,
            });
        }
        if (row.Problem.Length > 0)
        {
            stack.Children.Add(new SelectableTextBlock
            {
                Text = row.Problem,
                FontSize = 11,
                Opacity = 0.7,
                Foreground = Brushes.Goldenrod,
                TextWrapping = TextWrapping.Wrap,
            });
        }
        return stack;
    }

    // --- wake ------------------------------------------------------------

    private void OnWakeFromGo(long _)
    {
        // Marshalled onto the UI thread: the callback arrives on a Go
        // goroutine, and touching an ObservableCollection from it is a
        // cross-thread mutation of the visual tree.
        Dispatcher.UIThread.Post(() =>
        {
            if (_disposed)
            {
                return;
            }
            RefreshFollows();
        });
    }

    private static long ParseRegistration(string reply)
    {
        try
        {
            using var doc = JsonDocument.Parse(reply);
            if (doc.RootElement.TryGetProperty("registration", out var r) && r.TryGetInt64(out var v))
            {
                return v;
            }
        }
        catch
        {
            // A registration we cannot parse means no wake, which is a
            // stale list and not a crash. Swallowed here and visible as
            // the list not updating.
        }
        return -1;
    }

    private static string Short(string peerId) =>
        peerId.Length > 12 ? peerId[..12] + "…" : peerId;

    public void Dispose()
    {
        if (_disposed)
        {
            return;
        }
        _disposed = true;
        if (_wakeRegistration >= 0)
        {
            Bridge.TakeString(Bridge.SharingUnregisterWake(_peerHandle, _wakeRegistration));
            _wakeRegistration = -1;
        }
        if (_wakeCallbackHandle.IsAllocated)
        {
            _wakeCallbackHandle.Free();
        }
        _wakeCallback = null;
        PanelLog.Write("feed", "Dispose");
    }

    // --- view models -----------------------------------------------------

    private sealed class FollowRow
    {
        public FollowRow(string subject, string label, bool reachable, string why, Action<string> unfollow)
        {
            Subject = subject;
            Label = label;
            Reachable = reachable;
            Why = why;
            Unfollow = unfollow;
        }

        public string Subject { get; }
        public string Label { get; }
        public bool Reachable { get; }
        public string Why { get; }
        public Action<string> Unfollow { get; }

        public string Display => Label.Length > 0 ? $"{Label}  ({Short(Subject)})" : Subject;
    }

    private sealed class SourceRow
    {
        public SourceRow(TimelineSourceDto dto)
        {
            Subject = dto.Subject ?? "";
            Label = dto.Label ?? "";
            HasError = !string.IsNullOrEmpty(dto.Error);
            Notes = dto.Notes ?? new List<string>();

            if (HasError)
            {
                // An unreachable publisher is an EXCLUSION from this view
                // and is named as one. Dropping the row would make the
                // view claim a completeness it does not have — FEED-R23.
                Summary = "excluded: " + dto.Error;
            }
            else if (!dto.Published)
            {
                Summary = "this peer has published nothing";
            }
            else
            {
                var via = string.IsNullOrEmpty(dto.Via) ? "" : $" via {dto.Via}";
                var fresh = string.IsNullOrEmpty(dto.Freshness) ? "" : $" — {dto.Freshness}";
                Summary = $"{dto.Count} entries{via}{fresh}";
            }
        }

        public string Subject { get; }
        public string Label { get; }
        public string Summary { get; }
        public bool HasError { get; }
        public List<string> Notes { get; }

        public string Display => Label.Length > 0 ? $"{Label}  ({Short(Subject)})" : Subject;
    }

    private sealed class EntryRow
    {
        public EntryRow(TimelineEntryDto dto)
        {
            var label = dto.Label ?? "";
            Who = label.Length > 0 ? label : Short(dto.Subject ?? "");
            Text = dto.Text ?? "";
            Attributed = dto.Attributed;
            Listed = dto.Listed;
            Rejected = dto.Rejected;
            Problem = dto.Problem ?? "";
            AttributionMark = dto.Attributed
                ? "signed by them"
                : "unattributed" + (string.IsNullOrEmpty(dto.Attribution) ? "" : $" — {dto.Attribution}");
        }

        public string Who { get; }
        public string Text { get; }
        public bool Attributed { get; }
        public bool Listed { get; }
        public bool Rejected { get; }
        public string Problem { get; }
        public string AttributionMark { get; }
    }

    // --- DTOs ------------------------------------------------------------
    //
    // Every field the bridge sends is declared. An undeclared one is
    // dropped by System.Text.Json in total silence (AP49), and on this
    // surface the dropped field is the one that says an entry is NOT
    // attributable or that a reference has MOVED — which renders as
    // everything being fine.

    private sealed class FollowsDto
    {
        [JsonPropertyName("rows")] public List<FollowRowDto>? Rows { get; set; }
        [JsonPropertyName("problems")] public List<string>? Problems { get; set; }
        [JsonPropertyName("error")] public string? Error { get; set; }
    }

    private sealed class FollowRowDto
    {
        [JsonPropertyName("subject")] public string? Subject { get; set; }
        [JsonPropertyName("label")] public string? Label { get; set; }
        [JsonPropertyName("via")] public string? Via { get; set; }
        [JsonPropertyName("reachable")] public bool Reachable { get; set; }
        [JsonPropertyName("why")] public string? Why { get; set; }
    }

    private sealed class TimelineDto
    {
        [JsonPropertyName("sources")] public List<TimelineSourceDto>? Sources { get; set; }
        [JsonPropertyName("entries")] public List<TimelineEntryDto>? Entries { get; set; }
        [JsonPropertyName("advanced")] public bool Advanced { get; set; }
        [JsonPropertyName("error")] public string? Error { get; set; }
    }

    private sealed class TimelineSourceDto
    {
        [JsonPropertyName("subject")] public string? Subject { get; set; }
        [JsonPropertyName("label")] public string? Label { get; set; }
        [JsonPropertyName("count")] public int Count { get; set; }
        [JsonPropertyName("via")] public string? Via { get; set; }
        [JsonPropertyName("published")] public bool Published { get; set; }
        [JsonPropertyName("prefix")] public string? Prefix { get; set; }
        [JsonPropertyName("freshness")] public string? Freshness { get; set; }
        [JsonPropertyName("notes")] public List<string>? Notes { get; set; }
        [JsonPropertyName("error")] public string? Error { get; set; }
    }

    private sealed class TimelineEntryDto
    {
        [JsonPropertyName("subject")] public string? Subject { get; set; }
        [JsonPropertyName("label")] public string? Label { get; set; }
        [JsonPropertyName("hash")] public string? Hash { get; set; }
        [JsonPropertyName("page")] public ulong Page { get; set; }
        [JsonPropertyName("createdAtMillis")] public ulong CreatedAtMillis { get; set; }
        [JsonPropertyName("text")] public string? Text { get; set; }
        [JsonPropertyName("mediaType")] public string? MediaType { get; set; }
        [JsonPropertyName("isReply")] public bool IsReply { get; set; }
        [JsonPropertyName("listed")] public bool Listed { get; set; }
        [JsonPropertyName("attributed")] public bool Attributed { get; set; }
        [JsonPropertyName("attribution")] public string? Attribution { get; set; }
        [JsonPropertyName("rejected")] public bool Rejected { get; set; }
        [JsonPropertyName("problem")] public string? Problem { get; set; }
    }

    private sealed class RefDto
    {
        [JsonPropertyName("peer")] public string? Peer { get; set; }
        [JsonPropertyName("path")] public string? Path { get; set; }
        [JsonPropertyName("row")] public string? Row { get; set; }
        [JsonPropertyName("moved")] public bool Moved { get; set; }
        [JsonPropertyName("have")] public bool Have { get; set; }
        [JsonPropertyName("entityType")] public string? EntityType { get; set; }
        [JsonPropertyName("resolved")] public string? Resolved { get; set; }
        [JsonPropertyName("seen")] public string? Seen { get; set; }
        [JsonPropertyName("provenance")] public string? Provenance { get; set; }
        [JsonPropertyName("prefix")] public string? Prefix { get; set; }
        [JsonPropertyName("note")] public string? Note { get; set; }
        [JsonPropertyName("error")] public string? Error { get; set; }
    }
}
