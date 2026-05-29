import "./styles.css";

type Citation = {
  chunk_uid: string;
  document_id: number;
  chunk_id: number;
  title_path: string;
  category: string;
  score: number;
};

type RetrievedChunk = Citation & {
  content: string;
};

type AgentStep = {
  step: number;
  thought: string;
  action: string;
  action_input?: unknown;
  observation?: unknown;
  latency_ms: number;
};

type ChatResponse = {
  conversation_id: number;
  answer: string;
  citations?: Citation[];
  retrieved_chunks?: RetrievedChunk[];
  agent_steps?: AgentStep[];
};

type IndexTask = {
  task_uid: string;
  status: string;
  total_chunks: number;
  indexed_chunks: number;
  error_message?: string;
};

type DocumentItem = {
  id: number;
  name: string;
  category: string;
  status: string;
  chunk_count: number;
  created_at: string;
};

type ApiBody<T> = {
  code: number;
  message: string;
  data: T;
};

type Message = {
  id: number;
  role: "user" | "assistant";
  content: string;
  citations?: Citation[];
  retrievedChunks?: RetrievedChunk[];
  agentSteps?: AgentStep[];
};

const state = {
  conversationId: 0,
  messages: [] as Message[],
  nextMessageId: 1,
  pending: false,
};

document.querySelector<HTMLDivElement>("#app")!.innerHTML = `
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark">B</div>
        <div>
          <strong>BaguAgent</strong>
          <span>个人八股 RAG</span>
        </div>
      </div>

      <button class="new-chat" id="newChatBtn" type="button" title="新会话">
        <span>＋</span>
        新会话
      </button>

      <section class="panel">
        <div class="panel-title">文档</div>
        <label class="file-picker">
          <input id="fileInput" type="file" accept=".md,.markdown" />
          <span id="fileName">选择 Markdown</span>
        </label>
        <div class="compact-grid">
          <label>
            用户
            <input id="uploadUserId" type="number" value="1" min="1" />
          </label>
          <label>
            分类
            <input id="uploadCategory" type="text" value="Go" />
          </label>
        </div>
        <button class="secondary-action" id="uploadBtn" type="button">上传并索引</button>
        <div class="status" id="uploadStatus">等待上传</div>
      </section>

      <section class="panel">
        <div class="panel-title-row">
          <div class="panel-title">文档管理</div>
          <button class="icon-action" id="refreshDocsBtn" type="button" title="刷新文档">↻</button>
        </div>
        <div class="status" id="docsStatus">尚未加载</div>
        <div class="document-list" id="documentList"></div>
      </section>

      <section class="panel">
        <div class="panel-title">问答参数</div>
        <div class="compact-grid">
          <label>
            用户
            <input id="chatUserId" type="number" value="1" min="1" />
          </label>
          <label>
            Top K
            <input id="topK" type="number" value="5" min="1" max="20" />
          </label>
        </div>
        <label>
          分类
          <input id="chatCategory" type="text" value="Go" />
        </label>
        <div class="switch-row">
          <label class="switch">
            <input id="streamMode" type="checkbox" checked />
            <span></span>
          </label>
          <span>流式输出</span>
        </div>
        <div class="switch-row">
          <label class="switch">
            <input id="debugMode" type="checkbox" />
            <span></span>
          </label>
          <span>调试信息</span>
        </div>
      </section>
    </aside>

    <main class="chat-pane">
      <header class="chat-header">
        <div>
          <h1>面试复习助手</h1>
          <p id="conversationMeta">新会话</p>
        </div>
        <div class="server-pill">localhost:8080</div>
      </header>

      <section class="messages" id="messages">
        <div class="empty-state">
          <h2>今天复习什么？</h2>
          <div class="prompt-grid">
            <button type="button" data-prompt="Go 语言的 GMP 模型是什么？">GMP 模型</button>
            <button type="button" data-prompt="MySQL B+ 树索引为什么适合范围查询？">MySQL 索引</button>
            <button type="button" data-prompt="Redis RDB 和 AOF 有什么区别？">Redis 持久化</button>
            <button type="button" data-prompt="Go channel 的底层结构是什么？">Channel 底层</button>
          </div>
        </div>
      </section>

      <form class="composer" id="chatForm">
        <textarea id="questionInput" rows="1" placeholder="输入一个面试问题，按 Enter 发送，Shift + Enter 换行"></textarea>
        <button id="sendBtn" type="submit" title="发送">↑</button>
      </form>
    </main>
  </div>
`;

