# Metrics

[Ghoti](../README.md) reference: [Protocol](protocol.md) · [Configuration](configuration.md) · [Authentication](authentication.md) · [Metrics](metrics.md) · [Cluster](../cluster.md)

Ghoti can optionally expose runtime metrics for [Prometheus](https://prometheus.io/) to scrape. When enabled, a dedicated HTTP listener serves `GET /metrics` in the Prometheus text exposition format. Instrumentation uses lock-free counters, so it adds negligible overhead to request handling, and is disabled by default.

To enable it, add a `metrics:` section to the configuration:

```yaml
metrics:
  enabled: true
  addr: "127.0.0.1:9100"   # host:port where GET /metrics is served
```

|Config      |Description                                                          |
|------------|----------------------------------------------------------------------|
|enabled     |Enables metrics collection and the `/metrics` endpoint. Default: false.|
|addr        |Address (`host:port`) the metrics HTTP server listens on. Required when enabled.|

The endpoint is unauthenticated, so bind it to a private interface or firewall it as you would any Prometheus target.

Point Prometheus at it with a standard scrape job:

```yaml
scrape_configs:
  - job_name: ghoti
    static_configs:
      - targets: ["127.0.0.1:9100"]
```

The following Ghoti-specific metrics are exposed, in addition to the standard Go runtime (`go_*`) and process (`process_*`) collectors:

|Metric                                  |Type      |Description                                                  |
|-----------------------------------------|----------|--------------------------------------------------------------|
|`ghoti_connected_clients`                 |gauge     |Number of currently connected clients.                        |
|`ghoti_requests_total`                    |counter   |Total number of requests processed. Use `rate()` for requests per second.|
|`ghoti_request_duration_seconds`          |histogram |Request processing duration in seconds, with buckets from 50µs to 1s. Use `histogram_quantile()` for tail latency.|

For example, requests per second and p99 latency over the last five minutes:

```
rate(ghoti_requests_total[5m])
histogram_quantile(0.99, rate(ghoti_request_duration_seconds_bucket[5m]))
```
