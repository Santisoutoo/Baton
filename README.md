# Baton

**Claude Code orchestrates, OpenCode executes.**

`Baton` is a small local proxy that sits between Claude Code and the model APIs.
The main model (opus/sonnet) keeps planning/orchestrating through **your Claude
subscription**, while execution-tier work (the `haiku` tier / your implementation
subagents) is routed to cheaper **OpenCode** models. Claude Code doesn't change —
you point one environment variable at Baton and keep working as usual.

```
Claude Code ──(Anthropic /v1/messages)──▶ Baton ──┬─ plan  → api.anthropic.com (your subscription, passthrough)
             ANTHROPIC_BASE_URL=127.0.0.1:8787      └─ exec  → opencode.ai (translated to OpenAI wire if needed)
```

## Why

Claude Code only speaks the Anthropic Messages protocol and only lets you change
its backend via `ANTHROPIC_BASE_URL`. OpenCode models are mostly OpenAI-compatible
(some Anthropic-native). Baton is the translator in the middle: it routes by
model tier, forwards the Claude lane untouched (so your subscription auth is
reused verbatim), and translates the OpenCode lane both ways — including
incremental SSE streaming and tool calls.

## Install

Requires [Go](https://go.dev/dl/) 1.24+.

```bash
go build -o bin/Baton ./cmd/Baton      # or: go install ./cmd/Baton
```

## Quickstart

```bash
Baton init                 # store your OpenCode key + pick the execution model
Baton claude               # launches Claude Code through the proxy for you
```

…or wire it up manually:

```bash
Baton login opencode                          # store the OpenCode API key (OS keyring)
Baton models --exec deepseek-v4-flash         # choose the execution model
Baton serve                                   # start the proxy on 127.0.0.1:8787
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ENABLE_TOOL_SEARCH=true                 # if you use MCP tool search
claude                                         # Claude Code, unchanged
```

To send execution work to OpenCode, mark your implementation subagents with
`model: haiku` in their `.claude/agents/*.md` frontmatter — Baton routes the
`haiku` tier to the execute lane.

## Commands

| Command | What it does |
|---|---|
| `Baton init` | First-run wizard: OpenCode login + execution model + prints the env. |
| `Baton claude [args…]` | Starts the proxy, sets the env, and launches `claude` for you. |
| `Baton serve [--port] [--echo] [--log-level]` | Runs the proxy (loopback only). `--echo` = validation mode. |
| `Baton login <svc>` / `logout <svc>` | Store / remove a provider key in the OS keyring. |
| `Baton models [--plan] [--exec]` | List models, or set the plan/exec model. |
| `Baton usage [--since] [--by]` | Local token/cost report (model\|backend\|day). |
| `Baton status` | Config paths, routing, and which credentials are set. |
| `Baton config path\|show\|init` | Inspect or scaffold configuration. |

## Validation step 0 (do this first)

The one hard assumption is that Claude Code forwards your subscription token to a
custom `ANTHROPIC_BASE_URL`. Confirm it before relying on the Claude lane:

```bash
Baton serve --echo --port 8799
# in another shell:
ANTHROPIC_BASE_URL=http://127.0.0.1:8799 claude
```

Watch the echo logs: `has_authorization=true` means the subscription token is
being forwarded and passthrough will work. (Secrets are redacted in logs.)

## Configuration

Two layers, merged key-by-key: global `~/.config/Baton/config.toml`, then a
per-project `.Baton.toml` found by walking up from the working directory. See
[`config.example.toml`](config.example.toml).

```toml
on_exec_error = "fallback-claude"   # or "error"

[roles.plan]
backend = "anthropic"               # subscription passthrough; model kept as-is

[roles.execute]
backend = "opencode"
model   = "deepseek-v4-flash"       # the cheap execution model

[tiers]
opus = "plan"; sonnet = "plan"; haiku = "execute"

[backends.opencode]
type = "openai"                     # or "anthropic" for MiniMax/Qwen on OpenCode Go
base_url = "https://opencode.ai/zen"
auth = "bearer"
credential = "opencode"
```

Adding a new provider (OpenRouter, Groq, a local model, …) is a new
`[backends.x]` table plus a role that points at it — no code changes.

## Latency

The proxy adds single-digit milliseconds; perceived latency is dominated by which
model you route to, not by Baton. The Claude lane is a straight reverse-proxy;
the OpenCode lane translates incrementally and **never buffers the stream**, so
time-to-first-token is preserved.

## Security

- Binds to `127.0.0.1` only — never reachable from the network.
- API keys live in the OS keyring, never in config files.
- Auth headers and keys are redacted in logs.

## Notes

- OpenCode Go has usage limits ($12/5h, $30/week, $60/month); on 429/5xx the
  execute lane falls back to your Claude subscription so the session never breaks.
- Free OpenCode Zen models may use your traffic to improve their models — don't
  send sensitive code to them; use OpenCode Go (zero-retention) for that.
