const $ = (s) => document.querySelector(s);
const grid = $("#books-grid");
const listEl = $("#books-list");
const searchEl = $("#search");
const emptyState = $("#empty-state");
const fileInput = $("#file-input");
const fileInputTop = $("#file-input-top");
const uploadProgress = $("#upload-progress");
const uploadFill = $("#upload-fill");
const uploadText = $("#upload-text");
const dropOverlay = $("#drop-overlay");
const inspector = $("#inspector");
const viewGridBtn = $("#view-grid");
const viewListBtn = $("#view-list");
const catalogCount = $("#catalog-count");
const sidebarTags = $("#sidebar-tags");
const tagNewInput = $("#tag-new-input");
const tagNewBtn = $("#tag-new-btn");
const inspectorTags = $("#inspector-tags");
const inspectorTagInput = $("#inspector-tag-input");
const tagSuggestions = $("#tag-suggestions");

const sortSelect = $("#sort-select");
let books = [];
let allTags = [];
let viewMode = "grid";
let selectedBook = null;
let currentFilter = "";
let currentTagFilter = 0;

const STATUS = { want: "quero ler", reading: "lendo", read: "lido" };

// ═══════════════════ Sidebar Nav ═══════════════════
document.querySelectorAll(".nav-item").forEach((btn) => {
  btn.addEventListener("click", () => {
    document.querySelectorAll(".nav-item").forEach((b) => b.classList.remove("active"));
    sidebarTags.querySelectorAll(".sidebar-tag").forEach((b) => b.classList.remove("active"));
    btn.classList.add("active");
    currentFilter = btn.dataset.status;
    currentTagFilter = 0;
    loadBooks();
  });
});

// ═══════════════════ Fetch ═══════════════════
function loadBooks() {
  const params = new URLSearchParams();
  const q = searchEl.value.trim();
  if (q) params.set("q", q);
  if (currentFilter) params.set("status", currentFilter);
  if (currentTagFilter) params.set("tag_id", currentTagFilter);
  params.set("sort", sortSelect.value);

  fetch("/api/books?" + params)
    .then((r) => r.json())
    .then((data) => {
      books = data.books || [];
      render();
    });
}

function loadCounts() {
  fetch("/api/stats")
    .then((r) => r.json())
    .then((s) => {
      $("#count-all").textContent = s.total;
      $("#count-want").textContent = s.want;
      $("#count-reading").textContent = s.reading;
      $("#count-read").textContent = s.read;
      const label = s.total + (s.total === 1 ? " volume" : " volumes");
      catalogCount.textContent = label;
      $("#status-info").textContent = label;
    });
}

// ═══════════════════ Render ═══════════════════
function render() {
  const empty = books.length === 0;
  emptyState.hidden = !empty;
  grid.hidden = empty || viewMode !== "grid";
  listEl.hidden = empty || viewMode !== "list";

  if (!empty) {
    viewMode === "grid" ? renderGrid() : renderList();
  }
}

function renderGrid() {
  grid.innerHTML = books
    .map((b) => {
      const authors = (b.authors || []).join(", ") || "Desconhecido";
      const cover = b.has_cover
        ? `<img src="${coverUrl(b)}" alt="" loading="lazy">`
        : `<div class="cover-placeholder">${esc(b.title)}</div>`;
      const sel = selectedBook && selectedBook.id === b.id ? " selected" : "";
      const fmt = (b.format || "epub").toUpperCase();
      const fmtClass = (b.format || "epub").toLowerCase();
      const statusLabel = STATUS[b.read_status] || b.read_status;
      return `
      <div class="book-card${sel}" data-id="${b.id}">
        <div class="cover-wrap">${cover}</div>
        <div class="card-info">
          <div class="card-badges">
            <span class="badge badge-format badge-${fmtClass}">${fmt}</span>
            <span class="badge badge-status" data-status="${b.read_status}">${esc(statusLabel)}</span>
          </div>
          <div class="card-title">${esc(b.title)}</div>
          <div class="card-author">${esc(authors)}</div>
        </div>
        <div class="card-bar" data-status="${b.read_status}"></div>
      </div>`;
    })
    .join("");
}

