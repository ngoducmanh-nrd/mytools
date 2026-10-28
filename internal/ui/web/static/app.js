/**
 * mt — DevTools Split-Workspace Frontend (100% Vanilla ES6+)
 * Giữ nguyên giao thức API:
 *   - GET  /api/tools
 *   - POST /api/run          ({ id, params }) -> { run_id }
 *   - GET  /api/events?runID=<run_id> (SSE: ask, error, progress, result, ...)
 *   - POST /api/answer       ({ run_id, answer }) -> { ok: true }
 *   - POST /api/app/add      (Cài đặt / Thêm tool mới)
 */

const TOKEN_META = document.querySelector('meta[name="mt-token"]');
const TOKEN = TOKEN_META ? TOKEN_META.content : "";

/* ==========================================================================
   TRẠNG THÁI ỨNG DỤNG (STATE)
   ========================================================================== */
const state = {
  tools: [],
  selectedKind: "all",       // "all" | "fav" | kind cụ thể
  selectedTag: null,         // string | null
  searchQuery: "",
  viewMode: "grid",          // "grid" | "list"
  centerView: "tools",       // "tools" | "install"
  selectedToolId: null,      // ID của tool đang mở trong Dock cấu hình
  favorites: new Set(JSON.parse(localStorage.getItem("mt_favorites") || "[]")),
  recentParams: JSON.parse(localStorage.getItem("mt_recent_params") || "{}"),

  // Quản lý các phiên chạy (runs) trong Terminal Dock (Cột 3)
  runs: new Map(),           // runID -> RunSession
  activeRunId: null,         // runID đang hiển thị trên Terminal Dock
  autoScroll: true,

  // Phiên chạy cài đặt tool trong màn hình Install (Cột 2)
  installSession: {
    runId: null,
    status: "idle",
    startTime: 0,
    timerId: null,
    source: null,
    rawLog: ""
  }
};

/* ==========================================================================
   TIỆN ÍCH GỌI API & TOAST NOTIFICATION
   ========================================================================== */
async function api(path, body, method) {
  const opts = {
    headers: { "X-MT-Token": TOKEN }
  };
  if (body !== undefined) {
    opts.method = method || "POST";
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || `Lỗi HTTP ${res.status}`);
  }
  return data;
}

/**
 * Hiển thị Toast Notification hiện đại thay cho dòng text đỏ cũ
 * @param {string} message Nội dung thông báo
 * @param {"info"|"success"|"warn"|"error"} type Loại thông báo
 * @param {string} [title] Tiêu đề ngắn
 */
function showToast(message, type = "info", title = "") {
  const container = document.getElementById("toast-container");
  if (!container) return;

  const meta = {
    success: { icon: "✓", defaultTitle: "Thành công", cls: "toast-success" },
    error:   { icon: "✕", defaultTitle: "Đã xảy ra lỗi", cls: "toast-error" },
    warn:    { icon: "⚠", defaultTitle: "Chú ý", cls: "toast-warn" },
    info:    { icon: "ℹ", defaultTitle: "Thông báo", cls: "toast-info" }
  }[type] || { icon: "ℹ", defaultTitle: "Thông báo", cls: "toast-info" };

  const toast = document.createElement("div");
  toast.className = `toast ${meta.cls}`;

  const iconEl = document.createElement("div");
  iconEl.className = "toast-icon";
  iconEl.textContent = meta.icon;

  const contentEl = document.createElement("div");
  contentEl.className = "toast-content";

  const titleEl = document.createElement("div");
  titleEl.className = "toast-title";
  titleEl.textContent = title || meta.defaultTitle;

  const msgEl = document.createElement("p");
  msgEl.className = "toast-msg";
  msgEl.textContent = message;

  contentEl.append(titleEl, msgEl);

  const closeBtn = document.createElement("button");
  closeBtn.className = "toast-close";
  closeBtn.textContent = "✕";
  closeBtn.onclick = () => toast.remove();

  toast.append(iconEl, contentEl, closeBtn);
  container.appendChild(toast);

  setTimeout(() => {
    if (toast.parentNode) toast.remove();
  }, 4500);
}

/* ==========================================================================
   TIỆN ÍCH ĐỊNH DẠNG & ICON THEO KIND
   ========================================================================== */
function getKindIcon(kind = "") {
  const k = String(kind).toLowerCase();
  if (k.includes("cli") || k.includes("cmd") || k.includes("sh")) return "⌨";
  if (k.includes("gui") || k.includes("app") || k.includes("win")) return "❖";
  if (k.includes("url") || k.includes("web") || k.includes("link")) return "🌐";
  if (k.includes("git") || k.includes("py")) return "📦";
  if (k.includes("srv") || k.includes("service")) return "⚙";
  return "⚡";
}

function getKindClass(kind = "") {
  const k = String(kind).toLowerCase();
  if (k.includes("cli") || k.includes("cmd")) return "kind-cli";
  if (k.includes("gui")) return "kind-gui";
  if (k.includes("url") || k.includes("web")) return "kind-url";
  if (k.includes("git") || k.includes("app")) return "kind-git";
  return "";
}

function formatTimeNow() {
  const d = new Date();
  return d.toTimeString().split(" ")[0];
}

function formatDuration(ms) {
  const totalSec = Math.max(0, ms / 1000);
  const mins = Math.floor(totalSec / 60);
  const secs = (totalSec % 60).toFixed(1);
  return `${String(mins).padStart(2, "0")}:${secs.padStart(4, "0")}s`;
}

function hasParams(tool) {
  return Array.isArray(tool.params) && tool.params.length > 0;
}

/* ==========================================================================
   KHỞI TẠO & TẢI DANH SÁCH CÔNG CỤ (GET /api/tools)
   ========================================================================== */
async function loadTools(showSuccessToast = false, autoSelect = true) {
  const skeleton = document.getElementById("tools-skeleton");
  const listEl = document.getElementById("tool-list");
  const emptyEl = document.getElementById("tools-empty");

  skeleton.hidden = false;
  listEl.hidden = true;
  emptyEl.hidden = true;

  try {
    const tools = await api("/api/tools");
    state.tools = Array.isArray(tools) ? tools : [];
    skeleton.hidden = true;

    renderSidebarFilters();
    renderToolList();

    // Nếu chưa chọn tool nào ở Cột 3 và có danh sách tool, chọn sẵn tool đầu tiên
    if (autoSelect && !state.selectedToolId && state.tools.length > 0) {
      selectToolInDock(state.tools[0].id, false);
    } else if (state.selectedToolId) {
      const stillExists = state.tools.some((t) => t.id === state.selectedToolId);
      if (stillExists) selectToolInDock(state.selectedToolId, false);
    }

    if (showSuccessToast) {
      showToast(`Đã tải ${state.tools.length} công cụ từ hệ thống.`, "success", "Đã làm mới");
    }
  } catch (err) {
    skeleton.hidden = true;
    emptyEl.hidden = false;
    document.getElementById("empty-title").textContent = "Không thể tải danh sách công cụ";
    document.getElementById("empty-desc").textContent = err.message;
    showToast(err.message, "error", "Lỗi kết nối /api/tools");
  }
}

