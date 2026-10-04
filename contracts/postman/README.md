# Postman tooling

## Purpose

Produces the generated Postman collection and validates its deterministic scripts and authenticated request order.

## Entry points

- `postprocess.mjs` — processes converter output into the committed collection.
- `postprocess.test.mjs` — Node test runner regression suite.
- `run.mjs` — environment-based programmatic Newman runner; emits a summary without request/response content.
- `run.test.mjs` — runner configuration and execution regressions with a fake Newman adapter.
- `thomasagent.postman_collection.json` — generated collection, imported into Postman or executed by Newman.
- `local.postman_environment.json` — local URL and empty owner credential variables.

## Dependencies

- Uses: `../openapi/core.v1.yaml`, Node, openapi-to-postmanv2, and Newman.
- Used by: `task gen`, `task test:contracts`, and `task api-test`.

## Run & test

From `contracts/`:

```bash
npm test
npm run gen:postman
```

From the repository root, against a disposable running core:

```bash
task api-test
```

## Conventions

Never hand-edit the collection; edit the OpenAPI or postprocessor and regenerate.
The converter's random IDs/responses are removed, and optional generated filters are disabled.
The cookie jar authenticates public requests. Login stores the CSRF token; mutations send it.
Role assignment runs before the in-use deletion checks; logout runs last.
Run the complete collection against an isolated backend: it assigns `chat.fast`.
The example provider uses a denied loopback HTTP endpoint so connection/sync
checks deterministically return 400 without calling a real external provider.
Positive catalog/sync behavior is covered by Registry integration tests.
Internal resolution retains its required role query and uses `serviceToken`; the runner includes it when `CORE_SERVICE_TOKEN` is supplied.
Keep credentials out of committed environments and generated artifacts. See the parent README for environment overrides.

## Common failures

- Generation changes on each run → random converter metadata was retained → extend deterministic normalization with a regression test.
- Protected request returns unauthorized → cookie/session flow failed → verify login and the active server URL.
- Required query missing → required examples were disabled → preserve the required query in normalization.
