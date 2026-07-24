// adminkit: the handful of behaviours every panel needs, on the global
// `adminkit` object. Tabler (Bootstrap) already provides modals, dropdowns and
// tooltips, so this file only covers what it does not: talking to a JSON API,
// reporting the outcome, and confirming a destructive action.
(function () {
  'use strict'

  var TOASTS = 'adminkit-toasts'

  // toast reports an outcome in the bottom-right corner. kind is
  // 'success' | 'danger' | 'warning' | 'info' (a Bootstrap colour).
  function toast(message, kind) {
    var host = document.getElementById(TOASTS)
    if (!host) return
    kind = kind || 'info'

    var el = document.createElement('div')
    el.className = 'toast align-items-center border-0'
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
    icon.className = 'ti ms-3 me-n2 my-auto text-' + kind
    icon.classList.add(kind === 'success' ? 'ti-check'
      : kind === 'danger' ? 'ti-alert-circle'
        : kind === 'warning' ? 'ti-alert-triangle' : 'ti-info-circle')

    flex.append(icon, body, close)
    el.appendChild(flex)
    host.appendChild(el)

    // Tabler bundles Bootstrap's JS; fall back to a plain timeout if a panel
    // ever loads this file without it.
    if (window.bootstrap && window.bootstrap.Toast) {
      var t = new window.bootstrap.Toast(el, { delay: 4000 })
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

  // confirm wires up any element carrying data-ak-confirm so a destructive
  // action asks first. It is applied on click, capture phase, so the page's own
  // handler never runs when the operator backs out.
  document.addEventListener('click', function (e) {
    var el = e.target.closest('[data-ak-confirm]')
    if (!el) return
    if (!window.confirm(el.getAttribute('data-ak-confirm'))) {
      e.preventDefault()
      e.stopImmediatePropagation()
    }
  }, true)

  // Tooltips are opt-in in Bootstrap; the layout's theme switcher uses them.
  document.addEventListener('DOMContentLoaded', function () {
    if (!window.bootstrap || !window.bootstrap.Tooltip) return
    document.querySelectorAll('[data-bs-toggle="tooltip"]').forEach(function (el) {
      new window.bootstrap.Tooltip(el)
    })
  })

  window.adminkit = {
    toast: toast,
    request: request,
    submit: submit,
    errorMessage: errorMessage,
    get: function (url) { return request('GET', url) },
    post: function (url, body, okMessage) { return submit('POST', url, body, okMessage) },
    del: function (url, okMessage) { return submit('DELETE', url, null, okMessage) },
  }
})()
