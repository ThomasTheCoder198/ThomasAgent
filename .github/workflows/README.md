# Continuous integration

## Purpose

Checks contracts, Go core, Python RAG and frontend changes independently on Linux.

## Entry points

- `ci.yml` runs on pull requests and pushes to main.

## Dependencies

- Uses pinned Go, Task, Node, golangci-lint and uv versions from the project plan.
- Used by repository verification before merging.

## Run & test

Run `task gen`, `task lint` and `task test` locally with Docker available.
The core job installs Task and contract npm dependencies because its generated
Postman integration test exercises real HTTP against disposable Postgres/Redis.
The web job uses Node 24.21.0 and `npm ci` with the frontend lockfile, then runs
the production dependency audit (high or critical fails), lint, typecheck, unit
tests and the production build. E2E stays outside CI because
it needs a running app and core (or fake core).

## Conventions

After `task gen`, CI checks both tracked diffs and untracked files from `git status --porcelain --untracked-files=all`; new generated outputs cannot silently pass. Generation remains deterministic. Linux retains Go race checks; Windows uses
bounded package concurrency. API fixtures never change existing platform roles.

## Common failures

- Missing npm dependencies: run `npm ci` from contracts before API integration tests.
- Docker unavailable: Postgres/Redis integration checks require a reachable engine.
