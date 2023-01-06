// Package collector implements the metrics-prometheus-collector which exports
// metrics to prometheus when scraped.
//
// Collector reads utilization metrics from the metrics-node-sampler, and other
// metrics directly from the Kubernetes objects.
//
// Collector is highly configurable with respect to how metrics are labeled and
// aggregated at the time they are collected.
package collector
