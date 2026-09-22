# Rendering an HTML page in the browser panel

**Status: text lowering ships; a document renderer does not.** This file
records what we do today, what a real HTML renderer would cost, and the
one property that makes the decision harder here than in an ordinary
application. Written 2026-08-31, when the lowering landed.

## What a site can publish

The site convention's `SitePage.format` admits two values in this cohort:

| `format` | what it is |
|---|---|
| `markdown` | the universal base — the form almost every page uses |
| `html` | the **web-tier escape hatch**: a pre-rendered document (a Pandoc paper, a book) stored verbatim |

`html` is not a degenerate case or an authoring mistake. It is how a
publisher ships something markdown cannot express, and the live
federation uses it heavily. Measured on `billslab.com`, 2026-08-31:

```
sites/billslab-entity-system/pages/papers/full-corpus         html   8,267,316 bytes
sites/billslab-entity-system/pages/papers/narrative-order      html   8,267,821
sites/billslab-entity-system/pages/papers/tree-order           html   8,267,511
sites/billslab-entity-system/pages/papers/methodology-path     html   4,458,626
sites/billslab-entity-system/pages/papers/architecture-path    html   2,992,498
sites/billslab-entity-system/pages/papers/12-abiogenesis-theory html  2,498,516
  … 23 HTML papers in that one site
```

## What we do today

`workbench.NewBodyView` lowers an HTML body to plain text
(`PlainTextFromHTML`), caps the result at `MaxDisplayBytes`, and states
both facts in `BodyView.Note`. The renderer is told `IsMarkdown=false`
and must not parse the result.

The lowering is deliberately **not a parse**. There is no DOM, no
resource loading, no style resolution — the input is bytes from a remote
publisher and the job is to show a reader what the document says without
this process doing anything the document asks for. Block tags become
line breaks, `script`/`style`/`head`/`svg` contents are dropped, entities
are decoded **last** so a decoded `&lt;script&gt;` cannot become a tag.

What is lost, and is stated to the reader rather than hidden: tables
collapse to their cell text, images vanish, links become their labels,
and any layout the author expressed in markup is gone.

## Why not a WebView

An embedded browser engine is the obvious answer and it is a much larger
decision than "add a package". Three costs, in increasing order of how
much they matter.

**1. The dependency.** Avalonia has no first-party WebView. The options
are `WebViewControl-Avalonia` (CEF — a full Chromium, ~150 MB in the
publish output, and this project ships a self-contained linux-x64 build
from a podman image), or a WebKitGTK binding (smaller, but adds a
GTK dependency to a project that deliberately runs on bare X11/Skia and
whose substrate model documents seven boundaries it already has to
reason about — `MODEL-AVALONIA-RUNTIME.md`). Either one adds a process,
a renderer, and a crash surface to a stack we have spent eight commits
learning to keep alive.

**2. It is a new trust boundary, and it is the wrong shape.** Every
other byte this panel displays arrives with a chain beside it, and the
chain's claim is *"these exact bytes hash to what the signed root
committed"*. A browser engine does not display bytes; it **executes** a
document. Handed an 8 MB HTML page it will resolve `<img src>`,
`<link rel=stylesheet>`, `<script src>`, `@font-face`, and anything else
the author wrote — against origins nobody in the chain vouched for. A
single `<img src="https://tracker/x.gif">` in a published page turns
every reader of that page into a request the publisher can count, from a
browser whose entire premise is that it fetches nothing it cannot
verify.

That is not hypothetical: it is the same class of hole
`workbench.AssetNameFromRef` exists to close for figures, where the
answer was to **refuse every reference that is not a site-local asset the
signed root commits to**. A WebView would need the identical policy
applied to every subresource the engine can request, enforced by
intercepting the engine's network stack — which is possible in CEF and
is a substantial piece of work whose failure mode is silent.

**3. `format: html` is an escape hatch, and rendering it well makes it
attractive.** Today a publisher who wants structure that markdown cannot
express reaches for HTML and accepts that consumers may lower it. If the
GUI rendered HTML as a document, HTML would become the natural way to
publish anything rich — and a page whose meaning depends on a browser
engine is a page the tview renderer, the shell, and any future consumer
cannot show at all. The multi-renderer discipline in this repo exists to
keep that from happening by accident.

## What would be cheaper and is probably better

Ordered by value per unit of work:

1. **Structured lowering instead of flat text.** `PlainTextFromHTML`
   throws away the block structure it already identifies. Lowering to
   *markdown* rather than to text — headings to `#`, `<table>` to a pipe
   table, `<a>` to a link, `<img src="assets/…">` to an embed — would
   put HTML pages through the renderer that already draws tables,
   clickable links and figures. Same security posture as today (no
   fetches, no execution), and the `<img>` case reuses
   `AssetNameFromRef` verbatim, so a tracking pixel is refused by the
   rule that already refuses one. **This is the recommended next step.**
2. **An "open in your browser" verb.** Write the verified bytes to a
   temp file and hand them to the system browser, with a sentence saying
   the chain does not extend past that boundary. Honest, trivial, and
   correct for the reader who wants the paper as a paper.
3. **Raise the cap for text.** `MaxDisplayBytes` is 256 KiB because an
   inline flow is the bottleneck, not the bytes. A virtualized text view
   for the lowered form would let a whole paper be readable.

## The measurement that constrains any of this

The cap is not only about the renderer. Before it, an 8.27 MB body
crossed the cgo bridge as a JSON string on every render and was handed
to Markdig on the UI thread. `BrowseRender` now blanks
`Content.BodyMarkdown` and sends only the projection; the shell still
reads the full body, because a terminal has a pager behind it.

Any HTML renderer has to answer the same question: **what crosses the
boundary, and how often.** A WebView that is handed the document once
per navigation is fine; one fed from a re-marshalled render envelope is
the same defect wearing a Chromium.
