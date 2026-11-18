// In-app reader. EPUB chapters load one at a time in a sandboxed iframe (no scripts:
// book content is untrusted) and are paginated with CSS columns; PDFs use the
// browser's own viewer.
(() => {
  const el = (id) => document.getElementById(id);
  const reader = el("reader");
  const stage = el("rd-stage");
  const pageBox = el("rd-page");
  const frame = el("rd-frame");
  const pdfFrame = el("rd-pdf");
  const message = el("rd-message");
  const scrub = el("rd-scrub");
  const tocPanel = el("rd-toc");
  const aaPanel = el("rd-aa");

  const FONTS = [
    { id: "original", label: "Original do livro", css: "" },
    { id: "iowan", label: "Iowan", css: `"Iowan Old Style", "Palatino Linotype", Palatino, Georgia, serif` },
    { id: "charter", label: "Charter", css: `Charter, "Bitstream Charter", "Sitka Text", Georgia, serif` },
    { id: "palatino", label: "Palatino", css: `Palatino, "Palatino Linotype", "Book Antiqua", serif` },
    { id: "georgia", label: "Georgia", css: `Georgia, "Times New Roman", serif` },
    { id: "athelas", label: "Athelas", css: `Athelas, Georgia, serif` },
    { id: "sans", label: "Sem serifa", css: `-apple-system, BlinkMacSystemFont, "Segoe UI", "Helvetica Neue", Arial, sans-serif` },
  ];
  const THEME_COLORS = {
    white: { fg: "#1c1c1e", link: "#2f6f71" },
    sepia: { fg: "#5b4636", link: "#8a5a32" },
    gray: { fg: "#e4e4e6", link: "#9cc7c8" },
    night: { fg: "#b4b4b8", link: "#7fb3b4" },
  };
  const SPACING = { tight: 1.35, normal: 1.6, loose: 1.9 };
  const MARGINS = { narrow: 28, normal: 64, wide: 120 };
  const DEFAULTS = { theme: "white", font: "iowan", size: 100, spacing: "normal", margins: "normal", layout: "auto", justify: true };
  // Space kept free above and below the text so it doesn't move when the bars hide.
  const PAD_TOP = 64;
  const PAD_BOTTOM = 56;

  let settings = loadSettings();
  const st = {
    book: null,
    spine: [],
    toc: [],
    sizes: [],
    before: [],
    total: 1,
    idx: 0,
    page: 0,
    pages: 1,
    stride: 1,
    htmlMode: {},
    pending: null,
    saveTimer: null,
    chromeTimer: null,
    wheelLock: 0,
    pdf: false,
  };

  function loadSettings() {
    try {
      return { ...DEFAULTS, ...JSON.parse(localStorage.getItem("lb-reader") || "{}") };
    } catch {
      return { ...DEFAULTS };
    }
  }
  function saveSettings() {
    try { localStorage.setItem("lb-reader", JSON.stringify(settings)); } catch {}
  }

  // ─── Open / close ───
  window.openReader = async function (bookId) {
    let info;
    try {
      const r = await fetch(`/api/books/${bookId}/reader`);
      if (!r.ok) throw new Error((await r.text()).trim());
      info = await r.json();
    } catch (err) {
      alert("Nao foi possivel abrir o livro: " + err.message);
      return;
    }
    st.book = info;
    st.htmlMode = {};
    st.pdf = info.format === "pdf";
    el("rd-title").textContent = info.title;
    el("rd-chapter").textContent = (info.authors || []).join(", ");
    reader.hidden = false;
    document.body.style.overflow = "hidden";
    applyTheme();
    closePanels();
    showChrome();

    reader.classList.toggle("pdf-mode", st.pdf);
    pageBox.hidden = st.pdf;
    pdfFrame.hidden = !st.pdf;
    message.hidden = true;
    if (st.pdf) {
      pdfFrame.src = `/api/books/${info.id}/file?inline=1`;
      saveProgressNow({ spine: 0, fraction: 0, percent: info.progress ? info.progress.percent : 0 });
      return;
    }
    if (info.format !== "epub") {
      showMessage("O leitor abre EPUB e PDF. Este livro esta em " + info.format.toUpperCase() + ".");
      return;
    }

    st.spine = info.spine || [];
    st.toc = info.toc || [];
    st.sizes = st.spine.map((s) => Math.max(s.size || 0, 1));
    st.before = [];
    let acc = 0;
    for (const s of st.sizes) {
      st.before.push(acc);
      acc += s;
    }
    st.total = acc || 1;
    renderTOC();

    const p = info.progress;
    if (p && p.spine < st.spine.length) openChapter(p.spine, { fraction: p.fraction });
    else openChapter(firstReadable(), { fraction: 0 });
  };

  function firstReadable() {
    const i = st.spine.findIndex((s) => s.linear);
    return i >= 0 ? i : 0;
  }

  function closeReader() {
    if (st.book && !st.pdf) saveProgressNow(currentPosition());
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
    reader.hidden = true;
    document.body.style.overflow = "";
    frame.src = "about:blank";
    pdfFrame.src = "about:blank";
    const id = st.book && st.book.id;
    st.book = null;
    if (typeof loadBooks === "function") loadBooks();
    if (typeof loadCounts === "function") loadCounts();
    if (id && typeof selectedBook !== "undefined" && selectedBook && selectedBook.id === id) openInspector(id);
  }

  function showMessage(text) {
    message.textContent = text;
    message.hidden = false;
  }

  // ─── Chapters ───
  function contentURL(href, html) {
    const path = href.split("/").map(encodeURIComponent).join("/");
    return `/api/books/${st.book.id}/content/${path}${html ? "?html=1" : ""}`;
  }

  function openChapter(i, where) {
    if (i < 0 || i >= st.spine.length) return;
    st.idx = i;
    st.pending = where || { fraction: 0 };
    message.hidden = true;
    frame.style.visibility = "hidden";
    frame.src = contentURL(st.spine[i].href, st.htmlMode[i]);
  }

  frame.addEventListener("load", () => {
    if (!st.book || st.pdf || frame.src === "about:blank") return;
    const doc = frame.contentDocument;
    if (!doc || !doc.documentElement) return;
    // XHTML that isn't well-formed renders a parser error; retry it as HTML.
    if (!st.htmlMode[st.idx] && (doc.getElementsByTagName("parsererror").length || doc.documentElement.nodeName === "parsererror")) {
      st.htmlMode[st.idx] = true;
      frame.src = contentURL(st.spine[st.idx].href, true);
      return;
    }
    wireDocument(doc);
    layout();
    const done = () => {
      layout();
      placePending();
      frame.style.visibility = "visible";
    };
    if (doc.fonts && doc.fonts.ready) doc.fonts.ready.then(done, done);
    else done();
  });

  function placePending() {
    const w = st.pending || { fraction: 0 };
    st.pending = null;
    if (w.anchor && scrollToAnchor(w.anchor)) return;
    if (w.last) goToPage(st.pages - 1);
    else goToPage(Math.round((w.fraction || 0) * (st.pages - 1)));
  }

  // ─── Layout ───
  function geometry() {
    const sw = stage.clientWidth;
    const sh = stage.clientHeight;
    const margin = Math.min(MARGINS[settings.margins] || 64, Math.round(sw * 0.08));
    const avail = sw - 2 * margin;
    let cols = 1;
    if (settings.layout === "double" || (settings.layout === "auto" && avail >= 980)) cols = 2;
    if (avail < 560) cols = 1;
    const gap = cols === 2 ? Math.max(56, margin) : margin * 2;
    // Comfortable line length, like a printed page.
    const maxCol = cols === 2 ? 560 : 720;
    const w = Math.max(200, Math.min(avail, cols * maxCol + (cols - 1) * gap));
    const h = Math.max(200, sh - PAD_TOP - PAD_BOTTOM);
    return { w: Math.floor(w), h: Math.floor(h), cols, gap };
  }

  function readerCSS(g) {
    const c = THEME_COLORS[settings.theme] || THEME_COLORS.white;
    const font = FONTS.find((f) => f.id === settings.font);
    const lh = SPACING[settings.spacing] || 1.6;
    return `
      :root { color-scheme: ${darkTheme() ? "dark" : "light"} !important; }
      html {
        height: ${g.h}px !important; width: ${g.w}px !important;
        margin: 0 !important; padding: 0 !important; overflow: hidden !important;
        column-count: ${g.cols} !important; column-gap: ${g.gap}px !important; column-fill: auto !important;
        background: transparent !important; font-size: ${settings.size}% !important;
        -webkit-text-size-adjust: none; text-size-adjust: none;
      }
      body {
        margin: 0 !important; padding: 0 !important; background: transparent !important;
        color: ${c.fg} !important; max-width: none !important; width: auto !important;
        overflow-wrap: break-word;
      }
      body, body * { color: ${c.fg} !important; background-color: transparent !important; border-color: ${c.fg}33 !important; }
      ${font && font.css ? `body, body *:not(code):not(pre):not(kbd):not(samp) { font-family: ${font.css} !important; }` : ""}
      p, li, blockquote, dd, div { line-height: ${lh} !important; }
      ${settings.justify ? `p { text-align: justify; hyphens: auto; -webkit-hyphens: auto; }` : ""}
      p { orphans: 2; widows: 2; }
      h1, h2, h3, h4 { break-after: avoid; line-height: 1.25 !important; }
      a, a * { color: ${c.link} !important; }
      img, svg, video, figure { max-width: 100% !important; max-height: ${g.h - 16}px !important; height: auto; object-fit: contain; break-inside: avoid; }
      svg { height: auto !important; }
      pre { white-space: pre-wrap !important; }
      table { max-width: 100%; }
      ::selection { background: ${c.link}55; }
    `;
  }

  function darkTheme() {
    return settings.theme === "gray" || settings.theme === "night";
  }

  function layout() {
    const doc = frame.contentDocument;
    if (!doc || !doc.documentElement || st.pdf) return;
    const g = geometry();
    // When the frame's colour scheme differs from the page's, browsers paint an opaque
    // backdrop behind it; matching them keeps the reading theme showing through.
    frame.style.colorScheme = darkTheme() ? "dark" : "light";
    frame.style.width = g.w + "px";
    frame.style.height = g.h + "px";
    pageBox.style.marginTop = PAD_TOP - PAD_BOTTOM + "px";
    let style = doc.getElementById("lb-reader-style");
    if (!style) {
      style = doc.createElement("style");
      style.id = "lb-reader-style";
      (doc.head || doc.documentElement).appendChild(style);
    }
    style.textContent = readerCSS(g);
    st.stride = g.w + g.gap;
    const se = doc.scrollingElement || doc.documentElement;
    st.pages = Math.max(1, Math.round((se.scrollWidth + g.gap) / st.stride));
  }

  function relayoutKeepingPlace() {
    if (reader.hidden || st.pdf || !frame.contentDocument) return;
    const frac = st.pages > 1 ? st.page / (st.pages - 1) : 0;
    layout();
    goToPage(Math.round(frac * (st.pages - 1)));
  }

  function goToPage(p) {
    const doc = frame.contentDocument;
    if (!doc) return;
    st.page = Math.max(0, Math.min(p, st.pages - 1));
    const se = doc.scrollingElement || doc.documentElement;
    se.scrollLeft = st.page * st.stride;
    updateStatus();
    saveSoon();
  }

  function scrollToAnchor(id) {
    const doc = frame.contentDocument;
    if (!doc || !id) return false;
    const target = doc.getElementById(id) || doc.getElementsByName(id)[0];
    if (!target) return false;
    const se = doc.scrollingElement || doc.documentElement;
    const x = target.getBoundingClientRect().left + se.scrollLeft;
    goToPage(Math.floor(x / st.stride));
    return true;
  }

  // ─── Turning pages ───
  function next() {
    if (st.pdf || !st.book) return;
    if (st.page < st.pages - 1) goToPage(st.page + 1);
    else if (st.idx < st.spine.length - 1) openChapter(st.idx + 1, { fraction: 0 });
  }
  function prev() {
    if (st.pdf || !st.book) return;
    if (st.page > 0) goToPage(st.page - 1);
    else if (st.idx > 0) openChapter(st.idx - 1, { last: true });
  }

  // ─── Position & progress ───
  function currentPosition() {
    const fraction = st.pages > 1 ? st.page / (st.pages - 1) : 0;
    const percent = ((st.before[st.idx] || 0) + fraction * (st.sizes[st.idx] || 0)) / st.total * 100;
    return { spine: st.idx, fraction, percent: Math.min(100, Math.max(0, percent)) };
  }

  function saveSoon() {
    clearTimeout(st.saveTimer);
    st.saveTimer = setTimeout(() => st.book && saveProgressNow(currentPosition()), 800);
  }

  function saveProgressNow(pos) {
    clearTimeout(st.saveTimer);
    if (!st.book) return;
    fetch(`/api/books/${st.book.id}/progress`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(pos),
      keepalive: true,
    }).catch(() => {});
  }

  function chapterLabel() {
    const href = st.spine[st.idx] && st.spine[st.idx].href;
    let label = "";
    for (const t of st.toc) {
      const path = t.href.split("#")[0];
      const ti = st.spine.findIndex((s) => s.href === path);
      if (ti === -1) continue;
      if (ti <= st.idx) label = t.label;
      if (ti > st.idx) break;
      if (path === href) {
        label = t.label;
        break;
      }
    }
    return label;
  }

  function updateStatus() {
    const pos = currentPosition();
    el("rd-pageinfo").textContent = `Pág. ${st.page + 1} de ${st.pages}`;
    el("rd-percent").textContent = Math.round(pos.percent) + "%";
    if (document.activeElement !== scrub) scrub.value = Math.round(pos.percent * 10);
    const label = chapterLabel();
    el("rd-chapter").textContent = label || (st.book.authors || []).join(", ");
    tocPanel.querySelectorAll("li").forEach((li) => li.classList.toggle("current", li.dataset.spine === String(st.idx) && li.dataset.first === "1"));
  }

  scrub.addEventListener("input", () => {
    el("rd-percent").textContent = Math.round(scrub.value / 10) + "%";
  });
  scrub.addEventListener("change", () => {
    const x = (scrub.value / 1000) * st.total;
    let i = st.before.length - 1;
    while (i > 0 && st.before[i] > x) i--;
    const frac = Math.min(1, Math.max(0, (x - st.before[i]) / st.sizes[i]));
    if (i === st.idx) goToPage(Math.round(frac * (st.pages - 1)));
    else openChapter(i, { fraction: frac });
    scrub.blur();
  });

  // ─── Table of contents ───
  function renderTOC() {
    const list = el("rd-toc-list");
    const seen = new Set();
    list.innerHTML = st.toc.length
      ? st.toc
          .map((t, n) => {
            const path = t.href.split("#")[0];
            const si = st.spine.findIndex((s) => s.href === path);
            const first = !seen.has(si);
            seen.add(si);
            return `<li data-n="${n}" data-spine="${si}" data-first="${first ? 1 : 0}" data-level="${Math.min(t.level, 3)}">${esc(t.label)}</li>`;
          })
          .join("")
      : `<li>Este livro nao tem sumario.</li>`;
  }

  el("rd-toc-list").addEventListener("click", (e) => {
    const li = e.target.closest("li[data-n]");
    if (!li) return;
    goToHref(st.toc[parseInt(li.dataset.n)].href);
    closePanels();
  });

  function goToHref(href) {
    const [path, frag] = href.split("#");
    let i = st.spine.findIndex((s) => s.href === path);
    if (i === -1) i = st.spine.findIndex((s) => s.href.toLowerCase() === path.toLowerCase());
    if (i === -1) return false;
    if (i === st.idx && frame.contentDocument) {
      if (!frag || !scrollToAnchor(frag)) goToPage(0);
    } else {
      openChapter(i, frag ? { anchor: frag } : { fraction: 0 });
    }
    return true;
  }

  // ─── Events inside the book page ───
  function wireDocument(doc) {
    doc.addEventListener("keydown", onKey);
    doc.addEventListener("wheel", onWheel, { passive: false });
    doc.addEventListener("mousemove", showChrome);
    doc.addEventListener("click", (e) => {
      const a = e.target.closest && e.target.closest("a[href]");
      if (a) {
        e.preventDefault();
        followLink(a, doc);
        return;
      }
      if (String(doc.getSelection && doc.getSelection()).trim()) return;
      const x = e.clientX / frame.clientWidth;
      if (x < 0.25) prev();
      else if (x > 0.75) next();
      else toggleChrome();
    });
    let touch = null;
    doc.addEventListener("touchstart", (e) => {
      const t = e.changedTouches[0];
      touch = { x: t.clientX, y: t.clientY };
    }, { passive: true });
    doc.addEventListener("touchend", (e) => {
      if (!touch) return;
      const t = e.changedTouches[0];
      const dx = t.clientX - touch.x;
      const dy = t.clientY - touch.y;
      touch = null;
      if (Math.abs(dx) > 40 && Math.abs(dx) > Math.abs(dy)) (dx < 0 ? next : prev)();
    }, { passive: true });
  }

  function followLink(a, doc) {
    let url;
    try {
      url = new URL(a.getAttribute("href"), doc.baseURI);
    } catch {
      return;
    }
    const base = `/api/books/${st.book.id}/content/`;
    if (url.origin === location.origin && url.pathname.startsWith(base)) {
      const path = url.pathname.slice(base.length).split("/").map(decodeURIComponent).join("/");
      goToHref(path + (url.hash || ""));
    } else if (url.protocol === "http:" || url.protocol === "https:") {
      window.open(url.href, "_blank", "noopener,noreferrer");
    }
  }

  function onWheel(e) {
    if (reader.hidden || st.pdf) return;
    e.preventDefault();
    const now = Date.now();
    const d = Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.deltaY;
    if (Math.abs(d) < 25 || now < st.wheelLock) return;
    st.wheelLock = now + 450;
    (d > 0 ? next : prev)();
  }

  // ─── Keyboard ───
  function onKey(e) {
    if (reader.hidden) return;
    if (e.target && /^(INPUT|SELECT|TEXTAREA)$/.test(e.target.tagName) && e.target !== scrub) return;
    const k = e.key;
    if (k === "Escape") {
      if (!aaPanel.hidden || !tocPanel.hidden) closePanels();
      else if (!document.fullscreenElement) closeReader();
      e.preventDefault();
      return;
    }
    if (k === "f" || k === "F") return toggleFullscreen(), e.preventDefault();
    if (k === "t" || k === "T") return toggleTOC(), e.preventDefault();
    if (st.pdf) return;
    if (k === "ArrowRight" || k === "ArrowDown" || k === "PageDown" || (k === " " && !e.shiftKey)) {
      next();
      e.preventDefault();
    } else if (k === "ArrowLeft" || k === "ArrowUp" || k === "PageUp" || (k === " " && e.shiftKey)) {
      prev();
      e.preventDefault();
    }
  }
  window.addEventListener("keydown", (e) => {
    if (!reader.hidden) {
      onKey(e);
      e.stopImmediatePropagation();
    }
  }, true);

  // ─── Chrome (bars) ───
  function showChrome() {
    reader.classList.remove("chrome-hidden");
    clearTimeout(st.chromeTimer);
    st.chromeTimer = setTimeout(hideChromeIfIdle, 2600);
  }
  function hideChromeIfIdle() {
    if (!aaPanel.hidden || !tocPanel.hidden || reader.matches(":hover") && document.querySelector(".reader-bar:hover")) {
      st.chromeTimer = setTimeout(hideChromeIfIdle, 1500);
      return;
    }
    if (st.book && !st.pdf) reader.classList.add("chrome-hidden");
  }
  function toggleChrome() {
    if (reader.classList.contains("chrome-hidden")) showChrome();
    else {
      clearTimeout(st.chromeTimer);
      reader.classList.add("chrome-hidden");
    }
  }
  reader.addEventListener("mousemove", (e) => {
    if (e.clientY < 70 || e.clientY > window.innerHeight - 60) showChrome();
  });

  // ─── Fullscreen ───
  function toggleFullscreen() {
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
    else reader.requestFullscreen({ navigationUI: "hide" }).catch(() => {});
  }
  document.addEventListener("fullscreenchange", () => {
    const fs = !!document.fullscreenElement;
    el("rd-fs-btn").classList.toggle("active", fs);
    el("rd-fs-icon").innerHTML = fs
      ? `<path d="M8 3v3a2 2 0 01-2 2H3M21 8h-3a2 2 0 01-2-2V3M3 16h3a2 2 0 012 2v3M16 21v-3a2 2 0 012-2h3"/>`
      : `<path d="M8 3H5a2 2 0 00-2 2v3M21 8V5a2 2 0 00-2-2h-3M3 16v3a2 2 0 002 2h3M16 21h3a2 2 0 002-2v-3"/>`;
    setTimeout(relayoutKeepingPlace, 120);
  });

  // ─── Panels ───
  function closePanels() {
    tocPanel.hidden = true;
    aaPanel.hidden = true;
    el("rd-toc-btn").classList.remove("active");
    el("rd-aa-btn").classList.remove("active");
  }
  function toggleTOC() {
    const open = tocPanel.hidden;
    closePanels();
    tocPanel.hidden = !open;
    el("rd-toc-btn").classList.toggle("active", open);
    if (open) {
      showChrome();
      const cur = tocPanel.querySelector("li.current");
      if (cur) cur.scrollIntoView({ block: "center" });
    }
  }
  function toggleAa() {
    const open = aaPanel.hidden;
    closePanels();
    aaPanel.hidden = !open;
    el("rd-aa-btn").classList.toggle("active", open);
    if (open) showChrome();
  }
  el("rd-toc-btn").addEventListener("click", toggleTOC);
  el("rd-toc-close").addEventListener("click", closePanels);
  el("rd-aa-btn").addEventListener("click", toggleAa);
  el("rd-fs-btn").addEventListener("click", toggleFullscreen);
  el("rd-close").addEventListener("click", closeReader);
  el("rd-prev").addEventListener("click", prev);
  el("rd-next").addEventListener("click", next);
  stage.addEventListener("wheel", onWheel, { passive: false });
  stage.addEventListener("click", (e) => {
    if (e.target === stage || e.target === pageBox) closePanels();
  });

  // ─── Appearance ───
  el("rd-fonts").innerHTML = FONTS.map(
    (f) => `<button type="button" data-font="${f.id}" style="${f.css ? `font-family:${esc(f.css)}` : ""}">${esc(f.label)}</button>`
  ).join("");

  function applyTheme() {
    reader.dataset.readerTheme = settings.theme;
    aaPanel.querySelectorAll(".rd-theme").forEach((b) => b.classList.toggle("selected", b.dataset.theme === settings.theme));
    aaPanel.querySelectorAll("[data-font]").forEach((b) => b.classList.toggle("selected", b.dataset.font === settings.font));
    aaPanel.querySelectorAll(".rd-seg").forEach((seg) =>
      seg.querySelectorAll("button").forEach((b) => b.classList.toggle("selected", b.dataset.value === settings[seg.dataset.setting]))
    );
    el("rd-size").textContent = settings.size + "%";
    el("rd-justify").checked = !!settings.justify;
  }

  function changeSetting(key, value) {
    settings[key] = value;
    saveSettings();
    applyTheme();
    relayoutKeepingPlace();
  }

  aaPanel.addEventListener("click", (e) => {
    const theme = e.target.closest(".rd-theme");
    if (theme) return changeSetting("theme", theme.dataset.theme);
    const font = e.target.closest("[data-font]");
    if (font) return changeSetting("font", font.dataset.font);
    const seg = e.target.closest(".rd-seg button");
    if (seg) return changeSetting(seg.parentElement.dataset.setting, seg.dataset.value);
  });
  el("rd-smaller").addEventListener("click", () => changeSetting("size", Math.max(70, settings.size - 10)));
  el("rd-bigger").addEventListener("click", () => changeSetting("size", Math.min(220, settings.size + 10)));
  el("rd-justify").addEventListener("change", (e) => changeSetting("justify", e.target.checked));

  // ─── Resizing ───
  let resizeTimer;
  new ResizeObserver(() => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(relayoutKeepingPlace, 150);
  }).observe(stage);

  window.addEventListener("pagehide", () => {
    if (st.book && !st.pdf) saveProgressNow(currentPosition());
  });

  applyTheme();
})();
