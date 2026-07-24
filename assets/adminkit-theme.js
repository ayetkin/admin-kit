// Resolves the colour theme before first paint, so a panel never flashes the
// wrong one. Loaded right after <body> for that reason, not deferred.
//
// Precedence: ?theme= in the URL (which is then remembered and stripped),
// otherwise what the operator last chose, otherwise the server-rendered
// Config.Theme already on <html>.
(function () {
  var KEY = 'adminkit-theme'
  var root = document.documentElement
  var valid = { dark: 1, light: 1 }

  var param = null
  try {
    param = new URLSearchParams(window.location.search).get('theme')
  } catch (e) { /* pre-URLSearchParams browser: fall through to storage */ }

  var stored = null
  try {
    if (param && valid[param]) window.localStorage.setItem(KEY, param)
    stored = window.localStorage.getItem(KEY)
  } catch (e) { /* storage blocked (private mode, cookie policy): not fatal */ }

  var theme = (param && valid[param] && param) || (stored && valid[stored] && stored) ||
    root.getAttribute('data-bs-theme') || 'dark'
  root.setAttribute('data-bs-theme', theme)

  // Drop ?theme= once applied: it has served its purpose and would otherwise
  // ride along in every link the operator copies out of the address bar.
  if (param && window.history && window.history.replaceState) {
    try {
      var url = new URL(window.location.href)
      url.searchParams.delete('theme')
      window.history.replaceState(null, '', url.pathname + url.search + url.hash)
    } catch (e) { /* leave the URL as it is */ }
  }
})()
