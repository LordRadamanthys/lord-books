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
        <div class="card-bar" data-status="${b.read_status}">${b.progress ? `<span class="card-progress" style="width:${Math.min(100, b.progress).toFixed(1)}%"></span>` : ""}</div>
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
// Double-clicking a book opens it in the reader.
document.addEventListener("dblclick", (e) => {
  const card = e.target.closest(".book-card[data-id], .list-row[data-id]");
  if (card && window.openReader) openReader(parseInt(card.dataset.id));
});
$("#inspector-read").addEventListener("click", () => selectedBook && window.openReader && openReader(selectedBook.id));

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
      seriesEl.textContent = book.series
        ? book.series + (book.series_index ? ` #${book.series_index}` : "")
        : "";
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
      const readable = ["epub", "pdf"].includes((book.format || "").toLowerCase());
      $("#inspector-read").disabled = !readable;
      $("#inspector-read-label").textContent = !readable
        ? `Leitura de ${(book.format || "").toUpperCase()} indisponivel`
        : book.progress > 0.5 && book.progress < 99.5
          ? `Continuar lendo · ${Math.round(book.progress)}%`
          : book.progress >= 99.5 ? "Ler de novo" : "Ler";
      inspector.hidden = false;
      setEditMode(false);
      renderInspectorTags();
      render();
    });
}

function closeInspector() {
  setEditMode(false);
  inspector.hidden = true;
  selectedBook = null;
  render();
}

$("#inspector-close").addEventListener("click", closeInspector);
document.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  if (!freeModal.hidden) freeModal.hidden = true;
  else if (!netModal.hidden) netModal.hidden = true;
  else if (!sendModal.hidden) closeSendModal();
  else if (!calibreModal.hidden) calibreModal.hidden = true;
  else if (!metaModal.hidden) closeMetaModal();
  else if (!editForm.hidden) setEditMode(false);
  else if (!inspector.hidden) closeInspector();
});

// ═══════════════════ Manual edit ═══════════════════
const editForm = $("#inspector-edit");
const inspectorView = $("#inspector-view");
const editError = $("#edit-error");
const editFields = {
  title: $("#edit-title"),
  authors: $("#edit-authors"),
  series: $("#edit-series"),
  series_index: $("#edit-series-index"),
  publisher: $("#edit-publisher"),
  pub_date: $("#edit-pubdate"),
  isbn: $("#edit-isbn"),
  language: $("#edit-language"),
  description: $("#edit-description"),
};
let editOriginal = null;

function setEditMode(on) {
  editForm.hidden = !on;
  inspectorView.hidden = on;
  editError.hidden = true;
  $(".inspector-label").textContent = on ? "Editar Livro" : "Detalhes do Livro";
}

// Values as the form shows them, so "changed" means the user actually edited the field.
function editValuesFrom(book) {
  return {
    title: book.title || "",
    authors: (book.authors || []).join("\n"),
    series: book.series || "",
    series_index: book.series ? String(book.series_index || "") : "",
    publisher: book.publisher || "",
    pub_date: (book.pub_date || "").slice(0, 10),
    isbn: book.isbn || "",
    language: book.language || "",
    description: htmlToText(book.description),
  };
}

$("#inspector-edit-btn").addEventListener("click", () => {
  if (!selectedBook) return;
  editOriginal = editValuesFrom(selectedBook);
  const lang = editFields.language;
  if (editOriginal.language && !lang.querySelector(`option[value="${CSS.escape(editOriginal.language)}"]`)) {
    lang.add(new Option(langName(editOriginal.language), editOriginal.language));
  }
  for (const [k, el] of Object.entries(editFields)) el.value = editOriginal[k];
  fetch("/api/series")
    .then((r) => r.json())
    .then((list) => {
      $("#series-suggestions").innerHTML = (list || []).map((s) => `<option value="${esc(s.name)}">`).join("");
    });
  setEditMode(true);
  editFields.title.focus();
});

$("#edit-cancel").addEventListener("click", () => setEditMode(false));

