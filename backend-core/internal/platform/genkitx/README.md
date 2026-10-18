# genkitx

## Purpose

Pins Genkit and verifies the provider behavior required by the M2 agent run loop with local contract fixtures. This module contains tests and reusable test fixtures only.

## Entry points

- `contract_test.go` — checks provider key isolation, streamed tool requests and reasoning, tool response messages, and System One decisions.
- `genkitxtest.NewOpenAIServer` and `genkitxtest.NewDecisionsServer` — scripted provider fixtures for core tests.

## Dependencies

- Uses: `github.com/firebase/genkit/go` at `v1.13.2-0.20261015143913-59ba69c57cb1`, OpenAI Go v1.8.2, and the core request ID context.
- Used by: later M2 runtime and integration tests.

The Genkit pseudo-version pins a main-branch commit because the latest tag does not include the System One plugin required by the OpenRouter Decisions API.

## Run & test

```bash
cd backend-core
go test ./internal/platform/genkitx/... -count=1
```

## Conventions

A Genkit bump must keep `contract_test.go` green. `genkitxtest` is test support only.

## Common failures

- A contract test fails after a bump → read the pinned module source and check the provider wire shape; do not patch the run loop around it.
