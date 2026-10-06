# VictoriaLogs image

## Purpose
Keeps VictoriaLogs pinned to `v1.53.0` while adding the static BusyBox binary needed by its HTTP healthcheck. The upstream image contains no shell or `wget`.

## Entry points
- `Dockerfile` copies BusyBox from `busybox:1.37.0-musl` into the pinned upstream image.
- The parent compose file invokes `/bin/busybox wget` against `/health`.

## Dependencies
- Uses: pinned BusyBox and VictoriaLogs images.
- Used by: the core stack's `victorialogs` service.

## Run & test
```bash
task up
curl -fsS http://127.0.0.1:9428/health
```

## Conventions
The probe runs inside the existing container. VictoriaLogs entrypoint and runtime configuration remain upstream defaults; retention and log rotation are configured by the parent stack.

## Common failures
- Healthcheck executable missing: rebuild the wrapper image before starting the stack.