editForm.addEventListener("submit", (e) => {
  e.preventDefault();
  if (!selectedBook) return;
  const cur = {};
  for (const [k, el] of Object.entries(editFields)) cur[k] = el.value.trim();

  const body = {};
  for (const k of Object.keys(editFields)) {
    if (cur[k] === editOriginal[k].trim()) continue;
    if (k === "authors") body.authors = cur.authors.split("\n").map((a) => a.trim()).filter(Boolean);
    else if (k === "series_index") body.series_index = cur.series_index === "" ? 0 : parseFloat(cur.series_index);
    else body[k] = cur[k];
  }
  if (body.series === "") body.series_index = 0;

  if (Object.keys(body).length === 0) {
    setEditMode(false);
    return;
  }
  if (body.authors && body.authors.length === 0) return showEditError("Informe ao menos um autor.");
  if (body.series_index !== undefined && Number.isNaN(body.series_index)) return showEditError("Numero da serie invalido.");

  const saveBtn = $("#edit-save");
  saveBtn.disabled = true;
  fetch(`/api/books/${selectedBook.id}/metadata`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then(() => {
      openInspector(selectedBook.id);
      loadBooks();
    })
    .catch((err) => showEditError(err.message))
    .finally(() => (saveBtn.disabled = false));
});

function showEditError(msg) {
  editError.textContent = msg;
  editError.hidden = false;
}

$("#edit-cover-file").addEventListener("change", (e) => {
  const file = e.target.files[0];
  e.target.value = "";
  if (!file || !selectedBook) return;
  const id = selectedBook.id;
  fetch(`/api/books/${id}/cover`, { method: "PUT", body: file })
    .then((r) => (r.ok ? null : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then(() => {
      coverVersion[id] = Date.now();
      selectedBook.has_cover = true;
      $("#inspector-cover").innerHTML = `<img src="${coverUrl(selectedBook)}" alt="">`;
      loadBooks();
    })
    .catch((err) => showEditError(err.message));
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
  if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + " MB";
  return (bytes / 1073741824).toFixed(1) + " GB";
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

// ═══════════════════ Send to device ═══════════════════
const sendModal = $("#send-modal");
const sendDeviceList = $("#send-device-list");
const sendConfig = $("#send-device-config");
const sendKind = $("#send-kind");
const sendFolder = $("#send-folder");
const sendBookList = $("#send-book-list");
const sendFilter = $("#send-filter");
const sendGo = $("#send-go");
const sendStatus = $("#send-status");

const sendState = {
  devices: [],
  profiles: {},
  deviceId: "",
  books: [],
  checked: new Set(),
  results: {},
  converter: false,
};

const SEND_RESULT = {
  sent: "enviado",
  exists: "ja estava",
  duplicate: "provavel duplicado",
  unsupported: "formato incompativel",
  error: "erro",
};

$("#send-open").addEventListener("click", () => openSendModal([]));
$("#inspector-send").addEventListener("click", () => selectedBook && openSendModal([selectedBook.id]));
for (const id of ["#send-close", "#send-cancel"]) $(id).addEventListener("click", closeSendModal);
sendModal.addEventListener("click", (e) => {
  if (e.target === sendModal) closeSendModal();
});
$("#send-refresh").addEventListener("click", loadDevices);

function openSendModal(preselect) {
  sendState.checked = new Set(preselect);
  sendState.results = {};
  sendFilter.value = "";
  deviceFilter.value = "";
  deviceBooks.loadedFor = "";
  showTab("send");
  setSendStatus("");
  sendModal.hidden = false;
  fetch("/api/books?sort=title")
    .then((r) => r.json())
    .then((data) => {
      sendState.books = data.books || [];
      renderSendBooks();
    });
  loadDevices();
}

function closeSendModal() {
  sendModal.hidden = true;
}

function setSendStatus(text, isError) {
  sendStatus.textContent = text;
  sendStatus.classList.toggle("error", !!isError);
}

function loadDevices() {
  sendDeviceList.innerHTML = `<li class="send-empty">Procurando...</li>`;
  fetch("/api/devices")
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then((data) => {
      sendState.devices = data.devices || [];
      sendState.profiles = data.profiles || {};
      sendState.converter = !!data.converter;
      sendKind.innerHTML = Object.values(sendState.profiles)
        .filter((p) => p.kind !== "wireless")
        .map((p) => `<option value="${esc(p.kind)}">${esc(p.label)}</option>`)
        .join("");
      if (!sendState.devices.some((d) => d.id === sendState.deviceId)) {
        sendState.deviceId = sendState.devices.length ? sendState.devices[0].id : "";
      }
      renderDevices();
      selectDevice(sendState.deviceId);
    })
    .catch((err) => {
      sendDeviceList.innerHTML = `<li class="send-empty">Erro ao listar dispositivos: ${esc(err.message)}</li>`;
    });
}

function renderDevices() {
  if (sendState.devices.length === 0) {
    sendDeviceList.innerHTML = `<li class="send-empty">
      Nenhum dispositivo encontrado. Conecte o e-reader pelo cabo (ou o cartao SD no leitor) e clique em <strong>Atualizar</strong>.<br><br>
      Boox e outros leitores Android usam MTP e nao aparecem como disco; Kindles mais novos podem ter o mesmo problema no Mac.
    </li>`;
    return;
  }
  sendDeviceList.innerHTML = sendState.devices
    .map((d) => {
      const kind = deviceKind(d);
      const label = (sendState.profiles[kind] || d).label;
      const free = d.total_bytes
        ? `${formatSize(d.free_bytes)} livres de ${formatSize(d.total_bytes)}`
        : d.free_bytes ? `${formatSize(d.free_bytes)} livres` : "";
      return `<li class="send-device${d.id === sendState.deviceId ? " selected" : ""}" data-device="${esc(d.id)}">
        <div class="send-device-name">${esc(d.name)}</div>
        <div class="send-device-info">
          <span class="send-kind-badge" data-kind="${esc(kind)}">${esc(label)}</span>
          ${free ? `<span>${esc(free)}</span>` : ""}
        </div>
      </li>`;
    })
    .join("");
}

sendDeviceList.addEventListener("click", (e) => {
  const li = e.target.closest(".send-device");
  if (li) selectDevice(li.dataset.device);
});

function currentDevice() {
  return sendState.devices.find((d) => d.id === sendState.deviceId);
}

// The user's type/folder choices are remembered per volume name.
function prefKey(d, what) {
  return `lb-dev-${what}:${d.name}`;
}
function loadPref(d, what) {
  try { return localStorage.getItem(prefKey(d, what)); } catch { return null; }
}
function savePref(d, what, val) {
  try { localStorage.setItem(prefKey(d, what), val); } catch {}
}

function deviceKind(d) {
  if (d.wireless) return d.kind;
  const saved = loadPref(d, "kind");
  return saved && sendState.profiles[saved] ? saved : d.kind;
}

function selectDevice(id) {
  const changed = sendState.deviceId !== id;
  sendState.deviceId = id;
  sendState.results = {};
  folderBrowser.hidden = true;
  if (changed || !deviceBooks.loadedFor) loadDeviceBooks();
  sendDeviceList.querySelectorAll(".send-device").forEach((li) =>
    li.classList.toggle("selected", li.dataset.device === id)
  );
  const d = currentDevice();
  sendConfig.hidden = !d;
  if (d) {
    // A Wi-Fi reader decides where books go and announces its own formats.
    sendKind.closest(".edit-field").hidden = !!d.wireless;
    sendFolder.closest(".edit-field").hidden = !!d.wireless;
    $("#send-eject").textContent = d.wireless ? "Desconectar" : "Ejetar com seguranca";
    if (!d.wireless) {
      const kind = deviceKind(d);
      sendKind.value = kind;
      const savedFolder = loadPref(d, "folder");
      sendFolder.value = savedFolder !== null ? savedFolder : sendState.profiles[kind].folder;
    }
    renderProfileInfo();
  }
  renderSendBooks();
}

sendKind.addEventListener("change", () => {
  const d = currentDevice();
  if (!d) return;
  savePref(d, "kind", sendKind.value);
  sendFolder.value = sendState.profiles[sendKind.value].folder;
  savePref(d, "folder", sendFolder.value);
  sendState.results = {};
  renderDevices();
  renderProfileInfo();
  renderSendBooks();
});
sendFolder.addEventListener("change", () => {
  const d = currentDevice();
  if (d) savePref(d, "folder", sendFolder.value.trim());
});

// ─── Tabs: books to send / books already on the device ───
const deviceBooks = { list: [], loadedFor: "", seq: 0, checked: new Set(), results: {} };
const deviceBookList = $("#device-book-list");
const deviceFilter = $("#device-filter");

function showTab(which) {
  const dev = which === "device";
  $("#tab-send").classList.toggle("active", !dev);
  $("#tab-device").classList.toggle("active", dev);
  $("#tab-send").setAttribute("aria-selected", String(!dev));
  $("#tab-device").setAttribute("aria-selected", String(dev));
  $("#pane-send").hidden = dev;
  $("#pane-device").hidden = !dev;
}
$("#tab-send").addEventListener("click", () => showTab("send"));
$("#tab-device").addEventListener("click", () => showTab("device"));
$("#device-books-refresh").addEventListener("click", loadDeviceBooks);
deviceFilter.addEventListener("input", renderDeviceBooks);

function loadDeviceBooks() {
  const d = currentDevice();
  const seq = ++deviceBooks.seq;
  deviceBooks.list = [];
  if (deviceBooks.loadedFor !== (d ? d.id : "")) deviceBooks.checked.clear();
  deviceBooks.loadedFor = d ? d.id : "";
  updateDeviceActions();
  $("#device-book-count").textContent = "";
  if (!d) {
    $("#device-books-summary").textContent = "";
    deviceBookList.innerHTML = `<li class="send-empty">Nenhum dispositivo selecionado.</li>`;
    return;
  }
  $("#device-books-summary").textContent = `Lendo ${d.name}... (a primeira leitura de um cartao pode levar alguns segundos)`;
  deviceBookList.innerHTML = "";
  fetch("/api/devices/books?" + new URLSearchParams({ device: d.id }))
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then((data) => {
      if (seq !== deviceBooks.seq) return;
      deviceBooks.list = data.books || [];
      const n = deviceBooks.list.length;
      const inLib = deviceBooks.list.filter((b) => b.match).length;
      $("#device-book-count").textContent = `(${n})`;
      $("#device-books-summary").textContent =
        `${n} livro${n === 1 ? "" : "s"} · ${formatSize(data.total_size) || "0 B"} · ${inLib} na biblioteca · ${n - inLib} so no dispositivo`;
      renderDeviceBooks();
    })
    .catch((err) => {
      if (seq !== deviceBooks.seq) return;
      $("#device-books-summary").textContent = err.message;
    });
}

function renderDeviceBooks() {
  const q = deviceFilter.value.trim().toLowerCase();
  const rows = deviceBooks.list.filter(
    (b) => !q || b.title.toLowerCase().includes(q) || (b.authors || []).join(" ").toLowerCase().includes(q) || b.path.toLowerCase().includes(q)
  );
  if (rows.length === 0) {
    deviceBookList.innerHTML = deviceBooks.list.length
      ? `<li class="send-empty">Nenhum livro corresponde ao filtro.</li>`
      : deviceBooks.loadedFor ? `<li class="send-empty">Nenhum livro (EPUB, PDF, AZW3, MOBI) encontrado neste dispositivo.</li>` : "";
    return;
  }
  const badge = { exact: "na biblioteca", likely: "parecido na biblioteca" };
  deviceBookList.innerHTML = rows
    .map((b) => {
      const fmt = (b.format || "").toLowerCase();
      return `<li class="send-book device-book" data-path="${esc(b.path)}">
        <input type="checkbox" ${deviceBooks.checked.has(b.path) ? "checked" : ""} aria-label="Selecionar ${esc(b.title)}">
        <div class="send-book-text">
          <div class="send-book-title">${esc(b.title)}</div>
          <div class="send-book-author">${esc((b.authors || []).join(", ")) || "&nbsp;"}</div>
          <div class="device-book-path" title="${esc(b.path)}">${esc(b.path)} · ${esc(formatSize(b.size))}</div>
        </div>
        <span class="lib-badge" data-match="${esc(b.match || "")}">${esc(badge[b.match] || "so no dispositivo")}</span>
        <span class="list-format list-format-${esc(fmt)}">${esc(fmt.toUpperCase())}</span>
      </li>`;
    })
    .join("");
  updateDeviceActions();
}

deviceBookList.addEventListener("click", (e) => {
  const li = e.target.closest(".device-book");
  if (!li) return;
  const cb = li.querySelector("input");
  if (e.target !== cb) cb.checked = !cb.checked;
  if (cb.checked) deviceBooks.checked.add(li.dataset.path);
  else deviceBooks.checked.delete(li.dataset.path);
  updateDeviceActions();
});

$("#device-select-only").addEventListener("click", () => {
  deviceBooks.checked = new Set(deviceBooks.list.filter((b) => !b.match).map((b) => b.path));
  renderDeviceBooks();
});
$("#device-select-none").addEventListener("click", () => {
  deviceBooks.checked.clear();
  renderDeviceBooks();
});

function selectedDeviceBooks() {
  return deviceBooks.list.filter((b) => deviceBooks.checked.has(b.path));
}

// Books already in the library (even as a copy rewritten by another program) are not
// imported again; only EPUB and PDF can be.
function importable(b) {
  return !b.match && ["epub", "pdf"].includes((b.format || "").toLowerCase());
}

function updateDeviceActions() {
  const sel = selectedDeviceBooks();
  $("#device-delete").disabled = sel.length === 0;
  $("#device-delete").textContent = sel.length ? `Apagar ${sel.length} do dispositivo` : "Apagar do dispositivo";
  const imp = sel.filter(importable).length;
  $("#device-import").disabled = imp === 0;
  $("#device-import").textContent = imp ? `Trazer ${imp} para a biblioteca` : "Trazer para a biblioteca";
}

function deviceAction(path, paths) {
  const d = currentDevice();
  return fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ device: d.id, paths }),
  }).then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))));
}

