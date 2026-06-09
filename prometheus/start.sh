#!/bin/sh
set -eu

: "${TAP_METRICS_TARGET:?Set TAP_METRICS_TARGET, e.g. tap.railway.internal:9090}"
: "${PORT:=9090}"
: "${SCRAPE_INTERVAL:=15s}"

case "${PORT}" in
  ''|*[!0-9]*)
    echo "PORT must be numeric (1-65535)." >&2
    exit 1
    ;;
esac

if [ "${PORT}" -lt 1 ] || [ "${PORT}" -gt 65535 ]; then
  echo "PORT must be numeric (1-65535)." >&2
  exit 1
fi

mkdir -p /prometheus

cat > /tmp/prometheus.yml <<EOF
global:
  scrape_interval: ${SCRAPE_INTERVAL}
  evaluation_interval: ${SCRAPE_INTERVAL}

scrape_configs:
  - job_name: tap
    metrics_path: /metrics
    static_configs:
      - targets:
          - "${TAP_METRICS_TARGET}"
EOF

echo "Starting Prometheus. Scraping Tap target: ${TAP_METRICS_TARGET}"

exec /bin/prometheus \
  --config.file=/tmp/prometheus.yml \
  --storage.tsdb.path=/prometheus \
  "--web.listen-address=[::]:${PORT}"
