"use strict";

const TOKEN = document.querySelector('meta[name="mt-token"]').content;

const toolList = document.getElementById("tool-list");
const detailBody = document.getElementById("detail-body");
const runList = document.getElementById("run-list");
const installSource = document.getElementById("install-source");
const installButton = document.getElementById("install-btn");
const installStatus = document.getElementById("install-status");
const installLog = document.getElementById("install-log");
const manualForm = document.getElementById("manual-form");
const manualName = document.getElementById("manual-name");
const manualEntry = document.getElementById("manual-entry");
const manualGUI = document.getElementById("manual-gui");
const manualButton = document.getElementById("manual-btn");
const addToolButton = document.getElementById("add-tool");
const notice = document.getElementById("notice");

let tools = [];
let selectedID = null;
let noticeTimer = null;

function showNotice(message) {
  notice.textContent = message;
  notice.hidden = false;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => {
    notice.hidden = true;
  }, 5000);
}

async function api(path, body, method = "POST") {
  const headers = { "X-MT-Token": TOKEN };
  const options = { method, headers };
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(body);
  }
  const response = await fetch(path, options);
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || `HTTP ${response.status}`);
  }
  return data;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function paramInput(param) {
  const wrap = document.createElement("div");
  const label = el("label", null, param.name);
  label.title = param.name;
  if (param.required) {
    label.appendChild(el("span", "req", " *"));
  }

  let input;
  if (param.type === "bool") {
    wrap.className = "bool-row";
    input = document.createElement("input");
    input.type = "checkbox";
    if (param.default === true) input.checked = true;
    input.id = "param-" + param.name;
    wrap.appendChild(input);
    const text = el("span", null, param.name);
    wrap.appendChild(text);
    return { wrap, input };
  }

  wrap.className = "param-row";

  if (param.type === "enum") {
    input = document.createElement("select");
    for (const choice of param.choices || []) {
      const option = document.createElement("option");
      option.value = choice;
      option.textContent = choice;
      if (param.default === choice) option.selected = true;
      input.appendChild(option);
    }
  } else {
    input = document.createElement("input");
    if (param.type === "int") {
      input.type = "number";
      input.step = "1";
    } else {
      input.type = "text";
      if (param.type === "string" && param.name.toLowerCase().includes("url")) {
        input.placeholder = "https://…";
      }
    }
    if (param.default !== undefined && param.default !== null) {
      input.value = String(param.default);
    }
  }
  if (param.required) input.required = true;
  input.id = "param-" + param.name;

  wrap.appendChild(label);
  wrap.appendChild(input);
  return { wrap, input };
}

function renderToolList() {
  toolList.replaceChildren();
  for (const tool of tools) {
    const item = el("div", "tool-item", tool.name);
    item.dataset.id = tool.id;
    item.title = tool.name;
    if (tool.id === selectedID) item.classList.add("selected");

    item.appendChild(el("span", "tool-kind", tool.kind));

    const removeButton = el("button", "remove-btn");
    removeButton.title = "Xóa tool " + tool.id;
    removeButton.setAttribute("aria-label", "Xóa tool " + tool.id);
    removeButton.innerHTML =
      '<svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">' +
      '<path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/>' +
      "</svg>";
    removeButton.addEventListener("click", (event) => {
      event.stopPropagation();
      removeTool(tool);
    });
    item.appendChild(removeButton);

    item.addEventListener("click", () => selectTool(tool.id));
    toolList.appendChild(item);
  }
}

function selectTool(id) {
  selectedID = id;
  for (const item of toolList.children) {
    item.classList.toggle("selected", item.dataset.id === id);
  }
  const tool = tools.find((t) => t.id === id);
  renderDetail(tool);
}

