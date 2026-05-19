# Prometheus metrics service

This directory contains a tiny Prometheus image for scraping Tap metrics. It generates `prometheus.yml` at startup because Prometheus does not expand environment variables inside its config file.

## Railway service

Deploy this as its own Railway service with these Railway settings:

- Root Directory: `/prometheus`
- Railway Config File: `/prometheus/railway.toml`
- Dockerfile Path: `Dockerfile`

The included `railway.toml` sets the healthcheck path to `/-/healthy`.

If build logs still say `load build definition from prometheus/Dockerfile`, the service is still building from the repository root instead of `/prometheus`.

Required variables:

```env
TAP_METRICS_TARGET=${{Tap.RAILWAY_PRIVATE_DOMAIN}}:9090
PORT=9090
SCRAPE_INTERVAL=15s
```

If the Tap service is named `tap`, use:

```env
TAP_METRICS_TARGET=${{tap.RAILWAY_PRIVATE_DOMAIN}}:9090
```

`TAP_METRICS_TARGET` must be a `host:port` value only, with no `http://` prefix.

## Tap service

Set this on the Tap service and redeploy it:

```env
TAP_METRICS_LISTEN=:9090
```

## Grafana

Add Prometheus as a Grafana data source using the Prometheus private domain:

```txt
http://PROMETHEUS_PRIVATE_DOMAIN:9090
```

Test in Grafana Explore:

```promql
up{job="tap"}
```

It should return `1` once Prometheus can scrape Tap.
