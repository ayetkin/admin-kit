package adminkit

import (
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// The go command drops any directory named "vendor" when it packages a module,
// so an asset filed under one is embedded here and missing for every project
// that imports the kit - a 404 no other test in this repo would see, because
// they all build from the source tree. Keep vendored files somewhere else.
func TestNoAssetLivesUnderAVendorDirectory(t *testing.T) {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	err = fs.WalkDir(sub, ".", func(name string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		for _, part := range strings.Split(name, "/") {
			if part == "vendor" {
				t.Errorf("%s is under a vendor directory, so it will not ship in the module zip", name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk assets: %v", err)
	}
}

func TestMountServesVendoredAssets(t *testing.T) {
	k := newTestKit(t, Config{})
	mux := http.NewServeMux()
	k.Mount(mux)

	for _, name := range []string{
		"/adminkit/tabler/tabler.min.css",
		"/adminkit/tabler/tabler.min.js",
		"/adminkit/tabler/tabler-icons.min.css",
		"/adminkit/tabler/fonts/tabler-icons.woff2",
		"/adminkit/adminkit.css",
		"/adminkit/adminkit.js",
		"/adminkit/adminkit-theme.js",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, name, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", name, rec.Code)
			continue
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s: empty body", name)
		}
	}
}

func TestAssetsAreServedGzippedWhenAccepted(t *testing.T) {
	k := newTestKit(t, Config{})
	mux := http.NewServeMux()
	k.Mount(mux)
	const name = "/adminkit/tabler/tabler.min.css"

	plain := httptest.NewRecorder()
	mux.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, name, nil))
	if enc := plain.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding without Accept-Encoding = %q, want none", enc)
	}

	req := httptest.NewRequest(http.MethodGet, name, nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	zipped := httptest.NewRecorder()
	mux.ServeHTTP(zipped, req)

	if got := zipped.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := zipped.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", got)
	}
	if !strings.HasPrefix(zipped.Header().Get("Content-Type"), "text/css") {
		t.Errorf("Content-Type = %q, want text/css", zipped.Header().Get("Content-Type"))
	}
	if zipped.Body.Len() >= plain.Body.Len() {
		t.Errorf("gzipped body (%d) is not smaller than plain (%d)", zipped.Body.Len(), plain.Body.Len())
	}

	// The compressed bytes must decode back to exactly what the plain request
	// served, or a browser would render a truncated stylesheet.
	zr, err := gzip.NewReader(zipped.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if string(got) != plain.Body.String() {
		t.Errorf("decompressed body differs from the plain one (%d vs %d bytes)", len(got), plain.Body.Len())
	}
}

// A font is already compressed; gzipping it again would only cost CPU.
func TestAlreadyCompressedAssetsAreNotGzipped(t *testing.T) {
	k := newTestKit(t, Config{})
	mux := http.NewServeMux()
	k.Mount(mux)

	req := httptest.NewRequest(http.MethodGet, "/adminkit/tabler/fonts/tabler-icons.woff2", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding for woff2 = %q, want none", enc)
	}
}

func TestAcceptsGzip(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"gzip, deflate, br", true},
		{"br;q=1.0, gzip;q=0.8", true},
		{"deflate", false},
		{"gzip;q=0", false}, // explicitly refused
		{"identity", false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		if tc.header != "" {
			r.Header.Set("Accept-Encoding", tc.header)
		}
		if got := acceptsGzip(r); got != tc.want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

func TestAssetPathIsConfigurable(t *testing.T) {
	k := newTestKit(t, Config{AssetPath: "/static/kit/"}) // trailing slash trimmed
	if got := k.AssetPath(); got != "/static/kit" {
		t.Fatalf("AssetPath() = %q, want /static/kit", got)
	}
	mux := http.NewServeMux()
	k.Mount(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/kit/adminkit.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// Pages must reference the same prefix, or the panel loads no styling.
	body := get(t, k, "/admin", "dashboard.html", map[string]string{})
	if !strings.Contains(body, "/static/kit/tabler/tabler.min.css") {
		t.Error("layout did not use the configured asset path")
	}
}

func TestPrecompressSkipsIncompressibleFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"a.css":   &fstest.MapFile{Data: []byte(strings.Repeat("a{color:red}", 100))},
		"b.woff2": &fstest.MapFile{Data: []byte("not really a font")},
	}
	got := precompress(fsys)
	if _, ok := got["a.css"]; !ok {
		t.Error("a.css should have been compressed")
	}
	if _, ok := got["b.woff2"]; ok {
		t.Error("b.woff2 should have been left alone")
	}
}