$("#device-delete").addEventListener("click", () => {
  const d = currentDevice();
  const sel = selectedDeviceBooks();
  if (!d || sel.length === 0) return;
  const names = sel.slice(0, 10).map((b) => `• ${b.title}`).join("\n") + (sel.length > 10 ? `\n... e mais ${sel.length - 10}` : "");
  const msg = `Apagar ${sel.length} livro${sel.length > 1 ? "s" : ""} de ${d.name}?\n\n${names}\n\nOs arquivos serao apagados do dispositivo e isso nao pode ser desfeito. A biblioteca nao e alterada.`;
  if (!confirm(msg)) return;
  setSendStatus(`Apagando de ${d.name}...`);
  deviceAction("/api/devices/delete", sel.map((b) => b.path))
    .then((data) => {
      const errors = (data.results || []).filter((r) => r.status === "error");
      const ok = (data.results || []).length - errors.length;
      setSendStatus(
        `${ok} apagado${ok === 1 ? "" : "s"}` + (errors.length ? ` · ${errors.length} com erro: ${errors[0].error}` : ""),
        errors.length > 0
      );
      deviceBooks.checked.clear();
      loadDeviceBooks();
      loadDevices();
    })
    .catch((err) => setSendStatus(err.message, true));
});

$("#device-import").addEventListener("click", () => {
  const d = currentDevice();
  const sel = selectedDeviceBooks();
  const paths = sel.filter(importable).map((b) => b.path);
  if (!d || paths.length === 0) return;
  const ignored = sel.length - paths.length;
  setSendStatus(`Copiando ${paths.length} livro${paths.length > 1 ? "s" : ""} de ${d.name}...`);
  deviceAction("/api/devices/import", paths)
    .then((data) => {
      const res = data.results || [];
      const imported = res.filter((r) => r.status === "imported").length;
      const skipped = res.filter((r) => r.status === "skipped").length + ignored;
      const errors = res.filter((r) => r.status === "error");
      const parts = [`${imported} importado${imported === 1 ? "" : "s"} para a biblioteca`];
      if (skipped) parts.push(`${skipped} ja estava${skipped > 1 ? "m" : ""} na biblioteca`);
      if (errors.length) parts.push(`${errors.length} com erro: ${errors[0].error}`);
      setSendStatus(parts.join(" · "), errors.length > 0);
      deviceBooks.checked.clear();
      loadDeviceBooks();
      loadBooks();
      loadCounts();
      loadTags();
      // Newly imported books should be offered in the send tab too.
      fetch("/api/books?sort=title").then((r) => r.json()).then((x) => {
        sendState.books = x.books || [];
        renderSendBooks();
      });
    })
    .catch((err) => setSendStatus(err.message, true));
});

