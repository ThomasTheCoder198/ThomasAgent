# ThomasAgent

Self-hosted AI workspace (one operator, many tenants): a built-in agent that answers from your Knowledge Base with verifiable citations and acts through Apps, MCP servers and Composio.

Org settings carry a monotonic `settingsVersion` from core to ragx, so concurrent edits settle on the newest version. Ragx's HTTP keep-alive (75 seconds) must exceed core's ragx idle connection timeout (60 seconds).

- Product truth: `PRODUCT.md` · Design spec: `docs/superpowers/specs/2026-10-03-thomasagent-design.md`
- Plans: `docs/superpowers/plans/` · Handoffs: `docs/superpowers/handoffs/`
- Agent rules: `AGENTS.md`

## Quick start

```bash
cp deploy/core/.env.example deploy/core/.env   # fill in secrets
cp deploy/ragx/.env.example deploy/ragx/.env   # ragx stack (task up:ragx)
task ragx:dev-key   # put the printed RAG_CORE_PUBLIC_KEYS into deploy/ragx/.env
task up:ragx   # ragx + MinIO/Qdrant/Phoenix
task ragx:seed-dev   # embedding role that org provisioning needs
task up        # core + Postgres/Redis + observability
task smoke     # health checks, including the upload path through core
task web:dev   # run in a separate terminal; host frontend, WEB_CORE_URL defaults to http://localhost:8080
```

## Development checks

```bash
task fmt
task lint:core
task test:core
task web:dev:mock   # frontend + fake core on the host
task lint:frontend # lint + typecheck; also included in task lint
task test:frontend # unit tests; also included in task test
```

Local Go tests omit the race detector; Linux CI runs it.
Install frontend dependencies with `npm install` from `frontend/`; see [frontend/README.md](frontend/README.md).