/* ==========================================================================
   CỘT 1: RENDER SIDEBAR FILTER THEO KIND & TAGS
   ========================================================================== */
function renderSidebarFilters() {
  const kindNav = document.getElementById("kind-filters");
  const tagCloud = document.getElementById("tag-filters");
  const clearTagBtn = document.getElementById("btn-clear-tag");

  kindNav.innerHTML = "";
  tagCloud.innerHTML = "";

  // Đếm số lượng theo kind và tags
  const kindCounts = new Map();
  const tagCounts = new Map();

  for (const t of state.tools) {
    const k = (t.kind || "other").toLowerCase();
    kindCounts.set(k, (kindCounts.get(k) || 0) + 1);

    if (Array.isArray(t.tags)) {
      for (const tag of t.tags) {
        if (!tag) continue;
        tagCounts.set(tag, (tagCounts.get(tag) || 0) + 1);
      }
    }
  }

  const favCount = state.tools.filter((t) => state.favorites.has(t.id)).length;

  // Mục "Tất cả" và "Yêu thích"
  const baseItems = [
    { key: "all", label: "Tất cả công cụ", icon: "⚡", count: state.tools.length },
    { key: "fav", label: "Đã ghim yêu thích", icon: "★", count: favCount }
  ];

  for (const [k, count] of kindCounts.entries()) {
    baseItems.push({
      key: k,
      label: `Loại: ${k.toUpperCase()}`,
      icon: getKindIcon(k),
      count
    });
  }

  for (const item of baseItems) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = `filter-item ${state.selectedKind === item.key && state.centerView === "tools" ? "active" : ""}`;

    const left = document.createElement("span");
    left.className = "filter-item-left";
    left.textContent = `${item.icon} ${item.label}`;

    const badge = document.createElement("span");
    badge.className = "filter-count";
    badge.textContent = String(item.count);

    btn.append(left, badge);
    btn.onclick = () => {
      state.selectedKind = item.key;
      switchCenterView("tools");
      renderSidebarFilters();
      renderToolList();
    };
    kindNav.appendChild(btn);
  }

  // Render Tag Cloud
  clearTagBtn.hidden = !state.selectedTag;
  if (tagCounts.size === 0) {
    const noTag = document.createElement("span");
    noTag.className = "field-hint";
    noTag.textContent = "Chưa có tag nào trong tools.toml";
    tagCloud.appendChild(noTag);
  } else {
    for (const [tag, count] of tagCounts.entries()) {
      const pill = document.createElement("button");
      pill.type = "button";
      pill.className = `tag-pill ${state.selectedTag === tag ? "active" : ""}`;
      pill.innerHTML = `<span>#${escapeHTML(tag)}</span><span class="tag-pill-count">${count}</span>`;
      pill.onclick = () => {
        state.selectedTag = state.selectedTag === tag ? null : tag;
        switchCenterView("tools");
        renderSidebarFilters();
        renderToolList();
      };
      tagCloud.appendChild(pill);
    }
  }
}

/* ==========================================================================
   CỘT 2 (VIEW A): RENDER DANH SÁCH TOOL & BỘ LỌC TÌM KIẾM
   ========================================================================== */
function getFilteredTools() {
  const q = state.searchQuery.trim().toLowerCase();

  return state.tools.filter((tool) => {
    // Lọc theo Kind / Favorites
    if (state.selectedKind === "fav" && !state.favorites.has(tool.id)) {
      return false;
    }
    if (
      state.selectedKind !== "all" &&
      state.selectedKind !== "fav" &&
      String(tool.kind || "other").toLowerCase() !== state.selectedKind
    ) {
      return false;
    }

    // Lọc theo Tag
    if (state.selectedTag) {
      if (!Array.isArray(tool.tags) || !tool.tags.includes(state.selectedTag)) {
        return false;
      }
    }

    // Lọc theo từ khóa Search (tên, id, target, tags)
    if (q) {
      const cleanQ = q.startsWith("#") ? q.slice(1) : q;
      const inName = String(tool.name || "").toLowerCase().includes(cleanQ);
      const inId = String(tool.id || "").toLowerCase().includes(cleanQ);
      const inTarget = String(tool.target || "").toLowerCase().includes(cleanQ);
      const inTags = Array.isArray(tool.tags) && tool.tags.some((t) => String(t).toLowerCase().includes(cleanQ));
      if (!inName && !inId && !inTarget && !inTags) return false;
    }

    return true;
  });
}

function renderToolList() {
  const listEl = document.getElementById("tool-list");
  const emptyEl = document.getElementById("tools-empty");
  const countEl = document.getElementById("filtered-count");
  const headingEl = document.getElementById("center-heading");
  const filterBar = document.getElementById("active-filter-bar");
  const filterText = document.getElementById("active-filter-text");

  // Cập nhật tiêu đề Cột 2
  if (state.selectedKind === "all") headingEl.textContent = "Tất cả công cụ";
  else if (state.selectedKind === "fav") headingEl.textContent = "Công cụ yêu thích";
  else headingEl.textContent = `Công cụ loại ${state.selectedKind.toUpperCase()}`;

  // Hiển thị chip báo đang lọc
  const activeFilters = [];
  if (state.selectedKind !== "all") activeFilters.push(`Kind: ${state.selectedKind}`);
  if (state.selectedTag) activeFilters.push(`#${state.selectedTag}`);
  if (state.searchQuery.trim()) activeFilters.push(`"${state.searchQuery.trim()}"`);

  if (activeFilters.length > 0) {
    filterBar.hidden = false;
    filterText.textContent = activeFilters.join(" • ");
  } else {
    filterBar.hidden = true;
  }

  const filtered = getFilteredTools();
  countEl.textContent = String(filtered.length);

  listEl.innerHTML = "";
  listEl.classList.toggle("list-mode", state.viewMode === "list");

  if (filtered.length === 0) {
    listEl.hidden = true;
    emptyEl.hidden = false;
    return;
  }

  emptyEl.hidden = true;
  listEl.hidden = false;

  // Sắp xếp công cụ được ghim yêu thích lên đầu
  const sorted = [...filtered].sort((a, b) => {
    const favA = state.favorites.has(a.id) ? 1 : 0;
    const favB = state.favorites.has(b.id) ? 1 : 0;
    return favB - favA;
  });

  for (const tool of sorted) {
    listEl.appendChild(createToolCard(tool));
  }
}

