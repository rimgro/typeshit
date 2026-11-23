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
    demoFontId: "",
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
      id: "paper", title: "Бумага", tag: "тетрадь", open: false, fields: [
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
      id: "style", title: "Стиль письма", tag: "вариативность", open: false, fields: [
        { k: "style.scale", t: "range", label: "Размер букв", min: 0, max: 2, step: 0.05, def: 0, zeroAuto: "по листу" },
        { k: "style.variability", t: "range", label: "Разброс", min: 0, max: 2, step: 0.05, def: 1 },
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
  var fonts = [];
  var DEFAULT_SAMPLE = "Съешь ещё этих мягких французских булок";
  var LATIN_SAMPLE = "The quick brown fox jumps over the lazy dog";
  var FONT_CAT_LABELS = {
    handwriting: "Рукописные",
    cursive: "Прописи",
    calligraphy: "Каллиграфия",
    sans: "Печатные"
  };

  var fontState = {
    open: false,
    query: "",
    category: "all",
    cyrillicOnly: true,
    sample: DEFAULT_SAMPLE,
    selected: "",
    loading: false
  };
  var loadedFontFaces = {};
  var fontCardEls = {};
  var fontObserver = null;
  var lastFontFocus = null;

  function fontById(id) {
    for (var i = 0; i < fonts.length; i++) if (fonts[i].id === id) return fonts[i];
    return null;
  }

  function fontCategoryLabel(cat) {
    if (!cat) return "Другие";
    return FONT_CAT_LABELS[cat] || (String(cat).charAt(0).toUpperCase() + String(cat).slice(1));
  }

  function fontFamilyName(id) {
    return "ts-font-" + String(id).replace(/[^a-zA-Z0-9_-]/g, "-");
  }

  function hasCyrillic(text) {
    return /[А-Яа-яЁё]/.test(text || "");
  }

  function sampleFor(font) {
    var custom = (fontState.sample || "").trim();
    if (!custom) custom = DEFAULT_SAMPLE;
    if (font.cyrillic === false && hasCyrillic(custom)) return LATIN_SAMPLE;
    return custom;
  }

  async function loadFonts() {
    try {
      fonts = A.asList(await A.get("/fonts"), "fonts").filter(function (f) { return f && f.id; });
    } catch (err) {
      fonts = [];
      A.toast("Не удалось загрузить каталог шрифтов: " + A.errorText(err), "err");
    }
    if (state.demoFontId && fontById(state.demoFontId)) fontState.selected = state.demoFontId;
    renderFontCats();
    if (fontState.open) {
      renderFontGrid();
      updateFontFooter();
    }
    return fonts;
  }

  function filteredFonts() {
    var q = fontState.query.trim().toLowerCase();
    return fonts.filter(function (f) {
      if (fontState.cyrillicOnly && f.cyrillic === false) return false;
      if (fontState.category !== "all" && String(f.category || "") !== fontState.category) return false;
      if (!q) return true;
      return (f.id + " " + f.name + " " + (f.category || "")).toLowerCase().indexOf(q) !== -1;
    });
  }

  function renderFontCats() {
    var host = $("#font-cats");
    if (!host) return;
    var cats = [{ id: "all", label: "Все" }];
    var seen = {};
    fonts.forEach(function (f) {
      var c = String(f.category || "").trim();
      if (!c || seen[c]) return;
      seen[c] = 1;
      cats.push({ id: c, label: fontCategoryLabel(c) });
    });
    if (fontState.category !== "all" && !seen[fontState.category]) fontState.category = "all";
    host.innerHTML = "";
    cats.forEach(function (c) {
      var b = el("button", {
        type: "button",
        text: c.label,
        attrs: { "data-cat": c.id, "aria-pressed": fontState.category === c.id ? "true" : "false" }
      });
      b.addEventListener("click", function () {
        fontState.category = c.id;
        $$("button", host).forEach(function (x) { x.setAttribute("aria-pressed", x === b ? "true" : "false"); });
        renderFontGrid();
      });
      host.appendChild(b);
    });
  }

  function rememberFontCard(id, card) {
    (fontCardEls[id] = fontCardEls[id] || []).push(card);
  }

  function markFontCards(id, status) {
    (fontCardEls[id] || []).forEach(function (card) {
      card.classList.toggle("is-loading", status === "loading");
      card.classList.toggle("is-error", status === "err");
    });
  }

  function loadFontFace(card) {
    var id = card.getAttribute("data-font-id");
    var f = fontById(id);
    if (!f) return;
    var state = loadedFontFaces[id];
    if (state === "ok") return;
    if (state === "err") { card.classList.add("is-error"); return; }
    if (state === "loading") { card.classList.add("is-loading"); return; }
    if (!("FontFace" in window) || !document.fonts) {
      loadedFontFaces[id] = "err";
      markFontCards(id, "err");
      return;
    }
    loadedFontFaces[id] = "loading";
    markFontCards(id, "loading");
    try {
      var face = new FontFace(
        fontFamilyName(id),
        'url("' + A.url("/fonts/" + encodeURIComponent(id) + "/file") + '")'
      );
      face.display = "swap";
      document.fonts.add(face);
      face.load().then(function () {
        loadedFontFaces[id] = "ok";
        markFontCards(id, "ok");
      }, function () {
        loadedFontFaces[id] = "err";
        markFontCards(id, "err");
      });
    } catch (err) {
      loadedFontFaces[id] = "err";
      markFontCards(id, "err");
    }
  }

  function ensureFontObserver() {
    if (fontObserver) return;
    var root = $("#font-grid");
    if (!root) return;
    if (!("IntersectionObserver" in window)) {
      fontObserver = false;
      return;
    }
    fontObserver = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        fontObserver.unobserve(entry.target);
        loadFontFace(entry.target);
      });
    }, { root: root, rootMargin: "260px 0px" });
  }

  function observeFontCard(card) {
    var id = card.getAttribute("data-font-id");
    if (loadedFontFaces[id] === "ok") return;
    if (!("IntersectionObserver" in window) || fontObserver === false) {
      loadFontFace(card);
      return;
    }
    ensureFontObserver();
    if (fontObserver) fontObserver.observe(card);
    else loadFontFace(card);
  }

  function updateFontSamples() {
    $$(".font-card", $("#font-grid")).forEach(function (card) {
      var f = fontById(card.getAttribute("data-font-id"));
      if (!f) return;
      var prev = card.querySelector(".font-preview");
      if (!prev) return;
      var text = sampleFor(f);
      prev.textContent = text;
      prev.title = text;
    });
  }

  function selectFont(id) {
    fontState.selected = id;
    state.demoFontId = id;
    saveState();
    $$(".font-card", $("#font-grid")).forEach(function (card) {
      var on = card.getAttribute("data-font-id") === id;
      card.classList.toggle("is-selected", on);
      card.setAttribute("aria-pressed", on ? "true" : "false");
    });
    updateFontFooter();
  }

  function updateFontFooter() {
    var box = $("#font-selected");
    var create = $("#font-create");
    if (!box || !create) return;
    var f = fontById(fontState.selected);
    box.innerHTML = "";
    if (!f) {
      box.appendChild(document.createTextNode("Выберите шрифт в списке"));
      create.disabled = true;
      return;
    }
    box.appendChild(document.createTextNode("Выбран: "));
    box.appendChild(el("b", { text: f.name || f.id }));
    if (f.cyrillic === false) {
      box.appendChild(document.createTextNode(" "));
      box.appendChild(el("span", { class: "warn", text: "только латиница" }));
    }
    create.disabled = false;
  }

  function renderFontGrid() {
    var host = $("#font-grid");
    if (!host) return;
    host.innerHTML = "";
    fontCardEls = {};
    if (!fonts.length) {
      host.appendChild(el("p", {
        class: "hint",
        text: fontState.loading ? "Загружаю каталог шрифтов…" : "Каталог шрифтов недоступен. Проверьте сервер."
      }));
      var count = $("#font-count");
      if (count) count.textContent = fontState.loading ? "Загружаю каталог…" : "Каталог пуст";
      updateFontFooter();
      return;
    }

    var list = filteredFonts();
    if (fontState.selected && !list.some(function (f) { return f.id === fontState.selected; })) {
      fontState.selected = "";
    }
    var countEl = $("#font-count");
    if (countEl) countEl.textContent = list.length + " из " + fonts.length + " шрифтов";
    if (!list.length) {
      host.appendChild(el("p", { class: "hint", text: "Ничего не нашлось. Попробуйте другой запрос или снимите фильтр." }));
      updateFontFooter();
      return;
    }

    list.forEach(function (f) {
      var name = f.name || f.id;
      var selected = fontState.selected === f.id;
      var card = el("button", {
        type: "button",
        class: "font-card" + (selected ? " is-selected" : ""),
        attrs: {
          "data-font-id": f.id,
          "aria-pressed": selected ? "true" : "false",
          "aria-label": "Выбрать шрифт " + name,
          title: name
        }
      });
      var check = el("span", { class: "font-card-check", text: "✓", attrs: { "aria-hidden": "true" } });
      var head = el("div", { class: "font-card-head" }, el("span", { class: "font-card-name", text: name }));
      var sample = sampleFor(f);
      var prev = el("span", {
        class: "font-preview",
        text: sample,
        style: "font-family:'" + fontFamilyName(f.id) + "', var(--font)"
      });
      prev.title = sample;
      var bits = [fontCategoryLabel(f.category)];
      if (f.builtin) bits.push("встроенный");
      if (f.cyrillic === false) bits.push("только латиница");
      else if (!fontState.cyrillicOnly) bits.push("кириллица");
      var meta = el("div", { class: "font-card-meta", text: bits.join(" · ") });
      card.appendChild(check);
      card.appendChild(head);
      card.appendChild(prev);
      card.appendChild(meta);
      card.addEventListener("click", function () { selectFont(f.id); });
      host.appendChild(card);

      rememberFontCard(f.id, card);
      if (loadedFontFaces[f.id] === "loading") card.classList.add("is-loading");
      if (loadedFontFaces[f.id] === "err") card.classList.add("is-error");
      observeFontCard(card);
    });
    updateFontFooter();
  }

  function syncFontSelection() {
    var f = state.demoFontId ? fontById(state.demoFontId) : null;
    if (f) {
      fontState.selected = f.id;
      if (f.cyrillic === false) fontState.cyrillicOnly = false;
      if (fontState.category !== "all" && String(f.category || "") !== fontState.category) {
        fontState.category = "all";
      }
    }
    var chip = $("#font-cyr");
    if (chip) chip.setAttribute("aria-pressed", fontState.cyrillicOnly ? "true" : "false");
  }

  function openFontBrowser() {
    var modal = $("#font-modal");
    if (!modal) return;
    lastFontFocus = document.activeElement;
    fontState.open = true;
    fontState.loading = !fonts.length;
    fontState.query = "";
    var searchInput = $("#font-search");
    if (searchInput) searchInput.value = "";
    modal.classList.add("on");
    modal.setAttribute("aria-hidden", "false");
    document.body.style.overflow = "hidden";
    var shell = document.querySelector(".app-shell");
    if (shell) shell.inert = true;
    var sampleInput = $("#font-sample");
    if (sampleInput) sampleInput.value = fontState.sample;
    if (!fonts.length) {
      var host = $("#font-grid");
      if (host) host.innerHTML = '<p class="hint">Загружаю каталог шрифтов…</p>';
      var count = $("#font-count");
      if (count) count.textContent = "Загружаю каталог…";
      loadFonts().then(function () {
        fontState.loading = false;
        if (!fontState.open) return;
        syncFontSelection();
        renderFontCats();
        renderFontGrid();
        updateFontFooter();
        revealSelectedFont();
      });
    } else {
      syncFontSelection();
      renderFontCats();
      renderFontGrid();
      updateFontFooter();
      revealSelectedFont();
    }
    if (window.matchMedia("(min-width: 721px)").matches) {
      setTimeout(function () {
        var search = $("#font-search");
        if (search) search.focus();
      }, 40);
    }
  }

  function closeFontBrowser() {
    var modal = $("#font-modal");
    if (!modal || !modal.classList.contains("on")) return;
    modal.classList.remove("on");
    modal.setAttribute("aria-hidden", "true");
    document.body.style.overflow = "";
    var shell = document.querySelector(".app-shell");
    if (shell) shell.inert = false;
    fontState.open = false;
    if (lastFontFocus && lastFontFocus.focus) lastFontFocus.focus();
  }

  function revealSelectedFont() {
    var host = $("#font-grid");
    if (!host) return;
    var card = host.querySelector(".font-card.is-selected");
    if (!card) return;
    var hr = host.getBoundingClientRect();
    var cr = card.getBoundingClientRect();
    if (cr.top < hr.top || cr.bottom > hr.bottom) {
      host.scrollTop += cr.top - hr.top - 12;
    }
  }

  function initFontBrowser() {
    var trigger = $("#btn-font-browser");
    var modal = $("#font-modal");
    if (!trigger || !modal) return;
    trigger.addEventListener("click", openFontBrowser);
    $("#font-modal-close").addEventListener("click", closeFontBrowser);
    $("#font-modal-cancel").addEventListener("click", closeFontBrowser);
    modal.addEventListener("click", function (e) {
      if (e.target === modal) closeFontBrowser();
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && modal.classList.contains("on")) closeFontBrowser();
    });
    $("#font-search").addEventListener("input", A.debounce(function (e) {
      fontState.query = e.target.value;
      renderFontGrid();
    }, 120));
    $("#font-sample").addEventListener("input", A.debounce(function (e) {
      fontState.sample = e.target.value;
      updateFontSamples();
    }, 120));
    $("#font-cyr").addEventListener("click", function () {
      fontState.cyrillicOnly = !fontState.cyrillicOnly;
      this.setAttribute("aria-pressed", fontState.cyrillicOnly ? "true" : "false");
      renderFontGrid();
    });
    $("#font-create").addEventListener("click", function () {
      createFontProfile(fontState.selected, this);
    });
  }

  function profileById(id) {
    for (var i = 0; i < profiles.length; i++) if (profiles[i].id === id) return profiles[i];
    return null;
  }

  function renderProfiles() {
    var host = $("#profiles");
    host.innerHTML = "";
    if (!profiles.length) {
      host.appendChild(el("div", { class: "hint", text: "Пока пусто. Соберите профиль из шрифта или снимите свой почерк." }));
      return;
    }
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
      var meta = p.source === "scan" ? "с листа" : p.source === "manual" ? "разметка" : "шрифт";
      if (p.meta && p.meta.fontId) {
        var mf = fontById(p.meta.fontId);
        if (mf && mf.cyrillic === false) meta += " · латиница";
      }
      card.appendChild(el("div", { class: "meta", text: meta + (glyphs ? " · " + glyphs + " симв." : "") }));
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
    else setStatus("Выберите почерк");
  }

  async function createFontProfile(fontId, btn) {
    var font = fontById(fontId);
    if (!font) {
      A.toast("Сначала выберите шрифт", "warn");
      return;
    }
    if (!btn) btn = $("#font-create");
    A.setBusy(btn, true, "Собираю…");
    try {
      var lang = font.cyrillic === false ? "latin" : "ru";
      var created = await A.post("/profiles", {
        name: font.name || font.id,
        fontId: font.id,
        variants: 3,
        lang: lang
      });
      var prof = A.unwrap(created, "profile");
      if (!prof || !prof.id) throw new Error("сервер не вернул профиль");
      state.profileId = prof.id;
      state.demoFontId = font.id;
      saveState();
      closeFontBrowser();
      A.toast("Профиль «" + (prof.name || font.name || font.id) + "» готов" +
        (font.cyrillic === false ? " · только латиница" : ""), "ok");
      await loadProfiles();
      renderProfiles();
      schedulePreview(0);
    } catch (err) {
      A.toast("Не получилось: " + A.errorText(err), "err");
    } finally {
      A.setBusy(btn, false, "Создать профиль");
    }
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
    if (!state.profileId) {
      setStatus("Выберите почерк");
      return;
    }
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
    if (img.classList.contains("hidden")) skel.classList.remove("hidden");
    setStatus("Рендер…");
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
      setStatus((state.text.length > 1200 ? "Фрагмент · " : "") + A.fmtMs(norm.millis));
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
    setStatus("Рендер…");
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
      var actions = $("#preview-actions");
      if (actions) actions.classList.remove("hidden");
    }
    openViewerData = norm.pages;

    setStatus(norm.pages.length + " стр." + (norm.millis ? " · " + A.fmtMs(norm.millis) : ""));
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
      c.className = "panel-meta counter" + (n > 120000 ? " err" : n > 90000 ? " warn" : "");
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
    initFontBrowser();
    $("#btn-generate").addEventListener("click", generateFull);
    $("#btn-open-viewer").addEventListener("click", function () {
      if (openViewerData.length) openViewer(openViewerData, 0);
    });
    loadProfiles();
    loadFonts();
    if (state.profileId) schedulePreview(400);
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();
})();