// ─── Folder browser inside the selected device ───
const folderBrowser = $("#send-folder-browser");
const folderCrumbs = $("#folder-crumbs");
const folderList = $("#folder-list");
const folderIcon = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 7a2 2 0 012-2h4l2 2h8a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2z"/></svg>`;
let browsePath = "";
let browseSeq = 0;

$("#send-folder-browse").addEventListener("click", () => {
  if (!folderBrowser.hidden) return (folderBrowser.hidden = true);
  folderBrowser.hidden = false;
  browseTo(sendFolder.value.trim());
});
$("#folder-cancel").addEventListener("click", () => (folderBrowser.hidden = true));
$("#folder-use").addEventListener("click", () => {
  sendFolder.value = browsePath;
  sendFolder.dispatchEvent(new Event("change"));
  folderBrowser.hidden = true;
});

function browseTo(path) {
  const d = currentDevice();
  if (!d) return;
  const seq = ++browseSeq;
  folderList.innerHTML = `<li class="folder-empty">Carregando...</li>`;
  const params = new URLSearchParams({ device: d.id, path });
  fetch("/api/devices/folders?" + params)
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then((res) => {
      if (seq !== browseSeq) return;
      browsePath = res.path;
      renderCrumbs(d);
      folderList.innerHTML = res.folders.length
        ? res.folders.map((f) => `<li class="folder-item" data-name="${esc(f)}">${folderIcon}<span>${esc(f)}</span></li>`).join("")
        : `<li class="folder-empty">Nenhuma subpasta. Use esta, ou digite um nome novo no campo para criar.</li>`;
    })
    .catch((err) => {
      if (seq !== browseSeq) return;
      // A typed folder that doesn't exist yet: fall back to the device root.
      if (path) return browseTo("");
      folderList.innerHTML = `<li class="folder-empty">${esc(err.message)}</li>`;
    });
}

