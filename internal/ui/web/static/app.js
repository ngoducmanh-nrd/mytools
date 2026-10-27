"use strict";

const TOKEN = document.querySelector('meta[name="mt-token"]').content;

const toolList = document.getElementById("tool-list");
const runList = document.getElementById("run-list");
const notice = document.getElementById("notice");

let noticeTimer = null;

function showNotice(message) {
  notice.textContent = message;
  notice.hidden = false;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => {
    notice.hidden = true;
  }, 5000);
}

async function api(path, body) {
  const headers = { "X-MT-Token": TOKEN };
  let options = { headers };
  if (body !== undefined) {
    options.method = "POST";
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

function renderTool(tool) {
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
    toolList.appendChild(card);
    return;
  }

  if (tool.kind !== "app") {
    const unsupported = el("p", "card-error", `Kind "${tool.kind}" chưa hỗ trợ trên web.`);
    card.appendChild(unsupported);
    toolList.appendChild(card);
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
  toolList.appendChild(card);
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

async function loadTools() {
  try {
    const tools = await api("/api/tools");
    toolList.replaceChildren();
    for (const tool of tools) {
      renderTool(tool);
    }
  } catch (err) {
    showNotice("Không tải được danh sách tool: " + err.message);
  }
}

loadTools();
