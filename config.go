package adminkit

import (
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
)

// Config describes one admin panel. Every field is optional except Templates:
// the zero value yields a dark, brandless panel with no navigation, which is
// enough to render a page.
type Config struct {
	// Brand is the name shown at the top left; BrandHref is where it links
	// (default "/admin").
	Brand     string
	BrandHref string

	// Nav is the top navigation. Leave it empty for a single-page panel: the
	// navbar then carries only the brand and the user menu.
	Nav []NavItem

	// Templates holds the project's page templates, each defining a "content"
	// block. Patterns selects them (default "*.html"). Every page is parsed onto
	// its own copy of the kit layout, so two pages may define the same block
	// names without colliding.
	Templates fs.FS
	Patterns  []string

	// Funcs adds project-specific template functions on top of the kit's own
	// (see funcs.go). A name defined here wins.
	Funcs template.FuncMap

	// CurrentUser fills the user menu. Returning nil renders the navbar signed
	// out. The auth package's Auth.CurrentUser satisfies this, but any source
	// works - the kit deliberately knows nothing about how sign-in happens.
	CurrentUser func(*http.Request) *User

	// SignOutHref is the user menu's sign-out link (default "/admin/logout").
	// Ignored when CurrentUser is unset.
	SignOutHref string

	// AssetPath is the URL prefix the kit's CSS/JS/fonts are served under
	// (default "/adminkit"). Change it only on a path collision.
	AssetPath string

	// Theme is "dark" (default), "light" or "system" (follows the OS).
	// Operators can flip it at runtime from the navbar; this is the initial
	// value.
	Theme string

	// Favicon is the browser-tab icon's URL, used as-is in <link rel="icon">:
	// a path the panel serves or a data: URL. Empty keeps the kit's sparkle.
	Favicon string

	// Logger receives render failures, which are reported to the browser as a
	// bare 500 so template internals never reach it. Defaults to discarding.
	Logger *slog.Logger
}

// NavItem is one top-navigation entry.
type NavItem struct {
	Label string
	Href  string
	// Icon is a Tabler icon name without the "ti-" prefix, e.g. "key".
	// See https://tabler.io/icons. Empty renders a label-only item.
	Icon string
	// Prefix marks the item active for any URL path under it. Defaults to Href,
	// which is right unless one item's path is a prefix of another's.
	Prefix string
}

// User is who is signed in, as shown in the navbar's user menu.
type User struct {
	Name    string
	Email   string
	Picture string // avatar URL; initials are drawn when empty
}

// defaultFavicon is the sparkle every panel shows until it sets its own.
const defaultFavicon = "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'><text y='26' font-size='26'>✨</text></svg>"

// withDefaults returns c with every optional field filled in.
func (c Config) withDefaults() Config {
	if c.BrandHref == "" {
		c.BrandHref = "/admin"
	}
	if c.SignOutHref == "" {
		c.SignOutHref = "/admin/logout"
	}
	if c.AssetPath == "" {
		c.AssetPath = "/adminkit"
	}
	// Trailing slashes would double up when the path is joined with a file name.
	for len(c.AssetPath) > 1 && c.AssetPath[len(c.AssetPath)-1] == '/' {
		c.AssetPath = c.AssetPath[:len(c.AssetPath)-1]
	}
	if c.Theme != "light" && c.Theme != "system" {
		c.Theme = "dark"
	}
	if c.Favicon == "" {
		c.Favicon = defaultFavicon
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if len(c.Patterns) == 0 {
		c.Patterns = []string{"*.html"}
	}
	for i, n := range c.Nav {
		if n.Prefix == "" {
			c.Nav[i].Prefix = n.Href
		}
	}
	return c
}
