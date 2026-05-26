# Nettopo Switch (Codex ↔ DeepSeek Local Proxy)

[中文](README.zh.md)

## Overview

Nettopo Switch is a local proxy that adapts Codex-compatible `POST /v1/responses` (including SSE streaming) and forwards requests to DeepSeek `POST /v1/chat/completions`. It also provides a desktop UI to manage endpoints, API keys, ports, and model mappings, plus tools to manage Codex `config.toml` with safe backups.

This is designed to work with Codex Desktop by pointing Codex's Base URL to the local proxy, so Codex can use DeepSeek models without changing its workflow.

## Features

- **Codex Responses adapter**: supports `POST /v1/responses` (SSE) and forwards to DeepSeek `POST /v1/chat/completions`
- **Model mapping**: maps Codex-side model names (e.g. `gpt-5.4-mini`) to DeepSeek models (e.g. `deepseek-v4-flash`)
- **Visual configuration**: configure Base URL, API key, port, and mappings in the desktop app
- **Codex config.toml management**: merge-write, edit raw content, create/restore/delete/clean backup history
- **Health check & logs**: one-click upstream connectivity check; logs for each request
- **UI i18n**: `zh-CN` (简体中文), `en-US` (English), `ja-JP` (日本語), `ko-KR` (한국어), `fr-FR` (Français), `de-DE` (Deutsch), `es-ES` (Español)
- **Cross-platform builds**: macOS arm64 / Windows amd64 / Windows arm64

## Endpoints

- `GET /`: service info
- `GET /health`: health status
- `GET /v1/models`: model list (for Codex UI model selection)
- `POST /v1/responses`: Codex entrypoint (recommended)
- `POST /v1/chat/completions`: compatibility endpoint

## Quick Start

1) In the desktop app, set:
- DeepSeek Base URL: `https://api.deepseek.com/v1`
- API Key: your DeepSeek key
- Default model: e.g. `deepseek-v4-flash`

2) Start the proxy service (default listen: `http://127.0.0.1:11434`)

3) Verify:

```bash
curl http://127.0.0.1:11434/health
```

## Configure Codex

In the app: **Preferences → Codex config.toml**

- **Merge write**: updates `~/.codex/config.toml` while preserving other existing settings
- **Backups**: each write creates a non-overwriting backup under `~/.codex/backups/` which can be restored or deleted

Codex Base URL should point to:

```
http://127.0.0.1:11434/v1
```

## FAQ

### Codex shows 502 / Reconnecting

- Check “Recent Logs” in the app for `upstream 4xx/5xx` or forwarding failures
- If DeepSeek does not recognize a model name, update “Model Mapping” to map Codex models to `deepseek-v4-pro` / `deepseek-v4-flash`

## Development & Build

### Local development

```bash
npm -C frontend install
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
wails dev
```

### Build (example)

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
wails build -platform darwin/arm64
```