function renderCrumbs(d) {
  const parts = browsePath ? browsePath.split("/") : [];
  const crumbs = [`<button type="button" data-path="">${esc(d.name)}</button>`];
  parts.forEach((p, i) => {
    const path = parts.slice(0, i + 1).join("/");
    crumbs.push("/");
    crumbs.push(
      i === parts.length - 1
        ? `<span class="crumb-current">${esc(p)}</span>`
        : `<button type="button" data-path="${esc(path)}">${esc(p)}</button>`
    );
  });
  folderCrumbs.innerHTML = crumbs.join("");
}

folderCrumbs.addEventListener("click", (e) => {
  const b = e.target.closest("button[data-path]");
  if (b) browseTo(b.dataset.path);
});
folderList.addEventListener("click", (e) => {
  const li = e.target.closest(".folder-item");
  if (li) browseTo(browsePath ? `${browsePath}/${li.dataset.name}` : li.dataset.name);
});

function currentProfile() {
  const d = currentDevice();
  if (!d) return null;
  return d.wireless ? sendState.profiles[d.kind] : sendState.profiles[sendKind.value];
}

function convertible(p) {
  return sendState.converter && p && p.convert ? Object.entries(p.convert) : [];
}

function renderProfileInfo() {
  const p = currentProfile();
  const conv = convertible(p);
  let formats = p ? p.formats.map((f) => f.toUpperCase()).join(", ") : "";
  if (conv.length) formats += ", " + conv.map(([from]) => from.toUpperCase()).join(", ");
  $("#send-formats").textContent = p ? `Formatos aceitos: ${formats}` : "";

  const note = $("#send-profile-note");
  let text = "";
  if (conv.length) {
    text = conv.map(([from, to]) => `${from.toUpperCase()} sera convertido para ${to.toUpperCase()} com o Calibre.`).join(" ") +
      " A primeira conversao de cada livro leva alguns segundos.";
  } else if (p && p.note) {
    text = p.note;
  }
  note.textContent = text;
  note.hidden = !text;
}

function bookSupported(b) {
  const p = currentProfile();
  if (!p) return true;
  const f = (b.format || "").toLowerCase();
  return p.formats.includes(f) || convertible(p).some(([from]) => from === f);
}