function createToolCard(tool) {
  const card = document.createElement("article");
  card.className = `tool-card ${state.selectedToolId === tool.id ? "selected" : ""}`;
  card.dataset.toolId = tool.id;

  // Phần trên: Icon + Tên + ID + Kind badge + Nút Yêu thích
  const top = document.createElement("div");
  top.className = "tool-card-top";

  const identity = document.createElement("div");
  identity.className = "tool-identity";

  const iconBox = document.createElement("div");
  iconBox.className = "tool-icon-box";
  iconBox.textContent = getKindIcon(tool.kind);

  const titleWrap = document.createElement("div");
  titleWrap.className = "tool-title-wrap";

  const nameEl = document.createElement("h3");
  nameEl.className = "tool-name";
  nameEl.textContent = tool.name || tool.id;

  const idSub = document.createElement("span");
  idSub.className = "tool-id-sub";
  idSub.textContent = tool.id;

  titleWrap.append(nameEl, idSub);
  identity.append(iconBox, titleWrap);

  const topRight = document.createElement("div");
  topRight.className = "tool-top-right";

  if (tool.kind) {
    const kindBadge = document.createElement("span");
    kindBadge.className = `kind-badge ${getKindClass(tool.kind)}`;
    kindBadge.textContent = tool.kind;
    topRight.appendChild(kindBadge);
  }

  const favBtn = document.createElement("button");
  favBtn.type = "button";
  const isFav = state.favorites.has(tool.id);
  favBtn.className = `fav-btn ${isFav ? "active" : ""}`;
  favBtn.title = isFav ? "Bỏ ghim yêu thích" : "Ghim lên đầu";
  favBtn.textContent = isFav ? "★" : "☆";
  favBtn.onclick = (e) => {
    e.stopPropagation();
    toggleFavorite(tool.id);
  };
  topRight.appendChild(favBtn);

  const delBtn = document.createElement("button");
  delBtn.type = "button";
  delBtn.className = "card-delete-btn";
  delBtn.title = `Xóa "${tool.name || tool.id}" khỏi hệ thống`;
  delBtn.textContent = "🗑";
  delBtn.onclick = (e) => {
    e.stopPropagation();
    deleteTool(tool);
  };
  topRight.appendChild(delBtn);

  top.append(identity, topRight);
  card.appendChild(top);

  // Hiển thị đường dẫn / target nếu có
  if (tool.target) {
    const targetLine = document.createElement("div");
    targetLine.className = "tool-target-line";
    targetLine.title = tool.target;
    targetLine.textContent = tool.target;
    card.appendChild(targetLine);
  }

  // Phần dưới: Tags + Nút hành động ("Chạy ngay" nếu không có param, hoặc "Cấu hình" nếu có param)
  const bottom = document.createElement("div");
  bottom.className = "tool-card-bottom";

  const tagsRow = document.createElement("div");
  tagsRow.className = "tool-tags-row";
  if (Array.isArray(tool.tags) && tool.tags.length > 0) {
    for (const t of tool.tags) {
      const tagSpan = document.createElement("span");
      tagSpan.className = "card-tag";
      tagSpan.textContent = `#${t}`;
      tagSpan.onclick = (e) => {
        e.stopPropagation();
        state.selectedTag = state.selectedTag === t ? null : t;
        renderSidebarFilters();
        renderToolList();
      };
      tagsRow.appendChild(tagSpan);
    }
  } else {
    const noTag = document.createElement("span");
    noTag.className = "field-hint";
    noTag.textContent = hasParams(tool) ? `${tool.params.length} tham số` : "Thực thi nhanh";
    tagsRow.appendChild(noTag);
  }

  const actionBtn = document.createElement("button");
  actionBtn.type = "button";

  if (!hasParams(tool)) {
    // Tool KHÔNG có param -> Nút "Chạy ngay"
    actionBtn.className = "btn btn-run-now";
    actionBtn.innerHTML = `<span>▶ Chạy ngay</span>`;
    actionBtn.onclick = (e) => {
      e.stopPropagation();
      selectToolInDock(tool.id, false);
      executeToolRun(tool, {});
    };
  } else {
    // Tool CÓ param -> Mở form nhập liệu gọn gàng ở Cột 3 & focus
    actionBtn.className = "btn btn-config-run";
    actionBtn.innerHTML = `<span>⚙ Cấu hình (${tool.params.length})</span>`;
    actionBtn.onclick = (e) => {
      e.stopPropagation();
      selectToolInDock(tool.id, true);
    };
  }

  bottom.append(tagsRow, actionBtn);
  card.appendChild(bottom);

  // Bấm vào thân Card sẽ chọn tool đó lên Cột 3
  card.onclick = () => {
    selectToolInDock(tool.id, hasParams(tool));
  };

  return card;
}

function toggleFavorite(toolId) {
  if (state.favorites.has(toolId)) {
    state.favorites.delete(toolId);
  } else {
    state.favorites.add(toolId);
  }
  localStorage.setItem("mt_favorites", JSON.stringify([...state.favorites]));
  renderSidebarFilters();
  renderToolList();
}

/* ==========================================================================
   CỘT 3 (PHẦN TRÊN): FORM CẤU HÌNH THAM SỐ & CHẠY TOOL
   ========================================================================== */
function selectToolInDock(toolId, focusFirstInput = false) {
  const tool = state.tools.find((t) => t.id === toolId);
  if (!tool) return;

  state.selectedToolId = tool.id;

  // Highlight card đang chọn ở Cột 2
  document.querySelectorAll(".tool-card").forEach((el) => {
    el.classList.toggle("selected", el.dataset.toolId === tool.id);
  });

  const iconEl = document.getElementById("dock-tool-icon");
  const nameEl = document.getElementById("dock-tool-name");
  const metaEl = document.getElementById("dock-tool-meta");
  const noToolEl = document.getElementById("dock-no-tool");
  const formEl = document.getElementById("dock-param-form");
  const paramsContainer = document.getElementById("dock-params-container");
  const resetParamsBtn = document.getElementById("btn-reset-params");
  const runLabel = document.getElementById("btn-run-selected-label");
  const toggleBtn = document.getElementById("btn-toggle-config");
  const configBody = document.getElementById("dock-config-body");

  iconEl.textContent = getKindIcon(tool.kind);
  nameEl.textContent = tool.name || tool.id;
  metaEl.textContent = `${tool.id} • ${(tool.kind || "tool").toUpperCase()}${
    tool.target ? ` • ${tool.target}` : ""
  }`;

  noToolEl.hidden = true;
  formEl.hidden = false;
  toggleBtn.hidden = false;
  document.getElementById("btn-delete-tool").hidden = false;
  configBody.hidden = false;
  toggleBtn.textContent = "Thu gọn";

  paramsContainer.innerHTML = "";
  const savedValues = state.recentParams[tool.id] || {};

  if (!hasParams(tool)) {
    resetParamsBtn.hidden = true;
    runLabel.textContent = "Chạy ngay";
    const info = document.createElement("div");
    info.className = "no-param-banner";
    info.textContent = "Công cụ này không yêu cầu tham số đầu vào. Nhấn \"Chạy ngay\" để khởi chạy.";
    paramsContainer.appendChild(info);
  } else {
    resetParamsBtn.hidden = false;
    runLabel.textContent = "Chạy";
    for (const p of tool.params) {
      paramsContainer.appendChild(createParamField(p, savedValues[p.name]));
    }
    if (focusFirstInput) {
      const firstInput = paramsContainer.querySelector("input:not([type='checkbox']), select");
      if (firstInput) firstInput.focus();
    }
  }
}

