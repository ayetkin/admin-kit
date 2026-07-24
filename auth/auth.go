// Package auth adds Google Workspace sign-in to an adminkit panel, using the
// server-side OAuth 2.0 authorization-code flow. Sessions live behind an
// HttpOnly cookie, in whatever Store the panel provides.
//
// A panel is closed by default: New fails when sign-in is not fully configured,
// so a missing client secret cannot quietly publish an admin console. Local
// development opts out with Config.Open, which announces itself in the log.
//
//	a, err := auth.New(auth.ConfigFromEnv(), auth.NewMemoryStore(), kit, logger)
//	a.Mount(mux)                                  // login, callback, logout
//	mux.Handle("GET /admin", a.RequirePage(page)) // redirects to sign-in
//	mux.Handle("POST /admin/keys", a.RequireAPI(createKey)) // 401s instead
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	adminkit "github.com/ayetkin/admin-kit"
)

const (
	sessionCookie = "adminkit_session"
	stateCookie   = "adminkit_oauth_state"

	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"

	// stateTTL bounds how long a started sign-in may take to come back.
	stateTTL = 10 * time.Minute
)

// Auth serves the sign-in flow and gates the panel.
type Auth struct {
	cfg   Config
	store Store
	kit   *adminkit.Kit
	http  *http.Client
}

// New builds the sign-in handler. It returns an error when sign-in is not
// configured, naming what is missing, unless Config.Open says the panel is
// deliberately unguarded.
func New(cfg Config, store Store, kit *adminkit.Kit) (*Auth, error) {
	cfg = cfg.withDefaults()
	if cfg.Open {
		cfg.Logger.Warn("adminkit/auth: panel is OPEN, every request is treated as signed in")
	} else if !cfg.configured() {
		return nil, fmt.Errorf("adminkit/auth: sign-in is not configured (missing: %s); "+
			"set the Google OAuth client, PUBLIC_HOST and ALLOWED_EMAIL_DOMAINS, "+
			"or set Open for local development", cfg.missing())
	}
	if store == nil {
		return nil, fmt.Errorf("adminkit/auth: a session Store is required")
	}
	return &Auth{
		cfg:   cfg,
		store: store,
		kit:   kit,
		http:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Open reports whether the gate is disabled.
func (a *Auth) Open() bool { return a.cfg.Open }

// Mount registers the sign-in routes on mux.
func (a *Auth) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+a.cfg.LoginPath, a.login)
	mux.HandleFunc("GET "+a.cfg.CallbackPath, a.callback)
	mux.HandleFunc("GET "+a.cfg.LogoutPath, a.logout)
}

// CurrentUser adapts a session to the kit's navbar. Assign it to
// adminkit.Config.CurrentUser.
func (a *Auth) CurrentUser(r *http.Request) *adminkit.User {
	s := a.Session(r)
	if s == nil {
		return nil
	}
	return &adminkit.User{Name: s.Name, Email: s.Email, Picture: s.Picture}
}

// Session returns the signed-in operator, or nil. With an open panel it returns
// a placeholder session, so callers need no special case.
func (a *Auth) Session(r *http.Request) *Session {
	if a.cfg.Open {
		return &Session{Name: "Local operator", Email: "open-access"}
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	s, err := a.store.Get(c.Value)
	if err != nil {
		a.cfg.Logger.Error("adminkit/auth: session lookup failed", "err", err)
		return nil
	}
	return s
}

// RequirePage gates a page: a signed-out operator is sent to the sign-in page,
// which is where a browser should end up.
func (a *Auth) RequirePage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Open || a.Session(r) != nil {
			next.ServeHTTP(w, r)
			return
		}
		a.ServeLogin(w, r)
	})
}

// RequireAPI gates a JSON endpoint: a signed-out caller gets 401 rather than a
// redirect to an HTML page it cannot use.
func (a *Auth) RequireAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Open || a.Session(r) != nil {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "operator sign-in required"})
	})
}

// ServeLogin renders the kit's sign-in page, including the reason a previous
// attempt failed when the URL carries one.
func (a *Auth) ServeLogin(w http.ResponseWriter, r *http.Request) {
	a.kit.RenderLogin(w, r, adminkit.LoginData{
		SignInHref: a.cfg.LoginPath,
		DomainHint: strings.Join(a.cfg.AllowedDomains, ", "),
		Error:      errorMessage(r.URL.Query().Get("auth_error")),
	})
}

// SweepSessions deletes expired sessions on start and every interval after,
// until ctx is done. Run it in a goroutine: without it a store only forgets the
// sessions it happens to look up.
func (a *Auth) SweepSessions(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	sweep := func() {
		n, err := a.store.DeleteExpired()
		if err != nil {
			a.cfg.Logger.Warn("adminkit/auth: session sweep failed", "err", err)
			return
		}
		if n > 0 {
			a.cfg.Logger.Info("adminkit/auth: session sweep", "deleted", n)
		}
	}
	sweep()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep()
		}
	}
}

// --- the flow --------------------------------------------------------------

