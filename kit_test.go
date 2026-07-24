package adminkit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// testFS is a minimal project template set: one page per file, each defining
// "content", exactly as a real panel does.
func testFS() fstest.MapFS {
	return fstest.MapFS{
		"templates/dashboard.html": &fstest.MapFile{Data: []byte(
			`{{define "content"}}<p id="page">dashboard {{.Data.Name}}</p>{{end}}`)},
		"templates/keys.html": &fstest.MapFile{Data: []byte(
			`{{define "content"}}<p id="page">keys</p>{{end}}`)},
	}
}

func newTestKit(t *testing.T, cfg Config) *Kit {
	t.Helper()
	if cfg.Templates == nil {
		cfg.Templates = testFS()
		cfg.Patterns = []string{"templates/*.html"}
	}
	k, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return k
}

// get renders one page through a real handler and returns the body.
func get(t *testing.T, k *Kit, path, page string, data any) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	k.Render(rec, req, page, data)
	if rec.Code != http.StatusOK {
		t.Fatalf("render %s: status = %d, want 200 (body %q)", page, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestRenderWrapsPageInTheLayout(t *testing.T) {
	k := newTestKit(t, Config{Brand: "Panel"})
	body := get(t, k, "/admin", "dashboard.html", map[string]string{"Name": "one"})

	for _, want := range []string{
		`<title>Panel</title>`,            // layout title from the brand
		`data-bs-theme="dark"`,            // default theme
		`<p id="page">dashboard one</p>`,  // the page's own content block
		`/adminkit/tabler/tabler.min.css`, // vendored assets
		`id="adminkit-toasts"`,            // toast host
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
}

// Two pages defining the same block must not bleed into each other; that is the
// whole reason each page is parsed onto its own clone of the layout.
func TestPagesDoNotShareContentBlocks(t *testing.T) {
	k := newTestKit(t, Config{})
	if body := get(t, k, "/admin", "dashboard.html", map[string]string{"Name": "x"}); !strings.Contains(body, "dashboard x") {
		t.Fatalf("dashboard rendered the wrong content: %s", body)
	}
	if body := get(t, k, "/admin/keys", "keys.html", nil); !strings.Contains(body, `<p id="page">keys</p>`) {
		t.Fatalf("keys page rendered the wrong content: %s", body)
	}
}

func TestActiveNavMarksTheLongestMatch(t *testing.T) {
	k := newTestKit(t, Config{Nav: []NavItem{
		{Label: "Dashboard", Href: "/admin"},
		{Label: "Keys", Href: "/admin/keys"},
	}})

	cases := []struct {
		path string
		want string
	}{
		{"/admin", "/admin"},
		{"/admin/keys", "/admin/keys"},     // not shadowed by the shorter /admin
		{"/admin/keys/abc", "/admin/keys"}, // a detail page keeps its section
		{"/admin/keysomething", "/admin"},  // a prefix must end on a boundary
		{"/other", ""},                     // nothing matches
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if got := k.activeNav(req); got != tc.want {
			t.Errorf("activeNav(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestUserMenuFollowsCurrentUser(t *testing.T) {
	var user *User
	k := newTestKit(t, Config{CurrentUser: func(*http.Request) *User { return user }})

	if body := get(t, k, "/admin", "dashboard.html", map[string]string{}); strings.Contains(body, "Sign out") {
		t.Error("signed-out navbar should not offer sign out")
	}
	user = &User{Name: "Ada Lovelace", Email: "ada@example.com"}
	body := get(t, k, "/admin", "dashboard.html", map[string]string{})
	for _, want := range []string{"Ada Lovelace", "ada@example.com", "Sign out", ">AL<"} {
		if !strings.Contains(body, want) {
			t.Errorf("signed-in navbar is missing %q", want)
		}
	}
}

func TestRenderUnknownPageIsA500WithoutDetail(t *testing.T) {
	k := newTestKit(t, Config{})
	rec := httptest.NewRecorder()
	k.Render(rec, httptest.NewRequest(http.MethodGet, "/admin", nil), "nope.html", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	// The template name is an internal detail; it belongs in the log, not the page.
	if strings.Contains(rec.Body.String(), "nope.html") {
		t.Errorf("error body leaked internals: %q", rec.Body.String())
	}
}

func TestNewRejectsDuplicatePageNames(t *testing.T) {
	fsys := fstest.MapFS{
		"a/page.html": &fstest.MapFile{Data: []byte(`{{define "content"}}a{{end}}`)},
		"b/page.html": &fstest.MapFile{Data: []byte(`{{define "content"}}b{{end}}`)},
	}
	_, err := New(Config{Templates: fsys, Patterns: []string{"*/*.html"}})
	if err == nil || !strings.Contains(err.Error(), "page.html") {
		t.Fatalf("err = %v, want a duplicate-name error", err)
	}
}

func TestNewRejectsAnEmptyTemplateSet(t *testing.T) {
	if _, err := New(Config{Templates: fstest.MapFS{}, Patterns: []string{"*.html"}}); err == nil {
		t.Fatal("expected an error when no template matches")
	}
}

func TestRenderLoginOffersSignInOnlyWhenConfigured(t *testing.T) {
	k := newTestKit(t, Config{Brand: "Panel"})

	rec := httptest.NewRecorder()
	k.RenderLogin(rec, httptest.NewRequest(http.MethodGet, "/admin/login", nil), LoginData{
		SignInHref: "/admin/login", DomainHint: "example.com", Error: "That account is not allowed.",
	})
	body := rec.Body.String()
	for _, want := range []string{"Sign in with Google", "example.com", "That account is not allowed.", "Panel"} {
		if !strings.Contains(body, want) {
			t.Errorf("login page is missing %q", want)
		}
	}

	rec = httptest.NewRecorder()
	k.RenderLogin(rec, httptest.NewRequest(http.MethodGet, "/admin/login", nil), LoginData{})
	if body := rec.Body.String(); strings.Contains(body, "Sign in with Google") {
		t.Error("unconfigured sign-in should not offer a dead button")
	}
}

func TestPageHandlerRendersWithRequestData(t *testing.T) {
	k := newTestKit(t, Config{})
	h := k.Page("dashboard.html", func(r *http.Request) any {
		return map[string]string{"Name": r.URL.Query().Get("n")}
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin?n=frompage", nil))
	if !strings.Contains(rec.Body.String(), "dashboard frompage") {
		t.Errorf("handler did not pass request data through: %s", rec.Body.String())
	}
}
