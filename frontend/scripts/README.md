# frontend/scripts

## Purpose

Local frontend development helpers and synthetic stream fixtures.

## Entry points

- `dev-mock.ts` starts mock-core and Next together for `npm run dev:mock`.
- `capture-ui-stream-goldens.ts` writes the AI SDK stream fixtures consumed by core's Go encoder tests.

## Dependencies

The capture script uses `ai` and Node file APIs; the dev script starts the local mock and Next processes.

## Run & test

From `frontend/`, run `node scripts/capture-ui-stream-goldens.ts` to refresh the core fixtures. Set
`UI_STREAM_GOLDENS_OUT` to a temporary directory to compare output without replacing the checked fixtures.
Run `npm run lint` after edits.

## Conventions

Keep stream content synthetic and the default golden output stable unless the wire contract changes.

## Common failures

Run the capture script from `frontend/`; its default destination is relative to the script's location.
