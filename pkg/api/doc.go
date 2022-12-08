// Package api defines the APIs.
//
// # Collector
//
// Collector exports aggregated metrics to prometheus.  It exposes a /metrics endpoint which
// is periodically scraped by prometheus.
//
// The metrics exported by the collector may be configured using the MetricPrometheusCollector API
// which is provided to the collector via a local file (e.g. frequently as  ConfigMap).
//
// The collector gets metrics from the following locations:
//
//   - Kubernetes objects
//   - Pods (containers) requests / limits
//   - ResourceQuota hard / used
//   - Node allocatable / capacity
//   - Sampler
//   - Utilization
//   - CPU throttling
//   - OOM events
//
// # Sampler
//
// Sampler runs on each node and periodically samples container stats.
//
//   - CPU usage
//   - Memory usage
//   - CPU throttling
//   - Memory OOM events
//
// The sampler stores a collection of samples made over a sliding time window
// in memory, and serves aggregated values as results -- e.g. the max, p95,
// average values of the samples.
package api
