package theme

// releasedLayoutParts are the data-part names the last 1.x release offered to
// theme CSS, copied verbatim from internal/theme/layout.json at v1.4.1 (the
// list has not changed since v1.3.0, the first release with parts). A theme
// written for 1.x may select any of them, so each is either a current part or
// renamed in renamedParts.
var releasedLayoutParts = []string{
	"app-shell",
	"sidebar",
	"sidebar-nav",
	"sidebar-item",
	"sidebar-separator",
	"sidebar-bottom",
	"sidebar-account",
	"topbar",
	"topbar-history",
	"topbar-search",
	"topbar-actions",
	"topbar-action",
	"window-controls",
	"main",
	"page",
	"game-hero",
	"game-actions",
	"game-tabs",
	"game-main",
	"game-side",
	"game-section",
}
