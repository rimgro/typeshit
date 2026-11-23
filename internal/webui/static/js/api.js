/* Shared client helpers: HTTP, toasts, DOM, storage, formatting.
   Vanilla ES2019+, no modules (works from file://-served static dirs). */
(function () {
  "use strict";

  var API = "/api/v1";

  /* ---------- HTTP ---------- */

  async function parse(res) {
    var ct = res.headers.get("content-type") || "";
    var body = null;
    if (ct.indexOf("application/json") >= 0) {
      body = await res.json().catch(function () { return null; });
    } else {
      body = await res.text().catch(function () { return ""; });
    }
    if (!res.ok) {
      var msg = "Ошибка " + res.status;
      if (body && typeof body === "object" && body.error) msg = body.error;
      else if (typeof body === "string" && body.trim()) msg = body.trim().slice(0, 300);
      var err = new Error(msg);
      err.status = res.status;
      err.code = (body && body.code) || "";
      throw err;
    }
    return body;
  }

  function qs(params) {
    if (!params) return "";
    var p = new URLSearchParams();
    Object.keys(params).forEach(function (k) {
      var v = params[k];
      if (v === undefined || v === null || v === "") return;
      p.set(k, String(v));
    });
    var s = p.toString();
    return s ? "?" + s : "";
  }

  async function get(path, params) {
    return parse(await fetch(API + path + qs(params), { headers: { accept: "application/json" } }));
  }

  async function post(path, body, opts) {
    opts = opts || {};
    var res = await fetch(API + path + qs(opts.query), {
      method: "POST",
      headers: Object.assign({ "content-type": "application/json", accept: "application/json" }, opts.headers || {}),
      body: JSON.stringify(body === undefined ? {} : body),
      signal: opts.signal,
    });
    return parse(res);
  }

  async function del(path) {
    return parse(await fetch(API + path, { method: "DELETE", headers: { accept: "application/json" } }));
  }

  async function upload(path, file, fields) {
    var fd = new FormData();
    fd.append("file", file, file.name || "sample.png");
    Object.keys(fields || {}).forEach(function (k) {
      var v = fields[k];
      if (v === undefined || v === null) return;
      fd.append(k, typeof v === "string" ? v : JSON.stringify(v));
    });
    var res = await fetch(API + path, { method: "POST", body: fd, headers: { accept: "application/json" } });
    return parse(res);
  }

  function url(path) { return API + path; }

  /* ---------- response normalisation ----------
     The API is written in parallel with this UI, so accept a few
     equivalent envelope shapes instead of assuming one. */

  function unwrap(body, key) {
    if (body && typeof body === "object" && body[key] !== undefined) return body[key];
    if (body && typeof body === "object" && body.profile !== undefined && key === "profile") return body.profile;
    return body;
  }

  function asList(body, key) {
    if (Array.isArray(body)) return body;
    if (body && Array.isArray(body[key])) return body[key];
    if (body && Array.isArray(body.items)) return body.items;
    return [];
  }

  function pageURL(item, id) {
    if (!item) return "";
    var s = typeof item === "string" ? item : (item.url || item.path || item.href || "");
    if (!s) return "";
    if (/^https?:\/\//.test(s)) return s;
    if (s.charAt(0) === "/") return s;
    if (s.indexOf("api/v1/") === 0) return "/" + s;
    if (id) return API + "/results/" + encodeURIComponent(id) + "/" + s.replace(/^\/+/, "");
    return s;
  }

  function normalizeResult(body) {
    body = body || {};
    var r = body.result || body;
    var id = r.id || r.resultId || r.result_id || "";
    var rawPages = r.pages || r.files || [];
    var pages = rawPages.map(function (p) { return pageURL(p, id); }).filter(Boolean);
    if (!id && pages.length) {
      var m = /\/results\/([^/]+)\//.exec(pages[0]);
      if (m) id = decodeURIComponent(m[1]);
    }
    return {
      id: id,
      pages: pages,
      width: r.width || 0,
      height: r.height || 0,
      millis: r.millis || r.ms || 0,
      jobId: r.jobId || r.jobID || body.jobId || "",
    };
  }

  /* ---------- DOM ---------- */

  function el(tag, props) {
    var node = document.createElement(tag);
    if (props) {
      Object.keys(props).forEach(function (k) {
        var v = props[k];
        if (v === null || v === undefined) return;
        if (k === "class") node.className = v;
        else if (k === "text") node.textContent = v;
        else if (k === "html") node.innerHTML = v;
        else if (k === "dataset") Object.assign(node.dataset, v);
        else if (k.slice(0, 2) === "on" && typeof v === "function") node.addEventListener(k.slice(2), v);
        else if (k === "attrs") Object.keys(v).forEach(function (a) { node.setAttribute(a, v[a]); });
        else node.setAttribute(k, v);
      });
    }
    for (var i = 2; i < arguments.length; i++) {
      var child = arguments[i];
      if (child === null || child === undefined || child === false) continue;
      if (Array.isArray(child)) child.forEach(function (c) { if (c) node.appendChild(c.nodeType ? c : document.createTextNode(String(c))); });
      else node.appendChild(child.nodeType ? child : document.createTextNode(String(child)));
    }
    return node;
  }

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }

  /* ---------- toasts ---------- */

  function toastRoot() {
    var root = document.querySelector(".toasts");
    if (!root) {
      root = el("div", { class: "toasts", attrs: { "aria-live": "polite", role: "status" } });
      document.body.appendChild(root);
    }
    return root;
  }

  function toast(message, kind, ms) {
    if (!message) return;
    var node = el("div", { class: "toast " + (kind || "") },
      el("span", { class: "dot" }),
      el("div", { text: message }),
      el("button", { type: "button", text: "×", attrs: { "aria-label": "Закрыть" } }));
    var close = function () {
      node.style.opacity = "0";
      node.style.transition = "opacity .15s";
      setTimeout(function () { node.remove(); }, 160);
    };
    node.querySelector("button").addEventListener("click", close);
    toastRoot().appendChild(node);
    setTimeout(close, ms || (kind === "err" ? 9000 : 4200));
  }

  /* ---------- storage ---------- */

  function load(key, fallback) {
    try {
      var raw = localStorage.getItem("typeshit:" + key);
      if (!raw) return fallback;
      var val = JSON.parse(raw);
      return val === null || val === undefined ? fallback : val;
    } catch (e) { return fallback; }
  }

  function save(key, value) {
    try { localStorage.setItem("typeshit:" + key, JSON.stringify(value)); } catch (e) { /* quota */ }
  }

  function remember(key, value, ms) {
    try { sessionStorage.setItem("typeshit:" + key, JSON.stringify({ v: value, t: Date.now() + (ms || 6e5) })); } catch (e) { /* ignore */ }
  }

  function recall(key) {
    try {
      var raw = sessionStorage.getItem("typeshit:" + key);
      if (!raw) return null;
      var obj = JSON.parse(raw);
      if (!obj || obj.t < Date.now()) return null;
      return obj.v;
    } catch (e) { return null; }
  }

  /* ---------- formatting ---------- */

  function fmtMs(ms) {
    if (!ms) return "";
    if (ms < 1000) return Math.round(ms) + " мс";
    return (ms / 1000).toFixed(ms < 10000 ? 1 : 0).replace(".", ",") + " с";
  }

  function fmtNum(n) {
    return new Intl.NumberFormat("ru-RU").format(n || 0);
  }

  function plural(n, one, few, many) {
    var m10 = n % 10, m100 = n % 100;
    if (m10 === 1 && m100 !== 11) return one;
    if (m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20)) return few;
    return many;
  }

  function debounce(fn, wait) {
    var t = 0;
    var wrapped = function () {
      var args = arguments, self = this;
      clearTimeout(t);
      t = setTimeout(function () { t = 0; fn.apply(self, args); }, wait);
    };
    wrapped.cancel = function () { clearTimeout(t); t = 0; };
    wrapped.flush = function () { if (t) { clearTimeout(t); t = 0; fn(); } };
    return wrapped;
  }

  function sleep(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }

  function randomSeed() {
    var v = Math.floor(Math.random() * 900000000) + 1;
    return v;
  }

  /* ---------- icons ---------- */

  var ICONS = {
    download: '<path d="M12 3v12m0 0 4-4m-4 4-4-4M4 19h16"/>',
    upload: '<path d="M12 21V9m0 0 4 4m-4-4-4 4M4 5h16"/>',
    dice: '<rect x="4" y="4" width="16" height="16" rx="3"/><circle cx="9" cy="9" r="1.2" fill="currentColor"/><circle cx="15" cy="15" r="1.2" fill="currentColor"/><circle cx="15" cy="9" r="1.2" fill="currentColor"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    minus: '<path d="M5 12h14"/>',
    trash: '<path d="M4 7h16M10 7V5h4v2m-7 0 1 13h8l1-13"/>',
    print: '<path d="M7 8V3h10v5M7 18H4V9h16v9h-3M7 14h10v7H7z"/>',
    zoomIn: '<circle cx="11" cy="11" r="7"/><path d="m16 16 4 4M8 11h6M11 8v6"/>',
    zoomOut: '<circle cx="11" cy="11" r="7"/><path d="m16 16 4 4M8 11h6"/>',
    left: '<path d="m14 6-6 6 6 6"/>',
    right: '<path d="m10 6 6 6-6 6"/>',
    close: '<path d="m6 6 12 12M18 6 6 18"/>',
    check: '<path d="m5 13 4 4L19 7"/>',
    redo: '<path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7"/>',
    undo: '<path d="M4 11a8 8 0 1 1 2.3 5.7M4 4v7h7"/>',
    wand: '<path d="M15 6l3 3M5 19 17 7M9 3v3M6 6h3M4 11h3"/>',
    eye: '<path d="M2 12s3.6-6 10-6 10 6 10 6-3.6 6-10 6S2 12 2 12z"/><circle cx="12" cy="12" r="2.6"/>',
    pen: '<path d="M4 20l4-1 10-10-3-3L5 16l-1 4z"/><path d="m14 6 3 3"/>',
    file: '<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v6h4"/>',
    alert: '<path d="M12 4 2.5 20h19L12 4z"/><path d="M12 10v4.5M12 17.4v.2"/>',
    refresh: '<path d="M20 12a8 8 0 1 1-2.3-5.7M20 4v5h-5"/>',
    move: '<path d="M12 3v18M3 12h18m-3-3 3 3-3 3M9 6l3-3 3 3M6 9l-3 3 3 3m12-3-3 3"/>',
    grid: '<rect x="3.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.5"/>',
    help: '<circle cx="12" cy="12" r="8.5"/><path d="M9.6 9.4a2.5 2.5 0 1 1 3.5 2.4c-.7.35-1.1.9-1.1 1.7v.6M12 17.2v.2"/>'
  };

  function icon(name, size) {
    var s = size || 16;
    return '<svg viewBox="0 0 24 24" width="' + s + '" height="' + s + '" fill="none" stroke="currentColor" ' +
      'stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + (ICONS[name] || "") + "</svg>";
  }

  /* ---------- misc ---------- */

  function copy(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).then(function () { toast("Скопировано", "ok", 1800); },
        function () { toast("Не удалось скопировать", "err"); });
    }
    toast("Копирование недоступно", "err");
    return Promise.resolve();
  }

  function download(url, filename) {
    var a = el("a", { href: url, download: filename || "", class: "hidden" });
    document.body.appendChild(a);
    a.click();
    setTimeout(function () { a.remove(); }, 1000);
  }

  function setBusy(button, busy, label) {
    if (!button) return;
    if (busy) {
      if (!button.dataset.label) {
        button.dataset.label = button.textContent.trim();
        button.dataset.html = button.innerHTML;
      }
      button.disabled = true;
      button.innerHTML = '<span class="spinner"></span>' + (label ? " " + label : "");
    } else {
      button.disabled = false;
      if (button.dataset.html) button.innerHTML = button.dataset.html;
      else button.innerHTML = button.dataset.label || label || "Готово";
    }
  }

  function errorText(err) {
    if (!err) return "Неизвестная ошибка";
    if (err.status === 0 || err.name === "TypeError") return "Сервер недоступен";
    return err.message || String(err);
  }

  /* ---------- icons in markup ---------- */

  function applyIcons(root) {
    var nodes = (root || document).querySelectorAll("[data-icon]");
    Array.prototype.forEach.call(nodes, function (node) {
      if (node.dataset.iconDone) return;
      node.dataset.iconDone = "1";
      node.insertAdjacentHTML("afterbegin", icon(node.dataset.icon, node.dataset.iconSize ? Number(node.dataset.iconSize) : 16));
    });
  }

  window.App = {
    API: API,
    get: get, post: post, del: del, upload: upload, url: url, qs: qs,
    unwrap: unwrap, asList: asList, normalizeResult: normalizeResult, pageURL: pageURL,
    el: el, $: $, $$: $$,
    toast: toast, load: load, save: save, remember: remember, recall: recall,
    fmtMs: fmtMs, fmtNum: fmtNum, plural: plural, debounce: debounce, sleep: sleep,
    randomSeed: randomSeed, icon: icon, copy: copy, download: download,
    setBusy: setBusy, errorText: errorText, applyIcons: applyIcons,
  };
})();
