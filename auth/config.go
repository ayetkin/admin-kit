package auth

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"
)

// Config describes operator sign-in for one panel.
//
// The panel is CLOSED unless sign-in is fully configured: a missing client id,
// secret or public host locks everyone out rather than letting everyone in.
// Local development opts out explicitly with Open, which is never a default.
type Config struct {
	// Google OAuth client credentials (Google Cloud -> Credentials -> OAuth
	// client, type Web application).
	ClientID     string
	ClientSecret string

	// PublicHost is the bare host the panel is served on, e.g. "admin.example.com"
	// or "localhost:8090". It is the single source of truth for the callback
	// URL and the cookie's Secure flag: anything but a loopback host is treated
	// as HTTPS, matching how these panels are deployed (TLS at the ingress).
	PublicHost string

	// AllowedDomains lists the email domains that may sign in. Empty means no
	// one can, which keeps a misconfiguration closed instead of open.
	AllowedDomains []string

	// Open disables the gate entirely: every request is treated as signed in.
	// It exists for local development against a panel with no OAuth client, and
	// logs a warning on every start so it cannot be left on unnoticed.
	Open bool

	// SessionTTL is how long a sign-in lasts (default 7 days).
	SessionTTL time.Duration

	// Paths the handlers are mounted on. The defaults suit a panel living under
	// /admin; CallbackPath must match the redirect URI registered with Google.
	LoginPath    string // default "/admin/login"
	CallbackPath string // default "/admin/callback"
	LogoutPath   string // default "/admin/logout"
	// HomePath is where a completed sign-in (or a sign-out) lands.
	HomePath string // default "/admin"

	Logger *slog.Logger
}

// ConfigFromEnv reads the standard environment variables, so every panel is
// configured the same way:
//
//	GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET  OAuth client
//	PUBLIC_HOST                             bare host, e.g. admin.example.com
//	ALLOWED_EMAIL_DOMAINS                   comma separated
//	ADMIN_OPEN=1                            development only: no sign-in at all
func ConfigFromEnv() Config {
	return Config{
		ClientID:       os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret:   os.Getenv("GOOGLE_CLIENT_SECRET"),
		PublicHost:     strings.TrimSpace(os.Getenv("PUBLIC_HOST")),
		AllowedDomains: splitCSV(os.Getenv("ALLOWED_EMAIL_DOMAINS")),
		Open:           isTruthy(os.Getenv("ADMIN_OPEN")),
	}
}

// withDefaults fills in the optional fields.
func (c Config) withDefaults() Config {
	if c.SessionTTL <= 0 {
		c.SessionTTL = 7 * 24 * time.Hour
	}
	if c.LoginPath == "" {
		c.LoginPath = "/admin/login"
	}
	if c.CallbackPath == "" {
		c.CallbackPath = "/admin/callback"
	}
	if c.LogoutPath == "" {
		c.LogoutPath = "/admin/logout"
	}
	if c.HomePath == "" {
		c.HomePath = "/admin"
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return c
}

// configured reports whether sign-in can actually run.
func (c Config) configured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.PublicHost != "" && len(c.AllowedDomains) > 0
}

// missing explains what sign-in still needs, for the startup error.
func (c Config) missing() string {
	var out []string
	if c.ClientID == "" {
		out = append(out, "ClientID")
	}
	if c.ClientSecret == "" {
		out = append(out, "ClientSecret")
	}
	if c.PublicHost == "" {
		out = append(out, "PublicHost")
	}
	if len(c.AllowedDomains) == 0 {
		out = append(out, "AllowedDomains")
	}
	return strings.Join(out, ", ")
}

// secure reports whether the panel is served over HTTPS, which follows from the
// public host: only a loopback host is plain HTTP.
func (c Config) secure() bool { return c.PublicHost != "" && !isLoopback(c.PublicHost) }

// redirectURL is the OAuth callback, derived from PublicHost so the domain is
// configured in exactly one place. It must match the redirect URI registered
// with Google.
func (c Config) redirectURL() string {
	if c.PublicHost == "" {
		return ""
	}
	scheme := "https"
	if !c.secure() {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s%s", scheme, c.PublicHost, c.CallbackPath)
}

// isLoopback reports whether host (optionally with a port) is a local address.
func isLoopback(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