function renderSendBooks() {
  const q = sendFilter.value.trim().toLowerCase();
  const visible = sendState.books.filter(
    (b) => !q || b.title.toLowerCase().includes(q) || (b.authors || []).join(" ").toLowerCase().includes(q)
  );
  sendBookList.innerHTML = visible
    .map((b) => {
      const ok = bookSupported(b);
      const res = sendState.results[b.id];
      let resLabel = res ? SEND_RESULT[res.status] || res.status : "";
      if (res && res.converted && res.status === "sent") resLabel = "convertido e enviado";
      return `<li class="send-book${ok ? "" : " incompatible"}" data-book="${b.id}">
        <input type="checkbox" ${sendState.checked.has(b.id) ? "checked" : ""} aria-label="Selecionar ${esc(b.title)}">
        <div class="send-book-text">
          <div class="send-book-title">${esc(b.title)}</div>
          <div class="send-book-author">${esc((b.authors || []).join(", "))}</div>
        </div>
        <span class="list-format list-format-${esc((b.format || "").toLowerCase())}">${esc((b.format || "").toUpperCase())}</span>
        <span class="send-result" data-status="${res ? esc(res.status) : ""}" title="${esc(resultTip(res))}">${esc(resLabel)}</span>
      </li>`;
    })
    .join("");
  updateSendButton();
}

function resultTip(res) {
  if (!res) return "";
  if (res.error) return res.error;
  if (res.existing) return `Ja esta no dispositivo em: ${res.existing}`;
  return res.file ? `Gravado como: ${res.file}` : "";
}

sendFilter.addEventListener("input", renderSendBooks);

sendBookList.addEventListener("click", (e) => {
  const li = e.target.closest(".send-book");
  if (!li) return;
  const id = parseInt(li.dataset.book);
  const cb = li.querySelector("input");
  if (e.target !== cb) cb.checked = !cb.checked;
  if (cb.checked) sendState.checked.add(id);
  else sendState.checked.delete(id);
  updateSendButton();
});

$("#send-all").addEventListener("click", () => {
  sendBookList.querySelectorAll(".send-book:not(.incompatible)").forEach((li) => sendState.checked.add(parseInt(li.dataset.book)));
  renderSendBooks();
});
$("#send-none").addEventListener("click", () => {
  sendState.checked.clear();
  renderSendBooks();
});

function updateSendButton() {
  const n = sendState.checked.size;
  sendGo.disabled = n === 0 || !currentDevice();
  sendGo.textContent = n ? `Enviar ${n} livro${n > 1 ? "s" : ""}` : "Enviar";
}

sendGo.addEventListener("click", () => {
  const d = currentDevice();
  if (!d || sendState.checked.size === 0) return;
  sendGo.disabled = true;
  const willConvert = sendState.books.some((b) => {
    const p = currentProfile();
    return sendState.checked.has(b.id) && p && !p.formats.includes((b.format || "").toLowerCase()) && bookSupported(b);
  });
  setSendStatus(willConvert ? `Convertendo com o Calibre e copiando para ${d.name}...` : `Copiando para ${d.name}...`);
  fetch("/api/devices/send", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      device: d.id,
      kind: sendKind.value,
      folder: sendFolder.value.trim(),
      book_ids: [...sendState.checked],
      force: $("#send-force").checked,
    }),
  })
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then((data) => {
      const counts = {};
      for (const res of data.results || []) {
        sendState.results[res.book_id] = res;
        counts[res.status] = (counts[res.status] || 0) + 1;
      }
      d.free_bytes = data.free_bytes || d.free_bytes;
      renderDevices();
      renderSendBooks();
      if (counts.sent) loadDeviceBooks();
      const parts = [];
      if (counts.sent) parts.push(`${counts.sent} enviado${counts.sent > 1 ? "s" : ""}`);
      if (counts.exists) parts.push(`${counts.exists} ja estava${counts.exists > 1 ? "m" : ""} la`);
      if (counts.duplicate) parts.push(`${counts.duplicate} provave${counts.duplicate > 1 ? "is" : "l"} duplicado${counts.duplicate > 1 ? "s" : ""} (passe o mouse para ver onde)`);
      if (counts.unsupported) parts.push(`${counts.unsupported} em formato incompativel`);
      if (counts.error) parts.push(`${counts.error} com erro`);
      setSendStatus(parts.join(" · "), !!counts.error);
    })
    .catch((err) => setSendStatus(err.message, true))
    .finally(updateSendButton);
});

$("#send-eject").addEventListener("click", () => {
  const d = currentDevice();
  if (!d) return;
  setSendStatus(d.wireless ? `Desconectando ${d.name}...` : `Ejetando ${d.name}...`);
  fetch("/api/devices/eject", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ device: d.id }),
  })
    .then((r) => (r.ok ? null : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then(() => {
      setSendStatus(d.wireless ? `${d.name} foi desconectado.` : `${d.name} pode ser desconectado.`);
      loadDevices();
    })
    .catch((err) => setSendStatus("Nao foi possivel ejetar: " + err.message, true));
});

// ═══════════════════ Free books ═══════════════════
const freeModal = $("#free-modal");
const freeQuery = $("#free-query");
const freeLang = $("#free-lang");
const freeResults = $("#free-results");
const freeStatus = $("#free-status");
const free = { source: "gutenberg", results: [], seq: 0, added: {} };

