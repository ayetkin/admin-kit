package adminkit

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/*.html templates/partials/*.html
var kitTemplates embed.FS

// assetsFS carries the vendored Tabler build plus the kit's own thin CSS/JS.
// Embedding rather than linking a CDN keeps a panel working offline and on an
// air-gapped network, and pins exactly what ships with each binary.
//
//go:embed assets
var assetsFS embed.FS

// assetMaxAge is how long a browser may cache an asset. The files are immutable
// for a given binary, but a redeploy replaces them under the same URL, so this
// stays short enough that an upgrade is picked up promptly.
const assetMaxAge = time.Hour

// Mount registers the kit's static assets under Config.AssetPath (default
// "/adminkit"). Call it once per mux, before serving.
func (k *Kit) Mount(mux *http.ServeMux) {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic("adminkit: embedded assets missing: " + err.Error()) // impossible: embed is compile-time
	}
	prefix := k.cfg.AssetPath + "/"
	mux.Handle("GET "+prefix, http.StripPrefix(prefix, newAssetHandler(sub)))
}

// AssetPath returns the URL prefix the kit's assets are served under, for
// projects that need to reference a vendored file directly.
func (k *Kit) AssetPath() string { return k.cfg.AssetPath }

// assetHandler serves the embedded assets, precompressed. Tabler's CSS is over
// half a megabyte and the icon sheet adds another 200 KB; gzip takes the pair
// to roughly a tenth of that. The assets never change within a binary, so each
// one is compressed once, on the first request that wants it, and kept.
type assetHandler struct {
	files   http.Handler
	fsys    fs.FS
	gzipped map[string][]byte
}

func newAssetHandler(fsys fs.FS) http.Handler {
	return &assetHandler{
		files:   http.FileServerFS(fsys),
		fsys:    fsys,
		gzipped: precompress(fsys),
	}
}

func (h *assetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(assetMaxAge.Seconds())))

	name := strings.TrimPrefix(r.URL.Path, "/")
	body, ok := h.gzipped[name]
	if !ok || !acceptsGzip(r) {
		h.files.ServeHTTP(w, r)
		return
	}
	// Vary matters even behind a CDN that only ever sees one client: a shared
	// cache must not hand this body to a client that cannot decode it.
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", contentType(name))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

// precompress gzips every text asset once at startup. Fonts and images are
// already compressed, so they are left alone and served straight from the
// embedded filesystem.
func precompress(fsys fs.FS) map[string][]byte {
	out := map[string][]byte{}
	_ = fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || contentType(name) == "" {
			return nil
		}
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil
		}
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return nil
		}
		if _, err := zw.Write(raw); err != nil {
			return nil
		}
		if err := zw.Close(); err != nil {
			return nil
		}
		// A file that does not shrink is not worth the round trip through gzip.
		if buf.Len() < len(raw) {
			out[name] = buf.Bytes()
		}
		return nil
	})
	return out
}

// contentType returns the type to serve a compressible asset as, or "" when the
// file should not be compressed at all.
func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".map"), strings.HasSuffix(name, ".json"):
		return "application/json"
	default:
		return ""
	}
}

func acceptsGzip(r *http.Request) bool {
	for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(enc), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		// Only an explicit q=0 refuses the encoding. The weight has to be
		// parsed, not matched: "q=0.8" contains "q=0" but means "yes, please".
		return qValue(params) > 0
	}
	return false
}

// qValue reads the weight out of an Accept-Encoding parameter list, defaulting
// to 1 when there is none (or it is malformed, which RFC 9110 says to ignore).
func qValue(params string) float64 {
	for _, p := range strings.Split(params, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(p), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "q") {
			continue
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 1
		}
		return q
	}
	return 1
}
