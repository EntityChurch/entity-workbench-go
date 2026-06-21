package workbench

// ScreenConfig describes the layout of a single screen.
// It's a tree of splits with content types at the leaves.
// Both renderers translate this to their own layout tree type.
type ScreenConfig struct {
	// Content is set for leaf panels (no split).
	Content string
	// Settings are per-window settings applied after creation.
	Settings map[string]string

	// Split is set for split panels (Content is empty).
	Dir    SplitDir
	First  *ScreenConfig
	Second *ScreenConfig
}

// Leaf creates a leaf panel config.
func Leaf(content string) *ScreenConfig {
	return &ScreenConfig{Content: content}
}

// LeafWithSettings creates a leaf panel config with settings.
func LeafWithSettings(content string, settings map[string]string) *ScreenConfig {
	return &ScreenConfig{Content: content, Settings: settings}
}

// HSplit creates a horizontal split (left | right).
func HSplit(first, second *ScreenConfig) *ScreenConfig {
	return &ScreenConfig{Dir: SplitH, First: first, Second: second}
}

// VSplit creates a vertical split (top / bottom).
func VSplit(first, second *ScreenConfig) *ScreenConfig {
	return &ScreenConfig{Dir: SplitV, First: first, Second: second}
}

// IsLeaf returns true if this is a leaf (content) node.
func (c *ScreenConfig) IsLeaf() bool {
	return c.Content != ""
}

// DefaultScreens returns the standard workbench screen configuration.
// Both console and canvas renderers use this for identical startup.
func DefaultScreens() []*ScreenConfig {
	return []*ScreenConfig{
		// Screen 1: tree + detail on top, shell on bottom
		VSplit(
			HSplit(
				Leaf("tree-browser"),
				Leaf("entity-detail"),
			),
			Leaf("entity-shell"),
		),

		// Screen 2: log viewer + peer info
		HSplit(
			Leaf("log-viewer"),
			Leaf("peer-info"),
		),

		// Screen 3: execute console
		Leaf("execute-console"),

		// Screen 4: three log viewers — debug / verbose / info
		VSplit(
			Leaf("log-viewer"),
			VSplit(
				LeafWithSettings("log-viewer", map[string]string{
					"log-display-level": "verbose",
				}),
				LeafWithSettings("log-viewer", map[string]string{
					"log-display-level": "info",
				}),
			),
		),

		// Screen 5: query browser + inspector
		HSplit(
			Leaf("query-browser"),
			Leaf("entity-detail"),
		),

		// Screen 6: markdown files — list + view. Defaults to any
		// doc/markdown-file entity in the tree; mount your dirs first.
		HSplit(
			Leaf("markdown-files"),
			Leaf("markdown-view"),
		),
	}
}
