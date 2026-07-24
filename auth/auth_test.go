package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	adminkit "github.com/ayetkin/admin-kit"
)

func testKit(t *testing.T) *adminkit.Kit {
	t.Helper()
	k, err := adminkit.New(adminkit.Config{
		Brand: "Panel",
		Templates: fstest.MapFS{
			"dashboard.html": &fstest.MapFile{Data: []byte(`{{define "content"}}<p>secret</p>{{end}}`)},
		},
	})
	if err != nil {
		t.Fatalf("adminkit.New: %v", err)
	}
	return k
}

// configured is a Config that passes New's checks, for tests about behaviour
// rather than configuration.
func configured() Config {
	return Config{
		ClientID:       "client",
		ClientSecret:   "secret",
		PublicHost:     "admin.example.com",
		AllowedDomains: []string{"example.com"},
	}
}

// A panel must be closed unless sign-in is fully configured: the whole point of
// the default is that a half-filled config cannot publish an admin console.
func TestNewRefusesAnIncompleteConfig(t *testing.T) {
	cases := map[string]func(*Config){
		"no client id":     func(c *Config) { c.ClientID = "" },
		"no client secret": func(c *Config) { c.ClientSecret = "" },
		"no public host":   func(c *Config) { c.PublicHost = "" },
		"no domains":       func(c *Config) { c.AllowedDomains = nil },
	}
	for name, break_ := range cases {
		cfg := configured()
		break_(&cfg)
		if _, err := New(cfg, NewMemoryStore(), testKit(t)); err == nil {
			t.Errorf("%s: New succeeded, want an error", name)
		}
	}

	if _, err := New(configured(), NewMemoryStore(), testKit(t)); err != nil {
		t.Errorf("complete config: New = %v, want nil", err)
	}
}

