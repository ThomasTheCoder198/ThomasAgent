# ThomasAgent

Self-hosted AI workspace (one operator, many tenants): a built-in agent that answers from your Knowledge Base with verifiable citations and acts through Apps, MCP servers and Composio.

- Product truth: `PRODUCT.md` · Design spec: `docs/superpowers/specs/2026-10-03-thomasagent-design.md`
- Plans: `docs/superpowers/plans/` · Handoffs: `docs/superpowers/handoffs/`
- Agent rules: `AGENTS.md`

## Quick start

```bash
cp deploy/compose/.env.example deploy/compose/.env   # fill in secrets
task up        # infra + observability + services
task smoke     # health checks
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
