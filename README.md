# BaguAgent

BaguAgent 是一个面向程序员面试复习场景的 Agent + RAG 系统。第一阶段先搭建 Go + Gin + Gorm + MySQL 的基础工程骨架，并准备 Milvus、Redis、本地开发 Docker Compose 环境。

## 本地启动依赖

```bash
docker compose up -d mysql redis etcd minio milvus
```

默认 MySQL 暴露在 `127.0.0.1:13306`，避免和本机 MySQL 的 `3306` 冲突。

## 启动后端

```bash
cd backend
go run ./cmd/server
```

健康检查：

```bash
curl http://127.0.0.1:8080/healthz
```

## 当前阶段

- Gin 路由骨架
- Viper 配置加载
- zap 日志
- Gorm + MySQL 初始化
- users/documents/document_chunks/conversations/messages/agent_runs/rag_eval 表模型
- Docker Compose: MySQL、Redis、Milvus standalone
- Markdown 上传、标题路径解析、语义 chunk 入 MySQL
- Eino Graph 编排（v3）：计划 -> 检索 -> 评估 -> 分支（回答 / 换分类重试一次 / 兜底），chat 与 stream 共用

## Markdown 导入接口

上传 Markdown：

```bash
curl -X POST http://127.0.0.1:8080/api/documents/upload \
  -F "file=@./go_interview.md" \
  -F "category=Go" \
  -F "user_id=1"
```

查看文档：

```bash
curl "http://127.0.0.1:8080/api/documents?user_id=1"
```

查看 chunk：

```bash
curl "http://127.0.0.1:8080/api/documents/1/chunks"
```

当前阶段只完成 Markdown -> MySQL chunk，`POST /api/documents/:id/index` 会在第三阶段接入 embedding 和 Milvus。

导入汇总型 Markdown 时，系统会根据每个 chunk 的 `title_path` 自动推断分类。例如同一个 `八股汇总.md` 中：

```text
一、Golang 八股 -> Go
二、MySQL 八股 -> MySQL
三、Redis 八股 -> Redis
```

识别不到时会回退到上传接口传入的 `category`。

## Milvus 索引与检索

第三阶段已经接入 embedding 和 Milvus。默认配置使用 `ai.provider: mock`，可以在没有大模型 API Key 的情况下跑通写入和检索链路；它不代表真实语义效果，只适合本地联调。

启动 Milvus 相关依赖：

```bash
docker compose up -d etcd minio milvus attu
```

Attu 是 Milvus 的 Web 管理页面，启动后访问：

```text
http://127.0.0.1:18000
```

如果页面要求填写 Milvus 地址，可以使用：

```text
milvus:19530
```

把某个文档的 chunks 写入 Milvus，假设文档 ID 是 `1`。该接口会创建异步索引任务并立即返回 `task_id`：

```bash
curl -X POST http://127.0.0.1:8080/api/documents/1/index
```

查询索引任务进度：

```bash
curl "http://127.0.0.1:8080/api/index-tasks/{task_id}"
```

任务状态：

```text
pending: 等待执行
running: 正在索引
succeeded: 完成
failed: 失败
```

检索调试：

```bash
curl -X POST http://127.0.0.1:8080/api/retrieval/search \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"query":"GMP 模型是什么？","category":"Go","top_k":5}'
```

## RAG 问答接口

第四阶段已经接入基础 RAG 问答链路。默认 `ai.provider: mock` 时，回答由 mock LLM 根据检索片段拼出，适合本地调通流程；接入真实大模型后会生成更自然的面试化回答。

```bash
curl -X POST http://127.0.0.1:8080/api/agent/chat \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"question":"GMP 模型是什么？","category":"Go","top_k":5}'
```

响应会包含：

```text
answer: 结构化面试回答
citations: 引用来源
retrieved_chunks: 本次召回的 chunk 和分数
conversation_id: 会话 ID
```

查看会话消息：

```bash
curl "http://127.0.0.1:8080/api/conversations/1/messages"
```

流式问答接口：

```bash
curl -N -X POST http://127.0.0.1:8080/api/agent/chat/stream \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"question":"GMP 模型是什么？","category":"Go","top_k":5}'
```

该接口使用 Server-Sent Events，事件类型包括：

```text
meta: 会话 ID
retrieved: 召回片段和引用
delta: 大模型增量文本
done: 完整答案和引用
error: 错误信息
```

## Agent 工具化

第五阶段已经把固定 RAG 链路拆成轻量 Tool 调用：

