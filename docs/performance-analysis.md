# Performance Analysis

This document covers the performance analysis tools included in this project's executables.
The goal of the document is not to list all of the past or potential performance improvements,
but rather describing the tools available for those wanting to perform their own analysis.

## metrics-node-sampler

The metrics-node-sampler binary exposes a pprof endpoint which can be used to profile the performance
of the program my analyzing its cpu, memory, and locking activity.

The endpoint is always accessible on the same port as the sampler's JSON API.
In order to perform your analysis, you will use the `pprof` tool that comes with your Go toolchain.
You will need to replace `SAMPLER_IP_PORT` with the IP and port where the sampler can be found.

```bash
    go tool pprof -http=:6060 -seconds 300 http://SAMPLER_IP_PORT/debug/pprof/profile
```

## metrics-prometheus-collector

The metrics-prometheus-collector also exposes a pprof endpoint, but in order to access it you will need
to enable via the flag `pprof=true`. This will expose the pprof endpoint on port 6060, but that can also
be configured via the flag `pprof-port`.

A series of metrics is also collected tracking the time taken to process each aggregation, level, and
operation. These metrics are named:

- `kube_usage_metric_aggregation_latency_seconds`: how long it took to complete each one of the four phases of metric aggregation/collection (i.e. `mapping`, `sorting`, `aggregation_total`, and `collection_total`).
- `kube_usage_metric_aggregation_per_aggregated_metric_latency_seconds`: how long it took to aggregate each one of the published aggregated metrics. Note that this does not include the mapping and sorting phases, common to all aggregated metrics.
- `kube_usage_metric_aggregation_per_operation_latency_seconds`: how long it took to perform each operation at a given level. The time that takes to perform all of the operations at a given level is the `aggregation_total` phase in the first metric of this list.

Note that since the operations above are performed in parallel, the actual collection time observed will always be shorter than the sum of latencies.
In order to track these global latencies, the collector also exposes the following metrics:

- `kube_usage_collection_latency_seconds`: the total collection clock time observed.
- `kube_usage_operation_latency_seconds`: the total collection time per operation, indicated in the `operation` label with one of the following values (`collect_quotas`, `collect_cluster_scoped`, `list_namespaces`, `list_pvs`, `list_rqds`, `list_quotas`, `collect_pvcs`, `list_pvcs`, `list_cluster_scoped`, `collect_containers`, `collect_pvs`, `list_sampler_pods`, `list_nodes`, `collect_billing_units`, `collect_cgroups`, `list_pods`, `collect_nodes`, `collect_namespace`)
- `metrics_prometheus_collector_save_local_latency_seconds`: the time that took to store the latest collection to local disk.