function clearDockSelection() {
  state.selectedToolId = null;
  document.querySelectorAll(".tool-card").forEach((el) => el.classList.remove("selected"));

  document.getElementById("dock-tool-icon").textContent = "⚡";
  document.getElementById("dock-tool-name").textContent = "Chưa chọn công cụ";
  document.getElementById("dock-tool-meta").textContent = "Chọn 1 tool ở cột giữa để cấu hình hoặc chạy ngay";
  document.getElementById("dock-no-tool").hidden = false;
  document.getElementById("dock-param-form").hidden = true;
  document.getElementById("btn-toggle-config").hidden = true;
  document.getElementById("btn-delete-tool").hidden = true;
  document.getElementById("dock-config-body").hidden = false;
  document.getElementById("dock-params-container").innerHTML = "";
}

/* ==========================================================================
   HỘP THOẠI XÁC NHẬN & XÓA TOOL (DELETE /api/app/remove)
   ========================================================================== */
function showConfirmDialog(message, title = "Xóa công cụ") {
  const overlay = document.getElementById("confirm-dialog");
  const titleEl = document.getElementById("confirm-title");
  const msgEl = document.getElementById("confirm-message");
  const okBtn = document.getElementById("btn-confirm-ok");
  const cancelBtn = document.getElementById("btn-confirm-cancel");

  titleEl.textContent = title;
  msgEl.textContent = message;
  overlay.hidden = false;
  okBtn.focus();

  return new Promise((resolve) => {
    const finish = (result) => {
      overlay.hidden = true;
      overlay.onclick = null;
      okBtn.onclick = null;
      cancelBtn.onclick = null;
      document.removeEventListener("keydown", onKeydown);
      resolve(result);
    };
    const onKeydown = (e) => {
      if (e.key === "Escape") finish(false);
    };
    okBtn.onclick = () => finish(true);
    cancelBtn.onclick = () => finish(false);
    overlay.onclick = (e) => {
      if (e.target === overlay) finish(false);
    };
    document.addEventListener("keydown", onKeydown);
  });
}

async function deleteTool(tool) {
  const displayName = tool.name || tool.id;
  const confirmed = await showConfirmDialog(`Bạn có chắc muốn xóa tool "${displayName}" không?`);
  if (!confirmed) return;

  try {
    await api("/api/app/remove", { id: tool.id }, "DELETE");
  } catch (err) {
    showToast(err.message, "error", "Xóa thất bại");
    return;
  }

  state.favorites.delete(tool.id);
  localStorage.setItem("mt_favorites", JSON.stringify([...state.favorites]));
  delete state.recentParams[tool.id];
  localStorage.setItem("mt_recent_params", JSON.stringify(state.recentParams));

  if (state.selectedToolId === tool.id) {
    clearDockSelection();
  }

  showToast(`Đã xóa tool "${displayName}" khỏi hệ thống.`, "success", "Đã xóa công cụ");
  await loadTools(false, false);
}

function createParamField(param, savedVal) {
  const wrap = document.createElement("div");
  wrap.className = "form-field";

  const type = String(param.type || "string").toLowerCase();
  const initialVal = savedVal !== undefined ? savedVal : param.default;

  if (type === "bool" || type === "boolean") {
    const label = document.createElement("label");
    label.className = "switch-label";

    const input = document.createElement("input");
    input.type = "checkbox";
    input.name = param.name;
    input.dataset.paramType = "bool";
    input.checked = initialVal === true || initialVal === "true";

    const slider = document.createElement("span");
    slider.className = "switch-slider";

    const text = document.createElement("span");
    text.className = "switch-text";
    text.textContent = param.name;

    label.append(input, slider, text);
    wrap.appendChild(label);
    return wrap;
  }

  const label = document.createElement("label");
  const titleSpan = document.createElement("span");
  titleSpan.textContent = param.name;
  if (param.required) {
    const req = document.createElement("span");
    req.className = "req";
    req.textContent = "*";
    titleSpan.appendChild(req);
  }

  const typeBadge = document.createElement("span");
  typeBadge.className = "param-type-tag";
  typeBadge.textContent = type;
  label.append(titleSpan, typeBadge);
  wrap.appendChild(label);

  let input;
  if (type === "enum" || (Array.isArray(param.choices) && param.choices.length > 0)) {
    input = document.createElement("select");
    for (const c of param.choices || []) {
      const opt = document.createElement("option");
      opt.value = c;
      opt.textContent = c;
      if (String(initialVal) === String(c)) opt.selected = true;
      input.appendChild(opt);
    }
  } else {
    input = document.createElement("input");
    input.type = type === "int" || type === "integer" || type === "number" ? "number" : "text";
    if (initialVal !== undefined && initialVal !== null) {
      input.value = String(initialVal);
    }
    input.placeholder = param.required
      ? `Nhập ${param.name} (bắt buộc)...`
      : param.default !== undefined && param.default !== ""
      ? `Mặc định: ${param.default}`
      : `Nhập ${param.name} (tùy chọn)...`;
  }

  input.name = param.name;
  input.required = Boolean(param.required);
  wrap.appendChild(input);
  return wrap;
}

function collectDockParams(tool) {
  const params = {};
  if (!hasParams(tool)) return params;

  const container = document.getElementById("dock-params-container");
  for (const p of tool.params) {
    const el = container.querySelector(`[name="${CSS.escape(p.name)}"]`);
    if (!el) continue;
    if (el.type === "checkbox") {
      params[p.name] = el.checked ? "true" : "false";
    } else {
      params[p.name] = String(el.value ?? "").trim();
    }
  }
  return params;
}

/* ==========================================================================
   CỘT 3 (PHẦN DƯỚI): THỰC THI TOOL (POST /api/run) & TERMINAL SSE (/api/events)
   ========================================================================== */
