# Grafana provisioning

## Purpose
Provisions Prometheus and VictoriaLogs datasources, the HTTP RED/LLM/jobs dashboards and three core alert rules.

## Entry points
- `provisioning/datasources/datasources.yaml` configures metric/log sources and request/trace links.
- `provisioning/dashboards/dashboards.yaml` loads `http-red.json`, `llm.json` and `jobs.json` into the Core folder.
- `provisioning/alerting/core-alerts.yaml` defines the 5xx ratio, p95 latency and DLQ rules.
- `entrypoint.sh` renders numeric alert thresholds before invoking the image's `/run.sh` entrypoint.

## Dependencies
- Uses: Grafana, the pinned VictoriaLogs datasource plugin, Prometheus and VictoriaLogs.
- Used by: the core compose stack and operator observability workflows.

## Run & test
```bash
task up
task smoke
# Sign in at http://localhost:3001 using the locally configured admin password.
```

## Conventions
Files are the source of truth (`allowUiUpdates: false`). Grafana expands environment variables during provisioning; `$$` preserves a literal dollar sign for later link interpolation.

The `request_id` exemplar links to VictoriaLogs Explore and preserves the source dashboard's time range using `__from` and `__to`, so older points still find their logs. The `ragx_trace_id` derived field links to `GRAFANA_PHOENIX_URL/redirects/traces/<id>`; Phoenix arrives in M0.5.3. Alert thresholds come from `GRAFANA_ALERT_5XX_RATIO`, `GRAFANA_ALERT_P95_SECONDS` and `GRAFANA_ALERT_DLQ_JOBS`. Alerts have no external notification destination.

Alert query models are raw JSON and do not receive Grafana's ordinary environment expansion. The entrypoint validates numeric thresholds, renders only the three alert variables into a writable temporary provisioning tree, and leaves the mounted source files unchanged. Datasource variables and escaped link macros retain Grafana's own interpolation.

OTLP stores log levels in `severity_text`; error filters and level-color rules use that field. Exemplar Explore queries explicitly request raw logs and group volume by the same observed field.

Outbox backlog and lag panels select `job="core-worker"`, the process that updates those gauges. The API registry also exposes their initial zero values; those series are excluded from the jobs dashboard.

## Common failures
- Datasource plugin missing: check `GF_PLUGINS_PREINSTALL`, network access and pinned plugin availability.
- Empty model/job panels: these series appear when the corresponding call sites or workers record activity.
