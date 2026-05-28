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

## Milvus 索引与检索

第三阶段已经接入 embedding 和 Milvus。默认配置使用 `ai.provider: mock`，可以在没有大模型 API Key 的情况下跑通写入和检索链路；它不代表真实语义效果，只适合本地联调。

启动 Milvus 相关依赖：

```bash
docker compose up -d etcd minio milvus
```

把某个文档的 chunks 写入 Milvus，假设文档 ID 是 `1`：

```bash
curl -X POST http://127.0.0.1:8080/api/documents/1/index
```

检索调试：

```bash
curl -X POST http://127.0.0.1:8080/api/retrieval/search \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"query":"GMP 模型是什么？","category":"Go","top_k":5}'
```

如果要使用真实 embedding 模型，把 `configs/config.yaml` 里的 `ai.provider` 改成 `openai-compatible`，并通过环境变量传入密钥：

```bash
export BAGU_AI_BASE_URL="https://api.openai.com/v1"
export BAGU_AI_API_KEY="your-api-key"
export BAGU_AI_EMBEDDING_MODEL="text-embedding-3-small"
```