async function executeToolRun(tool, params) {
  // Lưu lại lịch sử tham số vào localStorage
  if (hasParams(tool)) {
    state.recentParams[tool.id] = params;
    localStorage.setItem("mt_recent_params", JSON.stringify(state.recentParams));
  }

  const runBtn = document.getElementById("btn-run-selected");
  if (runBtn) runBtn.disabled = true;

  try {
    const res = await api("/api/run", { id: tool.id, params });
    const runID = res.run_id;
    if (!runID) throw new Error("Server không trả về run_id hợp lệ.");

    showToast(`Đã khởi chạy "${tool.name || tool.id}" (ID: ${runID})`, "info", "Đang chạy tiến trình");
    startTerminalSession(runID, tool);
  } catch (err) {
    showToast(err.message, "error", `Không thể chạy ${tool.name || tool.id}`);
  } finally {
    if (runBtn) runBtn.disabled = false;
  }
}

function startTerminalSession(runID, tool) {
  const session = {
    runID,
    toolId: tool.id,
    toolName: tool.name || tool.id,
    status: "running",        // "running" | "ask" | "ok" | "fail"
    startTime: performance.now(),
    elapsedMs: 0,
    timerId: null,
    pct: null,
    lines: [
      {
        time: formatTimeNow(),
        text: `$ Khởi chạy [${tool.id}] — phiên #${runID}`,
        cls: "log-info"
      }
    ],
    rawLog: `$ Khởi chạy [${tool.id}] — phiên #${runID}\n`,
    askPrompt: null,
    source: null
  };

  state.runs.set(runID, session);
  state.activeRunId = runID;

  // Bộ đếm thời gian thực 100ms
  session.timerId = setInterval(() => {
    if (session.status === "running" || session.status === "ask") {
      session.elapsedMs = performance.now() - session.startTime;
      if (state.activeRunId === runID) {
        document.getElementById("term-timer").textContent = formatDuration(session.elapsedMs);
      }
    }
  }, 100);

  // Mở kết nối Server-Sent Events (/api/events?runID=...)
  const source = new EventSource("/api/events?runID=" + encodeURIComponent(runID));
  session.source = source;

  source.onmessage = (message) => {
    let event;
    try {
      event = JSON.parse(message.data);
    } catch {
      appendSessionLog(session, message.data, "");
      return;
    }
    handleSessionSSEEvent(session, event);
  };

  source.addEventListener("closed", () => {
    finishSession(session, session.status === "fail" ? "fail" : "ok");
    source.close();
  });

  source.onerror = () => {
    if (session.status === "running" || session.status === "ask") {
      finishSession(session, "ok");
    }
    source.close();
  };

  updateGlobalRunIndicator();
  renderRunTabs();
  renderActiveTerminal();
}

function handleSessionSSEEvent(session, event) {
  switch (event.kind) {
    case "ask": {
      session.status = "ask";
      session.askPrompt = event.text || "Nhập phản hồi cho công cụ:";
      appendSessionLog(session, `[ASK] ${session.askPrompt}`, "log-warn");
      showToast(session.askPrompt, "warn", `${session.toolName} cần bạn nhập liệu`);
      break;
    }
    case "error": {
      session.status = "fail";
      appendSessionLog(session, `[ERROR] ${event.text || "Lỗi thực thi"}`, "log-err");
      showToast(event.text || "Lỗi trong quá trình chạy công cụ", "error", `Lỗi: ${session.toolName}`);
      break;
    }
    case "progress": {
      if (typeof event.pct === "number") {
        session.pct = Math.max(0, Math.min(100, Math.round(event.pct)));
      }
      if (event.text) {
        appendSessionLog(session, event.text, "log-muted");
      }
      break;
    }
    case "result": {
      const isOk = event.data && event.data.ok === false ? false : true;
      session.status = isOk ? "ok" : "fail";
      if (event.text) {
        appendSessionLog(session, event.text, isOk ? "log-ok" : "log-err");
      }
      break;
    }
    default: {
      if (event.text) {
        appendSessionLog(session, event.text, "");
      }
      break;
    }
  }

  updateGlobalRunIndicator();
  renderRunTabs();
  if (state.activeRunId === session.runID) {
    renderActiveTerminal();
  }
}

function appendSessionLog(session, text, cls = "") {
  const time = formatTimeNow();
  session.lines.push({ time, text, cls });
  session.rawLog += `[${time}] ${text}\n`;

  if (state.activeRunId === session.runID) {
    const out = document.getElementById("term-output");
    out.appendChild(createLogLineDOM({ time, text, cls }));
    if (state.autoScroll) {
      out.scrollTop = out.scrollHeight;
    }
  }
}

function finishSession(session, finalStatus) {
  if (session.timerId) {
    clearInterval(session.timerId);
    session.timerId = null;
  }
  session.elapsedMs = performance.now() - session.startTime;
  if (session.status === "running" || session.status === "ask") {
    session.status = finalStatus;
  }
  session.askPrompt = null;

  appendSessionLog(
    session,
    `✓ Phiên chạy kết thúc (${formatDuration(session.elapsedMs)})`,
    session.status === "fail" ? "log-err" : "log-ok"
  );

  updateGlobalRunIndicator();
  renderRunTabs();
  if (state.activeRunId === session.runID) {
    renderActiveTerminal();
  }
}

function createLogLineDOM(lineObj) {
  const span = document.createElement("span");
  span.className = `log-line ${lineObj.cls || ""}`;

  const timeEl = document.createElement("span");
  timeEl.className = "log-time";
  timeEl.textContent = `[${lineObj.time}]`;

  const textNode = document.createTextNode(lineObj.text);
  span.append(timeEl, textNode);
  return span;
}

function renderRunTabs() {
  const tabsList = document.getElementById("run-tabs-list");
  const clearAllBtn = document.getElementById("btn-clear-all-runs");
  tabsList.innerHTML = "";

  clearAllBtn.hidden = state.runs.size === 0;

  for (const [runID, session] of state.runs.entries()) {
    const tab = document.createElement("button");
    tab.type = "button";
    tab.className = `run-tab ${state.activeRunId === runID ? "active" : ""}`;

    const dot = document.createElement("span");
    dot.className = `status-dot ${
      session.status === "running"
        ? "dot-running"
        : session.status === "ask"
        ? "dot-ask"
        : "dot-idle"
    }`;
    if (session.status === "ok") dot.style.background = "var(--ok)";
    if (session.status === "fail") dot.style.background = "var(--fail)";

    const label = document.createElement("span");
    label.textContent = `${session.toolName}`;

    const closeBtn = document.createElement("span");
    closeBtn.className = "run-tab-close";
    closeBtn.textContent = "✕";
    closeBtn.title = "Đóng tab này";
    closeBtn.onclick = (e) => {
      e.stopPropagation();
      closeRunTab(runID);
    };

    tab.append(dot, label, closeBtn);
    tab.onclick = () => {
      state.activeRunId = runID;
      renderRunTabs();
      renderActiveTerminal();
    };
    tabsList.appendChild(tab);
  }
}