function renderDetail(tool) {
  detailBody.replaceChildren();
  if (!tool) {
    detailBody.appendChild(el("p", "detail-empty", "Chọn một tool ở cột trái để xem chi tiết."));
    return;
  }

  const card = el("div", "tool-card");
  card.appendChild(el("h3", null, tool.name));

  if (tool.tags && tool.tags.length > 0) {
    const tags = el("p", "tags");
    for (const tag of tool.tags) {
      tags.appendChild(el("span", "tag", tag));
    }
    card.appendChild(tags);
  }

  const errorLine = el("p", "card-error");

  if (tool.kind === "url") {
    const open = el("button", "btn", "Mở");
    open.addEventListener("click", () => {
      window.open(tool.target, "_blank", "noopener");
    });
    card.appendChild(open);
    card.appendChild(errorLine);
    detailBody.appendChild(card);
    return;
  }

  if (tool.kind !== "app") {
    card.appendChild(el("p", "card-error", `Kind "${tool.kind}" chưa hỗ trợ trên web.`));
    detailBody.appendChild(card);
    return;
  }

  const inputs = new Map();
  for (const param of tool.params || []) {
    const { wrap, input } = paramInput(param);
    card.appendChild(wrap);
    inputs.set(param.name, input);
  }

  const runButton = el("button", "btn", "Chạy");
  runButton.addEventListener("click", async () => {
    const params = {};
    for (const [name, input] of inputs) {
      if (input.type === "checkbox") {
        params[name] = input.checked ? "true" : "false";
      } else {
        params[name] = input.value;
      }
    }

    runButton.disabled = true;
    errorLine.textContent = "";
    try {
      const data = await api("/api/run", { id: tool.id, params });
      addRunPanel(tool, data.run_id);
    } catch (err) {
      errorLine.textContent = "Lỗi: " + err.message;
    } finally {
      runButton.disabled = false;
    }
  });

  card.appendChild(runButton);
  card.appendChild(errorLine);
  detailBody.appendChild(card);
}

function addRunPanel(tool, runID) {
  const card = el("div", "run-card");
  const head = el("div", "run-head");
  head.appendChild(el("h3", null, tool.name));
  head.appendChild(el("span", "run-id", runID));
  const status = el("span", "status", "Đang chạy…");
  head.appendChild(status);
  card.appendChild(head);

  const progressWrap = el("div", "progress-wrap");
  const progressFill = el("div", "progress-fill");
  progressWrap.appendChild(progressFill);
  card.appendChild(progressWrap);

  const log = el("pre", "log");
  card.appendChild(log);

  const askRow = el("div", "ask-row");
  const askInput = document.createElement("input");
  askInput.type = "text";
  askInput.placeholder = "Nhập câu trả lời…";
  const askButton = el("button", "btn", "Gửi");
  askRow.appendChild(askInput);
  askRow.appendChild(askButton);
  card.appendChild(askRow);

  runList.prepend(card);

  let progressLine = null;
  let done = false;

  const sendAnswer = async () => {
    const answer = askInput.value;
    askInput.value = "";
    hideAsk();
    log.appendChild(el("span", "line-answer", "> " + answer + "\n"));
    log.scrollTop = log.scrollHeight;
    try {
      await api("/api/answer", { run_id: runID, answer });
    } catch (err) {
      log.appendChild(el("span", "line-error", "Lỗi gửi câu trả lời: " + err.message + "\n"));
      log.scrollTop = log.scrollHeight;
    }
  };

  const showAsk = () => {
    askRow.classList.add("visible");
    askInput.focus();
  };
  const hideAsk = () => {
    askRow.classList.remove("visible");
  };

  askButton.addEventListener("click", sendAnswer);
  askInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      sendAnswer();
    }
  });

  const append = (text, className) => {
    log.appendChild(el("span", className, text + "\n"));
    log.scrollTop = log.scrollHeight;
  };

  const source = new EventSource("/api/events?runID=" + encodeURIComponent(runID));
  source.onmessage = (message) => {
    if (done) return;
    const event = JSON.parse(message.data);

    switch (event.kind) {
      case "ask":
        append(event.text, "line-ask");
        showAsk();
        break;
      case "error":
        append(event.text, "line-error");
        break;
      case "progress":
        if (!progressLine) {
          progressWrap.classList.add("visible");
          progressLine = el("span", "line-progress");
          log.appendChild(progressLine);
        }
        progressLine.textContent = event.text + "\n";
        progressFill.style.width = Math.max(0, Math.min(100, event.pct || 0)) + "%";
        status.textContent = (event.pct || 0).toFixed(1) + "%";
        log.scrollTop = log.scrollHeight;
        break;
      case "result":
        done = true;
        const ok = !event.data || event.data.ok !== false;
        append(event.text, ok ? "line-result ok" : "line-result fail");
        progressFill.style.width = ok ? "100%" : progressFill.style.width;
        card.classList.add(ok ? "done-ok" : "done-fail");
        status.textContent = ok ? "Xong" : "Thất bại";
        hideAsk();
        source.close();
        break;
      default:
        append(event.text, "");
        break;
    }
  };

  source.onerror = () => {
    if (!done) {
      status.textContent = "Mất kết nối…";
    }
  };
}

function appendInstall(text, className) {
  installLog.appendChild(el("span", className, text));
  installLog.scrollTop = installLog.scrollHeight;
}

function setInstalling(active) {
  installButton.disabled = active;
  installSource.disabled = active;
  installStatus.hidden = !active;
}

