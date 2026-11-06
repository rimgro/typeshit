/* Generator page: text + profile + controls -> POST /api/v1/generate. */
(function () {
  "use strict";

  var A = window.App;
  var el = A.el, $ = A.$, $$ = A.$$;

  /* ---------- defaults ---------- */

  var EXAMPLE_TEXT =
    "Тема: «Роль природы в романе Л. Н. Толстого \"Война и мир\"»\n\n" +
    "Природа в романе — не фон, а действующее лицо. Толстой показывает её через состояние " +
    "человека: ночное небо над Отрадным, старый дуб на краю дороги, утренний туман над " +
    "полем сражения. Пейзаж у него всегда связан с внутренним переломом героя.\n\n" +
    "Князь Андрей дважды проезжает мимо дуба. В первый раз дерево кажется ему " +
    "«неуклюжим, презрительным уродом», во второй — «раскинувшимся шатром сочной, тёмной " +
    "зелени». Между этими двумя встречами — ночь в Отрадном и голос Наташи в окне. " +
    "Именно тогда, по мысли автора, человек впервые за долгое время слышит не себя, " +
    "а мир вокруг.\n\n" +
    "Таким образом, пейзаж у Толстого становится способом проверки героя: природа " +
    "равнодушна к людям, но она честна. И тот, кто способен её увидеть, способен " +
    "услышать и другого человека.";

  var DEFAULT_STATE = {
    profileId: "",
    seed: A.randomSeed(),
    text: EXAMPLE_TEXT,
    title: "",
    format: "a4",
    page: {
      width: 1240, height: 1754, columns: 1, indent: 2, align: "left",
      marginTop: 90, marginSide: 110, marginBot: 90, pageNumber: true
    },
    paper: {
      kind: "lined", color: "#fbf8f1", rulingColor: "#a8bfd6",
      cellMm: 8, marginMm: 20, marginSide: "left", aging: 0.12, texture: ""
    },
    pen: {
      kind: "ballpoint", color: "#141a3a", width: 0, inkDensity: 1,
      pressure: 0.55, bleed: 0.25, sharp: false
    },
    style: { scale: 0, slant: null, lineGap: null, wordGap: null, variability: 1, join: null },
    effects: {
      kind: "phone", noise: 0.22, blur: 0.18, jpeg: 82, vignette: 0.3, shadow: 0.35,
      rotateDeg: 0, perspective: 0.18, contrast: 0, deskew: false, resample: 0
    },
    flaws: {
      lineWobble: 0.35, crossOut: 0.02, correction: 0.03, overwrite: 0.01, smudge: 0.25,
      blot: 0.2, stain: 0.05, marginNote: 0.08, underline: 0.12, crease: 0.15,
      pressureDot: 0.3, skipWord: 0.01
    }
  };

  /* ---------- controls spec ---------- */

  var PAPER_COLORS = ["#fbf8f1", "#fffdf7", "#f3efe3", "#eef1f3", "#efe7d9"];
  var RULING_COLORS = ["#a8bfd6", "#b9c9d8", "#c4bdae", "#94a3b1"];
  var PEN_COLORS = ["#141a3a", "#101010", "#1b3f9e", "#1b5b3a", "#9b2226"];

  var CONTROLS = [
    {
      id: "page", title: "Страница", tag: "разметка", open: true, fields: [
        { k: "format", t: "seg", label: "Формат", def: "a4", opts: [["a4", "A4"], ["a5", "A5"]] },
        { k: "page.columns", t: "seg", label: "Колонки", def: 1, opts: [[1, "Одна"], [2, "Две"], [3, "Три"]], num: true },
        { k: "page.align", t: "seg", label: "Выключка", def: "left", opts: [["left", "Влево"], ["justify", "По ширине"], ["center", "По центру"]] },
        { k: "page.indent", t: "range", label: "Абзацный отступ", unit: "em", min: 0, max: 5, step: 0.5, def: 2 },
        { k: "page.marginTop", t: "range", label: "Поле сверху", unit: "px", min: 40, max: 260, step: 10, def: 90 },
        { k: "page.marginSide", t: "range", label: "Поле сбоку", unit: "px", min: 40, max: 300, step: 10, def: 110 },
        { k: "page.marginBot", t: "range", label: "Поле снизу", unit: "px", min: 40, max: 260, step: 10, def: 90 },
        { k: "page.pageNumber", t: "toggle", label: "Номера страниц", def: true }
      ]
    },
    {
      id: "paper", title: "Бумага", tag: "тетрадь", open: true, fields: [
        { k: "paper.kind", t: "seg", label: "Фон", def: "lined", opts: [["lined", "В линейку"], ["grid", "Клетка"], ["dot", "Точки"], ["blank", "Чистая"]] },
        { k: "paper.cellMm", t: "range", label: "Шаг строки", unit: "мм", min: 4, max: 12, step: 0.5, def: 8 },
        { k: "paper.color", t: "color", label: "Оттенок бумаги", def: "#fbf8f1", opts: PAPER_COLORS },
        { k: "paper.rulingColor", t: "color", label: "Цвет линовки", def: "#a8bfd6", opts: RULING_COLORS },
        { k: "paper.marginSide", t: "seg", label: "Красное поле", def: "left", opts: [["left", "Слева"], ["right", "Справа"], ["none", "Нет"]] },
        { k: "paper.marginMm", t: "range", label: "Отступ поля", unit: "мм", min: 8, max: 35, step: 1, def: 20 },
        { k: "paper.aging", t: "range", label: "Состаривание", min: 0, max: 1, step: 0.05, def: 0.12 }
      ]
    },
    {
      id: "pen", title: "Ручка", tag: "чернила", open: false, fields: [
        { k: "pen.kind", t: "seg", label: "Тип", def: "ballpoint", opts: [["ballpoint", "Шариковая"], ["gel", "Гелевая"], ["pencil", "Карандаш"], ["fountain", "Перьевая"]] },
        { k: "pen.color", t: "color", label: "Цвет чернил", def: "#141a3a", opts: PEN_COLORS },
        { k: "pen.width", t: "range", label: "Толщина", unit: "em", min: 0, max: 0.12, step: 0.005, def: 0, zeroAuto: "по образцу" },
        { k: "pen.inkDensity", t: "range", label: "Плотность", min: 0, max: 1, step: 0.05, def: 1 },
        { k: "pen.pressure", t: "range", label: "Нажим", min: 0, max: 1, step: 0.05, def: 0.55 },
        { k: "pen.bleed", t: "range", label: "Растекание", min: 0, max: 1, step: 0.05, def: 0.25 }
      ]
    },
    {
      id: "style", title: "Стиль письма", tag: "вариативность", open: true, fields: [
        { k: "style.scale", t: "range", label: "Размер букв", min: 0, max: 2, step: 0.05, def: 0, zeroAuto: "по листу" },
        { k: "style.variability", t: "range", label: "Разброс", min: 0, max: 2, step: 0.05, def: 1, hint: "0 — как под копирку, 1 — как в жизни, 2 — нарочито неровно" },
        { k: "style.slant", t: "range", label: "Наклон", unit: "°", min: -15, max: 30, step: 1, def: null, nullable: true },
        { k: "style.lineGap", t: "range", label: "Межстрочный", unit: "em", min: 1.1, max: 2.6, step: 0.05, def: null, nullable: true },
        { k: "style.wordGap", t: "range", label: "Пробел между словами", unit: "em", min: 0.2, max: 1, step: 0.05, def: null, nullable: true },
        { k: "style.join", t: "range", label: "Слитность (прописи)", min: 0, max: 1, step: 0.05, def: null, nullable: true }
      ]
    },
    {
      id: "flaws", title: "Ошибки и помарки", tag: "по-человечески", open: false, fields: [
        { k: "flaws.lineWobble", t: "range", label: "Дрожание строки", min: 0, max: 1, step: 0.05, def: 0.35 },
        { k: "flaws.crossOut", t: "range", label: "Зачёркнутые слова", min: 0, max: 0.3, step: 0.01, def: 0.02 },
        { k: "flaws.correction", t: "range", label: "Исправления над строкой", min: 0, max: 0.3, step: 0.01, def: 0.03 },
        { k: "flaws.overwrite", t: "range", label: "Перечёркнутые буквы", min: 0, max: 0.2, step: 0.01, def: 0.01 },
        { k: "flaws.skipWord", t: "range", label: "Пропуск и вставка слова", min: 0, max: 0.2, step: 0.01, def: 0.01 },
        { k: "flaws.underline", t: "range", label: "Подчёркивания", min: 0, max: 1, step: 0.05, def: 0.12 },
        { k: "flaws.marginNote", t: "range", label: "Пометки на полях", min: 0, max: 1, step: 0.05, def: 0.08 },
        { k: "flaws.smudge", t: "range", label: "Размазано", min: 0, max: 1, step: 0.05, def: 0.25 },
        { k: "flaws.blot", t: "range", label: "Кляксы", min: 0, max: 1, step: 0.05, def: 0.2 },
        { k: "flaws.stain", t: "range", label: "Пятна (чай, вода)", min: 0, max: 1, step: 0.05, def: 0.05 },
        { k: "flaws.crease", t: "range", label: "Заломы", min: 0, max: 1, step: 0.05, def: 0.15 },
        { k: "flaws.pressureDot", t: "range", label: "Точки нажима", min: 0, max: 1, step: 0.05, def: 0.3 }
      ]
    },
    {
      id: "effects", title: "Как снята страница", tag: "эффекты", open: false, fields: [
        { k: "effects.kind", t: "seg", label: "Источник", def: "phone", opts: [["none", "Чистый файл"], ["scan", "Сканер"], ["phone", "Телефон"], ["fax", "Факс"]] },
        { k: "effects.noise", t: "range", label: "Шум", min: 0, max: 1, step: 0.05, def: 0.22 },
        { k: "effects.blur", t: "range", label: "Размытие", min: 0, max: 1, step: 0.05, def: 0.18 },
        { k: "effects.jpeg", t: "range", label: "Сжатие JPEG", min: 0, max: 95, step: 1, def: 82, zeroAuto: "выкл." },
        { k: "effects.vignette", t: "range", label: "Виньетка", min: 0, max: 1, step: 0.05, def: 0.3 },
        { k: "effects.shadow", t: "range", label: "Тень листа", min: 0, max: 1, step: 0.05, def: 0.35 },
        { k: "effects.rotateDeg", t: "range", label: "Поворот", unit: "°", min: -6, max: 6, step: 0.1, def: 0, zeroAuto: "случайно" },
        { k: "effects.perspective", t: "range", label: "Перспектива", min: 0, max: 1, step: 0.05, def: 0.18 },
        { k: "effects.resample", t: "range", label: "Мягкость матрицы", min: 0, max: 1, step: 0.05, def: 0, zeroAuto: "нет" }
      ]
    }
  ];

  /* ---------- state ---------- */

  function merge(base, extra) {
    var out = JSON.parse(JSON.stringify(base));
    if (!extra || typeof extra !== "object") return out;
    Object.keys(extra).forEach(function (k) {
      var v = extra[k];
      if (v && typeof v === "object" && !Array.isArray(v) && out[k] && typeof out[k] === "object") {
        out[k] = merge(out[k], v);
      } else if (v !== undefined) {
        out[k] = v;
      }
    });
    return out;
  }

  var state = merge(DEFAULT_STATE, A.load("generator-state", null));

  // Errors already surfaced to the user; kept in memory so a reload can warn again.
  var previewWarned = false;

  function getPath(path) {
    return path.split(".").reduce(function (o, k) { return o == null ? undefined : o[k]; }, state);
  }
  function setPath(path, value) {
    var keys = path.split(".");
    var node = state;
    for (var i = 0; i < keys.length - 1; i++) node = node[keys[i]] = node[keys[i]] || {};
    node[keys[keys.length - 1]] = value;
  }

  var saveState = A.debounce(function () {
    A.save("generator-state", state);
  }, 400);

  function changed() {
    saveState();
    schedulePreview();
  }

  /* ---------- control widgets ---------- */

  function fmtVal(f, v) {
    if (v === null || v === undefined) return f.zeroAuto || "авто";
    if (f.t === "seg") {
      var found = f.opts.filter(function (o) { return String(o[0]) === String(v); })[0];
      return found ? found[1] : String(v);
    }
    if (typeof v === "number") {
      var step = f.step || 1;
      var digits = (String(step).split(".")[1] || "").length;
      var s = v.toFixed(digits).replace(".", ",");
      return f.unit ? s + " " + f.unit : s;
    }
    return String(v);
  }

  function makeField(f) {
    var row = el("div", { class: "control-row" + (f.t === "seg" ? " wide" : "") });
    var label = el("div", { class: "control-label" }, f.label);
    var chip = el("span", { class: "val" });
    label.appendChild(chip);
    var body = el("div", { class: "control-body" });
    row.appendChild(label);
    row.appendChild(body);

    var value = getPath(f.k);
    if (value === undefined) { value = f.def; setPath(f.k, value); }

    function refresh() {
      chip.textContent = fmtVal(f, getPath(f.k));
    }

    if (f.t === "seg") {
      var seg = el("div", { class: "seg", attrs: { role: "group" } });
      f.opts.forEach(function (opt) {
        var btn = el("button", { type: "button", text: opt[1], attrs: { "aria-pressed": String(getPath(f.k)) === String(opt[0]) ? "true" : "false" } });
        btn.addEventListener("click", function () {
          setPath(f.k, f.num ? Number(opt[0]) : opt[0]);
          $$("button", seg).forEach(function (b) { b.setAttribute("aria-pressed", b === btn ? "true" : "false"); });
          refresh();
          changed();
        });
        seg.appendChild(btn);
      });
      body.appendChild(seg);
      chip.classList.add("hidden");
    } else if (f.t === "color") {
      var wrap = el("div", { class: "swatches", attrs: { role: "group" } });
      f.opts.forEach(function (c) {
        var b = el("button", { type: "button", class: "swatch", attrs: { "aria-pressed": String(getPath(f.k)).toLowerCase() === c ? "true" : "false", "aria-label": c, style: "background:" + c } });
        b.addEventListener("click", function () {
          setPath(f.k, c);
          $$(".swatch", wrap).forEach(function (x) { x.setAttribute("aria-pressed", x === b ? "true" : "false"); });
          refresh();
          changed();
        });
        wrap.appendChild(b);
      });
      body.appendChild(wrap);
      chip.classList.add("hidden");
    } else if (f.t === "toggle") {
      var cb = el("input", { type: "checkbox" });
      cb.checked = !!getPath(f.k);
      cb.addEventListener("change", function () { setPath(f.k, cb.checked); refresh(); changed(); });
      var lab = el("label", { class: "check" }, cb, el("span", { text: cb.checked ? "вкл" : "выкл" }));
      cb.addEventListener("change", function () { lab.lastChild.textContent = cb.checked ? "вкл" : "выкл"; });
      body.appendChild(lab);
      chip.classList.add("hidden");
    } else {
      var input = el("input", {
        type: "range", min: f.min, max: f.max, step: f.step,
        attrs: { "aria-label": f.label }
      });
      var current = getPath(f.k);
      input.value = current === null || current === undefined ? (f.def === null ? (f.min + f.max) / 2 : f.def) : current;
      input.addEventListener("input", function () {
        setPath(f.k, Number(input.value));
        refresh();
        changed();
      });
      body.appendChild(input);
      if (f.nullable) {
        chip.classList.add("actionable");
        chip.title = "Значение берётся из образца — нажмите, чтобы вернуть";
        chip.addEventListener("click", function () {
          setPath(f.k, null);
          input.value = (f.min + f.max) / 2;
          refresh();
          changed();
        });
      }
      if (f.hint) {
        row.classList.add("wide");
        row.appendChild(el("p", { class: "hint", text: f.hint, style: "grid-column:1/-1;margin:2px 0 0" }));
      }
    }
    refresh();
    return row;
  }

  function buildControls() {
    var host = $("#controls");
    CONTROLS.forEach(function (group) {
      var details = el("details", { class: "group" });
      if (group.open) details.open = true;
      var summary = el("summary", {}, el("span", { text: group.title }));
      if (group.tag) summary.appendChild(el("span", { class: "tag", text: group.tag }));
      summary.appendChild(el("span", { class: "chev", html: A.icon("right", 14) }));
      details.appendChild(summary);
      var body = el("div", { class: "group-body" });
      group.fields.forEach(function (f) { body.appendChild(makeField(f)); });
      details.appendChild(body);
      host.appendChild(details);
    });
  }

  /* ---------- profiles ---------- */

  var profiles = [];

  function profileById(id) {
    for (var i = 0; i < profiles.length; i++) if (profiles[i].id === id) return profiles[i];
    return null;
  }

  function renderProfiles() {
    var host = $("#profiles");
    host.innerHTML = "";
    if (!profiles.length) {
      host.appendChild(el("div", { class: "hint", text: "Почерков пока нет. Создайте демо-профиль или снимите свой.", style: "padding:8px 0" }));
      $("#btn-demo-profile").classList.remove("hidden");
      return;
    }
    $("#btn-demo-profile").classList.add("hidden");
    profiles.forEach(function (p) {
      var card = el("button", { type: "button", class: "profile-card", attrs: { "aria-pressed": String(state.profileId) === p.id ? "true" : "false", title: p.name || p.id } });
      var thumb = el("div", { class: "thumb" }, "…");
      var img = el("img", { alt: "", loading: "lazy", src: A.url("/profiles/" + encodeURIComponent(p.id) + "/preview?text=" + encodeURIComponent("Съешь ещё этих булок")) });
      img.addEventListener("load", function () { thumb.textContent = ""; thumb.appendChild(img); });
      img.addEventListener("error", function () { thumb.textContent = "нет превью"; });
      thumb.appendChild(img);
      card.appendChild(thumb);
      card.appendChild(el("div", { class: "name", text: p.name || p.id }));
      var glyphs = p.glyphs ? Object.keys(p.glyphs).length : (p.charCount || 0);
      card.appendChild(el("div", { class: "meta", text: (p.source === "scan" ? "с листа" : p.source === "manual" ? "разметка" : "шрифт") + (glyphs ? " · " + glyphs + " симв." : "") }));
      card.addEventListener("click", function () {
        state.profileId = p.id;
        $$(".profile-card", host).forEach(function (c) { c.setAttribute("aria-pressed", c === card ? "true" : "false"); });
        saveState();
        schedulePreview(0);
      });
      host.appendChild(card);
    });
  }

  async function loadProfiles() {
    try {
      profiles = A.asList(await A.get("/profiles"), "profiles");
      profiles = profiles.filter(function (p) { return p && p.id; });
    } catch (err) {
      profiles = [];
      A.toast("Не удалось загрузить почерки: " + A.errorText(err), "err");
    }
    var wanted = new URLSearchParams(location.search).get("profile");
    if (wanted && profiles.some(function (p) { return p.id === wanted; })) state.profileId = wanted;
    if (!state.profileId && profiles.length) state.profileId = profiles[0].id;
    if (state.profileId && !profiles.some(function (p) { return p.id === state.profileId; })) state.profileId = "";
    renderProfiles();
    if (state.profileId) schedulePreview(200);
  }

  async function createDemoProfile() {
    var btn = $("#btn-demo-profile");
    A.setBusy(btn, true, "Собираю…");
    try {
      var fonts = A.asList(await A.get("/fonts"), "fonts");
      if (!fonts.length) throw new Error("на сервере нет встроенных шрифтов");
      var font = fonts[0];
      var created = await A.post("/profiles", { name: font.name || font.id, fontId: font.id, variants: 3 });
      var prof = A.unwrap(created, "profile");
      A.toast("Демо-почерк готов", "ok");
      if (prof && prof.id) state.profileId = prof.id;
      await loadProfiles();
      renderProfiles();
      schedulePreview(0);
    } catch (err) {
      A.toast("Не получилось: " + A.errorText(err), "err");
      A.setBusy(btn, false, "Быстрый демо-профиль");
      return;
    }
    A.setBusy(btn, false, "Быстрый демо-профиль");
  }

  /* ---------- request ---------- */

  var FORMATS = { a4: [1240, 1754], a5: [874, 1240] };

  function buildRequest(preview) {
    var size = FORMATS[state.format] || FORMATS.a4;
    var width = size[0], height = size[1];
    var scale = 1;
    if (preview) {
      width = 800;
      scale = width / size[0];
      height = Math.round(size[1] * scale);
    }
    var text = preview ? state.text.slice(0, 1200) : state.text;
    var page = Object.assign({}, state.page, {
      width: width,
      height: height,
      marginTop: Math.round(state.page.marginTop * scale),
      marginSide: Math.round(state.page.marginSide * scale),
      marginBot: Math.round(state.page.marginBot * scale),
    });
    delete page.format;

    var style = Object.assign({}, state.style);
    if (style.scale !== undefined) style.scale = Number(style.scale) || 0;
    if (style.variability === undefined || style.variability === null) style.variability = 1;
    ["slant", "lineGap", "wordGap", "join"].forEach(function (k) {
      if (style[k] === null || style[k] === undefined || style[k] === "" || isNaN(Number(style[k]))) style[k] = null;
      else style[k] = Number(style[k]);
    });

    return {
      text: text,
      title: state.title,
      profileId: state.profileId,
      seed: Number(state.seed) || 0,
      page: page,
      paper: state.paper,
      pen: state.pen,
      style: style,
      effects: state.effects,
      flaws: state.flaws,
      format: "png",
    };
  }

  /* ---------- preview ---------- */

  var previewToken = 0;
  var previewTimer = 0;

  function setStatus(text) { $("#preview-status").textContent = text; }

  function schedulePreview(delay) {
    if (!state.profileId) return;
    clearTimeout(previewTimer);
    previewTimer = setTimeout(runPreview, delay === undefined ? 650 : delay);
  }

  async function runPreview() {
    if (!state.profileId) {
      setStatus("Выберите почерк");
      return;
    }
    var token = ++previewToken;
    var img = $("#preview-img");
    var skel = $("#preview-skeleton");
    skel.classList.remove("hidden");
    if (img.classList.contains("hidden")) skel.style.opacity = "1";
    setStatus("Рендерю предпросмотр…");
    try {
      var res = await A.post("/generate", buildRequest(true));
      if (token !== previewToken) return;
      var norm = A.normalizeResult(res);
      var url = norm.pages[0];
      if (!url) throw new Error("сервер не вернул страниц");
      await preload(url);
      if (token !== previewToken) return;
      img.src = url;
      img.classList.remove("hidden");
      $("#preview-empty").classList.add("hidden");
      setStatus("Предпросмотр первой страницы" +
        (state.text.length > 1200 ? " (первые 1200 символов)" : "") +
        " · " + A.fmtMs(norm.millis));
    } catch (err) {
      if (token !== previewToken) return;
      setStatus("Предпросмотр: " + A.errorText(err));
      if (!previewWarned) {
        previewWarned = true;
        A.toast("Предпросмотр не удался: " + A.errorText(err), "err");
      }
    } finally {
      if (token === previewToken) skel.classList.add("hidden");
    }
  }

  function preload(url) {
    return new Promise(function (resolve, reject) {
      var im = new Image();
      im.onload = function () { resolve(); };
      im.onerror = function () { reject(new Error("не удалось загрузить страницу")); };
      im.src = url;
    });
  }

  /* ---------- full generation ---------- */

  var current = null;

  async function generateFull() {
    if (!state.profileId) {
      A.toast("Сначала выберите почерк", "warn");
      return;
    }
    if (!state.text.trim()) {
      A.toast("Введите текст работы", "warn");
      return;
    }
    var btn = $("#btn-generate");
    A.setBusy(btn, true, "Рендер…");
    setStatus("Генерация…");
    var started = performance.now();
    try {
      var body = buildRequest(false);
      var norm;
      if (state.text.length > 3000) {
        body.async = true;
        var queued = A.normalizeResult(await A.post("/generate", body));
        if (!queued.jobId) throw new Error("сервер не вернул jobId");
        norm = await pollJob(queued.jobId, started);
      } else {
        norm = A.normalizeResult(await A.post("/generate", body));
      }
      if (!norm.pages.length) throw new Error("сервер вернул пустой результат");
      showResult(norm);
    } catch (err) {
      A.toast("Генерация не удалась: " + A.errorText(err), "err");
      setStatus("Ошибка: " + A.errorText(err));
    } finally {
      A.setBusy(btn, false, "Сгенерировать");
    }
  }

  async function pollJob(jobId, started) {
    for (var i = 0; i < 240; i++) {
      await A.sleep(600);
      var job = await A.get("/jobs/" + encodeURIComponent(jobId));
      var status = job.status || (job.job && job.job.status) || "";
      if (status === "done" || status === "finished" || status === "ok") {
        return jobResult(job);
      }
      if (status === "error" || status === "failed") {
        throw new Error(job.error || "задача завершилась ошибкой");
      }
      setStatus("Рендер… " + A.fmtMs(performance.now() - started) + (job.progress ? " · " + Math.round(job.progress * 100) + "%" : ""));
    }
    throw new Error("превышено время ожидания");
  }

  function jobResult(job) {
    var j = job.job || job;
    var resultId = j.resultId || j.resultID || j.result || j.id;
    var raw = j.pages || j.files || [];
    var pages = raw.map(function (p) {
      if (typeof p === "string" && p.indexOf("/results/") >= 0) return p;
      return A.pageURL(p, resultId);
    });
    return { id: resultId, pages: pages, width: j.width, height: j.height, millis: j.millis };
  }

  function showResult(norm) {
    current = norm;
    var id = norm.id;
    var thumbs = $("#thumbs");
    thumbs.innerHTML = "";
    thumbs.classList.remove("hidden");
    norm.pages.forEach(function (url, i) {
      var b = el("button", { type: "button", attrs: { title: "Страница " + (i + 1) } },
        el("img", { src: url, alt: "Страница " + (i + 1), loading: "lazy" }),
        el("span", { class: "num", text: String(i + 1) }));
      b.addEventListener("click", function () { openViewer(norm.pages, i); });
      thumbs.appendChild(b);
    });

    var img = $("#preview-img");
    img.src = norm.pages[0];
    img.classList.remove("hidden");
    $("#preview-empty").classList.add("hidden");

    if (id) {
      var pdf = $("#dl-pdf"), zip = $("#dl-zip"), png = $("#dl-png"), open = $("#btn-open-viewer");
      pdf.href = A.url("/results/" + encodeURIComponent(id) + "/export.pdf");
      zip.href = A.url("/results/" + encodeURIComponent(id) + "/export.zip");
      png.href = norm.pages[0];
      png.setAttribute("download", "page-1.png");
      [pdf, zip, png, open].forEach(function (n) { n.classList.remove("hidden"); });
    }
    openViewerData = norm.pages;

    setStatus(norm.pages.length + " " + A.plural(norm.pages.length, "страница", "страницы", "страниц") +
      (norm.millis ? " · " + A.fmtMs(norm.millis) : "") +
      (norm.width ? " · " + norm.width + "×" + norm.height : ""));
  }

  /* ---------- viewer ---------- */

  var openViewerData = [];
  var v = { scale: 1, x: 0, y: 0, index: 0, fit: 1 };

  function openViewer(pages, index) {
    if (!pages || !pages.length) return;
    openViewerData = pages;
    v.index = index || 0;
    $("#viewer").classList.add("on");
    $("#viewer").setAttribute("aria-hidden", "false");
    document.body.style.overflow = "hidden";
    showViewerPage();
  }

  function closeViewer() {
    $("#viewer").classList.remove("on");
    $("#viewer").setAttribute("aria-hidden", "true");
    document.body.style.overflow = "";
  }

  function showViewerPage() {
    var img = $("#viewer-img");
    $("#viewer-title").textContent = "Страница " + (v.index + 1) + " из " + openViewerData.length;
    img.onload = function () { resetView(); };
    img.src = openViewerData[v.index];
  }

  function applyView() {
    $("#viewer-img").style.transform = "translate(" + v.x + "px," + v.y + "px) scale(" + v.scale + ")";
  }

  function resetView() {
    var stage = $("#viewer-stage");
    var img = $("#viewer-img");
    var sw = stage.clientWidth - 32, sh = stage.clientHeight - 32;
    var iw = img.naturalWidth || 1000, ih = img.naturalHeight || 1400;
    v.fit = Math.min(sw / iw, sh / ih, 3);
    v.scale = v.fit;
    v.x = (stage.clientWidth - iw * v.scale) / 2;
    v.y = (stage.clientHeight - ih * v.scale) / 2;
    applyView();
  }

  function zoomAt(cx, cy, factor) {
    var next = Math.min(Math.max(v.scale * factor, 0.05), 12);
    var k = next / v.scale;
    v.x = cx - (cx - v.x) * k;
    v.y = cy - (cy - v.y) * k;
    v.scale = next;
    applyView();
  }

  function bindViewer() {
    var stage = $("#viewer-stage");
    var drag = null;

    stage.addEventListener("wheel", function (e) {
      e.preventDefault();
      var r = stage.getBoundingClientRect();
      zoomAt(e.clientX - r.left, e.clientY - r.top, e.deltaY < 0 ? 1.14 : 1 / 1.14);
    }, { passive: false });

    stage.addEventListener("pointerdown", function (e) {
      if (e.button !== 0 && e.button !== 1) return;
      drag = { x: e.clientX, y: e.clientY, vx: v.x, vy: v.y };
      stage.classList.add("dragging");
      stage.setPointerCapture(e.pointerId);
    });
    stage.addEventListener("pointermove", function (e) {
      if (!drag) return;
      v.x = drag.vx + (e.clientX - drag.x);
      v.y = drag.vy + (e.clientY - drag.y);
      applyView();
    });
    stage.addEventListener("pointerup", function () { drag = null; stage.classList.remove("dragging"); });
    stage.addEventListener("pointercancel", function () { drag = null; stage.classList.remove("dragging"); });
    stage.addEventListener("dblclick", resetView);

    $("#viewer-close").addEventListener("click", closeViewer);
    $("#viewer-zoom-in").addEventListener("click", function () { zoomAt(stage.clientWidth / 2, stage.clientHeight / 2, 1.25); });
    $("#viewer-zoom-out").addEventListener("click", function () { zoomAt(stage.clientWidth / 2, stage.clientHeight / 2, 0.8); });
    $("#viewer-reset").addEventListener("click", resetView);
    $("#viewer-prev").addEventListener("click", function () { stepPage(-1); });
    $("#viewer-next").addEventListener("click", function () { stepPage(1); });

    document.addEventListener("keydown", function (e) {
      if (!$("#viewer").classList.contains("on")) return;
      if (e.key === "Escape") closeViewer();
      else if (e.key === "ArrowRight") stepPage(1);
      else if (e.key === "ArrowLeft") stepPage(-1);
      else if (e.key === "+" || e.key === "=") zoomAt(stage.clientWidth / 2, stage.clientHeight / 2, 1.25);
      else if (e.key === "-") zoomAt(stage.clientWidth / 2, stage.clientHeight / 2, 0.8);
      else return;
      e.preventDefault();
    });

    window.addEventListener("resize", function () {
      if ($("#viewer").classList.contains("on")) resetView();
    });
  }

  function stepPage(delta) {
    var next = v.index + delta;
    if (next < 0 || next >= openViewerData.length) return;
    v.index = next;
    showViewerPage();
  }

  /* ---------- textarea ---------- */

  function bindText() {
    var ta = $("#text-input");
    var title = $("#title-input");
    ta.value = state.text || "";
    title.value = state.title || "";

    function counter() {
      var n = ta.value.length;
      var c = $("#counter");
      c.textContent = A.fmtNum(n) + " " + A.plural(n, "символ", "символа", "символов");
      c.className = "counter" + (n > 120000 ? " err" : n > 90000 ? " warn" : "");
    }
    function autoGrow() {
      ta.style.height = "auto";
      ta.style.height = Math.min(Math.max(ta.scrollHeight, 220), 620) + "px";
    }

    ta.addEventListener("input", function () {
      state.text = ta.value;
      counter();
      autoGrow();
      saveState();
      schedulePreview();
    });
    title.addEventListener("input", function () {
      state.title = title.value;
      saveState();
      schedulePreview();
    });
    $("#btn-example").addEventListener("click", function () {
      ta.value = EXAMPLE_TEXT;
      state.text = EXAMPLE_TEXT;
      counter(); autoGrow(); saveState(); schedulePreview(0);
    });
    bindImport(ta);
    counter();
    autoGrow();
  }

  // Import a .docx/.txt without leaving the page: drop it on the text area or
  // use the toolbar button.
  function bindImport(ta) {
    var btn = $("#btn-import");
    var input = $("#import-input");
    var hint = $("#import-hint");
    if (!btn || !input) return;

    var load = async function (file) {
      if (!file) return;
      if (!/\.(docx|txt|md)$/i.test(file.name || "")) {
        A.toast("Подойдут .docx, .txt и .md", "warn");
        return;
      }
      hint.hidden = false;
      hint.textContent = "Читаю «" + file.name + "»…";
      try {
        var res = await A.upload("/import", file);
        var text = A.unwrap(res, "import") && A.unwrap(res, "import").text
          || (res && res.text) || "";
        if (!text) throw new Error("в документе не нашлось текста");
        ta.value = text;
        state.text = text;
        counter(); autoGrow(); saveState(); schedulePreview(0);
        hint.textContent = "Загружено из «" + file.name + "»: " + text.length + " символов";
        A.toast("Документ загружен", "ok");
      } catch (err) {
        hint.textContent = "";
        hint.hidden = true;
        A.toast("Не удалось прочитать документ: " + A.errorText(err), "err");
      }
    };

    btn.addEventListener("click", function () { input.click(); });
    input.addEventListener("change", function () {
      if (input.files && input.files[0]) load(input.files[0]);
      input.value = "";
    });
    ["dragover", "dragenter"].forEach(function (ev) {
      ta.addEventListener(ev, function (e) { e.preventDefault(); ta.classList.add("import-over"); });
    });
    ["dragleave", "drop"].forEach(function (ev) {
      ta.addEventListener(ev, function (e) { e.preventDefault(); ta.classList.remove("import-over"); });
    });
    ta.addEventListener("drop", function (e) {
      var f = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0];
      if (f && /\.(docx|txt|md)$/i.test(f.name || "")) load(f);
    });
  }

  /* ---------- misc wiring ---------- */

  function bindSeed() {
    var input = $("#seed");
    input.value = String(state.seed || "");
    input.addEventListener("input", function () {
      var n = parseInt(input.value.replace(/\D/g, ""), 10);
      state.seed = isNaN(n) ? 0 : n;
      saveState();
    });
    input.addEventListener("change", function () { schedulePreview(0); });
    $("#btn-dice").addEventListener("click", function () {
      state.seed = A.randomSeed();
      input.value = String(state.seed);
      saveState();
      schedulePreview(0);
    });
  }

  /* ---------- init ---------- */

  function init() {
    A.applyIcons(document);
    buildControls();
    bindText();
    bindSeed();
    bindViewer();
    $("#btn-generate").addEventListener("click", generateFull);
    $("#btn-demo-profile").addEventListener("click", createDemoProfile);
    $("#btn-open-viewer").addEventListener("click", function () {
      if (openViewerData.length) openViewer(openViewerData, 0);
    });
    loadProfiles();
    if (state.profileId) schedulePreview(400);
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();
})();
