package adminkit

import (
	"bytes"
	"net/http"
)

// LoginData is what the sign-in page renders from. The auth package fills it,
// but a panel with its own sign-in mechanism can render the same page by
// passing its own values.
type LoginData struct {
	Brand string
	Theme string
	// SignInHref starts the sign-in flow. Empty means sign-in is unconfigured,
	// and the page says so instead of offering a dead button.
	SignInHref string
	// DomainHint names who may sign in, e.g. "second.test". Optional.
	DomainHint string
	// Error is a message to show above the button, e.g. after a rejected sign-in.
	Error string
}

// RenderLogin writes the kit's sign-in page. Brand and Theme default to the
// panel's own when left empty.
func (k *Kit) RenderLogin(w http.ResponseWriter, r *http.Request, d LoginData) {
	if d.Brand == "" {
		d.Brand = k.cfg.Brand
	}
	if d.Theme == "" {
		d.Theme = k.cfg.Theme
	}
	// The sign-in page is one of the kit's own templates, so it renders from the
	// base set: it needs no project page and works before any is defined.
	var buf bytes.Buffer
	if err := k.base.ExecuteTemplate(&buf, "login.html", d); err != nil {
		k.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}