function showManualForm() {
  manualForm.hidden = false;
  if (!manualName.value) {
    manualName.focus();
  } else {
    manualEntry.focus();
  }
}

function hideManualForm() {
  manualForm.hidden = true;
}

// Theo dõi một lần cài đặt qua SSE. manual=true là lần cài từ mini-form:
// nếu thất bại thật sự (không phải thiếu app.toml) thì hiện lại form để sửa.
function watchInstallRun(runID, manual) {
  const eventSource = new EventSource("/api/events?runID=" + encodeURIComponent(runID));
  let finished = false;

  eventSource.onmessage = (message) => {
    if (finished) return;
    const event = JSON.parse(message.data);

    switch (event.kind) {
      case "manual":
        appendInstall(event.text + "\n", "line-warn");
        break;
      case "progress":
        appendInstall(event.text + "\n", "line-progress");
        break;
      case "error":
        appendInstall(event.text + "\n", "line-error");
        break;
      case "result": {
        finished = true;
        const data = event.data || {};
        if (data.manual === true) {
          appendInstall(event.text + "\n", "line-warn");
          setInstalling(false);
          showManualForm();
        } else {
          const ok = data.ok !== false;
          appendInstall(event.text + "\n", ok ? "line-result ok" : "line-result fail");
          setInstalling(false);
          if (ok) {
            const installedID = data.id;
            showNotice(installedID ? `Đã cài tool "${installedID}".` : "Đã cài tool.");
            loadTools().then(() => {
              if (installedID) selectTool(installedID);
            });
          } else if (manual) {
            showManualForm();
          }
        }
        eventSource.close();
        break;
      }
      default:
        appendInstall(event.text + "\n", "");
        break;
    }
  };

  eventSource.onerror = () => {
    if (!finished) {
      appendInstall("(mất kết nối tới máy chủ)\n", "line-error");
      setInstalling(false);
      if (manual) showManualForm();
    }
  };
}

async function runInstall() {
  const source = installSource.value.trim();
  if (!source) {
    showNotice("Nhập nguồn cài đặt trước.");
    installSource.focus();
    return;
  }

  hideManualForm();
  setInstalling(true);
  installLog.hidden = false;
  installLog.replaceChildren();
  appendInstall("Đang gửi yêu cầu cài đặt " + source + "…\n");

  let runID;
  try {
    const data = await api("/api/app/add", { source });
    runID = data.run_id;
  } catch (err) {
    appendInstall("Lỗi: " + err.message + "\n", "line-error");
    setInstalling(false);
    return;
  }

  watchInstallRun(runID, false);
}

async function runManualInstall() {
  const source = installSource.value.trim();
  const name = manualName.value.trim();
  const entry = manualEntry.value.trim();
  if (!name) {
    showNotice("Điền Tên hiển thị trước.");
    manualName.focus();
    return;
  }
  if (!entry) {
    showNotice('Điền Lệnh chạy, ví dụ "python main.py".');
    manualEntry.focus();
    return;
  }

  hideManualForm();
  setInstalling(true);
  appendInstall("Đang tạo app.toml và cài đặt…\n");

  let runID;
  try {
    const data = await api("/api/app/install-manual", {
      source,
      name,
      entry,
      gui: manualGUI.checked,
    });
    runID = data.run_id;
  } catch (err) {
    appendInstall("Lỗi: " + err.message + "\n", "line-error");
    setInstalling(false);
    showManualForm();
    return;
  }

  watchInstallRun(runID, true);
}

async function removeTool(tool) {
  if (!window.confirm(`Xóa tool "${tool.name}"?`)) return;
  try {
    await api("/api/app/remove", { id: tool.id }, "DELETE");
    if (selectedID === tool.id) {
      selectedID = null;
      renderDetail(null);
    }
    showNotice(`Đã xóa tool "${tool.name}".`);
    await loadTools();
  } catch (err) {
    showNotice("Xóa thất bại: " + err.message);
  }
}

async function loadTools() {
  try {
    tools = await api("/api/tools", undefined, "GET");
    if (selectedID && !tools.some((tool) => tool.id === selectedID)) {
      selectedID = null;
    }
    renderToolList();
    if (selectedID) {
      selectTool(selectedID);
    } else {
      renderDetail(null);
    }
  } catch (err) {
    showNotice("Không tải được danh sách tool: " + err.message);
  }
}

addToolButton.addEventListener("click", () => {
  installSource.focus();
});

installButton.addEventListener("click", runInstall);

manualButton.addEventListener("click", runManualInstall);

manualEntry.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    runManualInstall();
  }
});

installSource.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    runInstall();
  }
});

loadTools();