function closeRunTab(runID) {
  const session = state.runs.get(runID);
  if (!session) return;
  if (session.source) session.source.close();
  if (session.timerId) clearInterval(session.timerId);
  state.runs.delete(runID);

  if (state.activeRunId === runID) {
    const remainingKeys = [...state.runs.keys()];
    state.activeRunId = remainingKeys.length > 0 ? remainingKeys[remainingKeys.length - 1] : null;
  }
  updateGlobalRunIndicator();
  renderRunTabs();
  renderActiveTerminal();
}

function renderActiveTerminal() {
  const titleEl = document.getElementById("term-active-title");
  const badgeEl = document.getElementById("term-status-badge");
  const timerEl = document.getElementById("term-timer");
  const progWrap = document.getElementById("term-progress-wrap");
  const progFill = document.getElementById("term-progress-fill");
  const progPct = document.getElementById("term-progress-pct");
  const outputEl = document.getElementById("term-output");
  const askBox = document.getElementById("term-ask-box");
  const askText = document.getElementById("term-ask-text");
  const askInput = document.getElementById("term-ask-input");

  const session = state.activeRunId ? state.runs.get(state.activeRunId) : null;
  if (!session) {
    titleEl.textContent = "Terminal SSE";
    badgeEl.className = "term-status status-idle";
    badgeEl.textContent = "CHỜ LỆNH";
    timerEl.textContent = "00:00.0s";
    progWrap.hidden = true;
    askBox.hidden = true;
    outputEl.innerHTML = `<span class="log-line log-muted">$ Sẵn sàng kết nối luồng sự kiện Server-Sent Events (/api/events)...</span>`;
    return;
  }

  titleEl.textContent = `${session.toolName} (#${session.runID.slice(0, 8)})`;
  timerEl.textContent = formatDuration(session.elapsedMs);

  const statusMeta = {
    running: { text: "ĐANG CHẠY", cls: "status-running" },
    ask:     { text: "CHỜ PHẢN HỒI", cls: "status-ask" },
    ok:      { text: "HOÀN TẤT", cls: "status-ok" },
    fail:    { text: "LỖI", cls: "status-fail" }
  }[session.status] || { text: "CHỜ LỆNH", cls: "status-idle" };

  badgeEl.className = `term-status ${statusMeta.cls}`;
  badgeEl.textContent = statusMeta.text;

  // Progress bar
  if (typeof session.pct === "number") {
    progWrap.hidden = false;
    progFill.style.width = `${session.pct}%`;
    progPct.textContent = `${session.pct}%`;
  } else {
    progWrap.hidden = true;
  }

  // Render các dòng log
  outputEl.innerHTML = "";
  for (const line of session.lines) {
    outputEl.appendChild(createLogLineDOM(line));
  }
  if (state.autoScroll) {
    outputEl.scrollTop = outputEl.scrollHeight;
  }

  // Ô nhập liệu nổi bật khi có sự kiện ask
  if (session.status === "ask" && session.askPrompt) {
    askBox.hidden = false;
    askText.textContent = session.askPrompt;
    askInput.focus();
  } else {
    askBox.hidden = true;
  }
}

async function submitTerminalAskAnswer() {
  const session = state.activeRunId ? state.runs.get(state.activeRunId) : null;
  if (!session) return;

  const input = document.getElementById("term-ask-input");
  const answer = input.value;
  const sendBtn = document.getElementById("btn-term-ask-send");

  try {
    sendBtn.disabled = true;
    await api("/api/answer", { run_id: session.runID, answer });
    appendSessionLog(session, `> Phản hồi đã gửi: ${answer}`, "log-info");
    input.value = "";
    session.status = "running";
    session.askPrompt = null;
    updateGlobalRunIndicator();
    renderRunTabs();
    renderActiveTerminal();
  } catch (err) {
    showToast(err.message, "error", "Gửi câu trả lời thất bại");
  } finally {
    sendBtn.disabled = false;
  }
}

function updateGlobalRunIndicator() {
  let runningCount = 0;
  let hasAsk = false;
  for (const s of state.runs.values()) {
    if (s.status === "running" || s.status === "ask") runningCount++;
    if (s.status === "ask") hasAsk = true;
  }

  const dot = document.getElementById("global-run-dot");
  const label = document.getElementById("global-run-count");

  dot.className = `status-dot ${hasAsk ? "dot-ask" : runningCount > 0 ? "dot-running" : "dot-idle"}`;
  label.textContent = hasAsk
    ? `${runningCount} chờ nhập liệu!`
    : `${runningCount} đang chạy`;
}

/* ==========================================================================
   CỘT 2 (VIEW B): MÀN HÌNH CÀI ĐẶT TOOL (POST /api/app/add + LOG STREAM)
   ========================================================================== */
function switchCenterView(viewName) {
  state.centerView = viewName;
  const viewTools = document.getElementById("view-tools");
  const viewInstall = document.getElementById("view-install");
  const sideInstallBtn = document.getElementById("sidebar-btn-install");

  if (viewName === "install") {
    viewTools.hidden = true;
    viewInstall.hidden = false;
    sideInstallBtn.classList.add("active");
    document.getElementById("inst-id").focus();
  } else {
    viewTools.hidden = false;
    viewInstall.hidden = true;
    sideInstallBtn.classList.remove("active");
  }
}

function appendInstallLog(text, cls = "") {
  const time = formatTimeNow();
  state.installSession.rawLog += `[${time}] ${text}\n`;
  const out = document.getElementById("install-log-output");
  out.appendChild(createLogLineDOM({ time, text, cls }));
  out.scrollTop = out.scrollHeight;
}

