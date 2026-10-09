// adminkit: the handful of behaviours every panel needs, on the global
// `adminkit` object. Tabler (Bootstrap) already provides modals, dropdowns and
// tooltips, so this file only covers what it does not: talking to a JSON API,
// reporting the outcome, holding a button while its request runs, confirming a
// destructive action, and keeping tooltips working on content added later.
(function () {
  'use strict'

  var TOASTS = 'adminkit-toasts'

  // Tabler's bundle publishes Bootstrap's components on `window.tabler`, not on
  // `window.bootstrap`: its UMD footer reads `t.Modal=…, t.bootstrap=…` with `t`
  // being the `tabler` global. Every Bootstrap and Tabler example, though, says
  // `new bootstrap.Modal(el)` - and pasting those examples verbatim is the whole
  // point of the kit. So resolve whichever namespace is really there and publish
  // it as `bootstrap` when nothing else has.
  var bs = [window.bootstrap, window.tabler && window.tabler.bootstrap, window.tabler]
    .find(function (ns) { return ns && ns.Modal })
  if (bs && !window.bootstrap) window.bootstrap = bs

  // modal returns the Bootstrap modal for an element or element id, creating it
  // on first use. Prefer it over touching the global directly.
  function modal(target) {
    var el = typeof target === 'string' ? document.getElementById(target) : target
    if (!el) throw new Error('adminkit.modal: no such element: ' + target)
    if (!bs) throw new Error('adminkit.modal: Bootstrap is not loaded')
    return bs.Modal.getOrCreateInstance(el)
  }

  // The toast icons, inline so they stay sharp (Tabler's check, alert-circle,
  // alert-triangle and info-circle).
  var TOAST_ICONS = {
    success: '<path d="M5 12l5 5l10 -10"/>',
    danger: '<path d="M3 12a9 9 0 1 0 18 0a9 9 0 0 0 -18 0"/><path d="M12 8v4"/><path d="M12 16h.01"/>',
    warning: '<path d="M12 9v4"/><path d="M10.363 3.591l-8.106 13.534a1.914 1.914 0 0 0 1.636 2.871h16.214a1.914 1.914 0 0 0 1.636 -2.87l-8.106 -13.536a1.914 1.914 0 0 0 -3.274 0z"/><path d="M12 16h.01"/>',
    info: '<path d="M3 12a9 9 0 1 0 9 -9a9 9 0 0 0 -9 9"/><path d="M12 9h.01"/><path d="M11 12h1v4h1"/>'
  }

  // toast reports an outcome in the top-right corner. kind is
  // 'success' | 'danger' | 'warning' | 'info' (a Bootstrap colour).
  function toast(message, kind) {
    var host = document.getElementById(TOASTS)
    if (!host) return
    kind = TOAST_ICONS[kind] ? kind : 'info'

    var el = document.createElement('div')
    el.className = 'toast align-items-center ak-toast ak-toast-' + kind
    el.setAttribute('role', 'alert')
    el.setAttribute('aria-live', 'polite')
    el.setAttribute('aria-atomic', 'true')

    var flex = document.createElement('div')
    flex.className = 'd-flex'
    var body = document.createElement('div')
    body.className = 'toast-body'
    // textContent, never innerHTML: messages carry server text and key names.
    body.textContent = message
    var close = document.createElement('button')
    close.type = 'button'
    close.className = 'btn-close me-2 m-auto'
    close.setAttribute('data-bs-dismiss', 'toast')
    close.setAttribute('aria-label', 'Close')

    var icon = document.createElement('span')
    icon.className = 'ak-toast-icon'
    icon.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" fill="none" ' +
      'stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + TOAST_ICONS[kind] + '</svg>'

    flex.append(icon, body, close)
    el.appendChild(flex)
    host.appendChild(el)

    // Tabler bundles Bootstrap's JS; fall back to a plain timeout if a panel
    // ever loads this file without it.
    if (bs && bs.Toast) {
      var t = new bs.Toast(el, { delay: 4000 })
      el.addEventListener('hidden.bs.toast', function () { el.remove() })
      t.show()
    } else {
      el.classList.add('show')
      setTimeout(function () { el.remove() }, 4000)
    }
    return el
  }

  // request sends JSON and always resolves to {ok, status, data}, so callers
  // handle a rejected request and a failed one the same way. A network error
  // becomes ok:false rather than a rejected promise.
  function request(method, url, body) {
    var opts = { method: method, headers: { Accept: 'application/json' } }
    if (body !== undefined && body !== null) {
      opts.headers['Content-Type'] = 'application/json'
      opts.body = JSON.stringify(body)
    }
    return fetch(url, opts).then(function (res) {
      return res.json().catch(function () { return {} }).then(function (data) {
        return { ok: res.ok, status: res.status, data: data }
      })
    }).catch(function (err) {
      return { ok: false, status: 0, data: { error: String(err && err.message || err) } }
    })
  }

  // submit is request plus the reporting every form does by hand otherwise: a
  // toast either way, and the server's own error message when it sends one.
  // Resolves to the same {ok, status, data}, so the caller can still branch.
  function submit(method, url, body, okMessage) {
    return request(method, url, body).then(function (res) {
      if (res.ok) {
        if (okMessage) toast(okMessage, 'success')
      } else {
        toast(errorMessage(res), 'danger')
      }
      return res
    })
  }

  // errorMessage digs the human-readable part out of an error response, whether
  // the API answers {error}, {detail}, or OpenAI-style {error:{message}}.
  function errorMessage(res) {
    var d = res && res.data
    if (d) {
      if (typeof d.error === 'string' && d.error) return d.error
      if (d.error && typeof d.error.message === 'string' && d.error.message) return d.error.message
      if (typeof d.detail === 'string' && d.detail) return d.detail
    }
    if (res && res.status) return 'Request failed (' + res.status + ')'
    return 'Request failed'
  }

  // A dropdown inside a scrollable table is clipped by it: Tabler's
  // .table-responsive sets overflow-x, so a row menu is cut off at the table's
  // edge - the last row loses most of its menu.
  //
  // The clipping is lifted only while a menu is open, and put back afterwards.
  // Configuring Popper to position against the viewport would be the other way
  // out, but Tabler instantiates every dropdown at load, so there is no moment
  // at which the kit could supply that configuration.
  var OPEN = 'ak-dropdown-open'
  document.addEventListener('show.bs.dropdown', function (e) {
    var scroller = e.target.closest('.table-responsive')
    if (scroller) scroller.classList.add(OPEN)
  })
  document.addEventListener('hidden.bs.dropdown', function (e) {
    var scroller = e.target.closest('.table-responsive')
    if (scroller) scroller.classList.remove(OPEN)
  })

  // busy holds a button while promise runs: disabled, a spinner in front of
  // its label, its width kept so the row does not jump. It is released when
  // promise settles, and promise's outcome is passed on, so it wraps a call:
  //   adminkit.busy(btn, adminkit.post(url, body, 'Saved')).then(...)
  function busy(btn, promise) {
    if (btn && !btn.classList.contains('ak-busy')) {
      btn.dataset.akLabel = btn.innerHTML
      btn.dataset.akWidth = btn.style.minWidth
      btn.style.minWidth = btn.getBoundingClientRect().width + 'px'
      btn.classList.add('ak-busy')
      btn.disabled = true
      btn.setAttribute('aria-busy', 'true')
      btn.insertAdjacentHTML('afterbegin', '<span class="spinner-border spinner-border-sm ak-spinner" role="status"></span>')
    }
    return Promise.resolve(promise).then(function (res) {
      idle(btn)
      return res
    }, function (err) {
      idle(btn)
      throw err
    })
  }

  // idle releases a button busy holds; a no-op on any other button.
  function idle(btn) {
    if (!btn || !btn.classList.contains('ak-busy')) return
    btn.innerHTML = btn.dataset.akLabel
    btn.style.minWidth = btn.dataset.akWidth || ''
    btn.classList.remove('ak-busy')
    btn.disabled = false
    btn.removeAttribute('aria-busy')
  }

  var ALERT_ICON = '<svg xmlns="http://www.w3.org/2000/svg" class="icon icon-lg mb-2" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 9v4"/><path d="M10.363 3.591l-8.106 13.534a1.914 1.914 0 0 0 1.636 2.871h16.214a1.914 1.914 0 0 0 1.636 -2.87l-8.106 -13.536a1.914 1.914 0 0 0 -3.274 0z"/><path d="M12 16h.01"/></svg>'

  // confirm asks message in a dialog and resolves to true when the operator
  // confirms. opts.ok labels the confirm button (default 'Confirm'); opts.tone
  // is 'danger' (the default: red, for what cannot be undone) or 'primary'.
  // Without Bootstrap it falls back to the browser's confirm().
  function confirm(message, opts) {
    opts = opts || {}
    if (!bs || !bs.Modal) return Promise.resolve(window.confirm(message))
    var tone = opts.tone === 'primary' ? 'primary' : 'danger'
    var dlg = document.createElement('div')
    dlg.className = 'modal modal-blur fade ak-confirm'
    dlg.tabIndex = -1
    dlg.setAttribute('role', 'dialog')
    dlg.innerHTML = '<div class="modal-dialog modal-sm modal-dialog-centered"><div class="modal-content">' +
      '<div class="modal-status bg-' + tone + '"></div>' +
      '<div class="modal-body text-center py-4"><div class="text-' + tone + '">' + ALERT_ICON + '</div>' +
      '<h3>Are you sure?</h3><div class="text-secondary ak-confirm-text"></div></div>' +
      '<div class="modal-footer"><div class="w-100"><div class="row g-2">' +
      '<div class="col"><button type="button" class="btn w-100" data-bs-dismiss="modal">Cancel</button></div>' +
      '<div class="col"><button type="button" class="btn btn-' + tone + ' w-100 ak-confirm-ok"></button></div>' +
      '</div></div></div></div></div>'
    // textContent, never innerHTML: the message often carries a name.
    dlg.querySelector('.ak-confirm-text').textContent = message
    var ok = dlg.querySelector('.ak-confirm-ok')
    ok.textContent = opts.ok || 'Confirm'
    document.body.appendChild(dlg)
    var m = new bs.Modal(dlg)
    return new Promise(function (resolve) {
      var yes = false
      ok.addEventListener('click', function () { yes = true; m.hide() })
      dlg.addEventListener('shown.bs.modal', function () { ok.focus() })
      dlg.addEventListener('hidden.bs.modal', function () { m.dispose(); dlg.remove(); resolve(yes) })
      m.show()
    })
  }

  // Any element carrying data-ak-confirm asks first, in the dialog above:
  // data-ak-confirm is the question, data-ak-confirm-ok the button's label (by
  // default the element's own text or tooltip) and data-ak-confirm-tone
  // "primary" for an action that is not destructive. The click is caught on
  // the capture phase, so the page's own handler never runs unless the
  // operator confirms; then the element is clicked again, letting it through.
  var confirmed = null
  document.addEventListener('click', function (e) {
    var el = e.target.closest('[data-ak-confirm]')
    if (!el || el === confirmed) return
    e.preventDefault()
    e.stopImmediatePropagation()
    if (el.disabled) return
    var label = el.getAttribute('data-ak-confirm-ok') || el.textContent.trim() ||
      el.getAttribute('title') || el.getAttribute('data-bs-original-title') || 'Confirm'
    confirm(el.getAttribute('data-ak-confirm'), { ok: label, tone: el.getAttribute('data-ak-confirm-tone') })
      .then(function (yes) {
        if (!yes) return
        confirmed = el
        try { el.click() } finally { confirmed = null }
      })
  }, true)

  // Tooltips are opt-in in Bootstrap; the layout's theme switcher uses them.
  // Each one is created when it is first hovered or focused rather than once
  // at load, so content added later gets its tooltips too, and disposed when
  // its element leaves the page, so a tooltip open on a row that is redrawn
  // does not stay behind on screen.
  function tooltipFor(e) {
    var el = e.target.closest && e.target.closest('[data-bs-toggle="tooltip"]')
    // Created now, it missed the event that should open it: open it once;
    // its own listeners take over from there.
    if (el && bs && bs.Tooltip && !bs.Tooltip.getInstance(el)) bs.Tooltip.getOrCreateInstance(el).show()
  }
  document.addEventListener('mouseover', tooltipFor)
  document.addEventListener('focusin', tooltipFor)
  function disposeTooltips(node) {
    if (node.nodeType !== 1 || !bs || !bs.Tooltip) return
    var list = node.matches('[data-bs-toggle="tooltip"]') ? [node] : []
    node.querySelectorAll('[data-bs-toggle="tooltip"]').forEach(function (el) { list.push(el) })
    list.forEach(function (el) {
      var t = bs.Tooltip.getInstance(el)
      if (t) t.dispose()
    })
  }
  new MutationObserver(function (records) {
    records.forEach(function (r) { r.removedNodes.forEach(disposeTooltips) })
  }).observe(document.documentElement, { childList: true, subtree: true })

  // on handles type events on every element matching selector, now and after
  // the page changes: one listener on document. handler gets the event and
  // the matching element (also as this).
  function on(type, selector, handler) {
    document.addEventListener(type, function (e) {
      var t = e.target && e.target.closest && e.target.closest(selector)
      if (t) handler.call(t, e, t)
    })
  }

  // debounce delays fn until ms have passed without another call, for a
  // search box that filters as the operator types.
  function debounce(fn, ms) {
    var timer
    return function () {
      var self = this, args = arguments
      clearTimeout(timer)
      timer = setTimeout(function () { fn.apply(self, args) }, ms)
    }
  }

  // ---- lists: filter, page, remember the page size ----

  function pagerEl(p) { return typeof p === 'string' ? document.getElementById(p) : p }

  // pager fills a pager (the "pager" partial, by element or id): "1-50 of
  // 120", "1 / 3" and the previous / next buttons. opts: {from, size, total,
  // page, empty}, from counting from 0; empty is the text for no rows.
  function pager(p, opts) {
    var el = pagerEl(p)
    if (!el) return
    var pages = Math.max(1, Math.ceil(opts.total / opts.size))
    var part = function (name) { return el.querySelector('[data-ak-pager-' + name + ']') }
    part('info').textContent = opts.total
      ? (opts.from + 1) + '–' + Math.min(opts.from + opts.size, opts.total) + ' of ' + opts.total
      : (opts.empty || 'No matches')
    part('page').textContent = opts.page + ' / ' + pages
    part('prev').disabled = opts.page <= 1
    part('next').disabled = opts.page >= pages
  }

  // serverPager fills a pager from a paged API answer {total, page, page_size}.
  function serverPager(p, d, empty) {
    pager(p, { from: (d.page - 1) * d.page_size, size: d.page_size, total: d.total, page: d.page, empty: empty })
  }

  // filterRows shows the rows that contain every word of query in their
  // data-search attribute (their text when they have none) and pass
  // opts.keep, one page at a time, and fills opts.pager. opts: {keep(row),
  // page, size, pager}. Returns the page actually shown (clamped) and how
  // many rows matched, for the caller to keep:
  //   state.page = adminkit.filterRows(rows, q.value, {page: state.page, size: 50, pager: 'keys-pager'}).page
  function filterRows(rows, query, opts) {
    opts = opts || {}
    var size = opts.size || 50
    var words = String(query || '').toLowerCase().split(/\s+/).filter(Boolean)
    var matched = []
    Array.prototype.forEach.call(rows, function (row) {
      var hay = (row.dataset.search || row.textContent).toLowerCase()
      if (words.every(function (w) { return hay.indexOf(w) >= 0 }) && (!opts.keep || opts.keep(row))) matched.push(row)
      else row.classList.add('d-none')
    })
    var pages = Math.max(1, Math.ceil(matched.length / size))
    var page = Math.min(Math.max(opts.page || 1, 1), pages)
    var from = (page - 1) * size
    matched.forEach(function (row, i) { row.classList.toggle('d-none', i < from || i >= from + size) })
    if (opts.pager) pager(opts.pager, { from: from, size: size, total: matched.length, page: page })
    return { page: page, matched: matched.length }
  }

  // pageSize reads a list's page size, a per-browser preference kept under
  // key; pageSize.save stores a new one.
  function pageSize(key, fallback) {
    try { return Number(localStorage.getItem(key)) || fallback } catch (e) { return fallback }
  }
  pageSize.save = function (key, size) {
    try { localStorage.setItem(key, String(size)) } catch (e) {}
  }

  // ---- copy ----

  // copy puts text on the clipboard and says so in a toast. The Clipboard
  // API needs a secure context and may still be refused (a browser policy,
  // an embedded view); a hidden textarea and execCommand is the fallback.
  function copy(text) {
    var report = function (ok) { toast(ok ? 'Copied to the clipboard' : 'Could not copy', ok ? 'success' : 'danger') }
    var legacy = function () {
      var ta = document.createElement('textarea')
      ta.value = text
      ta.setAttribute('readonly', '')
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      var ok = false
      try { ok = document.execCommand('copy') } catch (e) {}
      ta.remove()
      return ok
    }
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text).then(function () { report(true) }, function () { report(legacy()) })
    }
    report(legacy())
    return Promise.resolve()
  }

  // data-ak-copy="text" on a button copies that text; data-ak-copy-target=
  // "#selector" copies the value (or the text) of that element instead.
  on('click', '[data-ak-copy], [data-ak-copy-target]', function (e, el) {
    e.preventDefault()
    var sel = el.getAttribute('data-ak-copy-target')
    var src = sel && document.querySelector(sel)
    copy(src ? ('value' in src ? src.value : src.textContent) : el.getAttribute('data-ak-copy'))
  })

  // ---- forms ----

  // formData reads a form into a plain object for a JSON API: a number field
  // gives a number (null when empty), a lone checkbox true or false, several
  // checkboxes of one name an array of the checked values, a multiple select
  // an array, everything else its string. Disabled fields and buttons are
  // left out, as a browser would.
  function formData(form) {
    var out = {}, counts = {}
    Array.prototype.forEach.call(form.elements, function (f) {
      if (f.name && f.type === 'checkbox') counts[f.name] = (counts[f.name] || 0) + 1
    })
    Array.prototype.forEach.call(form.elements, function (f) {
      if (!f.name || f.disabled || /^(submit|button|reset|file)$/.test(f.type)) return
      if (f.type === 'checkbox') {
        if (counts[f.name] > 1) {
          out[f.name] = out[f.name] || []
          if (f.checked) out[f.name].push(f.value)
        } else {
          out[f.name] = f.checked
        }
      } else if (f.type === 'radio') {
        if (f.checked) out[f.name] = f.value
        else if (!(f.name in out)) out[f.name] = null
      } else if (f.type === 'number' || f.type === 'range') {
        out[f.name] = f.value === '' ? null : Number(f.value)
      } else if (f.tagName === 'SELECT' && f.multiple) {
        out[f.name] = Array.prototype.filter.call(f.options, function (o) { return o.selected })
          .map(function (o) { return o.value })
      } else {
        out[f.name] = f.value
      }
    })
    return out
  }

  // clearErrors removes what fieldErrors put on a form.
  function clearErrors(form) {
    form.querySelectorAll('.is-invalid').forEach(function (f) { f.classList.remove('is-invalid') })
    form.querySelectorAll('.invalid-feedback.ak-feedback').forEach(function (d) { d.remove() })
  }

  // fieldErrors marks each named field the server rejected, {name: message},
  // with Tabler's invalid style and the message under it, and focuses the
  // first. It returns how many fields it marked.
  function fieldErrors(form, errors) {
    clearErrors(form)
    var first = null, n = 0
    Object.keys(errors || {}).forEach(function (name) {
      var f = form.elements[name]
      if (!f) return
      if (f.length && !f.tagName) f = f[0] // radios or checkboxes: mark the first
      f.classList.add('is-invalid')
      var msg = document.createElement('div')
      msg.className = 'invalid-feedback ak-feedback'
      msg.textContent = errors[name]
      var anchor = f.closest('.input-group, .form-check') || f
      anchor.insertAdjacentElement('afterend', msg)
      if (anchor !== f) msg.style.display = 'block'
      first = first || f
      n++
    })
    if (first) first.focus()
    return n
  }

  // send submits a form as JSON to its action (method from data-ak-method,
  // else the form's own, POST by default), holding its submit button busy.
  // Field errors the server returns as {errors: {name: message}} are shown
  // on their fields; any other failure, and the success message
  // opts.ok (data-ak-ok), as a toast. Resolves to {ok, status, data}.
  function send(form, opts) {
    opts = opts || {}
    var method = (opts.method || form.getAttribute('data-ak-method') || form.getAttribute('method') || 'POST').toUpperCase()
    var url = opts.url || form.getAttribute('action') || location.pathname
    var body = opts.body || formData(form)
    var btn = form.querySelector('[type="submit"]') ||
      (form.id && document.querySelector('[type="submit"][form="' + form.id + '"]'))
    clearErrors(form)
    var req = request(method, url, body).then(function (res) {
      var errs = res.data && (res.data.errors || res.data.fields)
      if (!res.ok && errs && typeof errs === 'object' && fieldErrors(form, errs)) {
        toast(errorMessage(res) === 'Request failed (' + res.status + ')' ? 'Check the highlighted fields' : errorMessage(res), 'danger')
      } else if (!res.ok) {
        toast(errorMessage(res), 'danger')
      } else if (opts.ok || form.getAttribute('data-ak-ok')) {
        toast(opts.ok || form.getAttribute('data-ak-ok'), 'success')
      }
      return res
    })
    return btn ? busy(btn, req) : req
  }

  // A form carrying data-ak-form is sent by send() on submit instead of the
  // browser. On success it fires 'ak:sent' on the form (event.detail is the
  // response) and, with data-ak-refresh, refreshes the page's live parts.
  document.addEventListener('submit', function (e) {
    var form = e.target
    if (!form.matches || !form.matches('form[data-ak-form]')) return
    e.preventDefault()
    send(form).then(function (res) {
      if (!res.ok) return
      form.dispatchEvent(new CustomEvent('ak:sent', { bubbles: true, detail: res }))
      if (form.hasAttribute('data-ak-refresh')) refresh()
    })
  })

  // ---- live refresh ----

  // refresh brings the page up to date without reloading it: the page is
  // fetched again and every element marked data-ak-live is swapped for its
  // new version, matched by id. What the operator typed or chose in a
  // swapped part's form controls (those with an id) carries over, and so
  // does focus. A <script data-ak-data> in the new page (window.page = ...,
  // the data the page's own script reads) is run before the swap; nothing
  // else is. Listeners on document hear 'ak:beforerefresh' just before the
  // swap and 'ak:refresh' after it. When the page cannot be fetched (signed
  // out, server gone) it reloads instead, and the toasts on screen are shown
  // again on the reloaded page. Calls while one runs fold into one more run.
  // opts: {delay: ms to wait first; background: true leaves open dialogs
  // open, for a refresh the operator did not ask for}
  var refreshing = null, refreshAgain = false
  function refresh(opts) {
    opts = opts || {}
    if (!opts.background) {
      document.querySelectorAll('.modal.show').forEach(function (m) { modal(m).hide() })
    }
    return new Promise(function (resolve) { setTimeout(resolve, opts.delay || 0) }).then(refreshRun)
  }
  function refreshRun() {
    if (refreshing) {
      refreshAgain = true
      return refreshing
    }
    refreshing = fetch(location.pathname + location.search, {
      headers: { Accept: 'text/html' }, credentials: 'same-origin', cache: 'no-store'
    }).then(function (res) {
      // A redirect means the page is gone for this session (signed out).
      if (!res.ok || res.redirected) throw new Error('page ' + res.status)
      return res.text()
    }).then(function (html) {
      swapLive(new DOMParser().parseFromString(html, 'text/html'))
    }).catch(function () {
      carryToasts()
      location.reload()
    }).then(function () {
      refreshing = null
      if (refreshAgain) {
        refreshAgain = false
        return refreshRun()
      }
    })
    return refreshing
  }
  function swapLive(doc) {
    doc.querySelectorAll('script[data-ak-data]').forEach(function (s) { new Function(s.textContent)() })
    var pairs = []
    document.querySelectorAll('[data-ak-live]').forEach(function (old) {
      var fresh = old.id && doc.getElementById(old.id)
      if (fresh) pairs.push([old, fresh])
    })
    document.dispatchEvent(new CustomEvent('ak:beforerefresh'))
    var focused = document.activeElement
    pairs.forEach(function (p) {
      var old = p[0], node = document.importNode(p[1], true)
      if (!old.parentNode) return // inside a part already swapped
      if (old.closest('.modal.show')) return // a dialog opened meanwhile: leave it be
      old.querySelectorAll('input[id], select[id], textarea[id]').forEach(function (o) {
        var n = node.querySelector('#' + CSS.escape(o.id))
        if (!n || n.tagName !== o.tagName) return
        if (o.type === 'radio' || o.type === 'checkbox') n.checked = o.checked
        else if (o.type !== 'hidden' && o.type !== 'password' && o.type !== 'file') n.value = o.value
      })
      var refocus = focused && focused.id && old.contains(focused) && node.querySelector('#' + CSS.escape(focused.id))
      old.replaceWith(node)
      if (refocus) {
        refocus.focus()
        if (typeof focused.selectionStart === 'number' && typeof refocus.setSelectionRange === 'function') {
          try { refocus.setSelectionRange(focused.selectionStart, focused.selectionEnd) } catch (e) {}
        }
      }
    })
    document.dispatchEvent(new CustomEvent('ak:refresh'))
  }

  var CARRY = 'adminkit-toasts'
  function carryToasts() {
    var list = []
    document.querySelectorAll('#' + TOASTS + ' .ak-toast').forEach(function (t) {
      var body = t.querySelector('.toast-body')
      var kind = (t.className.match(/ak-toast-(success|danger|warning|info)/) || [])[1]
      if (body && body.textContent) list.push({ message: body.textContent, kind: kind || 'info' })
    })
    try {
      if (list.length) sessionStorage.setItem(CARRY, JSON.stringify({ at: Date.now(), list: list }))
    } catch (e) {}
  }
  document.addEventListener('DOMContentLoaded', function () {
    var saved = null
    try {
      saved = JSON.parse(sessionStorage.getItem(CARRY) || 'null')
      sessionStorage.removeItem(CARRY)
    } catch (e) {}
    if (!saved || Date.now() - saved.at > 10000) return
    saved.list.forEach(function (t) { toast(t.message, t.kind) })
  })

  // ---- times that stay current ----

  // {{agoLive .At}} writes a <time data-ak-ago="ms">: its text follows the
  // clock, in the words the server's `ago` uses. Past a month it is a date,
  // which does not change, so it is left alone.
  function agoText(ms) {
    var d = Date.now() - ms
    if (d < 60000) return 'just now'
    if (d < 3600000) return Math.floor(d / 60000) + 'm ago'
    if (d < 86400000) return Math.floor(d / 3600000) + 'h ago'
    if (d < 30 * 86400000) return Math.floor(d / 86400000) + 'd ago'
    return null
  }
  function updateAgo() {
    document.querySelectorAll('[data-ak-ago]').forEach(function (el) {
      var t = agoText(Number(el.getAttribute('data-ak-ago')))
      if (t && el.textContent !== t) el.textContent = t
    })
  }
  setInterval(updateAgo, 30000)

  window.adminkit = {
    toast: toast,
    modal: modal,
    bootstrap: bs,
    request: request,
    submit: submit,
    errorMessage: errorMessage,
    confirm: confirm,
    busy: busy,
    idle: idle,
    on: on,
    debounce: debounce,
    filterRows: filterRows,
    pager: pager,
    serverPager: serverPager,
    pageSize: pageSize,
    copy: copy,
    formData: formData,
    fieldErrors: fieldErrors,
    clearErrors: clearErrors,
    send: send,
    refresh: refresh,
    get: function (url) { return request('GET', url) },
    post: function (url, body, okMessage) { return submit('POST', url, body, okMessage) },
    del: function (url, okMessage) { return submit('DELETE', url, null, okMessage) },
  }
})()
