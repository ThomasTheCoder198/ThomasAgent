#!/bin/sh
set -eu

source_dir=/etc/grafana/provisioning
rendered_dir=/tmp/core-provisioning

validate_threshold() {
  printf '%s' "$2" | grep -Eq '^([0-9]+([.][0-9]*)?|[.][0-9]+)([eE][+-]?[0-9]+)?$' || {
    printf 'Invalid numeric alert setting: %s\n' "$1" >&2
    exit 1
  }
}

validate_threshold GRAFANA_ALERT_5XX_RATIO "${GRAFANA_ALERT_5XX_RATIO:?missing alert threshold}"
validate_threshold GRAFANA_ALERT_P95_SECONDS "${GRAFANA_ALERT_P95_SECONDS:?missing alert threshold}"
validate_threshold GRAFANA_ALERT_DLQ_JOBS "${GRAFANA_ALERT_DLQ_JOBS:?missing alert threshold}"

mkdir -p "$rendered_dir"
cp -R "$source_dir/." "$rendered_dir/"
mkdir -p "$rendered_dir/plugins"

# Alert query models are raw JSON; Grafana does not expand their numeric environment placeholders.
sed \
  -e "s|\${GRAFANA_ALERT_5XX_RATIO}|$GRAFANA_ALERT_5XX_RATIO|g" \
  -e "s|\${GRAFANA_ALERT_P95_SECONDS}|$GRAFANA_ALERT_P95_SECONDS|g" \
  -e "s|\${GRAFANA_ALERT_DLQ_JOBS}|$GRAFANA_ALERT_DLQ_JOBS|g" \
  "$source_dir/alerting/core-alerts.yaml" > "$rendered_dir/alerting/core-alerts.yaml"

export GF_PATHS_PROVISIONING="$rendered_dir"
exec /run.sh "$@"
