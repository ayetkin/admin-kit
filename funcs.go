package adminkit

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"
)

// builtinFuncs are the formatting helpers every panel ends up needing. A panel
// can add its own (or shadow these) through Config.Funcs.
//
// Timestamps are unix milliseconds throughout: that is what a JSON API hands a
// template, and it survives the round trip through JavaScript unchanged.
func builtinFuncs(cfg Config) template.FuncMap {
	return template.FuncMap{
		"asset": func(name string) string {
			return cfg.AssetPath + "/" + strings.TrimPrefix(name, "/")
		},
		"date":       formatDate,
		"datetime":   formatDateTime,
		"ago":        formatAgo,
		"money":      formatMoney,
		"num":        formatNum,
		"pct":        formatPct,
		"pctOf":      pctOf,
		"meterWidth": meterWidth,
		"initials":   initials,
		"icon":       icon,
		"truncate":   truncate,
		// dict builds a map inline, so a partial can be called with several
		// values: {{template "empty" dict "title" "No keys" "icon" "key"}}
		"dict": dict,
		// list builds a slice inline, mostly to range over literals.
		"list": func(items ...any) []any { return items },
	}
}

// formatDate renders a unix-millis timestamp as a date, or "-" for zero.
func formatDate(ms int64) string {
	if ms == 0 {
		return "-"
	}
	return time.UnixMilli(ms).Format("2006-01-02")
}

// formatDateTime renders a unix-millis timestamp to the minute, or "-".
func formatDateTime(ms int64) string {
	if ms == 0 {
		return "-"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04")
}

// formatAgo renders a unix-millis timestamp as a coarse age ("3d ago"). It is
// deliberately imprecise: a table wants a glance, not a stopwatch.
func formatAgo(ms int64) string {
	if ms == 0 {
		return "never"
	}
	d := time.Since(time.UnixMilli(ms))
	if d < 0 {
		return "just now"
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return formatDate(ms)
	}
}

// formatMoney renders a USD amount, keeping enough decimals that a fraction of
// a cent does not read as zero.
func formatMoney(value any) string {
	v := toFloat(value)
	switch {
	case v == 0:
		return "$0"
	case v < 0.01:
		return fmt.Sprintf("$%.4f", v)
	case v < 1:
		return fmt.Sprintf("$%.3f", v)
	default:
		return fmt.Sprintf("$%.2f", v)
	}
}

// formatNum abbreviates a count: 950 -> "950", 12300 -> "12.3K", 4.5e6 -> "4.5M".
func formatNum(value any) string {
	v := toFloat(value)
	abs := math.Abs(v)
	switch {
	case abs < 1000:
		return fmt.Sprintf("%.0f", v)
	case abs < 1e6:
		return trimZero(fmt.Sprintf("%.1f", v/1e3)) + "K"
	case abs < 1e9:
		return trimZero(fmt.Sprintf("%.1f", v/1e6)) + "M"
	default:
		return trimZero(fmt.Sprintf("%.1f", v/1e9)) + "B"
	}
}

// formatPct renders part/whole as a whole-number percentage, clamped to 100 and
// safe when whole is zero.
func formatPct(part, whole any) string {
	p := pctOf(part, whole)
	if p < 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", p)
}

// pctOf returns what share of whole part occupies, from 0 to 100, or -1 when
// whole is absent or non-positive. That sentinel lets a template ask "is there
// anything to show?" without comparing the caller's value directly, which would
// fail whenever it is an int and the literal is a float.
func pctOf(part, whole any) float64 {
	w := toFloat(whole)
	if w <= 0 {
		return -1
	}
	return math.Min(100, math.Max(0, toFloat(part)/w*100))
}

// meterWidth is the drawn width for a percentage: a sliver stays visible so
// "barely used" never looks identical to "untouched".
func meterWidth(pct float64) float64 {
	if pct <= 0 {
		return 0
	}
	return math.Max(2, pct)
}

// toFloat converts any numeric a template might hold. Anything else is 0, which
// renders as an empty meter rather than a template error mid-page.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	default:
		return 0
	}
}

func trimZero(s string) string { return strings.TrimSuffix(s, ".0") }

// initials reduces a name to at most two letters for an avatar placeholder.
func initials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "?"
	}
	out := string([]rune(fields[0])[:1])
	if len(fields) > 1 {
		out += string([]rune(fields[len(fields)-1])[:1])
	}
	return strings.ToUpper(out)
}

// icon renders a Tabler icon by name, e.g. {{icon "key"}}. See tabler.io/icons.
func icon(name string) template.HTML {
	name = strings.TrimPrefix(strings.TrimSpace(name), "ti-")
	if name == "" {
		return ""
	}
	return template.HTML(`<i class="ti ti-` + template.HTMLEscapeString(name) + `"></i>`)
}

// truncate shortens s to at most n runes, appending an ellipsis. It cuts on a
// rune boundary so a multi-byte character is never split in half.
func truncate(n int, s string) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// dict pairs its arguments into a map, for passing several values to a partial.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments (%d)", len(pairs))
	}
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %d is %T, want string", i, pairs[i])
		}
		out[key] = pairs[i+1]
	}
	return out, nil
}
