package adminkit

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"html/template"
	"io"
	"strings"
	"sync"
)

// tablerIcons is every Tabler icon as the inner markup of its SVG, keyed by
// name ("<name>-filled" for the filled set): a gzipped JSON map written by
// scripts/vendor-tabler.sh at the same version as the icon font.
//
//go:embed icons/tabler-icons.json.gz
var tablerIcons []byte

// iconPaths unpacks tablerIcons on first use: about 1.7 MB in memory, which a
// panel that never draws an icon does not pay for.
var iconPaths = sync.OnceValue(func() map[string]string {
	m := map[string]string{}
	zr, err := gzip.NewReader(bytes.NewReader(tablerIcons))
	if err != nil {
		return m
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return m
	}
	_ = json.Unmarshal(raw, &m)
	return m
})

// icon renders a Tabler icon by name as inline SVG, e.g. {{icon "key"}} or
// {{icon "star-filled"}}. See tabler.io/icons. Inline SVG stays sharp at any
// size, where the icon font is only crisp at its 24px grid; it takes Tabler's
// .icon sizing (--tblr-icon-size), as in Tabler's own examples. A name Tabler
// does not have falls back to the font, which shows nothing for it either.
func icon(name string) template.HTML {
	name = strings.TrimPrefix(strings.TrimSpace(name), "ti-")
	if name == "" {
		return ""
	}
	body, ok := iconPaths()[name]
	if !ok {
		return template.HTML(`<i class="ti ti-` + template.HTMLEscapeString(name) + `"></i>`)
	}
	paint := `fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"`
	if strings.HasSuffix(name, "-filled") {
		paint = `fill="currentColor"`
	}
	return template.HTML(`<svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" ` +
		paint + ` aria-hidden="true" focusable="false">` + body + `</svg>`)
}
