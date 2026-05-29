# BaguAgent Frontend

ChatGPT-style local web UI for BaguAgent.

## Start

Run backend first:

```powershell
cd ..\backend
go run ./cmd/server
```

Run frontend with hot reload:

```powershell
cd ..\frontend
npm.cmd install --cache "..\.npm-cache"
npm.cmd run dev -- --port 5173
```

Open:

```text
http://localhost:5173
```

The Vite dev server proxies `/api` to `http://localhost:8080`, so browser CORS is avoided during local development.

## Features

- ChatGPT-like interview Q&A page
- Normal and SSE streaming chat modes
- Markdown document upload
- Upload-and-index workflow
- Category, user id, and top-k controls
- Optional debug view for retrieved chunks and agent steps