function renderList() {
  const header = `<li class="list-header">
    <span></span>
    <span>Titulo</span>
    <span>Autor</span>
    <span>Tags</span>
    <span>Formato</span>
    <span>Status</span>
  </li>`;
  const rows = books
    .map((b) => {
      const authors = (b.authors || []).join(", ") || "Desconhecido";
      const thumb = b.has_cover
        ? `<img src="${coverUrl(b)}" alt="" loading="lazy">`
        : "";
      const sel = selectedBook && selectedBook.id === b.id ? " selected" : "";
      const fmt = (b.format || "epub").toUpperCase();
      const fmtClass = (b.format || "epub").toLowerCase();
      const allTags = b.tags || [];
      const tags =
        allTags.slice(0, 3).map((t) => `<span class="list-tag" title="${esc(t)}">${esc(t)}</span>`).join("") +
        (allTags.length > 3
          ? `<span class="list-tag list-tag--more" title="${esc(allTags.slice(3).join(", "))}">+${allTags.length - 3}</span>`
          : "");
      const statusLabel = STATUS[b.read_status] || b.read_status;
      return `
      <li class="list-row${sel}" data-id="${b.id}">
        <div class="list-thumb">${thumb}</div>
        <span class="list-title">${esc(b.title)}</span>
        <span class="list-author">${esc(authors)}</span>
        <span class="list-tags">${tags || '<span class="list-no-tags">—</span>'}</span>
        <span class="list-format list-format-${fmtClass}">${fmt}</span>
        <span class="list-status" data-status="${b.read_status}">${esc(statusLabel)}</span>
      </li>`;
    })
    .join("");
  listEl.innerHTML = header + rows;
}

// ═══════════════════ View toggle ═══════════════════
viewGridBtn.addEventListener("click", () => setView("grid"));
viewListBtn.addEventListener("click", () => setView("list"));

function setView(mode) {
  viewMode = mode;
  viewGridBtn.classList.toggle("active", mode === "grid");
  viewListBtn.classList.toggle("active", mode === "list");
  render();
}

// ═══════════════════ Sort ═══════════════════
try {
  const savedSort = localStorage.getItem("lb-sort");
  if (savedSort && sortSelect.querySelector(`option[value="${savedSort}"]`)) sortSelect.value = savedSort;
} catch {}
sortSelect.addEventListener("change", () => {
  try { localStorage.setItem("lb-sort", sortSelect.value); } catch {}
  loadBooks();
});

// ═══════════════════ Search ═══════════════════
let searchTimer;
searchEl.addEventListener("input", () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(loadBooks, 250);
});

// ═══════════════════ Inspector ═══════════════════
document.addEventListener("click", (e) => {
  const card = e.target.closest("[data-id]");
  if (!card) return;
  if (card.closest(".inspector")) return;
  openInspector(parseInt(card.dataset.id));
});

function openInspector(id) {
  fetch(`/api/books/${id}`)
    .then((r) => r.json())
    .then((book) => {
      selectedBook = book;

      const coverEl = $("#inspector-cover");
      coverEl.innerHTML = book.has_cover
        ? `<img src="${coverUrl(book)}" alt="">`
        : `<span class="no-cover">Sem capa</span>`;

      $("#inspector-title").textContent = book.title;
      $("#inspector-authors").textContent = (book.authors || []).join(", ");

      const seriesEl = $("#inspector-series");
      seriesEl.textContent = book.series || "";
      seriesEl.hidden = !book.series;

      const metaEl = $("#inspector-meta");
      metaEl.innerHTML = "";
      const fields = [
        ["Formato", book.format?.toUpperCase()],
        ["ISBN", book.isbn],
        ["Idioma", langName(book.language)],
        ["Editora", book.publisher],
        ["Data", formatDate(book.pub_date)],
        ["Tamanho", formatSize(book.file_size)],
      ];
      for (const [label, val] of fields) {
        if (!val) continue;
        metaEl.innerHTML += `<dt>${label}</dt><dd>${esc(val)}</dd>`;
      }

      const descEl = $("#inspector-desc");
      const desc = htmlToText(book.description);
      descEl.textContent = desc;
      descEl.hidden = !desc;

      const statusSelect = $("#inspector-status");
      statusSelect.innerHTML = Object.entries(STATUS)
        .map(
          ([k, v]) =>
            `<option value="${k}" ${k === book.read_status ? "selected" : ""}>${v}</option>`
        )
        .join("");

      $("#inspector-download").href = `/api/books/${book.id}/file`;
      inspector.hidden = false;
      renderInspectorTags();
      render();
    });
}