func TestOpenSkipsTheGate(t *testing.T) {
	a, err := New(Config{Open: true}, NewMemoryStore(), testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()
	a.RequirePage(okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "handled" {
		t.Fatalf("open panel: status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if u := a.CurrentUser(httptest.NewRequest(http.MethodGet, "/admin", nil)); u == nil {
		t.Error("open panel should still report a user for the navbar")
	}
}

func TestRequirePageShowsSignInWhenSignedOut(t *testing.T) {
	a, err := New(configured(), NewMemoryStore(), testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()
	a.RequirePage(okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (the sign-in page)", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sign in with Google") {
		t.Error("expected the sign-in page")
	}
	if strings.Contains(body, "handled") {
		t.Error("the gated handler ran for a signed-out request")
	}
}

// A JSON endpoint gets a 401, not an HTML page it cannot use.
func TestRequireAPIRejectsWithJSON(t *testing.T) {
	a, err := New(configured(), NewMemoryStore(), testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()
	a.RequireAPI(okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/keys", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
}

func TestASessionCookieOpensTheGate(t *testing.T) {
	store := NewMemoryStore()
	a, err := New(configured(), store, testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sess := Session{
		Token: "tok", Email: "ada@example.com", Name: "Ada Lovelace",
		CreatedAt: time.Now().UnixMilli(),
		ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
	}
	if err := store.Create(sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	rec := httptest.NewRecorder()
	a.RequirePage(okHandler()).ServeHTTP(rec, req)

	if rec.Body.String() != "handled" {
		t.Fatalf("gated handler did not run: %q", rec.Body.String())
	}
	u := a.CurrentUser(req)
	if u == nil || u.Email != "ada@example.com" || u.Name != "Ada Lovelace" {
		t.Fatalf("CurrentUser = %+v, want Ada", u)
	}
}

func TestAnExpiredSessionIsRejectedAndForgotten(t *testing.T) {
	store := NewMemoryStore()
	a, err := New(configured(), store, testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.Create(Session{Token: "old", ExpiresAt: time.Now().Add(-time.Minute).UnixMilli()}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "old"})
	if s := a.Session(req); s != nil {
		t.Fatal("an expired session was accepted")
	}
	if s, _ := store.Get("old"); s != nil {
		t.Error("an expired session should be dropped on lookup")
	}
}

func TestLoginRedirectsToGoogleWithState(t *testing.T) {
	a, err := New(configured(), NewMemoryStore(), testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	a.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/login", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, googleAuthURL) {
		t.Fatalf("Location = %q, want Google's consent screen", loc)
	}
	for _, want := range []string{
		"client_id=client",
		"redirect_uri=https%3A%2F%2Fadmin.example.com%2Fadmin%2Fcallback",
		"hd=example.com", // single allowed domain is preselected
		"state=",
	} {
		if !strings.Contains(loc, want) {
			t.Errorf("consent URL is missing %q: %s", want, loc)
		}
	}

	// The state must be echoed into a cookie, or the callback cannot verify it.
	var state string
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookie {
			state = c.Value
		}
	}
	if state == "" {
		t.Fatal("no state cookie was set")
	}
	if !strings.Contains(loc, "state="+state) {
		t.Error("the state cookie does not match the one sent to Google")
	}
}

// A callback carrying the wrong state is a forged or stale sign-in and must not
// create a session.
func TestCallbackRejectsAMismatchedState(t *testing.T) {
	a, err := New(configured(), NewMemoryStore(), testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	a.Mount(mux)

	req := httptest.NewRequest(http.MethodGet, "/admin/callback?code=abc&state=attacker", nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: "real"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "auth_error=state") {
		t.Errorf("Location = %q, want a state error", loc)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			t.Error("a session cookie was issued for a mismatched state")
		}
	}
}

func TestLogoutClearsTheSession(t *testing.T) {
	store := NewMemoryStore()
	a, err := New(configured(), store, testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.Create(Session{Token: "tok", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	mux := http.NewServeMux()
	a.Mount(mux)

	req := httptest.NewRequest(http.MethodGet, "/admin/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if s, _ := store.Get("tok"); s != nil {
		t.Error("the session survived a logout")
	}
	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the session cookie was not expired")
	}
}

func TestDomainAllowed(t *testing.T) {
	cfg := configured()
	cfg.AllowedDomains = []string{"example.com", "Second.test"}
	a, err := New(cfg, NewMemoryStore(), testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := map[string]bool{
		"ada@example.com":      true,
		"ada@second.test":      true, // domains compare case-insensitively
		"ada@EXAMPLE.COM":      true,
		"ada@evil.com":         false,
		"ada@sub.example.com":  false, // a subdomain is a different domain
		"not-an-email":         false,
		"ada@example.com.evil": false,
	}
	for email, want := range cases {
		if got := a.domainAllowed(email); got != want {
			t.Errorf("domainAllowed(%q) = %v, want %v", email, got, want)
		}
	}
}

// The public host decides both the callback scheme and the cookie's Secure
// flag, so a localhost panel still works over plain HTTP.
func TestPublicHostDrivesSchemeAndCookieSecurity(t *testing.T) {
	cases := []struct {
		host       string
		wantURL    string
		wantSecure bool
	}{
		{"admin.example.com", "https://admin.example.com/admin/callback", true},
		{"localhost:8090", "http://localhost:8090/admin/callback", false},
		{"127.0.0.1:8090", "http://127.0.0.1:8090/admin/callback", false},
		{"", "", false},
	}
	for _, tc := range cases {
		cfg := configured().withDefaults()
		cfg.PublicHost = tc.host
		if got := cfg.redirectURL(); got != tc.wantURL {
			t.Errorf("redirectURL(%q) = %q, want %q", tc.host, got, tc.wantURL)
		}
		if got := cfg.secure(); got != tc.wantSecure {
			t.Errorf("secure(%q) = %v, want %v", tc.host, got, tc.wantSecure)
		}
	}
}

func TestSessionCookieIsHttpOnly(t *testing.T) {
	a, err := New(configured(), NewMemoryStore(), testKit(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c := a.cookie(sessionCookie, "value", time.Hour)
	if !c.HttpOnly {
		t.Error("the session cookie must be HttpOnly")
	}
	if !c.Secure {
		t.Error("the session cookie must be Secure on an https host")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Error("the session cookie needs SameSite=Lax to survive the OAuth redirect")
	}
}

func TestMemoryStoreDeleteExpired(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now()
	_ = s.Create(Session{Token: "live", ExpiresAt: now.Add(time.Hour).UnixMilli()})
	_ = s.Create(Session{Token: "dead", ExpiresAt: now.Add(-time.Hour).UnixMilli()})

	n, err := s.DeleteExpired()
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
	if got, _ := s.Get("live"); got == nil {
		t.Error("a live session was swept")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("PUBLIC_HOST", " admin.example.com ")
	t.Setenv("ALLOWED_EMAIL_DOMAINS", "example.com, second.test ,")
	t.Setenv("ADMIN_OPEN", "yes")

	cfg := ConfigFromEnv()
	if cfg.PublicHost != "admin.example.com" {
		t.Errorf("PublicHost = %q, want it trimmed", cfg.PublicHost)
	}
	if len(cfg.AllowedDomains) != 2 {
		t.Errorf("AllowedDomains = %v, want two entries", cfg.AllowedDomains)
	}
	if !cfg.Open {
		t.Error("ADMIN_OPEN=yes should open the panel")
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("handled"))
	})
}