const messagesEl = getEl<HTMLDivElement>("messages");
const questionInput = getEl<HTMLTextAreaElement>("questionInput");
const chatForm = getEl<HTMLFormElement>("chatForm");
const sendBtn = getEl<HTMLButtonElement>("sendBtn");
const newChatBtn = getEl<HTMLButtonElement>("newChatBtn");
const fileInput = getEl<HTMLInputElement>("fileInput");
const fileName = getEl<HTMLSpanElement>("fileName");
const uploadBtn = getEl<HTMLButtonElement>("uploadBtn");
const uploadStatus = getEl<HTMLDivElement>("uploadStatus");
const conversationMeta = getEl<HTMLParagraphElement>("conversationMeta");
const refreshDocsBtn = getEl<HTMLButtonElement>("refreshDocsBtn");
const docsStatus = getEl<HTMLDivElement>("docsStatus");
const documentList = getEl<HTMLDivElement>("documentList");

chatForm.addEventListener("submit", (event) => {
  event.preventDefault();
  void sendQuestion();
});

questionInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    void sendQuestion();
  }
});

questionInput.addEventListener("input", () => {
  questionInput.style.height = "auto";
  questionInput.style.height = `${Math.min(questionInput.scrollHeight, 180)}px`;
});

newChatBtn.addEventListener("click", () => {
  state.conversationId = 0;
  state.messages = [];
  state.nextMessageId = 1;
  renderMessages();
});

fileInput.addEventListener("change", () => {
  fileName.textContent = fileInput.files?.[0]?.name || "选择 Markdown";
});

uploadBtn.addEventListener("click", () => {
  void uploadAndIndex();
});

refreshDocsBtn.addEventListener("click", () => {
  void loadDocuments();
});

documentList.addEventListener("click", (event) => {
  const target = event.target as HTMLElement;
  const button = target.closest<HTMLButtonElement>("[data-delete-document]");
  if (!button) return;
  const documentId = Number(button.dataset.deleteDocument);
  const documentName = button.dataset.documentName || `#${documentId}`;
  void deleteDocument(documentId, documentName);
});

document.querySelectorAll<HTMLButtonElement>("[data-prompt]").forEach((button) => {
  button.addEventListener("click", () => {
    questionInput.value = button.dataset.prompt || "";
    questionInput.focus();
  });
});

function getEl<T extends HTMLElement>(id: string): T {
  return document.getElementById(id) as T;
}

async function sendQuestion() {
  const question = questionInput.value.trim();
  if (!question || state.pending) return;

  state.pending = true;
  sendBtn.disabled = true;
  questionInput.value = "";
  questionInput.style.height = "auto";

  addMessage({ role: "user", content: question });
  const assistant = addMessage({ role: "assistant", content: "" });

  try {
    if (getEl<HTMLInputElement>("streamMode").checked) {
      await sendStream(question, assistant.id);
    } else {
      await sendOnce(question, assistant.id);
    }
  } catch (error) {
    updateAssistant(assistant.id, {
      content: `请求失败：${error instanceof Error ? error.message : String(error)}`,
    });
  } finally {
    state.pending = false;
    sendBtn.disabled = false;
    questionInput.focus();
  }
}

void loadDocuments();

async function sendOnce(question: string, assistantId: number) {
  const response = await fetch("/api/agent/chat", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(buildChatPayload(question)),
  });
  const body = await parseApiResponse<ChatResponse>(response);
  applyChatResponse(body, assistantId);
}