function closeInspector() {
  inspector.hidden = true;
  selectedBook = null;
  render();
}

$("#inspector-close").addEventListener("click", closeInspector);
document.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  if (!calibreModal.hidden) calibreModal.hidden = true;
  else if (!metaModal.hidden) closeMetaModal();
  else if (!inspector.hidden) closeInspector();
});

// ═══════════════════ Status change ═══════════════════
$("#inspector-status").addEventListener("change", (e) => {
  if (!selectedBook) return;
  fetch(`/api/books/${selectedBook.id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      title: selectedBook.title,
      read_status: e.target.value,
      rating: selectedBook.rating || 0,
    }),
  }).then(() => {
    loadBooks();
    loadCounts();
  });
});

// ═══════════════════ Delete ═══════════════════
$("#inspector-delete").addEventListener("click", () => {
  if (!selectedBook) return;
  if (!confirm(`Remover "${selectedBook.title}"?`)) return;
  fetch(`/api/books/${selectedBook.id}`, { method: "DELETE" }).then(() => {
    closeInspector();
    loadBooks();
    loadCounts();
  });
});

// ═══════════════════ Upload ═══════════════════
fileInput.addEventListener("change", () => {
  if (fileInput.files.length > 0) {
    uploadFiles(fileInput.files);
    fileInput.value = "";
  }
});
fileInputTop.addEventListener("change", () => {
  if (fileInputTop.files.length > 0) {
    uploadFiles(fileInputTop.files);
    fileInputTop.value = "";
  }
});

function uploadFiles(files) {
  const form = new FormData();
  for (const f of files) form.append("files", f);

  uploadProgress.hidden = false;
  uploadFill.style.width = "0%";
  uploadText.textContent = `Importando ${files.length} ${files.length === 1 ? "arquivo" : "arquivos"}...`;

  const xhr = new XMLHttpRequest();
  xhr.open("POST", "/api/books/upload");

  xhr.upload.addEventListener("progress", (e) => {
    if (e.lengthComputable) {
      const pct = Math.round((e.loaded / e.total) * 100);
      uploadFill.style.width = pct + "%";
      uploadText.textContent = `Enviando... ${pct}%`;
    }
  });

  xhr.addEventListener("load", () => {
    uploadFill.style.width = "100%";
    try {
      const data = JSON.parse(xhr.responseText);
      const results = data.results || [];
      const added = results.filter((r) => r.book_id).length;
      const skipped = results.filter((r) => r.skipped).length;
      const errors = results.filter((r) => r.error).length;
      const parts = [];
      if (added) parts.push(`${added} importado${added > 1 ? "s" : ""}`);
      if (skipped) parts.push(`${skipped} duplicado${skipped > 1 ? "s" : ""}`);
      if (errors) parts.push(`${errors} erro${errors > 1 ? "s" : ""}`);
      uploadText.textContent = parts.join(", ");
    } catch {
      uploadText.textContent = "Concluido";
    }
    setTimeout(() => (uploadProgress.hidden = true), 3000);
    loadBooks();
    loadCounts();
  });

  xhr.addEventListener("error", () => {
    uploadText.textContent = "Erro no upload";
    setTimeout(() => (uploadProgress.hidden = true), 3000);
  });

  xhr.send(form);
}

// ═══════════════════ Drag and drop ═══════════════════
let dragCounter = 0;

document.addEventListener("dragenter", (e) => {
  e.preventDefault();
  dragCounter++;
  if (dragCounter === 1) dropOverlay.hidden = false;
});
document.addEventListener("dragleave", (e) => {
  e.preventDefault();
  dragCounter--;
  if (dragCounter <= 0) {
    dragCounter = 0;
    dropOverlay.hidden = true;
  }
});
document.addEventListener("dragover", (e) => e.preventDefault());
document.addEventListener("drop", (e) => {
  e.preventDefault();
  dragCounter = 0;
  dropOverlay.hidden = true;
  const files = [...e.dataTransfer.files].filter((f) => {
    const n = f.name.toLowerCase();
    return n.endsWith(".epub") || n.endsWith(".pdf");
  });
  if (files.length > 0) uploadFiles(files);
});

// ═══════════════════ Helpers ═══════════════════
const coverVersion = {};

function coverUrl(b) {
  const v = coverVersion[b.id];
  return `/api/books/${b.id}/cover` + (v ? `?v=${v}` : "");
}

// DOMParser builds an inert document (no scripts run), so this is a safe way to strip markup.
function htmlToText(s) {
  if (!s) return "";
  s = s.trim();
  if (!/[<&]/.test(s)) return s;
  const withBreaks = s.replace(/<\s*br\s*\/?>/gi, "\n").replace(/<\/p\s*>/gi, "\n\n");
  const doc = new DOMParser().parseFromString(withBreaks, "text/html");
  return doc.body.textContent.replace(/\n{3,}/g, "\n\n").trim();
}

function esc(s) {
  if (!s) return "";
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

function formatDate(s) {
  if (!s) return "";
  try {
    return new Date(s).toLocaleDateString("pt-BR", { year: "numeric", month: "short" });
  } catch {
    return s;
  }
}

function formatSize(bytes) {
  if (!bytes) return "";
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1048576) return (bytes / 1024).toFixed(0) + " KB";
  return (bytes / 1048576).toFixed(1) + " MB";
}

function langName(code) {
  if (!code) return "";
  const names = { pt: "Portugues", en: "Ingles", es: "Espanhol", fr: "Frances", de: "Alemao", it: "Italiano" };
  return names[code] || code;
}

// ═══════════════════ Tags ═══════════════════
function loadTags() {
  fetch("/api/tags")
    .then((r) => r.json())
    .then((tags) => {
      allTags = tags || [];
      renderSidebarTags();
      renderTagSuggestions();
    });
}

function renderSidebarTags() {
  if (allTags.length === 0) {
    sidebarTags.innerHTML = `<div style="padding:4px 22px;font-size:.78rem;font-style:italic;color:rgba(244,241,234,.25)">Nenhuma tag</div>`;
    return;
  }
  sidebarTags.innerHTML = allTags
    .map((t) => `
      <div class="sidebar-tag${currentTagFilter === t.id ? ' active' : ''}" data-tag-id="${t.id}">
        <span class="tag-dot"></span>
        <span class="tag-name">${esc(t.name)}</span>
        <span class="tag-count">${t.count}</span>
        <button class="tag-delete" data-tag-id="${t.id}" aria-label="Remover tag">&times;</button>
      </div>`)
    .join("");

  sidebarTags.querySelectorAll(".sidebar-tag").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      if (e.target.closest(".tag-delete")) return;
      document.querySelectorAll(".nav-item").forEach((b) => b.classList.remove("active"));
      sidebarTags.querySelectorAll(".sidebar-tag").forEach((b) => b.classList.remove("active"));
      btn.classList.add("active");
      currentTagFilter = parseInt(btn.dataset.tagId);
      currentFilter = "";
      loadBooks();
    });
  });

  sidebarTags.querySelectorAll(".tag-delete").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      const tagId = parseInt(btn.dataset.tagId);
      if (!confirm("Remover esta tag?")) return;
      fetch(`/api/tags/${tagId}`, { method: "DELETE" }).then(() => {
        if (currentTagFilter === tagId) {
          currentTagFilter = 0;
          document.querySelector('.nav-item[data-status=""]').classList.add("active");
        }
        loadTags();
        loadBooks();
      });
    });
  });
}

function renderTagSuggestions() {
  tagSuggestions.innerHTML = allTags.map((t) => `<option value="${esc(t.name)}">`).join("");
}

tagNewBtn.addEventListener("click", createTag);
tagNewInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter") createTag();
});

function createTag() {
  const name = tagNewInput.value.trim();
  if (!name) return;
  fetch("/api/tags", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  })
    .then((r) => r.json())
    .then(() => {
      tagNewInput.value = "";
      loadTags();
    });
}

function renderInspectorTags() {
  if (!selectedBook) return;
  const tags = selectedBook.tags || [];
  if (tags.length === 0) {
    inspectorTags.innerHTML = `<span class="inspector-tags-empty">Sem tags</span>`;
  } else {
    inspectorTags.innerHTML = tags
      .map((name) => {
        const tag = allTags.find((t) => t.name === name);
        const tagId = tag ? tag.id : 0;
        return `<span class="inspector-tag-chip">
          ${esc(name)}
          <button class="chip-remove" data-tag-id="${tagId}" aria-label="Remover">&times;</button>
        </span>`;
      })
      .join("");

    inspectorTags.querySelectorAll(".chip-remove").forEach((btn) => {
      btn.addEventListener("click", () => {
        const tagId = parseInt(btn.dataset.tagId);
        if (!tagId || !selectedBook) return;
        fetch(`/api/books/${selectedBook.id}/tags/${tagId}`, { method: "DELETE" }).then(() => {
          openInspector(selectedBook.id);
          loadTags();
        });
      });
    });
  }
}

inspectorTagInput.addEventListener("keydown", (e) => {
  if (e.key !== "Enter") return;
  const name = inspectorTagInput.value.trim();
  if (!name || !selectedBook) return;
  fetch(`/api/books/${selectedBook.id}/tags`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  }).then(() => {
    inspectorTagInput.value = "";
    openInspector(selectedBook.id);
    loadTags();
    loadBooks();
  });
});

// ═══════════════════ Metadata search ═══════════════════
const metaModal = $("#meta-modal");
const metaForm = $("#meta-form");
const metaQuery = $("#meta-query");
const metaWarnings = $("#meta-warnings");
const metaResultsEl = $("#meta-results");
const metaCompare = $("#meta-compare");
const metaStatus = $("#meta-status");
const metaApply = $("#meta-apply");

const META_FIELDS = [
  ["title", "Titulo"],
  ["authors", "Autores"],
  ["isbn", "ISBN"],
  ["publisher", "Editora"],
  ["pub_date", "Data"],
  ["language", "Idioma"],
  ["description", "Descricao"],
];

let metaResults = [];
let metaSelected = -1;
let metaSeq = 0;

$("#inspector-meta-search").addEventListener("click", openMetaModal);
$("#meta-close").addEventListener("click", closeMetaModal);
$("#meta-cancel").addEventListener("click", closeMetaModal);
metaModal.addEventListener("click", (e) => {
  if (e.target === metaModal) closeMetaModal();
});
metaForm.addEventListener("submit", (e) => {
  e.preventDefault();
  runMetaSearch();
});

function openMetaModal() {
  if (!selectedBook) return;
  const author = (selectedBook.authors || []).find((a) => a !== "Desconhecido") || "";
  metaQuery.value = `${selectedBook.title} ${author}`.trim();
  metaModal.hidden = false;
  metaQuery.focus();
  runMetaSearch();
}

function closeMetaModal() {
  metaSeq++;
  metaModal.hidden = true;
  metaResults = [];
  metaSelected = -1;
  metaResultsEl.innerHTML = "";
  metaCompare.innerHTML = `<p class="meta-placeholder">Selecione um resultado para comparar com os dados atuais.</p>`;
  setMetaStatus("");
  metaApply.disabled = true;
}

function setMetaStatus(text, isError) {
  metaStatus.textContent = text;
  metaStatus.classList.toggle("error", !!isError);
}

function runMetaSearch() {
  const q = metaQuery.value.trim();
  if (!q) return;
  const seq = ++metaSeq;
  const params = new URLSearchParams({ q });

  metaResults = [];
  metaSelected = -1;
  metaApply.disabled = true;
  metaWarnings.hidden = true;
  metaResultsEl.innerHTML = "";
  metaCompare.innerHTML = `<p class="meta-placeholder">Buscando...</p>`;
  setMetaStatus("Buscando em Google Books e OpenLibrary...");

  fetch("/api/metadata/search?" + params)
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t)))))
    .then((data) => {
      if (seq !== metaSeq) return;
      metaResults = data.results || [];
      const warns = data.warnings || [];
      metaWarnings.hidden = warns.length === 0;
      metaWarnings.textContent = warns.join(" · ");
      renderMetaResults();
      if (metaResults.length === 0) {
        metaCompare.innerHTML = `<p class="meta-placeholder">Nenhum resultado. Tente outro termo.</p>`;
        setMetaStatus("");
      } else {
        setMetaStatus(`${metaResults.length} resultado${metaResults.length > 1 ? "s" : ""}`);
        selectMetaResult(0);
      }
    })
    .catch((err) => {
      if (seq !== metaSeq) return;
      metaCompare.innerHTML = "";
      setMetaStatus("Falha na busca: " + err.message, true);
    });
}

function renderMetaResults() {
  metaResultsEl.innerHTML = metaResults
    .map((c, i) => {
      const year = (c.pub_date || "").slice(0, 4);
      const thumb = c.cover_url
        ? `<img src="${esc(c.cover_url)}" alt="" loading="lazy" referrerpolicy="no-referrer">`
        : "";
      return `
      <li class="meta-result${i === metaSelected ? " selected" : ""}" data-idx="${i}">
        <div class="meta-result-thumb">${thumb}</div>
        <div>
          <div class="meta-result-title">${esc(c.title)}</div>
          <div class="meta-result-author">${esc((c.authors || []).join(", "))}</div>
          <div class="meta-result-info">
            <span class="meta-source" data-source="${esc(c.source)}">${esc(c.source)}</span>
            ${year ? `<span>${esc(year)}</span>` : ""}
            ${c.publisher ? `<span>${esc(c.publisher)}</span>` : ""}
            ${c.language ? `<span>${esc(c.language.toUpperCase())}</span>` : ""}
          </div>
        </div>
      </li>`;
    })
    .join("");
}

metaResultsEl.addEventListener("click", (e) => {
  const li = e.target.closest(".meta-result");
  if (li) selectMetaResult(parseInt(li.dataset.idx));
});

function metaValue(obj, field) {
  if (field === "authors") return (obj.authors || []).join(", ");
  if (field === "description") return htmlToText(obj.description);
  return (obj[field] || "").toString().trim();
}

function metaDisplay(field, val) {
  if (!val) return "—";
  if (field === "language") return langName(val);
  if (field === "pub_date" && /^\d{4}-\d{2}-\d{2}T/.test(val)) return val.slice(0, 10);
  return val;
}

function selectMetaResult(i) {
  metaSelected = i;
  metaResultsEl.querySelectorAll(".meta-result").forEach((li) =>
    li.classList.toggle("selected", parseInt(li.dataset.idx) === i)
  );
  const c = metaResults[i];
  const b = selectedBook;

  const curCover = b.has_cover
    ? `<img src="${coverUrl(b)}" alt="">`
    : `<span class="compare-nocover">Sem capa</span>`;
  const newCover = c.cover_url
    ? `<img src="${esc(c.cover_url)}" alt="" referrerpolicy="no-referrer">`
    : `<span class="compare-nocover">Sem capa</span>`;
  const coverCheck = c.cover_url
    ? `<label class="compare-cover-check"><input type="checkbox" data-field="cover" ${b.has_cover ? "" : "checked"}> Usar esta capa</label>`
    : "";

  const rows = META_FIELDS.map(([field, label]) => {
    const cur = metaValue(b, field);
    const nw = metaValue(c, field);
    const same = nw === cur;
    const usable = nw && !same;
    const checked = usable && !cur;
    const cls = field === "description" ? ' class="cmp-desc"' : "";
    return `
      <tr class="${usable ? "" : "cmp-disabled"}">
        <td>${label}</td>
        <td><div${cls}>${esc(metaDisplay(field, cur))}</div></td>
        <td class="${same && nw ? "cmp-same" : "cmp-new"}"><div${cls}>${same && nw ? "igual" : esc(metaDisplay(field, nw))}</div></td>
        <td><input type="checkbox" data-field="${field}" ${checked ? "checked" : ""} ${usable ? "" : "disabled"} aria-label="Usar ${label}"></td>
      </tr>`;
  }).join("");

  metaCompare.innerHTML = `
    <div class="compare-covers">
      <div class="compare-cover">${curCover}<div class="compare-label">Atual</div></div>
      <span class="compare-arrow">→</span>
      <div class="compare-cover">${newCover}<div class="compare-label">${esc(c.source)}</div></div>
      ${coverCheck}
    </div>
    <table class="compare-table">
      <thead><tr><th>Campo</th><th>Atual</th><th>Novo</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table>`;
  updateApplyState();
}

metaCompare.addEventListener("change", updateApplyState);

function updateApplyState() {
  metaApply.disabled = metaCompare.querySelectorAll("input[type=checkbox]:checked").length === 0;
}

metaApply.addEventListener("click", () => {
  if (metaSelected < 0 || !selectedBook) return;
  const c = metaResults[metaSelected];
  const body = {};
  metaCompare.querySelectorAll("input[type=checkbox]:checked").forEach((cb) => {
    const f = cb.dataset.field;
    if (f === "cover") body.cover_url = c.cover_url;
    else if (f === "authors") body.authors = c.authors;
    else body[f] = metaValue(c, f);
  });

  const bookId = selectedBook.id;
  metaApply.disabled = true;
  setMetaStatus("Aplicando...");
  fetch(`/api/books/${bookId}/metadata`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t)))))
    .then(() => {
      if (body.cover_url) coverVersion[bookId] = Date.now();
      closeMetaModal();
      openInspector(bookId);
      loadBooks();
      loadCounts();
    })
    .catch((err) => {
      setMetaStatus("Falha ao aplicar: " + err.message, true);
      updateApplyState();
    });
});

// ═══════════════════ Calibre import ═══════════════════
const calibreModal = $("#calibre-modal");
const calibrePath = $("#calibre-path");
const calibreStart = $("#calibre-start");
const calibreProgressEl = $("#calibre-progress");
const calibreFill = $("#calibre-fill");
const calibreCounts = $("#calibre-counts");
const calibreErrors = $("#calibre-errors");
const calibreStatus = $("#calibre-status");
let calibrePolling = false;

$("#calibre-open").addEventListener("click", () => {
  calibreModal.hidden = false;
  calibreStatus.textContent = "";
  calibreStatus.classList.remove("error");
  fetch("/api/import/calibre")
    .then((r) => r.json())
    .then((data) => {
      if (!calibrePath.value && data.default_path) calibrePath.value = data.default_path;
      if (data.progress.running || data.progress.total) showCalibreProgress(data.progress);
      if (data.progress.running) pollCalibre();
      calibrePath.focus();
    });
});
for (const id of ["#calibre-close", "#calibre-cancel"]) {
  $(id).addEventListener("click", () => (calibreModal.hidden = true));
}
calibreModal.addEventListener("click", (e) => {
  if (e.target === calibreModal) calibreModal.hidden = true;
});
calibrePath.addEventListener("keydown", (e) => {
  if (e.key === "Enter") calibreStart.click();
});
calibrePath.addEventListener("input", () => (calibrePathWarn.hidden = true));

const calibreBrowse = $("#calibre-browse");
const calibrePathWarn = $("#calibre-path-warn");
const browseLabel = calibreBrowse.innerHTML;

// The server opens the OS folder picker; the browser itself can't expose a folder path.
calibreBrowse.addEventListener("click", () => {
  calibreBrowse.disabled = true;
  calibreBrowse.textContent = "Aguardando...";
  calibrePathWarn.hidden = true;
  fetch("/api/dialog/folder", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ start: calibrePath.value.trim() }),
  })
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then((res) => {
      if (res.canceled) return;
      calibrePath.value = res.path;
      if (!res.is_calibre) {
        calibrePathWarn.textContent = "Essa pasta nao tem metadata.db. Escolha a pasta raiz da biblioteca do Calibre.";
        calibrePathWarn.hidden = false;
      }
    })
    .catch((err) => {
      calibrePathWarn.textContent = "Nao foi possivel abrir o seletor: " + err.message + ". Digite o caminho no campo.";
      calibrePathWarn.hidden = false;
    })
    .finally(() => {
      calibreBrowse.disabled = false;
      calibreBrowse.innerHTML = browseLabel;
    });
});

calibreStart.addEventListener("click", () => {
  const path = calibrePath.value.trim();
  if (!path) return;
  calibreStart.disabled = true;
  calibreStatus.classList.remove("error");
  calibreStatus.textContent = "Lendo biblioteca do Calibre...";
  fetch("/api/import/calibre", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path }),
  })
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then((p) => {
      showCalibreProgress(p);
      pollCalibre();
    })
    .catch((err) => {
      calibreStart.disabled = false;
      calibreStatus.classList.add("error");
      calibreStatus.textContent = err.message;
    });
});

function showCalibreProgress(p) {
  calibreProgressEl.hidden = false;
  const pct = p.total ? Math.round((p.done / p.total) * 100) : 0;
  calibreFill.style.width = pct + "%";
  const parts = [`${p.done} de ${p.total}`, `${p.imported} importados`];
  if (p.skipped) parts.push(`${p.skipped} ja existiam`);
  if (p.unsupported) parts.push(`${p.unsupported} sem EPUB/PDF`);
  if (p.errors && p.errors.length) parts.push(`${p.errors.length} com erro`);
  calibreCounts.textContent = parts.join(" · ");
  const errs = p.errors || [];
  calibreErrors.hidden = errs.length === 0;
  calibreErrors.innerHTML = errs.map((e) => `<li>${esc(e)}</li>`).join("");
  calibreStart.disabled = p.running;
  calibreStatus.classList.remove("error");
  calibreStatus.textContent = p.running ? "Importando..." : p.total ? "Concluido" : "";
}

function pollCalibre() {
  if (calibrePolling) return;
  calibrePolling = true;
  let lastDone = -1;
  const tick = () => {
    fetch("/api/import/calibre")
      .then((r) => r.json())
      .then(({ progress: p }) => {
        showCalibreProgress(p);
        if (p.done !== lastDone) {
          lastDone = p.done;
          loadBooks();
          loadCounts();
          loadTags();
        }
        if (p.running) setTimeout(tick, 800);
        else calibrePolling = false;
      })
      .catch(() => (calibrePolling = false));
  };
  tick();
}

// ═══════════════════ Theme Toggle ═══════════════════
const themeToggle = $("#theme-toggle");
const moonSvg = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/></svg>`;
const sunSvg = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>`;

function updateThemeIcon() {
  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  themeToggle.innerHTML = isDark ? sunSvg : moonSvg;
}

try {
  const saved = localStorage.getItem("lb-theme");
  if (saved === "dark") document.documentElement.setAttribute("data-theme", "dark");
} catch {}
updateThemeIcon();

themeToggle.addEventListener("click", () => {
  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  if (isDark) {
    document.documentElement.removeAttribute("data-theme");
  } else {
    document.documentElement.setAttribute("data-theme", "dark");
  }
  try { localStorage.setItem("lb-theme", isDark ? "light" : "dark"); } catch {}
  updateThemeIcon();
});

// ═══════════════════ Init ═══════════════════
loadBooks();
loadCounts();
loadTags();
