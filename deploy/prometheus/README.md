# prometheus

## Purpose
Scrapes collector metrics.

## Entry points
- prometheus.yml

## Dependencies
- Uses services configured in ../compose.
- Used by the deployment compose stack.

## Run & test
```bash
task up
task smoke
```

## Conventions
Configuration is mounted read only; credentials come from compose/.env.

## Common failures
- Startup errors: inspect compose logs and validate environment variables.