```text
SearchKnowledgeTool -> 从 Milvus 检索相关 chunks
InterviewAnswerTool -> 基于 chunks 生成面试化回答
QuestionGenerateTool -> 基于知识点生成面试题
```

普通问答和流式问答都进入同一套 Eino Graph（v3），保证分类推断、检索观察和兜底逻辑一致：

```text
START
  -> plan_react        判断是否需要检索、确定检索分类
  -> search_knowledge  从 Milvus 检索
  -> assess_retrieval  评估检索结果是否足够
  -> 分支：
       足够               -> interview_answer  生成结构化面试回答
       分类不匹配且未重试   -> retry_search      换用推断分类重试一次，回到 assess 重新评估
       其他不足            -> no_context_answer 保守兜底，不假装引用文档
  -> END
```

`assess_retrieval` 之后是真正的 Eino 分支（`AddBranch`），`retry_search` 会回边到 `assess_retrieval` 形成一次有界回环（`WithMaxRunSteps` 兜底）。这是默认的确定性编排（`ai.agent_mode: graph`）。

## 原生 ReAct Agent（模型自主调用工具）

在确定性 Graph 之外，又接入了基于 Eino `flow/agent/react` 的原生 ReAct Agent：检索不再由代码固定编排，而是由模型通过 tool-calling 自主决定何时调用 `search_knowledge`、用什么 query/category 检索、检索几次，再基于检索片段产出结构化面试回答。

切换方式（默认 `graph`，不影响既有行为）：

```bash
export BAGU_AI_AGENT_MODE="react"
```

或在 `configs/config.yaml` 里设置 `ai.agent_mode: react`。

两种模式对 `/api/agent/chat` 和 `/api/agent/chat/stream` 暴露相同的响应结构（answer / citations / retrieved_chunks / steps），可以无缝切换。底层实现要点：

```text
search_knowledge      Eino-native InvokableTool，JSON Schema 由结构体 tag 自动推断；
                      只向模型暴露 query/category，user_id/top_k 由服务端注入。
ToolCallingChatModel  自实现的 model.ToolCallingChatModel，走 OpenAI 兼容的
                      /chat/completions（含 tools + tool_choice=auto），支持流式。
mock 模式             内置脚本化 mock ToolCallingChatModel，先发起一次检索工具调用，
                      拿到工具结果后再产出结构化回答，无需真实 API Key 即可跑通整条 ReAct 链路。
```

注意：react 模式下检索发生在 Agent 内部，因此流式接口会先持续推送 `delta`，待最终回答结束后再补发一次 `retrieved` 事件（顺序与 graph 模式略有不同，引用来源仍然完整）。

`react` 模式同样需要真实模型才能体现自主决策能力，配置方式与下文一致（mock 仅用于本地联调）。

生成面试题：

```bash
curl -X POST http://127.0.0.1:8080/api/agent/questions \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"topic":"GMP 模型","category":"Go","count":5,"top_k":5}'
```

如果要使用真实模型，把 `configs/config.yaml` 里的 `ai.provider` 改成 `openai-compatible`，并通过环境变量传入密钥。对话模型和向量模型可以来自不同平台：

```bash
export BAGU_AI_PROVIDER="openai-compatible"
export BAGU_AI_CHAT_BASE_URL="https://ark.cn-beijing.volces.com/api/v3"
export BAGU_AI_CHAT_API_KEY="your-chat-api-key"
export BAGU_AI_CHAT_MODEL="your-chat-model"
export BAGU_AI_EMBEDDING_BASE_URL="https://dashscope.aliyuncs.com/compatible-mode/v1"
export BAGU_AI_EMBEDDING_API_KEY="your-embedding-api-key"
export BAGU_AI_EMBEDDING_MODEL="text-embedding-v4"
```

Windows PowerShell 使用 `$env:`：

```powershell
$env:BAGU_AI_PROVIDER="openai-compatible"
$env:BAGU_AI_CHAT_BASE_URL="https://ark.cn-beijing.volces.com/api/v3"
$env:BAGU_AI_CHAT_API_KEY="your-chat-api-key"
$env:BAGU_AI_CHAT_MODEL="your-chat-model"
$env:BAGU_AI_EMBEDDING_BASE_URL="https://dashscope.aliyuncs.com/compatible-mode/v1"
$env:BAGU_AI_EMBEDDING_API_KEY="your-embedding-api-key"
$env:BAGU_AI_EMBEDDING_MODEL="text-embedding-v4"
```

真实 API Key 不要写进 `configs/config.yaml`，也不要提交到 git。
