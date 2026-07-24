# admin-kit

The shared shell for internal admin panels. Every panel gets the same
navigation, theme, sign-in and toasts, so a new one is a config struct and a
page template rather than another round of UI design.

Components come from [Tabler](https://tabler.io) (MIT), vendored into the
binary. A page is ordinary HTML pasted from Tabler's docs - the kit deliberately
does not wrap it in a component library of its own.

```go
kit, err := adminkit.New(adminkit.Config{
    Brand:     "Acme Ops",
    Templates: templatesFS,
    Nav: []adminkit.NavItem{
        {Label: "Dashboard", Href: "/admin", Icon: "home"},
        {Label: "Keys", Href: "/admin/keys", Icon: "key"},
    },
})

kit.Mount(mux)  // serves /adminkit/* (CSS, JS, icon font)
mux.Handle("GET /admin", kit.Page("dashboard.html", func(r *http.Request) any {
    return dashboardData()
}))
```

`templates/dashboard.html` in the project:

```html
{{define "content"}}
  <div class="card">
    <div class="card-header"><h3 class="card-title">Keys</h3></div>
    ...
  </div>
{{end}}
```

That is the whole integration: no build step, no npm, no CDN. Assets ship inside
the binary and are served gzipped.

## Try it

```bash
make run    # http://localhost:8099/admin
```

The example panel under [`example/`](example/) is the living style guide: it
shows every kit component, its template call, and the JavaScript helpers. Check
a change there before adopting it in a real panel.

## Pages

Each file in `Config.Templates` is one page, defining a `content` block, and is
addressed by its file name:

```go
kit.Render(w, r, "dashboard.html", data)              // from a handler
kit.RenderTitled(w, r, "key.html", "Key ab12", data)  // explicit <title>
kit.Page("dashboard.html", dataFunc)                  // as an http.Handler
```

Project values arrive under `.Data`, so `{{.Data.Keys}}` reaches what the
handler returned while the shell reads `.Brand`, `.Nav` and `.User`. Pages may
also define `head` and `scripts` blocks to add their own assets.

Every page is parsed onto its own copy of the layout, so two pages can define
the same block names without colliding. Page names must be unique across the
whole set; `New` fails at startup if they are not.

## Sign-in

The `auth` subpackage adds Google Workspace sign-in. **A panel is closed unless
sign-in is configured**: `auth.New` returns an error naming what is missing,
rather than quietly publishing an admin console. Local development opts out with
`Open`, which logs a warning on every start.

```go
a, err := auth.New(auth.ConfigFromEnv(), auth.NewMemoryStore(), kit)
a.Mount(mux)                                          // login, callback, logout
mux.Handle("GET /admin", a.RequirePage(page))         // signed out -> sign-in page
mux.Handle("POST /admin/keys", a.RequireAPI(create))  // signed out -> 401 JSON
go a.SweepSessions(ctx, time.Hour)
```

Wire `CurrentUser: a.CurrentUser` into `adminkit.Config` to fill the navbar's
user menu.

| Variable | Purpose |
| --- | --- |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | OAuth client (Google Cloud -> Credentials -> Web application) |
| `PUBLIC_HOST` | Bare host, e.g. `admin.example.com`. Drives the callback URL and the Secure cookie flag; a loopback host implies plain HTTP |
| `ALLOWED_EMAIL_DOMAINS` | Comma-separated domains allowed to sign in |
| `ADMIN_OPEN=1` | Development only: no sign-in at all |

Sessions live behind an HttpOnly cookie in whatever `auth.Store` the panel
provides. `NewMemoryStore` is enough to start; implement the interface over the
project's own database to survive restarts and run more than one replica.

## What the kit adds to Tabler

| Piece | Call |
| --- | --- |
| Usage meter (budget fill bar) | `{{template "meter" dict "used" .Spent "limit" .Limit}}` |
| Status pill | `{{template "pill" dict "label" "Session" "state" "ok" "value" "Active"}}` |
| Empty state | `{{template "empty" dict "title" "No keys yet" "icon" "key"}}` |
| Icon | `{{icon "key"}}` (any name from [tabler.io/icons](https://tabler.io/icons)) |
| Formatting | `{{money .Cost}}` `{{num .Tokens}}` `{{pct .Used .Limit}}` `{{date .At}}` `{{ago .At}}` `{{truncate 20 .Name}}` |
| Toasts | `adminkit.toast(msg, 'success'\|'danger'\|'warning'\|'info')` |
| JSON calls | `adminkit.post(url, body, okMessage)` `adminkit.get(url)` `adminkit.del(url, okMessage)` |
| Confirm first | `data-ak-confirm="Revoke this key?"` on any clickable element |

Everything else - cards, tables, modals, forms, badges, dropdowns - is Tabler's,
used exactly as its documentation shows.

## Tabler

Vendored under `assets/vendor/tabler` at the versions pinned in the Makefile.
To adopt a new release:

```bash
make vendor TABLER_CORE=1.5.0 TABLER_ICONS=3.46.0
```

Then run `make run`, check the example panel, and commit the result. Tabler's
own `tabler-theme.js` is not vendored: it treats light as the default and strips
`data-bs-theme` whenever the resolved theme matches it, which would undo the
server-rendered `Config.Theme` on a first visit. `assets/adminkit-theme.js`
replaces it and honours the server value.

## Versioning

The kit is semver-tagged and each panel pins its own version in `go.mod`, so
upgrading one panel never touches another.
