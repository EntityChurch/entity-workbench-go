using Avalonia;
using Avalonia.Headless;
using EntityAvalonia;
using EntityAvalonia.Tests;

// AvaloniaTestApplication wires HeadlessUnitTestSession to the same
// App type the real binary uses.
//
// IMPORTANT — UseHeadlessDrawing=false + .UseSkia() turns on REAL
// Skia rendering. Previously this was true, which stubs paint; that
// hid every paint-layer bug from our tests (including the user-
// reported segfault that motivated the click-around stress suite).
// With Skia on, the test process actually rasterizes the visual
// tree just like the real app does. This catches bugs in:
//   - Markdown.Avalonia / our custom MarkdownRenderer's inline layout
//   - SelectableTextBlock with large Inline collections
//   - HarfBuzz text shaping on real fonts
//   - any visual-tree disposal race that affects paint
//
// Builder-stage dependency: libSkiaSharp requires fontconfig +
// freetype + harfbuzz + libpng. Containerfile installs those.
[assembly: AvaloniaTestApplication(typeof(TestAppBuilder))]

namespace EntityAvalonia.Tests;

public static class TestAppBuilder
{
    public static AppBuilder BuildAvaloniaApp() =>
        AppBuilder.Configure<App>()
            .UseSkia()
            .UseHeadless(new AvaloniaHeadlessPlatformOptions
            {
                UseHeadlessDrawing = false,
            });
}
