# Prometheus

## Purpose
Scrapes core and core-worker `/metrics` on ports 9464 and 9465, using OpenMetrics with request-id exemplars.

## Entry points
- `prometheus.yml` configures the two scrape targets.

## Dependencies
- Uses: core and core-worker internal metrics listeners.
- Used by: Grafana's Prometheus datasource and alerts.

## Run & test
```bash
task up
curl -fsS "http://127.0.0.1:${PROMETHEUS_HOST_PORT:-9090}/api/v1/targets"
```

## Conventions
Retention comes from `PROMETHEUS_RETENTION`; exemplar storage is enabled on the compose command line. The host port binds to `127.0.0.1`.

## Common failures
- Target down: check core health and whether `CORE_METRICS_ADDR` matches the configured target.