async function handleInstallSubmit(e) {
  e.preventDefault();

  const id = document.getElementById("inst-id").value.trim();
  const name = document.getElementById("inst-name").value.trim();
  const kind = document.getElementById("inst-kind").value;
  const target = document.getElementById("inst-target").value.trim();
  const entry = document.getElementById("inst-entry").value.trim();
  const rawTags = document.getElementById("inst-tags").value.trim();
  const venv = document.getElementById("inst-venv").checked;

  const tags = rawTags
    ? rawTags.split(",").map((s) => s.trim()).filter(Boolean)
    : [];

  const submitBtn = document.getElementById("btn-install-submit");
  const badge = document.getElementById("install-status-badge");
  const timerEl = document.getElementById("install-timer");
  const progWrap = document.getElementById("install-progress-wrap");
  const progFill = document.getElementById("install-progress-fill");
  const progPct = document.getElementById("install-progress-pct");
  const out = document.getElementById("install-log-output");

  // Khởi tạo trạng thái log cài đặt
  if (state.installSession.source) state.installSession.source.close();
  if (state.installSession.timerId) clearInterval(state.installSession.timerId);

  out.innerHTML = "";
  state.installSession.rawLog = "";
  state.installSession.startTime = performance.now();
  badge.className = "term-status status-running";
  badge.textContent = "ĐANG CÀI ĐẶT";
  progWrap.hidden = true;
  submitBtn.disabled = true;

  state.installSession.timerId = setInterval(() => {
    const ms = performance.now() - state.installSession.startTime;
    timerEl.textContent = formatDuration(ms);
  }, 100);

  appendInstallLog(`$ Bắt đầu cài đặt công cụ "${name}" (ID: ${id}, Kind: ${kind})...`, "log-info");
  appendInstallLog(`$ Nguồn: ${target}`, "log-muted");

  try {
    const payload = { id, name, kind, target, url: target, entry, tags, venv };
    const res = await api("/api/app/add", payload);

    // Nếu backend trả về run_id -> mở luồng SSE /api/events?runID=... để stream log cài đặt trực tiếp
    if (res && res.run_id) {
      const runID = res.run_id;
      state.installSession.runId = runID;
      appendInstallLog(`$ Đã kết nối luồng cài đặt SSE (runID: ${runID})...`, "log-info");

      const source = new EventSource("/api/events?runID=" + encodeURIComponent(runID));
      state.installSession.source = source;

      source.onmessage = (message) => {
        let ev;
        try {
          ev = JSON.parse(message.data);
        } catch {
          appendInstallLog(message.data, "");
          return;
        }

        if (ev.kind === "progress") {
          if (typeof ev.pct === "number") {
            const pct = Math.max(0, Math.min(100, Math.round(ev.pct)));
            progWrap.hidden = false;
            progFill.style.width = `${pct}%`;
            progPct.textContent = `${pct}%`;
          }
          if (ev.text) appendInstallLog(ev.text, "log-muted");
        } else if (ev.kind === "ask") {
          badge.className = "term-status status-ask";
          badge.textContent = "CHỜ XÁC NHẬN";
          const askBox = document.getElementById("install-ask-box");
          document.getElementById("install-ask-text").textContent = ev.text || "Xác nhận:";
          askBox.hidden = false;
          document.getElementById("install-ask-input").focus();
          appendInstallLog(`[ASK] ${ev.text}`, "log-warn");
        } else if (ev.kind === "error") {
          badge.className = "term-status status-fail";
          badge.textContent = "LỖI";
          appendInstallLog(`[ERROR] ${ev.text}`, "log-err");
          showToast(ev.text || "Cài đặt thất bại", "error", "Lỗi cài đặt");
        } else if (ev.kind === "result") {
          const ok = !(ev.data && ev.data.ok === false);
          badge.className = `term-status ${ok ? "status-ok" : "status-fail"}`;
          badge.textContent = ok ? "HOÀN TẤT" : "LỖI";
          if (ev.text) appendInstallLog(ev.text, ok ? "log-ok" : "log-err");
        } else if (ev.text) {
          appendInstallLog(ev.text, "");
        }
      };

      source.addEventListener("closed", () => {
        clearInterval(state.installSession.timerId);
        document.getElementById("install-ask-box").hidden = true;
        if (badge.textContent !== "LỖI") {
          badge.className = "term-status status-ok";
          badge.textContent = "HOÀN TẤT";
          appendInstallLog("✓ Cài đặt hoàn tất! Đang đồng bộ danh sách công cụ...", "log-ok");
          showToast(`Đã cài đặt xong "${name}".`, "success", "Cài đặt hoàn tất");
          loadTools(false);
        }
        submitBtn.disabled = false;
        source.close();
      });

      source.onerror = () => {
        clearInterval(state.installSession.timerId);
        submitBtn.disabled = false;
        source.close();
      };
    } else {
      // Trường hợp /api/app/add trả về kết quả trực tiếp
      clearInterval(state.installSession.timerId);
      badge.className = "term-status status-ok";
      badge.textContent = "HOÀN TẤT";
      if (res && res.text) appendInstallLog(res.text, "log-ok");
      appendInstallLog(`✓ Đã thêm công cụ "${name}" thành công!`, "log-ok");
      showToast(`Đã cài đặt công cụ "${name}" thành công.`, "success", "Cài đặt thành công");
      submitBtn.disabled = false;
      await loadTools(false);
    }
  } catch (err) {
    clearInterval(state.installSession.timerId);
    badge.className = "term-status status-fail";
    badge.textContent = "LỖI";
    appendInstallLog(`[ERROR] ${err.message}`, "log-err");
    showToast(err.message, "error", "Lỗi gọi POST /api/app/add");
    submitBtn.disabled = false;
  }
}

async function submitInstallAskAnswer() {
  const runID = state.installSession.runId;
  if (!runID) return;

  const input = document.getElementById("install-ask-input");
  const answer = input.value;
  try {
    await api("/api/answer", { run_id: runID, answer });
    appendInstallLog(`> Đã gửi phản hồi: ${answer}`, "log-info");
    input.value = "";
    document.getElementById("install-ask-box").hidden = true;
    const badge = document.getElementById("install-status-badge");
    badge.className = "term-status status-running";
    badge.textContent = "ĐANG CÀI ĐẶT";
  } catch (err) {
    showToast(err.message, "error", "Không thể gửi phản hồi");
  }
}

/* ==========================================================================
   TIỆN ÍCH GIAO DIỆN & GẮN SỰ KIỆN (EVENT LISTENERS)
   ========================================================================== */