// login starts the OAuth dance: remember a random state, then hand off to
// Google's consent screen.
func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Open {
		http.Redirect(w, r, a.cfg.HomePath, http.StatusFound)
		return
	}
	state := randToken()
	http.SetCookie(w, a.cookie(stateCookie, state, stateTTL))

	q := url.Values{}
	q.Set("client_id", a.cfg.ClientID)
	q.Set("redirect_uri", a.cfg.redirectURL())
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("access_type", "online")
	q.Set("prompt", "select_account")
	// With a single allowed domain, ask Google to preselect it. It is a hint for
	// the operator, not a control: the domain is enforced again on the way back.
	if len(a.cfg.AllowedDomains) == 1 {
		q.Set("hd", a.cfg.AllowedDomains[0])
	}
	http.Redirect(w, r, googleAuthURL+"?"+q.Encode(), http.StatusFound)
}

// callback completes the flow: verify state, exchange the code, check the
// profile, then create the session.
func (a *Auth) callback(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Open {
		http.Redirect(w, r, a.cfg.HomePath, http.StatusFound)
		return
	}
	q := r.URL.Query()
	if q.Get("error") != "" {
		a.deny(w, r, "google", nil)
		return
	}
	sc, err := r.Cookie(stateCookie)
	if err != nil || sc.Value == "" || sc.Value != q.Get("state") {
		a.deny(w, r, "state", nil)
		return
	}
	code := q.Get("code")
	if code == "" {
		a.deny(w, r, "code", nil)
		return
	}

	token, err := a.exchange(r.Context(), code)
	if err != nil {
		a.deny(w, r, "exchange", err)
		return
	}
	info, err := a.userinfo(r.Context(), token)
	if err != nil {
		a.deny(w, r, "userinfo", err)
		return
	}
	if !info.EmailVerified || !a.domainAllowed(info.Email) {
		a.cfg.Logger.Warn("adminkit/auth: sign-in denied",
			"email", info.Email, "verified", info.EmailVerified)
		a.deny(w, r, "domain", nil)
		return
	}

	now := time.Now()
	sess := Session{
		Token:     randToken(),
		Email:     strings.ToLower(info.Email),
		Name:      firstNonEmpty(info.Name, info.Email),
		Picture:   info.Picture,
		CreatedAt: now.UnixMilli(),
		ExpiresAt: now.Add(a.cfg.SessionTTL).UnixMilli(),
	}
	if err := a.store.Create(sess); err != nil {
		a.deny(w, r, "session", err)
		return
	}
	http.SetCookie(w, a.cookie(sessionCookie, sess.Token, a.cfg.SessionTTL))
	http.SetCookie(w, a.expire(stateCookie))
	a.cfg.Logger.Info("adminkit/auth: sign-in", "email", sess.Email)
	http.Redirect(w, r, a.cfg.HomePath, http.StatusFound)
}

// logout drops the session and its cookie.
func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := a.store.Delete(c.Value); err != nil {
			a.cfg.Logger.Warn("adminkit/auth: delete session failed", "err", err)
		}
	}
	http.SetCookie(w, a.expire(sessionCookie))
	http.Redirect(w, r, a.cfg.HomePath, http.StatusFound)
}

// --- google ----------------------------------------------------------------

type googleUser struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func (a *Auth) exchange(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {a.cfg.ClientID},
		"client_secret": {a.cfg.ClientSecret},
		"redirect_uri":  {a.cfg.redirectURL()},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, err := a.do(req, "token")
	if err != nil {
		return "", err
	}
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("token endpoint returned no access_token")
	}
	return tr.AccessToken, nil
}

func (a *Auth) userinfo(ctx context.Context, accessToken string) (googleUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return googleUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	body, err := a.do(req, "userinfo")
	if err != nil {
		return googleUser{}, err
	}
	var u googleUser
	if err := json.Unmarshal(body, &u); err != nil {
		return googleUser{}, err
	}
	return u, nil
}

// do performs one Google call and returns its body, capping how much is read so
// an unexpected response cannot exhaust memory.
func (a *Auth) do(req *http.Request, what string) ([]byte, error) {
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s endpoint status %d: %s", what, resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

func (a *Auth) domainAllowed(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range a.cfg.AllowedDomains {
		if strings.EqualFold(strings.TrimSpace(d), domain) {
			return true
		}
	}
	return false
}

// --- helpers ---------------------------------------------------------------

// deny logs the real cause and sends the operator back to sign-in with a coarse
// reason code. The code never carries detail from Google, which could describe
// an account that is none of the visitor's business.
func (a *Auth) deny(w http.ResponseWriter, r *http.Request, reason string, err error) {
	if err != nil {
		a.cfg.Logger.Error("adminkit/auth: sign-in failed", "reason", reason, "err", err)
	}
	http.SetCookie(w, a.expire(stateCookie))
	http.Redirect(w, r, a.cfg.LoginPath+"?auth_error="+url.QueryEscape(reason), http.StatusFound)
}

// errorMessage turns a reason code into something an operator can act on.
func errorMessage(reason string) string {
	switch reason {
	case "":
		return ""
	case "domain":
		return "That account is not allowed to sign in here."
	case "state":
		return "The sign-in attempt expired. Please try again."
	case "google":
		return "Google cancelled the sign-in."
	default:
		return "Sign-in failed. Please try again."
	}
}

func (a *Auth) cookie(name, value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cfg.secure(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	}
}

func (a *Auth) expire(name string) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cfg.secure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
