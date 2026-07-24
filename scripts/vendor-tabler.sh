#!/usr/bin/env bash
# Vendor the Tabler dist files into assets/vendor/tabler.
#
# The kit ships Tabler with the binary instead of loading it from a CDN, so an
# admin panel keeps working offline and on an air-gapped network. Run this to
# adopt a new Tabler release, then commit the result:
#
#   scripts/vendor-tabler.sh 1.4.0 3.45.0
#
# Only woff2 is vendored (universally supported since ~2016), and the icon CSS
# is rewritten to drop its woff/ttf fallbacks - they would add ~1.3 MB of fonts
# no browser we care about would ever request.
#
# Tabler's own tabler-theme.js is deliberately NOT vendored: it treats light as
# the default and strips data-bs-theme whenever the resolved theme equals it,
# which would undo the server-rendered Config.Theme on every first visit. The
# kit ships assets/adminkit-theme.js instead, which honours the server value.
set -euo pipefail

CORE_VERSION="${1:-1.4.0}"
ICONS_VERSION="${2:-3.45.0}"

cd "$(dirname "$0")/.."
DEST="assets/vendor/tabler"
CDN="https://cdn.jsdelivr.net/npm"

mkdir -p "$DEST/fonts"

fetch() { # url dest
  echo "  $2"
  curl -fsSL --max-time 120 "$1" -o "$2"
}

echo "Tabler core $CORE_VERSION, icons $ICONS_VERSION"
fetch "$CDN/@tabler/core@$CORE_VERSION/dist/css/tabler.min.css" "$DEST/tabler.min.css"
fetch "$CDN/@tabler/core@$CORE_VERSION/dist/js/tabler.min.js"   "$DEST/tabler.min.js"
fetch "$CDN/@tabler/icons-webfont@$ICONS_VERSION/dist/tabler-icons.min.css" "$DEST/tabler-icons.min.css"
fetch "$CDN/@tabler/icons-webfont@$ICONS_VERSION/dist/fonts/tabler-icons.woff2" "$DEST/fonts/tabler-icons.woff2"

# Keep only the woff2 source in @font-face.
python3 - "$DEST/tabler-icons.min.css" <<'PY'
import re, sys
path = sys.argv[1]
css = open(path, encoding="utf-8").read()
new, n = re.subn(
    r'src:url\("\./fonts/tabler-icons\.woff2[^"]*"\) format\("woff2"\)[^;}]*',
    'src:url("./fonts/tabler-icons.woff2") format("woff2")',
    css, count=1)
if n != 1:
    sys.exit("vendor-tabler: could not rewrite the @font-face src block")
open(path, "w", encoding="utf-8").write(new)
PY

# Record what is vendored, so the versions are visible without reading git log.
cat > "$DEST/VERSION" <<EOF
@tabler/core          $CORE_VERSION
@tabler/icons-webfont $ICONS_VERSION

Both MIT licensed. Fetched by scripts/vendor-tabler.sh; do not edit by hand.
EOF

echo
du -ch "$DEST"/* "$DEST"/fonts/* 2>/dev/null | tail -1
echo "vendored into $DEST"