async function sendStream(question: string, assistantId: number) {
  const response = await fetch("/api/agent/chat/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(buildChatPayload(question)),
  });
  if (!response.ok || !response.body) {
    throw new Error(`HTTP ${response.status}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let finalData: Partial<ChatResponse> = {};

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parts = buffer.split("\n\n");
    buffer = parts.pop() || "";
    for (const part of parts) {
      const event = parseSSEEvent(part);
      if (!event) continue;
      if (event.type === "meta" && typeof event.data.conversation_id === "number") {
        state.conversationId = event.data.conversation_id;
        updateConversationMeta();
      }
      if (event.type === "retrieved") {
        updateAssistant(assistantId, {
          citations: event.data.citations,
          retrievedChunks: event.data.retrieved_chunks,
          agentSteps: event.data.agent_steps,
        });
      }
      if (event.type === "delta") {
        appendAssistantContent(assistantId, event.data.delta || "");
      }
      if (event.type === "done") {
        finalData = {
          conversation_id: event.data.conversation_id,
          answer: event.data.answer,
          citations: event.data.citations,
          agent_steps: event.data.agent_steps,
        };
      }
      if (event.type === "error") {
        throw new Error(event.data.error || "stream error");
      }
    }
  }

  if (finalData.conversation_id) {
    applyChatResponse(finalData as ChatResponse, assistantId);
  }
}

function buildChatPayload(question: string) {
  return {
    user_id: numberValue("chatUserId", 1),
    conversation_id: state.conversationId || undefined,
    question,
    category: getEl<HTMLInputElement>("chatCategory").value.trim(),
    top_k: numberValue("topK", 5),
  };
}

function parseSSEEvent(raw: string) {
  const lines = raw.split("\n");
  const type = lines.find((line) => line.startsWith("event:"))?.slice(6).trim();
  const dataLine = lines.find((line) => line.startsWith("data:"));
  if (!type || !dataLine) return null;
  return { type, data: JSON.parse(dataLine.slice(5).trim()) };
}

async function parseApiResponse<T>(response: Response): Promise<T> {
  const body = (await response.json()) as ApiBody<T>;
  if (!response.ok || body.code !== 0) {
    throw new Error(body.message || `HTTP ${response.status}`);
  }
  return body.data;
}

async function loadDocuments() {
  docsStatus.textContent = "正在加载文档...";
  documentList.innerHTML = "";
  try {
    const userId = numberValue("uploadUserId", 1);
    const response = await fetch(`/api/documents?user_id=${userId}`);
    const docs = await parseApiResponse<DocumentItem[]>(response);
    renderDocumentList(docs);
  } catch (error) {
    docsStatus.textContent = `加载失败：${error instanceof Error ? error.message : String(error)}`;
  }
}

function renderDocumentList(docs: DocumentItem[]) {
  if (docs.length === 0) {
    docsStatus.textContent = "暂无文档";
    documentList.innerHTML = "";
    return;
  }
  docsStatus.textContent = `${docs.length} 个文档`;
  documentList.innerHTML = docs.map(renderDocumentItem).join("");
}

function renderDocumentItem(doc: DocumentItem) {
  return `
    <article class="document-item">
      <div class="document-main">
        <strong title="${escapeHtml(doc.name)}">${escapeHtml(doc.name)}</strong>
        <span>${escapeHtml(doc.category || "未分类")} · ${escapeHtml(statusText(doc.status))} · ${doc.chunk_count} chunks</span>
      </div>
      <button
        class="danger-action"
        type="button"
        title="删除文档和向量"
        data-delete-document="${doc.id}"
        data-document-name="${escapeHtml(doc.name)}"
      >
        删除
      </button>
    </article>
  `;
}

async function deleteDocument(documentId: number, documentName: string) {
  if (!Number.isFinite(documentId) || documentId <= 0) return;
  const confirmed = window.confirm(`确定删除「${documentName}」吗？\n\n会同时删除 MySQL 文档、chunks 和 Milvus 向量。`);
  if (!confirmed) return;

  docsStatus.textContent = `正在删除 ${documentName}...`;
  try {
    const response = await fetch(`/api/documents/${documentId}`, { method: "DELETE" });
    await parseApiResponse<{ deleted: boolean }>(response);
    docsStatus.textContent = "删除成功，正在刷新...";
    await loadDocuments();
  } catch (error) {
    docsStatus.textContent = `删除失败：${error instanceof Error ? error.message : String(error)}`;
  }
}

function applyChatResponse(data: ChatResponse, assistantId: number) {
  state.conversationId = data.conversation_id || state.conversationId;
  updateConversationMeta();
  updateAssistant(assistantId, {
    content: data.answer,
    citations: data.citations,
    retrievedChunks: data.retrieved_chunks,
    agentSteps: data.agent_steps,
  });
}

async function uploadAndIndex() {
  const file = fileInput.files?.[0];
  if (!file) {
    uploadStatus.textContent = "请先选择 Markdown 文件";
    return;
  }

  uploadBtn.disabled = true;
  uploadStatus.textContent = "正在上传...";
  try {
    const form = new FormData();
    form.append("file", file);
    form.append("user_id", String(numberValue("uploadUserId", 1)));
    form.append("category", getEl<HTMLInputElement>("uploadCategory").value.trim());

    const uploadResponse = await fetch("/api/documents/upload", {
      method: "POST",
      body: form,
    });
    const uploadData = await parseApiResponse<{ document: { id: number; chunk_count: number } }>(uploadResponse);
    uploadStatus.textContent = `上传成功，开始索引 ${uploadData.document.chunk_count} 个 chunk...`;

    const indexResponse = await fetch(`/api/documents/${uploadData.document.id}/index`, { method: "POST" });
    const task = await parseApiResponse<IndexTask>(indexResponse);
    await pollIndexTask(task.task_uid);
    await loadDocuments();
  } catch (error) {
    uploadStatus.textContent = `上传失败：${error instanceof Error ? error.message : String(error)}`;
  } finally {
    uploadBtn.disabled = false;
  }
}

async function pollIndexTask(taskId: string) {
  for (let i = 0; i < 120; i += 1) {
    const response = await fetch(`/api/index-tasks/${taskId}`);
    const task = await parseApiResponse<IndexTask>(response);
    uploadStatus.textContent = `索引${statusText(task.status)}：${task.indexed_chunks}/${task.total_chunks}`;
    if (task.status === "succeeded") return;
    if (task.status === "failed") {
      throw new Error(task.error_message || "索引失败");
    }
    await sleep(1500);
  }
  throw new Error("索引任务超时");
}

function statusText(status: string) {
  const map: Record<string, string> = {
    pending: "等待中",
    running: "进行中",
    succeeded: "完成",
    failed: "失败",
    uploaded: "已上传",
    parsed: "已解析",
    indexed: "已索引",
  };
  return map[status] || status;
}

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function numberValue(id: string, fallback: number) {
  const value = Number(getEl<HTMLInputElement>(id).value);
  return Number.isFinite(value) && value > 0 ? value : fallback;
}

function addMessage(input: Omit<Message, "id">) {
  const message: Message = { id: state.nextMessageId++, ...input };
  state.messages.push(message);
  renderMessages();
  return message;
}

function updateAssistant(id: number, patch: Partial<Message>) {
  const message = state.messages.find((item) => item.id === id);
  if (!message) return;
  Object.assign(message, patch);
  renderMessages();
}

function appendAssistantContent(id: number, text: string) {
  const message = state.messages.find((item) => item.id === id);
  if (!message) return;
  message.content += text;
  renderMessages();
}

function renderMessages() {
  updateConversationMeta();
  if (state.messages.length === 0) {
    messagesEl.innerHTML = `
      <div class="empty-state">
        <h2>今天复习什么？</h2>
        <div class="prompt-grid">
          <button type="button" data-prompt="Go 语言的 GMP 模型是什么？">GMP 模型</button>
          <button type="button" data-prompt="MySQL B+ 树索引为什么适合范围查询？">MySQL 索引</button>
          <button type="button" data-prompt="Redis RDB 和 AOF 有什么区别？">Redis 持久化</button>
          <button type="button" data-prompt="Go channel 的底层结构是什么？">Channel 底层</button>
        </div>
      </div>
    `;
    document.querySelectorAll<HTMLButtonElement>("[data-prompt]").forEach((button) => {
      button.addEventListener("click", () => {
        questionInput.value = button.dataset.prompt || "";
        questionInput.focus();
      });
    });
    return;
  }

  const debug = getEl<HTMLInputElement>("debugMode").checked;
  messagesEl.innerHTML = state.messages.map((message) => renderMessage(message, debug)).join("");
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function renderMessage(message: Message, debug: boolean) {
  const body = message.content ? renderMarkdownLite(message.content) : `<span class="typing">正在生成...</span>`;
  const citations = message.role === "assistant" && message.citations?.length
    ? `<div class="citations">${message.citations.map(renderCitation).join("")}</div>`
    : "";
  const debugBlock = debug && message.role === "assistant"
    ? renderDebug(message.retrievedChunks, message.agentSteps)
    : "";

  return `
    <article class="message ${message.role}">
      <div class="avatar">${message.role === "user" ? "我" : "AI"}</div>
      <div class="bubble">
        <div class="content">${body}</div>
        ${citations}
        ${debugBlock}
      </div>
    </article>
  `;
}

function renderCitation(citation: Citation) {
  return `
    <div class="citation" title="${escapeHtml(citation.chunk_uid)}">
      <strong>${escapeHtml(citation.category || "来源")}</strong>
      <span>${escapeHtml(shortTitle(citation.title_path))}</span>
      <em>${citation.score.toFixed(3)}</em>
    </div>
  `;
}

function renderDebug(chunks?: RetrievedChunk[], steps?: AgentStep[]) {
  const chunkBlock = chunks?.length
    ? `<details><summary>检索片段 ${chunks.length}</summary>${chunks.map((chunk) => `
        <div class="debug-item">
          <strong>${escapeHtml(shortTitle(chunk.title_path))}</strong>
          <p>${escapeHtml(chunk.content.slice(0, 240))}${chunk.content.length > 240 ? "..." : ""}</p>
        </div>
      `).join("")}</details>`
    : "";
  const stepBlock = steps?.length
    ? `<details><summary>Agent 步骤 ${steps.length}</summary>${steps.map((step) => `
        <div class="debug-item">
          <strong>${step.step}. ${escapeHtml(step.action)} · ${step.latency_ms}ms</strong>
          <p>${escapeHtml(step.thought)}</p>
          <pre>${escapeHtml(JSON.stringify(step.observation, null, 2))}</pre>
        </div>
      `).join("")}</details>`
    : "";
  return `<div class="debug-block">${chunkBlock}${stepBlock}</div>`;
}

function shortTitle(title: string) {
  const parts = title.split("/").map((part) => part.trim()).filter(Boolean);
  return parts.slice(-2).join(" / ") || title;
}

function renderMarkdownLite(text: string) {
  const escaped = escapeHtml(text);
  const withCode = escaped.replace(/```([\s\S]*?)```/g, "<pre>$1</pre>");
  return withCode
    .replace(/^## (.*)$/gm, "<h2>$1</h2>")
    .replace(/^### (.*)$/gm, "<h3>$1</h3>")
    .replace(/\*\*(.*?)\*\*/g, "<strong>$1</strong>")
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\n/g, "<br />");
}

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function updateConversationMeta() {
  conversationMeta.textContent = state.conversationId ? `会话 #${state.conversationId}` : "新会话";
}
