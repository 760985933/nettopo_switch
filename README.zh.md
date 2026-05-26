# Nettopo Switch (Codex ↔ DeepSeek 本地代理)

[English](README.md)

---

该项目面向 Codex Desktop：通过把 Codex 的 Base URL 指向本地代理地址，让 Codex Desktop 走 DeepSeek 的模型能力（无需改变 Codex 的使用方式）。

## 功能

- **Codex Responses 适配**：支持 `POST /v1/responses`（含流式 SSE），并转发到 DeepSeek `POST /v1/chat/completions`
- **模型映射**：把 Codex 侧模型名（如 `gpt-5.4-mini`）映射为 DeepSeek 可用模型（如 `deepseek-v4-flash`）
- **可视化配置**：桌面应用内完成 Base URL、Key、端口、映射等配置
- **Codex config.toml 管理**：支持合并写入、原文编辑、历史备份、选择恢复、删除/清理备份
- **健康检查与日志**：一键检查上游可达性，日志可追踪每次请求
- **界面多语言（i18n）**：`zh-CN`（简体中文）、`en-US`（English）、`ja-JP`（日本語）、`ko-KR`（한국어）、`fr-FR`（Français）、`de-DE`（Deutsch）、`es-ES`（Español）
- **跨平台构建**：macOS arm64 / Windows amd64 / Windows arm64

## 端点

- `GET /`：服务信息
- `GET /health`：健康状态
- `GET /v1/models`：模型列表（用于 Codex UI 显示可选模型）
- `POST /v1/responses`：Codex 入口（推荐）
- `POST /v1/chat/completions`：兼容入口

---

## 快速开始

1) 启动桌面应用，填写：
- DeepSeek Base URL：`https://api.deepseek.com/v1`
- API Key：你的 DeepSeek Key
- 默认模型：例如 `deepseek-v4-flash`

2) 启动代理服务（默认监听 `http://127.0.0.1:11434`）

3) 验证：

```bash
curl http://127.0.0.1:11434/health
```

---

## 配置 Codex

在应用内：**偏好设置 → Codex config.toml**

- **合并写入**：自动写入/更新 `~/.codex/config.toml`，并保留其它配置项
- **历史备份**：每次写入都会在 `~/.codex/backups/` 创建一份不可覆盖的备份，可选择恢复/删除/清理

Codex 侧的 Base URL 应指向：

```
http://127.0.0.1:11434/v1
```

---

## 常见问题

### Codex 报 502 / Reconnecting

- 先看应用内「最近日志」：是否有 `上游返回 4xx/5xx` 或 `转发失败`
- 若提示 DeepSeek 不支持某个模型名：在「模型映射」中把 Codex 模型映射到 `deepseek-v4-pro` / `deepseek-v4-flash`

---

## 开发与构建

### 本地开发

```bash
npm -C frontend install
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
wails dev
```

### 构建（示例）

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
wails build -platform darwin/arm64
```
