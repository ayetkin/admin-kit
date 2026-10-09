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
| Icon | `{{icon "key"}}`, `{{icon "star-filled"}}` (any name from [tabler.io/icons](https://tabler.io/icons)), drawn as inline SVG: sharp at every size, sized by Tabler's `.icon` (`--tblr-icon-size`) |
| Formatting | `{{money .Cost}}` `{{num .Tokens}}` `{{pct .Used .Limit}}` `{{date .At}}` `{{ago .At}}` `{{truncate 20 .Name}}` |
| Live age | `{{agoLive .AtMs}}` (Unix milliseconds) renders like `ago` and keeps counting in the browser, with the full date as its tooltip |
| Stat tile | `{{template "stat" dict "label" "Members" "value" .Count "unit" "total" "sub" "3 new" "icon" "users" "tone" "good"}}`; `tone` is `good`, `warn` or `bad`; with `id` the figure (and `id-sub` line) can be updated from script |
| Name with logo | `{{template "ident" dict "name" .Name "sub" .Email "logo" .LogoURL}}`, the logo falls back to initials |
| Pager | `{{template "pager" dict "id" "members-pager"}}`, filled by `adminkit.pager` |
| Row actions | `.ak-actions` holding `.ak-act` icon buttons (`.has-label`, `.is-danger`, `.is-current`) |
| List toolbar | `.ak-toolbar` with an `.ak-filter` search box |
| Settings form | `.ak-setting-group` of `.ak-srow` rows (`.is-narrow`, `.is-switch`) and an `.ak-savebar` |
| Two-pane dialog | `.ak-split` with an `.ak-side` list beside an `.ak-pane`, split into `.ak-section`s |
| Copy field | `.ak-copyfield` wraps an input and a `data-ak-copy-target="#id"` button; `data-ak-copy="text"` copies a fixed value; `adminkit.copy(text)` from script |
| Toasts | `adminkit.toast(msg, 'success'\|'danger'\|'warning'\|'info')`, tinted by kind with an icon badge so they stand out from the page |
| Filter and page a list | `adminkit.filterRows(rows, query, {page, size, pager})` matches `data-search` (or the row's text) and returns `{page, matched}`; `adminkit.pager(id, {from, size, total, page})`, `adminkit.serverPager(id, {total, page, page_size})` for server-side paging; `adminkit.pageSize(key, fallback)` / `.save(key, n)` remember the choice |
| Forms | `<form data-ak-form action=... data-ak-ok="Saved">` posts as JSON, marks fields from a 422 `{errors: {field: msg}}`, then fires `ak:sent` (`data-ak-method`, `data-ak-refresh` to refresh after). From script: `adminkit.send(form, {method, url, body, ok})`, `adminkit.formData(form)`, `adminkit.fieldErrors(form, errors)`, `adminkit.clearErrors(form)` |
| Refresh in place | `adminkit.refresh({delay, background})` swaps every `[data-ak-live]` element (by id) and reruns `script[data-ak-data]`, keeping form input and focus; fires `ak:beforerefresh` and `ak:refresh`. Falls back to a reload that keeps pending toasts |
| Event helpers | `adminkit.on(type, selector, handler)` delegates from the document; `adminkit.debounce(fn, ms)` |
| JSON calls | `adminkit.post(url, body, okMessage)` `adminkit.get(url)` `adminkit.del(url, okMessage)` |
| Modals | `adminkit.modal('dlgId').show()` / `.hide()` |
| Confirm first | `data-ak-confirm="Revoke this key?"` on any clickable element asks in a dialog; `data-ak-confirm-ok="Revoke"` labels its button, `data-ak-confirm-tone="primary"` for an action that is not destructive. From script: `adminkit.confirm(message, {ok, tone})` resolves to true or false |
| Busy button | `adminkit.busy(btn, promise)` disables the button with a spinner until the promise settles; `adminkit.idle(btn)` releases it early |
| Tooltips | `data-bs-toggle="tooltip"` works on content added after load, and a tooltip goes away with its element |
| Theme fixes | Ghost buttons tint on hover in dark mode instead of filling; green, yellow and orange text are darker in light mode, so they read on white |

Note on the Bootstrap global: Tabler's bundle publishes its components on
`window.tabler`, not `window.bootstrap`, so `new bootstrap.Modal(el)` copied
from the docs would throw. The kit resolves whichever namespace is present and
publishes it as `bootstrap`, so documentation snippets work verbatim.
`adminkit.modal()` is the shorter way to the same thing.

The example panel's Patterns page puts these together: stat tiles, a filtered
and paged list with row actions, a settings form and a two-pane dialog.

Asset links carry a content hash (`?v=...`), so a browser picks up new kit CSS
and JavaScript as soon as a panel ships a new version.

Everything else - cards, tables, modals, forms, badges, dropdowns - is Tabler's,
used exactly as its documentation shows.

## Tabler

Vendored under `assets/tabler` at the versions pinned in the Makefile.
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