function escapeHTML(str) {
  return String(str)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function initTheme() {
  const saved = localStorage.getItem("mt_theme") || "dark";
  document.documentElement.setAttribute("data-theme", saved);
}

function toggleTheme() {
  const current = document.documentElement.getAttribute("data-theme") || "dark";
  const next = current === "dark" ? "light" : "dark";
  document.documentElement.setAttribute("data-theme", next);
  localStorage.setItem("mt_theme", next);
  showToast(`Đã chuyển sang giao diện ${next === "dark" ? "Tối (Dark)" : "Sáng (Light)"}.`, "info", "Giao diện");
}

function resetAllFilters() {
  state.selectedKind = "all";
  state.selectedTag = null;
  state.searchQuery = "";
  const searchInput = document.getElementById("search-input");
  searchInput.value = "";
  document.getElementById("search-clear").hidden = true;
  document.getElementById("search-shortcut-hint").hidden = false;
  renderSidebarFilters();
  renderToolList();
}

function bindEvents() {
  const searchInput = document.getElementById("search-input");
  const searchClear = document.getElementById("search-clear");
  const shortcutHint = document.getElementById("search-shortcut-hint");

  // Thanh tìm kiếm tức thì
  searchInput.addEventListener("input", () => {
    state.searchQuery = searchInput.value;
    const hasText = state.searchQuery.length > 0;
    searchClear.hidden = !hasText;
    shortcutHint.hidden = hasText;
    if (state.centerView !== "tools") switchCenterView("tools");
    renderToolList();
  });

  searchClear.addEventListener("click", () => {
    searchInput.value = "";
    state.searchQuery = "";
    searchClear.hidden = true;
    shortcutHint.hidden = false;
    renderToolList();
    searchInput.focus();
  });

  // Phím tắt toàn cục: Ctrl+K (Tìm kiếm), Esc (Xóa tìm kiếm), Ctrl+Enter (Chạy tool đang chọn)
  window.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      switchCenterView("tools");
      searchInput.focus();
      searchInput.select();
    } else if (e.key === "Escape" && document.activeElement === searchInput) {
      if (searchInput.value) {
        searchClear.click();
      } else {
        searchInput.blur();
      }
    } else if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
      if (state.centerView === "tools" && state.selectedToolId) {
        const form = document.getElementById("dock-param-form");
        if (form && !form.hidden) {
          e.preventDefault();
          form.requestSubmit();
        }
      }
    }
  });

  // Nút chuyển màn hình Cài đặt Tool (+)
  document.getElementById("btn-open-install").onclick = () => switchCenterView("install");
  document.getElementById("sidebar-btn-install").onclick = () => switchCenterView("install");
  document.getElementById("btn-empty-install").onclick = () => switchCenterView("install");
  document.getElementById("btn-back-tools").onclick = () => switchCenterView("tools");

  // Nút Làm mới & Đổi Theme
  document.getElementById("btn-refresh").onclick = () => loadTools(true);
  document.getElementById("btn-theme").onclick = toggleTheme;

  // Xóa lọc
  document.getElementById("btn-clear-tag").onclick = () => {
    state.selectedTag = null;
    renderSidebarFilters();
    renderToolList();
  };
  document.getElementById("btn-reset-filters").onclick = resetAllFilters;
  document.getElementById("btn-empty-reset").onclick = resetAllFilters;

  // Đổi chế độ Grid / List ở Cột 2
  const btnGrid = document.getElementById("btn-view-grid");
  const btnList = document.getElementById("btn-view-list");
  btnGrid.onclick = () => {
    state.viewMode = "grid";
    btnGrid.classList.add("active");
    btnList.classList.remove("active");
    renderToolList();
  };
  btnList.onclick = () => {
    state.viewMode = "list";
    btnList.classList.add("active");
    btnGrid.classList.remove("active");
    renderToolList();
  };

  // Thu gọn / Mở rộng khung cấu hình ở Cột 3
  const toggleConfigBtn = document.getElementById("btn-toggle-config");
  const configBody = document.getElementById("dock-config-body");
  toggleConfigBtn.onclick = () => {
    configBody.hidden = !configBody.hidden;
    toggleConfigBtn.textContent = configBody.hidden ? "Mở rộng" : "Thu gọn";
  };

  // Xóa tool đang chọn từ Cột 3
  document.getElementById("btn-delete-tool").onclick = () => {
    const tool = state.tools.find((t) => t.id === state.selectedToolId);
    if (tool) deleteTool(tool);
  };

  // Submit form chạy tool ở Cột 3
  document.getElementById("dock-param-form").addEventListener("submit", (e) => {
    e.preventDefault();
    const tool = state.tools.find((t) => t.id === state.selectedToolId);
    if (!tool) return;
    const params = collectDockParams(tool);
    executeToolRun(tool, params);
  });

  // Khôi phục tham số mặc định
  document.getElementById("btn-reset-params").onclick = () => {
    if (!state.selectedToolId) return;
    delete state.recentParams[state.selectedToolId];
    localStorage.setItem("mt_recent_params", JSON.stringify(state.recentParams));
    selectToolInDock(state.selectedToolId, false);
    showToast("Đã đặt lại tham số về giá trị mặc định.", "info");
  };

  // Các nút điều khiển trên thanh Terminal Dock (Cột 3)
  const autoScrollBtn = document.getElementById("btn-term-autoscroll");
  autoScrollBtn.onclick = () => {
    state.autoScroll = !state.autoScroll;
    autoScrollBtn.classList.toggle("active", state.autoScroll);
  };

  document.getElementById("btn-term-copy").onclick = async () => {
    const session = state.activeRunId ? state.runs.get(state.activeRunId) : null;
    const text = session ? session.rawLog : document.getElementById("term-output").innerText;
    try {
      await navigator.clipboard.writeText(text);
      showToast("Đã sao chép toàn bộ nội dung log vào Clipboard.", "success", "Đã Copy Log");
    } catch {
      showToast("Trình duyệt không cho phép truy cập Clipboard.", "warn");
    }
  };

  document.getElementById("btn-term-clear").onclick = () => {
    const session = state.activeRunId ? state.runs.get(state.activeRunId) : null;
    if (session) {
      session.lines = [];
      session.rawLog = "";
    }
    document.getElementById("term-output").innerHTML = "";
  };

  document.getElementById("btn-clear-all-runs").onclick = () => {
    for (const [id, s] of state.runs.entries()) {
      if (s.status === "ok" || s.status === "fail") {
        state.runs.delete(id);
      }
    }
    const keys = [...state.runs.keys()];
    state.activeRunId = keys.length > 0 ? keys[keys.length - 1] : null;
    updateGlobalRunIndicator();
    renderRunTabs();
    renderActiveTerminal();
  };

  // Gửi phản hồi khi có sự kiện ASK ở Terminal Cột 3
  document.getElementById("btn-term-ask-send").onclick = submitTerminalAskAnswer;
  document.getElementById("term-ask-input").addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      submitTerminalAskAnswer();
    }
  });

  // Form Cài đặt Tool ở Cột 2
  document.getElementById("install-form").addEventListener("submit", handleInstallSubmit);
  document.getElementById("btn-install-copy").onclick = async () => {
    try {
      await navigator.clipboard.writeText(state.installSession.rawLog || "");
      showToast("Đã sao chép log cài đặt vào Clipboard.", "success");
    } catch {
      showToast("Không thể sao chép log.", "warn");
    }
  };
  document.getElementById("btn-install-clear").onclick = () => {
    state.installSession.rawLog = "";
    document.getElementById("install-log-output").innerHTML = "";
  };
  document.getElementById("btn-install-ask-send").onclick = submitInstallAskAnswer;
  document.getElementById("install-ask-input").addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      submitInstallAskAnswer();
    }
  });
}

/* ==========================================================================
   KHỞI CHẠY ỨNG DỤNG
   ========================================================================== */
initTheme();
bindEvents();
loadTools(false);