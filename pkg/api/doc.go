// Copyright 2023 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
