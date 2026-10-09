package adminkit

import (
	"strings"
	"testing"
)

func TestIconIsInlineSVG(t *testing.T) {
	got := string(icon("trash"))
	if !strings.HasPrefix(got, `<svg `) || !strings.Contains(got, `class="icon"`) ||
		!strings.Contains(got, `stroke="currentColor"`) || !strings.Contains(got, `<path d="M4 7l16 0"/>`) {
		t.Fatalf("icon(trash) = %s", got)
	}
	if strings.Contains(got, "M0 0h24v24H0z") {
		t.Error("the invisible grid path was kept")
	}
	if got := string(icon("ti-trash")); !strings.HasPrefix(got, "<svg ") {
		t.Errorf("a ti- prefix is not accepted: %s", got)
	}
}

func TestIconFilledVariant(t *testing.T) {
	got := string(icon("star-filled"))
	if !strings.Contains(got, `fill="currentColor"`) || strings.Contains(got, `stroke="currentColor"`) {
		t.Fatalf("icon(star-filled) = %s", got)
	}
}

func TestIconUnknownFallsBackToTheFont(t *testing.T) {
	if got := string(icon("no-such-icon")); got != `<i class="ti ti-no-such-icon"></i>` {
		t.Fatalf("unknown icon = %s", got)
	}
	if got := icon(" "); got != "" {
		t.Fatalf("empty name = %q", got)
	}
}

// Every icon the kit's own templates draw must be in the vendored set.
func TestKitIconsAreVendored(t *testing.T) {
	for _, name := range []string{"alert-triangle", "brand-google", "moon", "sun", "logout"} {
		if _, ok := iconPaths()[name]; !ok {
			t.Errorf("icon %q is not vendored", name)
		}
	}
}
