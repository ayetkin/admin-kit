// Package adminkit renders admin panels that all look and behave the same.
//
// It provides the parts every internal panel repeats - page shell, top
// navigation, user menu, theme, toasts, asset delivery - on top of a vendored
// copy of Tabler (https://tabler.io). Components come from Tabler itself: a
// page is written as ordinary HTML, pasted from Tabler's documentation, so the
// kit never has to grow a component library of its own.
//
// A panel is three things: a Config, one template per page defining a "content"
// block, and a mux.
//
//	kit, err := adminkit.New(adminkit.Config{
//		Brand:     "Acme Ops",
//		Templates: templatesFS,
//		Nav:       []adminkit.NavItem{{Label: "Keys", Href: "/admin/keys", Icon: "key"}},
//	})
//	kit.Mount(mux)
//	mux.Handle("GET /admin", kit.Page("dashboard.html", func(r *http.Request) any {
//		return dashboardData()
//	}))
//
// Sign-in is not part of the core: set Config.CurrentUser to show a user in the
// navbar, and see the auth subpackage for Google Workspace sign-in.
package adminkit

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Kit is a configured panel: the layout plus one parsed template per page.
// It is read-only after New and safe for concurrent use.
type Kit struct {
	cfg Config
	// base holds the kit's own templates (layout, login, partials); pages holds
	// one clone of it per project page.
	base  *template.Template
	pages map[string]*template.Template
}

// New parses the kit layout together with the project's page templates. It
// fails when a page template is missing or malformed, so a broken panel is
// caught at startup rather than on the first request.
func New(cfg Config) (*Kit, error) {
	cfg = cfg.withDefaults()

	base := template.New("adminkit").Funcs(builtinFuncs(cfg)).Funcs(cfg.Funcs)
	base, err := base.ParseFS(kitTemplates, "templates/*.html", "templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("adminkit: parse kit templates: %w", err)
	}

	k := &Kit{cfg: cfg, base: base, pages: map[string]*template.Template{}}
	if cfg.Templates == nil {
		return k, nil // a panel that only serves the kit's own pages (e.g. login)
	}

	names, err := matchTemplates(cfg.Templates, cfg.Patterns)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("adminkit: no templates matched %v", cfg.Patterns)
	}
	for _, name := range names {
		// Pages are addressed by file name ("dashboard.html"), not by the path
		// the pattern happened to match, so a handler never has to know where
		// the templates live.
		page := path.Base(name)
		if _, dup := k.pages[page]; dup {
			return nil, fmt.Errorf("adminkit: two templates are named %s; page names must be unique", page)
		}
		// Each page gets its own copy of the layout so that two pages may both
		// define "content" (and any other block) without colliding.
		clone, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("adminkit: clone layout for %s: %w", name, err)
		}
		if _, err := clone.ParseFS(cfg.Templates, name); err != nil {
			return nil, fmt.Errorf("adminkit: parse %s: %w", name, err)
		}
		k.pages[page] = clone
	}
	return k, nil
}

// MustNew is New for package-level initialisation; it panics on error.
func MustNew(cfg Config) *Kit {
	k, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return k
}

// matchTemplates lists the template names in fsys matching any pattern, in
// stable order.
func matchTemplates(fsys fs.FS, patterns []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, p := range patterns {
		matches, err := fs.Glob(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("adminkit: bad pattern %q: %w", p, err)
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out, nil
}

// PageData is what a page template sees. Project values live under .Data, so
// {{.Data.Keys}} reaches what the handler returned, while the shell reads
// .Brand, .Nav and friends.
type PageData struct {
	Title     string
	Brand     string
	BrandHref string
	Nav       []NavItem
	// Active is the Prefix of the nav item matching the current URL, "" if none.
	Active      string
	User        *User
	SignOutHref string
	AssetPath   string
	Theme       string
	Data        any
}

// Render writes one page. name is the template's file name as it appears in
// Config.Templates, e.g. "dashboard.html".
func (k *Kit) Render(w http.ResponseWriter, r *http.Request, name string, data any) {
	k.render(w, r, name, k.pageData(r, data))
}

// RenderTitled is Render with an explicit <title>, for pages whose title is not
// just the brand (a detail view, say).
func (k *Kit) RenderTitled(w http.ResponseWriter, r *http.Request, name, title string, data any) {
	pd := k.pageData(r, data)
	pd.Title = title
	k.render(w, r, name, pd)
}

// Page adapts Render to an http.Handler for the common case of a page whose
// data depends only on the request. A nil data function renders the page with
// no project data.
func (k *Kit) Page(name string, data func(*http.Request) any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v any
		if data != nil {
			v = data(r)
		}
		k.Render(w, r, name, v)
	})
}

func (k *Kit) pageData(r *http.Request, data any) PageData {
	pd := PageData{
		Title:       k.cfg.Brand,
		Brand:       k.cfg.Brand,
		BrandHref:   k.cfg.BrandHref,
		Nav:         k.cfg.Nav,
		Active:      k.activeNav(r),
		SignOutHref: k.cfg.SignOutHref,
		AssetPath:   k.cfg.AssetPath,
		Theme:       k.cfg.Theme,
		Data:        data,
	}
	if k.cfg.CurrentUser != nil {
		pd.User = k.cfg.CurrentUser(r)
	}
	return pd
}

// activeNav returns the Prefix of the nav item that best matches the request
// path. The longest match wins, so "/admin" does not shadow "/admin/keys".
func (k *Kit) activeNav(r *http.Request) string {
	if r == nil {
		return ""
	}
	path := r.URL.Path
	best := ""
	for _, n := range k.cfg.Nav {
		p := n.Prefix
		if p == "" || len(p) <= len(best) {
			continue
		}
		if path == p || (strings.HasPrefix(path, p) && (strings.HasSuffix(p, "/") || path[len(p)] == '/')) {
			best = p
		}
	}
	return best
}

// render executes a page into a buffer first, so a template error surfaces as a
// clean 500 instead of a half-written page appended to a 200 response.
func (k *Kit) render(w http.ResponseWriter, r *http.Request, name string, pd PageData) {
	tpl, ok := k.pages[name]
	if !ok {
		k.fail(w, r, fmt.Errorf("adminkit: no such page template %q", name))
		return
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout.html", pd); err != nil {
		k.fail(w, r, fmt.Errorf("adminkit: render %s: %w", name, err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// fail logs the real cause and shows the browser nothing but a 500: a template
// error names internal paths and field types, which has no business reaching a
// browser even on an internal panel.
func (k *Kit) fail(w http.ResponseWriter, r *http.Request, err error) {
	path := ""
	if r != nil {
		path = r.URL.Path
	}
	k.cfg.Logger.Error("adminkit render failed", "path", path, "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