const FREE_NOTES = {
  gutenberg: "Mais de 70 mil livros em dominio publico, inclusive classicos brasileiros e portugueses.",
  standard: "Classicos em ingles com revisao e diagramacao cuidadosas.",
};

$("#free-open").addEventListener("click", () => {
  freeModal.hidden = false;
  setFreeSource(free.source);
  freeQuery.focus();
});
for (const id of ["#free-close", "#free-done"]) $(id).addEventListener("click", () => (freeModal.hidden = true));
freeModal.addEventListener("click", (e) => {
  if (e.target === freeModal) freeModal.hidden = true;
});
document.querySelectorAll(".free-source").forEach((b) =>
  b.addEventListener("click", () => {
    setFreeSource(b.dataset.source);
    if (freeQuery.value.trim()) runFreeSearch();
  })
);

function setFreeSource(source) {
  free.source = source;
  document.querySelectorAll(".free-source").forEach((b) => {
    const on = b.dataset.source === source;
    b.classList.toggle("active", on);
    b.setAttribute("aria-checked", String(on));
  });
  freeLang.hidden = source !== "gutenberg";
  $("#free-source-note").textContent = FREE_NOTES[source];
}

$("#free-form").addEventListener("submit", (e) => {
  e.preventDefault();
  runFreeSearch();
});
freeLang.addEventListener("change", () => freeQuery.value.trim() && runFreeSearch());

function setFreeStatus(text, isError) {
  freeStatus.textContent = text;
  freeStatus.classList.toggle("error", !!isError);
}

function runFreeSearch() {
  const q = freeQuery.value.trim();
  if (!q) return;
  const seq = ++free.seq;
  const params = new URLSearchParams({ source: free.source, q });
  if (free.source === "gutenberg" && freeLang.value) params.set("lang", freeLang.value);
  freeResults.innerHTML = `<li class="free-empty">Buscando...</li>`;
  setFreeStatus("");
  fetch("/api/free/search?" + params)
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then((data) => {
      if (seq !== free.seq) return;
      free.results = data.results || [];
      renderFreeResults();
      const n = free.results.length;
      setFreeStatus(n ? `${n} resultado${n > 1 ? "s" : ""}` : "");
    })
    .catch((err) => {
      if (seq !== free.seq) return;
      freeResults.innerHTML = "";
      setFreeStatus(err.message, true);
    });
}

const LANG_SHORT = { pt: "PT", en: "EN", es: "ES", fr: "FR", de: "DE", it: "IT" };

function renderFreeResults() {
  if (free.results.length === 0) {
    freeResults.innerHTML = `<li class="free-empty">Nenhum livro encontrado.</li>`;
    return;
  }
  freeResults.innerHTML = free.results
    .map((r, i) => {
      const key = r.source + ":" + r.id;
      const state = free.added[key] || "";
      const label = { done: "Na biblioteca", busy: "Baixando...", error: "Tentar de novo" }[state] || "Adicionar";
      const langs = (r.languages || []).map((l) => LANG_SHORT[l] || l.toUpperCase()).join(" · ");
      const cover = r.cover_url
        ? `<img src="${esc(r.cover_url)}" alt="" loading="lazy" referrerpolicy="no-referrer" onerror="this.replaceWith(Object.assign(document.createElement('span'),{textContent:this.closest('li').dataset.title}))">`
        : `<span>${esc(r.title)}</span>`;
      return `<li class="free-result" data-idx="${i}" data-title="${esc(r.title)}">
        <div class="free-cover">${cover}</div>
        <div class="free-info">
          <div class="card-title" title="${esc(r.title)}">${esc(r.title)}</div>
          <div class="card-author">${esc((r.authors || []).join(", "))}</div>
          <div class="free-meta">${esc(langs)}${r.downloads ? ` · ${r.downloads.toLocaleString("pt-BR")} downloads` : ""}</div>
        </div>
        <button type="button" class="action-btn free-add" data-state="${state}" ${state === "busy" || state === "done" ? "disabled" : ""}>${label}</button>
      </li>`;
    })
    .join("");
}

freeResults.addEventListener("click", (e) => {
  const btn = e.target.closest(".free-add");
  if (!btn) return;
  const r = free.results[parseInt(btn.closest(".free-result").dataset.idx)];
  const key = r.source + ":" + r.id;
  free.added[key] = "busy";
  renderFreeResults();
  setFreeStatus(`Baixando "${r.title}"...`);
  fetch("/api/free/import", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ source: r.source, id: r.id }),
  })
    .then((res) => (res.ok ? res.json() : res.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then((data) => {
      free.added[key] = "done";
      setFreeStatus(data.status === "skipped" ? `"${r.title}" ja estava na biblioteca.` : `"${r.title}" adicionado a biblioteca.`);
      loadBooks();
      loadCounts();
      loadTags();
    })
    .catch((err) => {
      free.added[key] = "error";
      setFreeStatus(err.message, true);
    })
    .finally(renderFreeResults);
});

