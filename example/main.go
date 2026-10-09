// Command example is a working panel built on the kit: run it to see every
// component the kit ships, and to check a change before adopting it elsewhere.
//
//	go run ./example        # http://localhost:8099/admin
//
// It runs with ADMIN_OPEN=1 so no OAuth client is needed. Configure the Google
// variables instead to exercise the real sign-in flow (see auth.ConfigFromEnv).
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	adminkit "github.com/ayetkin/admin-kit"
	"github.com/ayetkin/admin-kit/auth"
)

//go:embed templates/*.html
var templatesFS embed.FS

const addr = ":8099"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// A panel with no OAuth client is closed, so the example opens it
	// explicitly. Real panels leave this to the environment.
	if os.Getenv("GOOGLE_CLIENT_ID") == "" && os.Getenv("ADMIN_OPEN") == "" {
		_ = os.Setenv("ADMIN_OPEN", "1")
	}

	authCfg := auth.ConfigFromEnv()
	authCfg.Logger = logger

	// The kit needs the user provider, and auth needs the kit for its sign-in
	// page, so auth is built first and its method handed over in the config.
	var a *auth.Auth
	kit, err := adminkit.New(adminkit.Config{
		Brand:     "admin-kit",
		Templates: templatesFS,
		Patterns:  []string{"templates/*.html"},
		Logger:    logger,
		Nav: []adminkit.NavItem{
			{Label: "Dashboard", Href: "/admin", Icon: "home"},
			{Label: "Components", Href: "/admin/components", Icon: "components"},
			{Label: "Patterns", Href: "/admin/patterns", Icon: "layout-grid"},
		},
		CurrentUser: func(r *http.Request) *adminkit.User { return a.CurrentUser(r) },
	})
	if err != nil {
		logger.Error("kit setup failed", "err", err)
		os.Exit(1)
	}

	a, err = auth.New(authCfg, auth.NewMemoryStore(), kit)
	if err != nil {
		logger.Error("auth setup failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.SweepSessions(ctx, time.Hour)

	mux := http.NewServeMux()
	kit.Mount(mux)
	a.Mount(mux)

	mux.Handle("GET /admin", a.RequirePage(kit.Page("dashboard.html", dashboardData)))
	mux.Handle("GET /admin/components", a.RequirePage(kit.Page("components.html", nil)))
	mux.Handle("POST /admin/demo", a.RequireAPI(http.HandlerFunc(demo)))
	mux.Handle("GET /admin/patterns", a.RequirePage(kit.Page("patterns.html", patternsData)))
	mux.Handle("POST /admin/demo/settings", a.RequireAPI(http.HandlerFunc(demoSettings)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusFound)
	})

	logger.Info("example panel listening", "addr", addr, "open", a.Open())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}

// key is a stand-in for whatever a real panel lists.
type key struct {
	Name       string
	Prefix     string
	CreatedAt  int64
	LastUsedAt int64
	Tokens     int64
	Spent      float64
	Limit      float64
	Revoked    bool
}

func dashboardData(*http.Request) any {
	now := time.Now()
	ms := func(d time.Duration) int64 { return now.Add(-d).UnixMilli() }
	return map[string]any{
		"Pills": []map[string]string{
			{"label": "Service", "value": "Operational", "state": "ok", "icon": "activity"},
			{"label": "Queue", "value": "2 waiting", "state": "warn", "icon": "clock"},
			{"label": "Upstream", "value": "Unknown", "state": "", "icon": "cloud"},
		},
		"Keys": []key{
			{Name: "backoffice", Prefix: "fnk-a8…6ec2", CreatedAt: ms(72 * time.Hour), LastUsedAt: ms(20 * time.Minute), Tokens: 1_240_000, Spent: 1.2, Limit: 10},
			{Name: "near-cap", Prefix: "fnk-bb…1111", CreatedAt: ms(30 * 24 * time.Hour), LastUsedAt: ms(3 * time.Hour), Tokens: 903_000, Spent: 7.9, Limit: 10},
			{Name: "exhausted", Prefix: "fnk-cc…2222", CreatedAt: ms(90 * 24 * time.Hour), LastUsedAt: ms(26 * time.Hour), Tokens: 512_000, Spent: 12.5, Limit: 12.5},
			{Name: "uncapped", Prefix: "fnk-dd…3333", CreatedAt: ms(5 * time.Hour), Tokens: 4_200, Spent: 0.004},
			{Name: "retired", Prefix: "fnk-ee…4444", CreatedAt: ms(120 * 24 * time.Hour), Tokens: 88_000, Spent: 3.1, Revoked: true},
		},
	}
}

// demo answers the buttons on the components page, so the toast and confirm
// helpers can be exercised end to end.
func demo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.URL.Query().Get("slow") != "" {
		time.Sleep(1500 * time.Millisecond) // long enough to see adminkit.busy hold the button
	}
	if r.URL.Query().Get("fail") != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"the server refused this one, on purpose"}`))
		return
	}
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// member is a stand-in row for the Patterns page's list.
type member struct {
	ID       int
	Name     string
	Email    string
	Role     string
	Active   bool
	LastSeen int64
}

var firstNames = []string{"Ada", "Alan", "Grace", "Linus", "Margaret", "Ken", "Barbara", "Dennis", "Radia", "Edsger", "Frances", "John"}
var lastNames = []string{"Lovelace", "Turing", "Hopper", "Torvalds", "Hamilton", "Thompson", "Liskov", "Ritchie", "Perlman", "Dijkstra", "Allen", "Backus"}

func patternsData(*http.Request) any {
	now := time.Now()
	var members []member
	active := 0
	for i := 0; i < 37; i++ {
		f, l := firstNames[i%len(firstNames)], lastNames[(i*5)%len(lastNames)]
		m := member{
			ID:       i + 1,
			Name:     f + " " + l,
			Email:    strings.ToLower(f+"."+l) + "@example.com",
			Role:     []string{"Admin", "Editor", "Viewer"}[i%3],
			Active:   i%4 != 3,
			LastSeen: now.Add(-time.Duration(i*i*7) * time.Minute).UnixMilli(),
		}
		if m.Active {
			active++
		}
		members = append(members, m)
	}
	return map[string]any{
		"Members":  members,
		"Active":   active,
		"Inactive": len(members) - active,
		"Now":      now.Format("15:04:05"),
		"Since":    now.Add(-90 * time.Second).UnixMilli(),
	}
}

// demoSettings checks the Patterns page's settings form the way a real API
// would, answering field errors as {error, errors: {field: message}}.
func demoSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Port    *int   `json:"port"`
		Webhook string `json:"webhook"`
		Notify  bool   `json:"notify"`
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"not JSON"}`))
		return
	}
	errs := map[string]string{}
	if strings.TrimSpace(body.Name) == "" {
		errs["name"] = "Give the panel a name."
	}
	if body.Port == nil || *body.Port < 1 || *body.Port > 65535 {
		errs["port"] = "A port is a number from 1 to 65535."
	}
	if body.Webhook != "" && !strings.HasPrefix(body.Webhook, "https://") {
		errs["webhook"] = "Use an https:// address."
	}
	if len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("%d fields need a fix", len(errs)), "errors": errs})
		return
	}
	_, _ = w.Write([]byte(`{"ok":true}`))
}
