# Continuous integration

## Purpose

Checks contracts, Go core and Python RAG changes independently on Linux.

## Entry points

- `ci.yml` runs on pull requests and pushes to main.

## Dependencies

- Uses pinned Go, Task, Node, golangci-lint and uv versions from the project plan.
- Used by repository verification before merging.

## Run & test

Run `task gen`, `task lint` and `task test` locally with Docker available.
The core job installs Task and contract npm dependencies because its generated
Postman integration test exercises real HTTP against disposable Postgres/Redis.

## Conventions

Generation remains deterministic. Linux retains Go race checks; Windows uses
bounded package concurrency. API fixtures never change existing platform roles.

## Common failures

- Missing npm dependencies: run `npm ci` from contracts before API integration tests.
- Docker unavailable: Postgres/Redis integration checks require a reachable engine.