// ═══════════════════ Wi-Fi services ═══════════════════
const netModal = $("#net-modal");
const wlToggle = $("#wl-toggle");
const opdsToggle = $("#opds-toggle");
const opdsPort = $("#opds-port");
const opdsPassword = $("#opds-password");
const netError = $("#net-error");
const net = { status: null, timer: null, wifiConnected: false };

$("#net-open").addEventListener("click", () => {
  netModal.hidden = false;
  netError.hidden = true;
  opdsPassword.value = "";
  refreshNetwork(true);
});
for (const id of ["#net-close", "#net-done"]) $(id).addEventListener("click", () => (netModal.hidden = true));
netModal.addEventListener("click", (e) => {
  if (e.target === netModal) netModal.hidden = true;
});

function netRequest(path, body) {
  netError.hidden = true;
  return fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
    .then((r) => (r.ok ? r.json() : r.text().then((t) => Promise.reject(new Error(t.trim())))))
    .then(applyNetwork)
    .catch((err) => {
      netError.textContent = err.message;
      netError.hidden = false;
    });
}

wlToggle.addEventListener("click", () => {
  wlToggle.disabled = true;
  netRequest("/api/network/wireless", { on: !(net.status && net.status.wireless.running) }).finally(() => (wlToggle.disabled = false));
});

opdsToggle.addEventListener("click", () => {
  const on = !(net.status && net.status.opds.running);
  const body = { on };
  if (on) {
    const port = parseInt(opdsPort.value);
    if (port) body.port = port;
    // An empty field keeps the saved password; typing replaces it.
    if (opdsPassword.value !== "") body.password = opdsPassword.value;
  }
  opdsToggle.disabled = true;
  netRequest("/api/network/opds", body).finally(() => {
    opdsToggle.disabled = false;
    opdsPassword.value = "";
  });
});

function refreshNetwork(fillForm) {
  return fetch("/api/network")
    .then((r) => r.json())
    .then((st) => {
      applyNetwork(st);
      if (fillForm) opdsPort.value = st.opds_port;
    })
    .catch(() => {});
}

function applyNetwork(st) {
  net.status = st;
  const wl = st.wireless;
  const op = st.opds;
  const addr = (st.addresses || [])[0];

  wlToggle.textContent = wl.running ? "Desligar" : "Ligar";
  wlToggle.setAttribute("aria-pressed", String(wl.running));
  $("#wl-steps").hidden = !wl.running || !!wl.device;
  $("#wl-address").textContent = addr ? `${addr}:${wl.port}` : `porta ${wl.port}`;
  const wlLines = [];
  if (!wl.running) {
    wlLines.push(`<span>Desligado</span>`);
  } else if (wl.device) {
    wlLines.push(`<span class="connected">● Conectado: ${esc(wl.device.name)} (${esc(wl.device.address)})</span>`);
    wlLines.push(`<span>Envie livros em <strong>Enviar para dispositivo</strong>, escolhendo "${esc(wl.device.name)}".</span>`);
  } else {
    wlLines.push(`<span class="on">● Aguardando o aparelho na porta ${wl.port}</span>`);
    if (!wl.discovery || wl.discovery.length === 0) wlLines.push(`<span>A busca automatica nao esta disponivel; digite o endereco no aparelho.</span>`);
  }
  if (wl.last_error) wlLines.push(`<span class="edit-error">${esc(wl.last_error)}</span>`);
  $("#wl-status").innerHTML = wlLines.join("");

  opdsToggle.textContent = op.running ? "Desligar" : "Ligar";
  opdsToggle.setAttribute("aria-pressed", String(op.running));
  opdsPort.disabled = op.running;
  opdsPassword.disabled = op.running;
  opdsPassword.placeholder = st.opds_password ? "senha salva (digite para trocar)" : "sem senha";
  const opLines = [];
  if (!op.running) {
    opLines.push(`<span>Desligado</span>`);
  } else {
    opLines.push(`<span class="on">● Ligado${op.protected ? " com senha" : " sem senha"}. No aparelho, adicione o catalogo:</span>`);
    for (const u of op.urls || []) opLines.push(`<code>${esc(u)}</code>`);
    if (!op.urls || op.urls.length === 0) opLines.push(`<span>Nenhuma rede encontrada neste computador.</span>`);
    else opLines.push(`<span>Digite com <strong>http://</strong> (sem "s").${op.protected ? " Usuario: preencha com qualquer nome (o Xteink so envia a senha se houver usuario). Senha: a que voce definiu." : ""}</span>`);
  }
  $("#opds-status").innerHTML = opLines.join("");

  // Sidebar indicator: on while any service runs, green when a reader is connected.
  const ind = $("#net-indicator");
  ind.hidden = !(wl.running || op.running);
  ind.dataset.state = wl.device ? "connected" : "on";

  const connected = !!wl.device;
  if (connected !== net.wifiConnected) {
    net.wifiConnected = connected;
    if (!sendModal.hidden) loadDevices();
  }
  scheduleNetworkPoll(wl.running || op.running);
}

function scheduleNetworkPoll(active) {
  clearTimeout(net.timer);
  if (active) net.timer = setTimeout(() => refreshNetwork(false), netModal.hidden ? 5000 : 2000);
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
refreshNetwork(false);
