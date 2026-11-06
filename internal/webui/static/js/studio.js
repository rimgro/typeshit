/* Handwriting studio: template -> photo -> markup -> profile. */
(function () {
  "use strict";

  var A = window.App;
  var el = A.el, $ = A.$, $$ = A.$$;

  /* ---------- constants ---------- */

  var CHARSETS = {
    "ru": "абвгдеёжзийклмнопрстуфхцчшщъыьэюяАБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ0123456789.,;:!?—–-()«»\"'/\\*+=№%",
    "ru-short": "абвгдеёжзийклмнопрстуфхцчшщъыьэюяАБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ0123456789",
    "latin": "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.,;:!?-()\"'/*+="
  };

  var MAX_UPLOAD = 20 * 1024 * 1024;

  /* ---------- state ---------- */

  var S = {
    lang: A.load("studio-lang", "ru"),
    reps: Number(A.load("studio-reps", 2)) || 2,
    templates: [],
    specs: [],
    sheetPage: 1,
    doc: null,
    sheets: [],
    active: -1,
    img: null,
    captureId: "",
    cells: [],
    rows: [],
    nextId: 1,
    selected: -1,
    view: { scale: 1, x: 0, y: 0 },
    canvas: null,
    ctx: null,
    cw: 0,
    ch: 0,
    dpr: 1,
    history: [],
    future: [],
    charset: CHARSETS["ru"],
    charCursor: {},
    dirty: false,
    raf: 0
  };

  var COLORS = {
    cell: "rgba(27, 42, 74, .55)",
    cellFill: "rgba(27, 42, 74, .04)",
    selected: "#1b6fd6",
    selectedFill: "rgba(27, 111, 214, .14)",
    labelBg: "rgba(255, 255, 255, .9)",
    label: "#12305e",
    row: "rgba(190, 54, 44, .55)",
    handle: "#1b6fd6",
    newCell: "rgba(27, 111, 214, .8)"
  };

  /* ---------- helpers ---------- */

  function firstRune(str) {
    var chars = Array.from(str || "");
    return chars.length ? chars[0] : "";
  }

  function clamp(v, lo, hi) { return v < lo ? lo : v > hi ? hi : v; }

  function cellByID(id) {
    for (var i = 0; i < S.cells.length; i++) if (S.cells[i].id === id) return i;
    return -1;
  }

  function selectedCell() { return S.selected >= 0 ? S.cells[S.selected] : null; }

  function markerKey() { return "markup:" + (S.doc ? S.doc.id : "none"); }

  /* ---------- steps ---------- */

  function unlock(stepId) {
    var node = $("#" + stepId);
    if (node) node.classList.remove("locked");
  }

  function markDone(stepId) {
    var node = $("#" + stepId);
    if (node) { node.classList.remove("locked"); node.classList.add("done"); }
  }

  /* ---------- templates ---------- */

  function tplURL(item, i) {
    var raw = (item && (item.url || item.image || item.path)) || "";
    var page = item && item.page !== undefined && item.page !== null ? item.page : i + 1;
    if (raw && /^https?:|^\//.test(raw)) return raw;
    if (raw && /\//.test(raw)) return "/" + raw.replace(/^\/+/, "");
    return A.url("/template/" + encodeURIComponent(S.lang) + "/" + S.reps + "/" + page + ".png");
  }

  function templateImage(src, alt) {
    var im = el("img", { src: src, alt: alt, loading: "lazy" });
    var tried = false;
    im.addEventListener("error", function () {
      if (tried) return;
      tried = true;
      // The API may number template pages from 0 or from 1; try both.
      var m = /\/template\/([^/]+)\/(\d+)\/(\d+)\.png$/.exec(src);
      if (!m) return;
      var alt2 = A.url("/template/" + m[1] + "/" + m[2] + "/" + (Number(m[3]) === 0 ? 1 : 0) + ".png");
      if (alt2 !== src) im.src = alt2;
    });
    return im;
  }

  async function loadTemplates() {
    var btn = $("#btn-template");
    A.setBusy(btn, true, "Готовлю…");
    var grid = $("#template-grid");
    grid.innerHTML = "";
    grid.appendChild(el("div", { class: "skeleton", style: "aspect-ratio:210/297" }));
    try {
      var res = await A.get("/template", { lang: S.lang, reps: S.reps });
      var list = A.asList(res, "templates");
      if (!list.length) throw new Error("сервер вернул пустой список шаблонов");
      S.templates = list.map(function (item, i) {
        var spec = (item && item.spec) || item || {};
        return { spec: spec, url: tplURL(item, i) };
      });
      S.specs = S.templates.map(function (t) { return t.spec; });
      S.sheetPage = 1;

      grid.innerHTML = "";
      if (S.templates.length > 1) {
        var pick = el("div", { class: "seg", style: "grid-column:1/-1; justify-self:start; margin-bottom:4px" });
        S.templates.forEach(function (t, i) {
          var b = el("button", { type: "button", text: "Лист " + (i + 1), attrs: { "aria-pressed": i === 0 ? "true" : "false" } });
          b.addEventListener("click", function () {
            S.sheetPage = i + 1;
            $$("button", pick).forEach(function (x) { x.setAttribute("aria-pressed", x === b ? "true" : "false"); });
          });
          pick.appendChild(b);
        });
        grid.appendChild(pick);
      }
      S.templates.forEach(function (t, i) {
        var fig = el("figure", {},
          templateImage(t.url, "Шаблон, лист " + (i + 1)),
          el("figcaption", { text: "Лист " + (i + 1) + (t.spec && t.spec.chars ? " · " + t.spec.chars.length + " клеток" : "") }));
        grid.appendChild(fig);
      });

      var chars = unionChars();
      if (chars.length) S.charset = chars.join("");
      renderReport();
      $("#btn-print").classList.remove("hidden");
      $("#template-hint").textContent =
        "Готово: " + S.templates.length + " " + A.plural(S.templates.length, "лист", "листа", "листов") +
        ". Распечатайте, заполните клетки и переходите к шагу 2.";
      unlock("step2");
      $("#btn-print").onclick = printTemplates;
    } catch (err) {
      grid.innerHTML = "";
      A.toast("Шаблон не получен: " + A.errorText(err), "err");
      $("#template-hint").textContent = "Не удалось получить шаблон. Сервер отвечает? " + A.errorText(err);
    } finally {
      A.setBusy(btn, false, "Получить шаблон");
    }
  }

  function unionChars() {
    var seen = {}, out = [];
    S.specs.forEach(function (spec) {
      (spec && spec.chars ? spec.chars : []).forEach(function (c) {
        if (!c || seen[c]) return;
        seen[c] = 1;
        out.push(c);
      });
    });
    return out;
  }

  async function printTemplates() {
    var area = $("#print-area");
    area.innerHTML = "";
    var loads = S.templates.map(function (t) {
      var img = el("img", { src: t.url, alt: "Шаблон" });
      area.appendChild(img);
      return new Promise(function (resolve) {
        if (img.complete) return resolve();
        img.addEventListener("load", resolve, { once: true });
        img.addEventListener("error", resolve, { once: true });
      });
    });
    await Promise.all(loads);
    document.body.classList.add("printing");
    setTimeout(function () { window.print(); }, 150);
  }

  window.addEventListener("afterprint", function () {
    document.body.classList.remove("printing");
  });

  /* ---------- upload ---------- */

  function bindUpload() {
    var zone = $("#dropzone");
    var input = $("#file-input");

    zone.addEventListener("click", function () { input.click(); });
    zone.addEventListener("keydown", function (e) {
      if (e.key === "Enter" || e.key === " ") { e.preventDefault(); input.click(); }
    });
    input.addEventListener("change", function () {
      if (input.files && input.files.length) uploadMany(input.files);
      input.value = "";
    });
    ["dragenter", "dragover"].forEach(function (ev) {
      zone.addEventListener(ev, function (e) { e.preventDefault(); zone.classList.add("over"); });
    });
    ["dragleave", "drop"].forEach(function (ev) {
      zone.addEventListener(ev, function (e) { e.preventDefault(); zone.classList.remove("over"); });
    });
    zone.addEventListener("drop", function (e) {
      var files = e.dataTransfer && e.dataTransfer.files;
      if (files && files.length) uploadMany(files);
    });
  }

  async function uploadMany(fileList) {
    var files = Array.prototype.slice.call(fileList);
    if (files.length > 1) {
      // A batch of photos is a fresh capture session: replace whatever is on
      // the canvas instead of stacking it behind the old sheets.
      S.sheets = [];
      S.active = -1;
      S.doc = null;
      S.img = null;
      renderSheetTabs();
    }
    for (var i = 0; i < files.length; i++) {
      // The n-th chosen photo belongs to the n-th template page, so every
      // sheet gets its own cell labels.
      await upload(files[i], i === files.length - 1, i);
    }
  }

  async function upload(file, isLast, sheetIndex) {
    if (!/^image\//.test(file.type) && !/\.(png|jpe?g|webp|heic|bmp)$/i.test(file.name || "")) {
      A.toast("Нужен файл изображения", "warn");
      return;
    }
    if (file.size > MAX_UPLOAD) {
      A.toast("Файл больше 20 МБ — сожмите фото", "err");
      return;
    }
    var status = $("#upload-status");
    status.textContent = "Анализирую лист: выравниваю, ищу клетки…";

    var spec = { lang: S.lang, reps: S.reps, page: 0, pages: 0, chars: [] };
    var sheet = S.specs[S.sheetPage - 1];
    if (S.specs.length > 1 && typeof sheetIndex === "number" && S.specs[sheetIndex]) {
      sheet = S.specs[sheetIndex];
    }
    if (sheet) spec = Object.assign({}, sheet);

    try {
      var res = await A.upload("/capture", file, { spec: JSON.stringify(spec) });
      var doc = A.unwrap(res, "doc");
      if (!doc || !doc.cells) throw new Error("сервер не вернул разметку");
      setDoc(doc);
      status.textContent = "Найдено ячеек: " + doc.cells.length +
        (doc.notes && doc.notes.length ? " · " + doc.notes.join("; ") : "") +
        (S.sheets.length > 1 ? " · листов: " + S.sheets.length : "");
      markDone("step2");
      unlock("step3");
      unlock("step4");
      if (isLast !== false) $("#step3").scrollIntoView({ behavior: "smooth", block: "start" });
    } catch (err) {
      status.textContent = "";
      A.toast("Не удалось разобрать лист: " + A.errorText(err), "err");
    }
  }

  // normalize a server CaptureDoc into the local sheet shape.
  function sheetFromDoc(doc) {
    var cells = (doc.cells || []).map(function (c, i) {
      return {
        id: c.id !== undefined && c.id !== null ? c.id : i + 1,
        rect: (c.rect || [0, 0, 50, 50]).slice(0, 4).map(Number),
        label: c.label || "",
        row: c.row || 0,
        col: c.col || 0,
        score: typeof c.score === "number" ? c.score : 1,
        manual: !!c.manual
      };
    });
    var rows = (doc.rows || []).map(function (r) { return Object.assign({}, r); });
    var saved = A.load("markup:" + doc.id, null);
    var restored = false;
    if (saved && saved.cells && saved.cells.length) {
      cells = saved.cells;
      rows = saved.rows || rows;
      restored = true;
    }
    return {
      id: doc.id,
      image: doc.image && doc.image.charAt(0) === "/" ? doc.image
        : A.url("/capture/" + encodeURIComponent(doc.id) + "/image"),
      cells: cells,
      rows: rows,
      nextId: cells.reduce(function (m, c) { return Math.max(m, c.id + 1); }, 1),
      history: [],
      future: [],
      dirty: false,
      restored: restored
    };
  }

  // stash copies the live editing state back into the active sheet.
  function stash() {
    var sh = S.sheets[S.active];
    if (!sh) return;
    sh.cells = S.cells;
    sh.rows = S.rows;
    sh.nextId = S.nextId;
    sh.history = S.history;
    sh.future = S.future;
    sh.dirty = S.dirty;
  }

  // activate switches the canvas to one of the captured sheets.
  function activate(index) {
    stash();
    var sh = S.sheets[index];
    if (!sh) return;
    S.active = index;
    S.doc = { id: sh.id, image: sh.image };
    S.captureId = sh.id;
    S.cells = sh.cells;
    S.rows = sh.rows;
    S.nextId = sh.nextId;
    S.history = sh.history || [];
    S.future = sh.future || [];
    S.dirty = !!sh.dirty;
    S.selected = -1;
    renderSheetTabs();
    updateHistoryButtons();

    var img = new Image();
    img.onload = function () {
      if (S.sheets[S.active] !== sh) return;
      S.img = img;
      resizeCanvas(true);
      draw();
      updateSide();
      $("#markup-state").textContent = "разметка";
      $("#markup-state").className = "badge ok";
    };
    img.onerror = function () { A.toast("Не удалось открыть скан", "err"); };
    img.src = sh.image;
  }

  function setDoc(doc) {
    var sheet = sheetFromDoc(doc);
    S.sheets.push(sheet);
    if (sheet.restored) A.toast("Восстановлена разметка из браузера", "ok");
    activate(S.sheets.length - 1);
  }

  function removeSheet(index) {
    if (S.sheets.length <= 1) {
      A.toast("Нельзя убрать единственный лист", "warn");
      return;
    }
    stash();
    S.sheets.splice(index, 1);
    if (S.active >= S.sheets.length) S.active = S.sheets.length - 1;
    else if (index < S.active) S.active--;
    A.save("markup:" + (S.doc && S.doc.id), null);
    activate(S.active);
    updateSide();
  }

  function refreshSheetTabs() { renderSheetTabs(); }

  function renderSheetTabs() {
    var host = $("#sheet-tabs");
    if (!host) return;
    host.innerHTML = "";
    if (S.sheets.length < 2) {
      host.hidden = true;
      return;
    }
    host.hidden = false;
    S.sheets.forEach(function (sh, i) {
      var cells = i === S.active ? S.cells : sh.cells;
      var labelled = cells.filter(function (c) { return c.label; }).length;
      var dirty = i === S.active ? S.dirty : sh.dirty;
      var tab = el("button", {
        type: "button",
        class: "sheet-tab",
        "data-dirty": dirty ? "1" : "0",
        title: labelled + " размечено из " + cells.length + " · двойной клик убирает лист",
        "aria-selected": i === S.active ? "true" : "false"
      },
        el("span", { class: "dot" }),
        el("span", { text: "Лист " + (i + 1) }),
        el("i", { text: labelled + "/" + cells.length })
      );
      tab.addEventListener("click", function () { activate(i); });
      tab.addEventListener("dblclick", function (e) { e.preventDefault(); removeSheet(i); });
      host.appendChild(tab);
    });
  }

  /* ---------- canvas ---------- */

  function resizeCanvas(fit) {
    var wrap = $("#canvas-wrap");
    if (!wrap) return;
    var rect = wrap.getBoundingClientRect();
    S.dpr = Math.min(window.devicePixelRatio || 1, 2);
    S.cw = Math.max(100, rect.width);
    S.ch = Math.max(100, rect.height);
    S.canvas.width = Math.round(S.cw * S.dpr);
    S.canvas.height = Math.round(S.ch * S.dpr);
    S.canvas.style.width = S.cw + "px";
    S.canvas.style.height = S.ch + "px";
    if (fit) fitView();
    draw();
  }

  function fitView() {
    if (!S.img) return;
    var pad = 24;
    var s = Math.min((S.cw - pad * 2) / S.img.width, (S.ch - pad * 2) / S.img.height);
    s = clamp(s, 0.05, 4);
    S.view.scale = s;
    S.view.x = (S.cw - S.img.width * s) / 2;
    S.view.y = (S.ch - S.img.height * s) / 2;
    updateZoomLabel();
  }

  function updateZoomLabel() {
    var node = $("#zoom-label");
    if (node) node.textContent = Math.round(S.view.scale * 100) + "%";
  }

  function scheduleDraw() {
    if (S.raf) return;
    S.raf = requestAnimationFrame(function () { S.raf = 0; draw(); });
  }

  function draw() {
    var ctx = S.ctx;
    if (!ctx) return;
    var v = S.view, s = v.scale;
    ctx.setTransform(S.dpr, 0, 0, S.dpr, 0, 0);
    ctx.clearRect(0, 0, S.cw, S.ch);
    if (!S.img) {
      ctx.fillStyle = "rgba(120,120,120,.5)";
      ctx.font = "14px system-ui, sans-serif";
      ctx.textAlign = "center";
      ctx.fillText("Загрузите фото листа", S.cw / 2, S.ch / 2);
      return;
    }

    ctx.imageSmoothingEnabled = true;
    ctx.drawImage(S.img, v.x, v.y, S.img.width * s, S.img.height * s);

    // rows / baselines
    ctx.save();
    ctx.strokeStyle = COLORS.row;
    ctx.lineWidth = 1;
    ctx.setLineDash([6, 5]);
    S.rows.forEach(function (row) {
      var y = v.y + row.baseline * s;
      ctx.beginPath();
      ctx.moveTo(v.x, y);
      ctx.lineTo(v.x + S.img.width * s, y);
      ctx.stroke();
    });
    ctx.restore();

    // cells
    S.cells.forEach(function (cell, idx) {
      var x = v.x + cell.rect[0] * s;
      var y = v.y + cell.rect[1] * s;
      var w = (cell.rect[2] - cell.rect[0]) * s;
      var h = (cell.rect[3] - cell.rect[1]) * s;
      if (w < 2 || h < 2) return;
      var isSel = idx === S.selected;
      ctx.fillStyle = isSel ? COLORS.selectedFill : COLORS.cellFill;
      ctx.fillRect(x, y, w, h);
      ctx.strokeStyle = isSel ? COLORS.selected : COLORS.cell;
      ctx.lineWidth = isSel ? 2 : 1;
      ctx.strokeRect(x + 0.5, y + 0.5, w - 1, h - 1);

      if (cell.label) {
        var fs = clamp(11 + s * 8, 12, 22);
        ctx.font = "600 " + fs.toFixed(0) + "px system-ui, sans-serif";
        var tw = ctx.measureText(cell.label).width;
        var bw = tw + 10, bh = fs + 6;
        var bx = x + 3, by = y + 3;
        ctx.fillStyle = COLORS.labelBg;
        roundRect(ctx, bx, by, bw, bh, 5);
        ctx.fill();
        ctx.fillStyle = COLORS.label;
        ctx.textAlign = "left";
        ctx.textBaseline = "middle";
        ctx.fillText(cell.label, bx + 5, by + bh / 2 + 0.5);
      }

      if (isSel) {
        var hs = 7;
        [[x, y], [x + w, y], [x, y + h], [x + w, y + h]].forEach(function (p) {
          ctx.fillStyle = COLORS.handle;
          ctx.fillRect(p[0] - hs / 2, p[1] - hs / 2, hs, hs);
        });
      }
    });

    if (drag && drag.mode === "new" && drag.rect) {
      var r = drag.rect;
      ctx.strokeStyle = COLORS.newCell;
      ctx.lineWidth = 2;
      ctx.setLineDash([5, 4]);
      ctx.strokeRect(v.x + Math.min(r[0], r[2]) * s, v.y + Math.min(r[1], r[3]) * s,
        Math.abs(r[2] - r[0]) * s, Math.abs(r[3] - r[1]) * s);
      ctx.setLineDash([]);
    }
  }

  function roundRect(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
  }

  /* ---------- pointer interaction ---------- */

  var drag = null;
  var pointers = new Map();
  var pinch = null;
  var spaceDown = false;

  function localPoint(e) {
    var r = S.canvas.getBoundingClientRect();
    return { x: e.clientX - r.left, y: e.clientY - r.top };
  }

  function toImage(p) {
    return { x: (p.x - S.view.x) / S.view.scale, y: (p.y - S.view.y) / S.view.scale };
  }

  function hitHandle(cell, p) {
    if (!cell) return -1;
    var v = S.view;
    var x = v.x + cell.rect[0] * v.scale, y = v.y + cell.rect[1] * v.scale;
    var w = (cell.rect[2] - cell.rect[0]) * v.scale, h = (cell.rect[3] - cell.rect[1]) * v.scale;
    var pts = [[x, y], [x + w, y], [x, y + h], [x + w, y + h]];
    for (var i = 0; i < 4; i++) {
      if (Math.abs(p.x - pts[i][0]) <= 9 && Math.abs(p.y - pts[i][1]) <= 9) return i;
    }
    return -1;
  }

  function hitCell(p) {
    var ip = toImage(p);
    for (var i = S.cells.length - 1; i >= 0; i--) {
      var r = S.cells[i].rect;
      if (ip.x >= r[0] && ip.x <= r[2] && ip.y >= r[1] && ip.y <= r[3]) return i;
    }
    return -1;
  }

  function bindCanvas() {
    var canvas = $("#canvas");
    S.canvas = canvas;
    S.ctx = canvas.getContext("2d");

    canvas.addEventListener("pointerdown", function (e) {
      try { canvas.setPointerCapture(e.pointerId); } catch (err) { /* synthetic pointer */ }
      pointers.set(e.pointerId, localPoint(e));
      if (pointers.size === 2) {
        var pts = Array.from(pointers.values());
        pinch = { dist: Math.hypot(pts[0].x - pts[1].x, pts[0].y - pts[1].y), scale: S.view.scale };
        drag = null;
        return;
      }
      var p = localPoint(e);
      if (e.button === 1 || spaceDown || e.button === 2) {
        drag = { mode: "pan", start: p, vx: S.view.x, vy: S.view.y };
        return;
      }
      if (e.shiftKey) {
        var ip = toImage(p);
        drag = { mode: "new", start: ip, rect: [ip.x, ip.y, ip.x, ip.y] };
        return;
      }
      var handle = hitHandle(selectedCell(), p);
      if (handle >= 0) {
        drag = { mode: "resize", handle: handle, index: S.selected, rect: selectedCell().rect.slice() };
        return;
      }
      var idx = hitCell(p);
      if (idx >= 0) {
        S.selected = idx;
        drag = { mode: "move", index: idx, rect: S.cells[idx].rect.slice(), start: toImage(p) };
        updateSide();
        scheduleDraw();
        return;
      }
      S.selected = -1;
      updateSide();
      drag = { mode: "pan", start: p, vx: S.view.x, vy: S.view.y };
      scheduleDraw();
    });

    canvas.addEventListener("pointermove", function (e) {
      if (pointers.has(e.pointerId)) pointers.set(e.pointerId, localPoint(e));
      if (pinch && pointers.size === 2) {
        var pts = Array.from(pointers.values());
        var dist = Math.hypot(pts[0].x - pts[1].x, pts[0].y - pts[1].y);
        if (dist > 1) {
          var mid = { x: (pts[0].x + pts[1].x) / 2, y: (pts[0].y + pts[1].y) / 2 };
          zoomAt(mid, (pinch.scale * dist) / pinch.dist);
        }
        return;
      }
      if (!drag) return;
      var p = localPoint(e);
      if (drag.mode === "pan") {
        S.view.x = drag.vx + (p.x - drag.start.x);
        S.view.y = drag.vy + (p.y - drag.start.y);
      } else if (drag.mode === "move") {
        var ip = toImage(p);
        var dx = ip.x - drag.start.x, dy = ip.y - drag.start.y;
        var r = drag.rect;
        S.cells[drag.index].rect = [r[0] + dx, r[1] + dy, r[2] + dx, r[3] + dy];
        S.dirty = true;
      } else if (drag.mode === "resize") {
        var rp = toImage(p);
        var rr = drag.rect.slice();
        if (drag.handle === 0) { rr[0] = rp.x; rr[1] = rp.y; }
        if (drag.handle === 1) { rr[2] = rp.x; rr[1] = rp.y; }
        if (drag.handle === 2) { rr[0] = rp.x; rr[3] = rp.y; }
        if (drag.handle === 3) { rr[2] = rp.x; rr[3] = rp.y; }
        S.cells[drag.index].rect = normalizeRect(rr);
        S.dirty = true;
      } else if (drag.mode === "new") {
        var np = toImage(p);
        drag.rect = [drag.start.x, drag.start.y, np.x, np.y];
      }
      scheduleDraw();
    });

    function endPointer(e) {
      pointers.delete(e.pointerId);
      if (pointers.size < 2) pinch = null;
      if (!drag) return;
      if (drag.mode === "new" && drag.rect) {
        var r = normalizeRect(drag.rect);
        if (r[2] - r[0] > 8 && r[3] - r[1] > 8) {
          pushHistory();
          addCell(r, "");
        }
      } else if (drag.mode === "move" || drag.mode === "resize") {
        var before = JSON.stringify(drag.rect);
        var now = JSON.stringify(S.cells[drag.index].rect);
        if (before !== now) {
          // undo must restore the pre-drag rect, so swap the snapshot back in.
          var saved = S.cells[drag.index].rect;
          S.cells[drag.index].rect = drag.rect;
          pushHistory();
          S.cells[drag.index].rect = saved;
        }
        updateSide();
      }
      drag = null;
      scheduleDraw();
      saveMarkup();
    }

    canvas.addEventListener("pointerup", endPointer);
    canvas.addEventListener("pointercancel", endPointer);
    canvas.addEventListener("contextmenu", function (e) { e.preventDefault(); });

    canvas.addEventListener("wheel", function (e) {
      e.preventDefault();
      var p = localPoint(e);
      zoomAt(p, e.deltaY < 0 ? 1.12 : 1 / 1.12);
    }, { passive: false });

    canvas.addEventListener("dblclick", function (e) {
      var idx = hitCell(localPoint(e));
      if (idx >= 0) removeCell(idx);
    });

    document.addEventListener("keydown", function (e) {
      if (e.code === "Space" && !isTyping(e.target)) {
        spaceDown = true;
        e.preventDefault();
      }
    });
    document.addEventListener("keyup", function (e) {
      if (e.code === "Space") spaceDown = false;
    });

    new ResizeObserver(function () { resizeCanvas(false); }).observe($("#canvas-wrap"));
  }

  function zoomAt(p, factor) {
    var v = S.view;
    var next = clamp(v.scale * factor, 0.06, 8);
    var k = next / v.scale;
    v.x = p.x - (p.x - v.x) * k;
    v.y = p.y - (p.y - v.y) * k;
    v.scale = next;
    updateZoomLabel();
    scheduleDraw();
  }

  function normalizeRect(r) {
    return [Math.min(r[0], r[2]), Math.min(r[1], r[3]), Math.max(r[0], r[2]), Math.max(r[1], r[3])];
  }

  /* ---------- cells ---------- */

  function addCell(rect, label) {
    var cell = {
      id: S.nextId++,
      rect: normalizeRect(rect).map(function (v) { return Math.round(v); }),
      label: label || "",
      row: inferRow(cell0Center(rect)),
      col: 0,
      score: 1,
      manual: true
    };
    S.cells.push(cell);
    S.selected = S.cells.length - 1;
    S.dirty = true;
    updateSide();
    scheduleDraw();
    saveMarkup();
    return cell;
  }

  function cell0Center(rect) {
    var r = normalizeRect(rect);
    return (r[1] + r[3]) / 2;
  }

  function inferRow(y) {
    for (var i = 0; i < S.rows.length; i++) {
      var row = S.rows[i];
      if (y >= row.top && y <= row.bottom) return i;
    }
    return 0;
  }

  function removeCell(idx) {
    if (idx < 0 || idx >= S.cells.length) return;
    pushHistory();
    S.cells.splice(idx, 1);
    S.selected = -1;
    S.dirty = true;
    updateSide();
    scheduleDraw();
    saveMarkup();
  }

  function addCellAtCenter() {
    var cx = (S.cw / 2 - S.view.x) / S.view.scale;
    var cy = (S.ch / 2 - S.view.y) / S.view.scale;
    var size = S.img ? Math.max(60, S.img.width / 24) : 100;
    pushHistory();
    addCell([cx - size / 2, cy - size / 2, cx + size / 2, cy + size / 2], "");
    A.toast("Ячейка добавлена — введите букву", "ok", 2200);
  }

  /* ---------- history ---------- */

  function snapshot() {
    return JSON.stringify({ cells: S.cells, rows: S.rows });
  }

  function pushHistory() {
    S.history.push(snapshot());
    if (S.history.length > 80) S.history.shift();
    S.future = [];
    updateHistoryButtons();
  }

  function restore(str) {
    var data = JSON.parse(str);
    S.cells = data.cells;
    S.rows = data.rows;
    S.selected = -1;
    updateSide();
    scheduleDraw();
    saveMarkup();
  }

  function undo() {
    if (!S.history.length) return;
    S.future.push(snapshot());
    restore(S.history.pop());
    updateHistoryButtons();
  }

  function redo() {
    if (!S.future.length) return;
    S.history.push(snapshot());
    restore(S.future.pop());
    updateHistoryButtons();
  }

  function updateHistoryButtons() {
    var u = $("#btn-undo"), r = $("#btn-redo");
    if (u) u.disabled = !S.history.length;
    if (r) r.disabled = !S.future.length;
  }

  /* ---------- keyboard ---------- */

  function isTyping(node) {
    if (!node) return false;
    var tag = (node.tagName || "").toLowerCase();
    return tag === "input" || tag === "textarea" || tag === "select" || node.isContentEditable;
  }

  function bindKeys() {
    document.addEventListener("keydown", function (e) {
      if (isTyping(e.target)) return;

      var mod = e.ctrlKey || e.metaKey;
      if (mod && (e.key === "z" || e.key === "Z")) {
        e.preventDefault();
        if (e.shiftKey) redo(); else undo();
        return;
      }
      if (mod && (e.key === "y" || e.key === "Y")) {
        e.preventDefault();
        redo();
        return;
      }
      if (e.key === "Escape") {
        S.selected = -1;
        updateSide();
        scheduleDraw();
        return;
      }
      if (!S.doc) return;

      var cell = selectedCell();
      if (e.key === "Delete" || e.key === "Backspace") {
        if (!cell) return;
        e.preventDefault();
        if (e.key === "Backspace" && cell.label) {
          setLabel(S.selected, "");
        } else {
          removeCell(S.selected);
        }
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        if (!S.cells.length) return;
        e.preventDefault();
        cycleSelection(e.shiftKey ? -1 : 1);
        return;
      }
      if (e.key.indexOf("Arrow") === 0) {
        if (!cell) return;
        e.preventDefault();
        var step = e.shiftKey ? 10 : 1;
        var dx = e.key === "ArrowLeft" ? -step : e.key === "ArrowRight" ? step : 0;
        var dy = e.key === "ArrowUp" ? -step : e.key === "ArrowDown" ? step : 0;
        pushHistory();
        cell.rect = [cell.rect[0] + dx, cell.rect[1] + dy, cell.rect[2] + dx, cell.rect[3] + dy];
        S.dirty = true;
        updateSide();
        scheduleDraw();
        saveMarkup();
        return;
      }
      if (mod || e.altKey) return;
      if (e.key === " ") { e.preventDefault(); return; }
      if (e.key.length === 1 && cell) {
        e.preventDefault();
        setLabel(S.selected, e.key);
        cycleSelection(1);
      }
    });
  }

  function cycleSelection(delta) {
    if (!S.cells.length) return;
    if (S.selected < 0) S.selected = delta > 0 ? 0 : S.cells.length - 1;
    else S.selected = (S.selected + delta + S.cells.length) % S.cells.length;
    updateSide();
    scheduleDraw();
    var cell = selectedCell();
    if (cell) focusCell(cell);
  }

  function focusCell(cell) {
    var v = S.view;
    var cx = (cell.rect[0] + cell.rect[2]) / 2 * v.scale + v.x;
    var cy = (cell.rect[1] + cell.rect[3]) / 2 * v.scale + v.y;
    var margin = 80;
    if (cx < margin || cx > S.cw - margin || cy < margin || cy > S.ch - margin) {
      v.x += S.cw / 2 - cx;
      v.y += S.ch / 2 - cy;
      scheduleDraw();
    }
  }

  // setLabelInPlace changes the label without re-rendering the inspector, so
  // the text input keeps focus and the cursor while the user types.
  function setLabelInPlace(cell, label) {
    if (!cell || cell.label === label) return;
    pushHistory();
    cell.label = label;
    S.dirty = true;
    renderCharList();
    renderReport();
    updateHistoryButtons();
    scheduleDraw();
    saveMarkup();
    $("#cells-count").textContent = String(S.cells.length);
  }

  function setLabel(idx, label) {
    if (idx < 0 || idx >= S.cells.length) return;
    var next = firstRune(label);
    if (S.cells[idx].label === next) return;
    pushHistory();
    S.cells[idx].label = next;
    S.dirty = true;
    updateSide();
    scheduleDraw();
    saveMarkup();
  }

  /* ---------- side panel ---------- */

  function updateSide() {
    renderInspector();
    renderCharList();
    renderReport();
    updateHistoryButtons();
    var cell = selectedCell();
    $("#cells-count").textContent = String(S.cells.length);
    var labelled = S.cells.filter(function (c) { return c.label; }).length;
    var pct = S.cells.length ? Math.round((labelled / S.cells.length) * 100) : 0;
    $("#cells-progress").style.width = pct + "%";
    $("#markup-state").textContent = S.dirty ? "есть изменения" : "разметка";
    $("#markup-state").className = "badge " + (S.dirty ? "warn" : "ok");
    if (cell) $("#markup-state").textContent = "ячейка " + cell.id;
    renderSheetTabs();
  }

  function renderInspector() {
    var host = $("#inspector");
    host.innerHTML = "";
    var cell = selectedCell();
    if (!cell) {
      host.appendChild(el("p", { class: "hint", text: "Кликните по ячейке на листе. Букву можно ввести с клавиатуры или в поле ниже." }));
      return;
    }
    var score = Math.round((cell.score || 0) * 100);
    host.appendChild(el("div", { class: "cell-id", text: "Ячейка #" + cell.id + " · строка " + (cell.row + 1) + " · уверенность " + score + "%" }));
    var input = el("input", { type: "text", value: cell.label, maxlength: 2, attrs: { "aria-label": "Буква для ячейки" } });
    input.addEventListener("input", function () {
      var v = firstRune(input.value);
      input.value = v;
      setLabelInPlace(cell, v);
    });
    input.addEventListener("keydown", function (e) {
      if (e.key === "Enter") { e.preventDefault(); cycleSelection(1); }
    });
    host.appendChild(el("div", { class: "label-input" }, el("label", { class: "label", text: "Буква" }), input));

    var del = el("button", { type: "button", class: "btn btn-sm btn-danger", text: "Удалить" });
    del.addEventListener("click", function () { removeCell(S.cells.indexOf(cell)); });
    var next = el("button", { type: "button", class: "btn btn-sm", text: "Следующая →" });
    next.addEventListener("click", function () { cycleSelection(1); });
    host.appendChild(el("div", { class: "btn-group" }, next, del));
  }

  function renderCharList() {
    var host = $("#char-list");
    host.innerHTML = "";
    var groups = {};
    S.cells.forEach(function (c) {
      if (!c.label) return;
      groups[c.label] = (groups[c.label] || 0) + 1;
    });
    var labels = Object.keys(groups).sort(function (a, b) { return a.localeCompare(b, "ru"); });
    if (!labels.length) {
      host.appendChild(el("span", { class: "hint", text: "Пока ни одной буквы." }));
      return;
    }
    labels.forEach(function (label) {
      var chip = el("button", { type: "button", class: "char-chip ok", title: "Перейти к ячейке" },
        el("b", { text: label }), el("i", { text: "×" + groups[label] }));
      chip.addEventListener("click", function () {
        var cursor = S.charCursor[label] || 0;
        var matches = [];
        S.cells.forEach(function (c, i) { if (c.label === label) matches.push(i); });
        if (!matches.length) return;
        S.selected = matches[cursor % matches.length];
        S.charCursor[label] = cursor + 1;
        updateSide();
        scheduleDraw();
        focusCell(S.cells[S.selected]);
      });
      host.appendChild(chip);
    });
  }

  function renderReport() {
    var host = $("#report");
    if (!host) return;
    host.innerHTML = "";
    if (!S.doc) {
      host.appendChild(el("p", { class: "hint", text: "Загрузите лист — здесь появится, каких букв не хватает в разметке." }));
      return;
    }
    var charset = (S.charset || "").split("");
    var present = {};
    S.cells.forEach(function (c) { if (c.label) present[c.label] = 1; });
    var missing = charset.filter(function (ch) { return !present[ch] && ch !== " "; });
    var labelled = S.cells.filter(function (c) { return c.label; }).length;
    var covered = charset.filter(function (ch) { return present[ch]; }).length;

    host.appendChild(el("div", { class: "line" }, el("span", { text: "Ячеек с буквой" }),
      el("b", { text: labelled + " / " + S.cells.length })));
    host.appendChild(el("div", { class: "line" }, el("span", { text: "Символов набора" }),
      el("b", { text: covered + " / " + charset.length })));
    if (missing.length) {
      host.appendChild(el("p", { class: "hint", style: "margin:4px 0 0", text: "Не хватает: " }));
      var box = el("div", { class: "char-list" });
      missing.slice(0, 80).forEach(function (ch) {
        box.appendChild(el("span", { class: "char-chip missing", title: "Буквы нет в разметке" }, el("b", { text: ch })));
      });
      if (missing.length > 80) box.appendChild(el("span", { class: "hint", text: "+" + (missing.length - 80) }));
      host.appendChild(box);
    } else {
      host.appendChild(el("span", { class: "badge ok", text: "весь набор размечен" }));
    }
  }

  /* ---------- persistence ---------- */

  var saveMarkup = A.debounce(function () {
    if (!S.doc) return;
    A.save(markerKey(), { cells: S.cells, rows: S.rows, savedAt: Date.now() });
  }, 500);

  /* ---------- save / commit ---------- */

  function cellsPayload() {
    return S.cells.map(function (c) {
      return {
        id: c.id,
        rect: c.rect.map(function (v) { return Math.round(v); }),
        label: c.label,
        row: c.row,
        col: c.col,
        score: c.score,
        manual: c.manual
      };
    });
  }

  // pushCells persists the markup. `rows` is an extension: the baseline lines can
  // be tuned in the canvas, but older servers only read `cells`. If the server
  // rejects the extra key we retry with the documented payload.
  async function pushCells() {
    var path = "/capture/" + encodeURIComponent(S.doc.id) + "/cells";
    var payload = { cells: cellsPayload(), rows: S.rows };
    try {
      return await A.post(path, payload);
    } catch (err) {
      if (/unknown field|invalid|unmarshal/i.test(err.message || "")) {
        return A.post(path, { cells: cellsPayload() });
      }
      throw err;
    }
  }

  async function saveCells() {
    if (!S.doc) { A.toast("Сначала загрузите лист", "warn"); return; }
    var btn = $("#btn-save-cells");
    A.setBusy(btn, true, "Сохраняю…");
    try {
      await pushCells();
      S.dirty = false;
      updateSide();
      A.toast("Разметка сохранена на сервере", "ok");
    } catch (err) {
      A.toast("Не сохранилось: " + A.errorText(err), "err");
    } finally {
      A.setBusy(btn, false, "Сохранить разметку");
    }
  }

  async function pushSheetCells(sh) {
    var payload = {
      cells: sh.cells.map(function (c) {
        return {
          id: c.id,
          rect: c.rect.map(function (v) { return Math.round(v); }),
          label: c.label,
          row: c.row,
          col: c.col,
          score: c.score,
          manual: c.manual
        };
      })
    };
    return A.post("/capture/" + encodeURIComponent(sh.id) + "/cells", payload);
  }

  async function commit() {
    if (!S.sheets.length) { A.toast("Сначала загрузите лист", "warn"); return; }
    stash();
    var totalLabelled = S.sheets.reduce(function (n, sh) {
      return n + sh.cells.filter(function (c) { return c.label; }).length;
    }, 0);
    if (!totalLabelled) { A.toast("Разметьте хотя бы одну букву", "warn"); return; }

    var btn = $("#btn-commit");
    var name = $("#profile-name").value.trim() || defaultProfileName();
    A.setBusy(btn, true, "Собираю…");
    try {
      var prof = null;
      for (var i = 0; i < S.sheets.length; i++) {
        var sh = S.sheets[i];
        var labelled = sh.cells.filter(function (c) { return c.label; });
        if (!labelled.length) continue;
        await pushSheetCells(sh);
        var body = {
          name: name,
          assignments: labelled.map(function (c) { return { cellId: c.id, label: c.label }; }),
          style: {}
        };
        if (prof && prof.id) body.mergeInto = prof.id;
        var res = await A.post("/capture/" + encodeURIComponent(sh.id) + "/commit", body);
        var got = A.unwrap(res, "profile");
        if (got && got.id) prof = got;
        if (!prof) throw new Error("сервер не вернул профиль");
        btn.textContent = "Собираю " + (i + 1) + "/" + S.sheets.length + "…";
      }
      if (!prof || !prof.id) throw new Error("сервер не вернул профиль");
      markDone("step4");
      S.sheets.forEach(function (sh) { sh.dirty = false; });
      S.dirty = false;
      updateSide();
      showCommitResult(prof);
      A.toast("Почерк «" + (prof.name || name) + "» создан из " + S.sheets.length + " лист(ов)", "ok");
    } catch (err) {
      A.toast("Не получилось собрать почерк: " + A.errorText(err), "err");
    } finally {
      A.setBusy(btn, false, "Создать почерк");
    }
  }

  function defaultProfileName() {
    var d = new Date();
    var pad = function (n) { return n < 10 ? "0" + n : String(n); };
    return "Почерк " + pad(d.getDate()) + "." + pad(d.getMonth() + 1) + " " + pad(d.getHours()) + ":" + pad(d.getMinutes());
  }

  function showCommitResult(prof) {
    var host = $("#commit-result");
    host.innerHTML = "";
    var card = el("div", { class: "card" });
    var head = el("div", { class: "card-head" },
      el("div", {},
        el("h3", { text: prof.name || "Почерк" }),
        el("div", { class: "sub", text: (prof.source === "scan" || prof.source === "manual" ? "с листа" : "из шрифта") +
          (prof.glyphs ? " · " + Object.keys(prof.glyphs).length + " символов" : "") })));
    head.appendChild(el("span", { class: "spacer" }));
    var open = el("a", { class: "btn btn-primary btn-sm", href: "index.html?profile=" + encodeURIComponent(prof.id) + "", text: "Открыть в генераторе" });
    head.appendChild(open);
    card.appendChild(head);
    var body = el("div", { class: "card-body" });
    var img = el("img", {
      alt: "Тестовая строка",
      src: A.url("/profiles/" + encodeURIComponent(prof.id) + "/preview?text=" +
        encodeURIComponent("Съешь же ещё этих мягких французских булок") + "&t=" + Date.now()),
      style: "width:100%;border-radius:8px;background:#fff;border:1px solid var(--border)"
    });
    body.appendChild(img);
    body.appendChild(el("p", { class: "hint", style: "margin-top:10px",
      text: "Это тестовая строка, нарисованная вашим почерком. В генераторе можно менять бумагу, чернила и помарки." }));
    card.appendChild(body);
    host.appendChild(card);
    card.scrollIntoView({ behavior: "smooth", block: "center" });
  }

  /* ---------- misc wiring ---------- */

  function bindSeg(sel, key, onChange) {
    var seg = $(sel);
    if (!seg) return;
    $$("button", seg).forEach(function (btn) {
      var active = String(btn.dataset.value) === String(key === "lang" ? S.lang : S.reps);
      btn.setAttribute("aria-pressed", active ? "true" : "false");
      btn.addEventListener("click", function () {
        $$("button", seg).forEach(function (b) { b.setAttribute("aria-pressed", b === btn ? "true" : "false"); });
        if (key === "lang") { S.lang = btn.dataset.value; A.save("studio-lang", S.lang); S.charset = CHARSETS[S.lang] || CHARSETS.ru; }
        else { S.reps = Number(btn.dataset.value); A.save("studio-reps", S.reps); }
        S.templates = [];
        S.specs = [];
        $("#template-grid").innerHTML = "";
        renderReport();
        if (onChange) onChange();
      });
    });
  }

  function init() {
    A.applyIcons(document);
    S.charset = CHARSETS[S.lang] || CHARSETS.ru;
    bindSeg("#seg-lang", "lang");
    bindSeg("#seg-reps", "reps");
    bindUpload();
    bindCanvas();
    bindKeys();

    $("#btn-template").addEventListener("click", loadTemplates);
    $("#btn-add-cell").addEventListener("click", addCellAtCenter);
    $("#btn-undo").addEventListener("click", undo);
    $("#btn-redo").addEventListener("click", redo);
    $("#btn-fit").addEventListener("click", function () { fitView(); scheduleDraw(); });
    $("#btn-save-cells").addEventListener("click", saveCells);
    $("#btn-commit").addEventListener("click", commit);
    $("#markup-state").className = "badge";
    updateHistoryButtons();
    renderReport();

    // Restore an unfinished markup session when the tab is reopened.
    var last = A.load("studio-last-capture", null);
    if (last && last.id && last.doc) {
      setDoc(last.doc);
      unlock("step2");
      unlock("step3");
      unlock("step4");
    }
  }

  // Remember the last capture so a reload does not lose the markup.
  var _setDoc = setDoc;
  setDoc = function (doc) {
    _setDoc(doc);
    A.save("studio-last-capture", { id: doc.id, doc: doc });
  };

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();
})();
