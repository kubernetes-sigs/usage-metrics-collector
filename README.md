# Usage Metrics Collector

A Prometheus metrics collector optimized for Kubernetes usage and capacity metrics.

## Overview

The usage-metrics-collector provides enhanced metrics collection for Kubernetes environments, focusing on resource usage and capacity monitoring with improved scalability and insights.

## Why Use This Instead of Standard Prometheus?

### Scale Optimization
- Aggregates metrics at collection time to reduce Prometheus workload
- Exports only aggregated metrics to reduce storage requirements

### Enhanced Insights
- Joins labels at collection time (e.g., adds priority class to pod metrics)
- Sets hard-to-resolve labels (e.g., workload kind on pod metrics)
- Provides node-level cgroup utilization metrics (kubepods vs system.slice)

### High Fidelity
- Scrapes utilization at 1s intervals as raw metrics
- Performs aggregations on high-frequency metrics (e.g., p95 utilization across workload replicas)

### Configuration Example
See [collector.yaml](config/metrics-prometheus-collector/configmaps/collector.yaml) for a sample configuration.

## Architecture

### Design Considerations

- **Configurability**: Highly configurable metrics with customizable labels and aggregations
- **Extensibility**: Push metrics to additional sources (cloud storage, BigQuery)
- **Scalability**: Parallel aggregations and result caching for large clusters
- **Responsiveness**: Immediate results for Prometheus scrapes, even for complex calculations
- **Accuracy**: Only publishes metrics after collecting sufficient data from nodes
- **Reliability**: Maintains healthy replicas for consistent scraping
- **Resilience**: Auto-recreates unhealthy sampler pods
- **Efficiency**: Optimized memory usage for cached objects and samples
- **Performance**: Offloads computation to samplers where possible

### Components

#### metrics-node-sampler
- Runs as a DaemonSet on each Node
- Collects CPU and memory utilization data in a configurable ring buffer
- Reads host metrics from cgroups filesystem and container metrics from containerd
- Pushes metrics to collectors on schedule, pod creation, and before shutdown
- Discovers collectors via DNS and accepts manual collector registration
- Performs initial aggregations like averages

#### metrics-prometheus-collector
- Runs as a high-availability Deployment
- Configures metrics with rich label derivation capabilities
- Pre-aggregates metrics to reduce cardinality and compute histograms/quantiles
- Caches computed results to minimize scrape time
- Registers with all node samplers to receive utilization data
- Waits for sufficient samples before exposing metrics
- Signals readiness after initial scrape
- Supports writing additional metrics to local files

#### collector side-cars
- Expose metrics from external sources
- Write metrics to persistent storage for further analysis

## Metrics

A sample of the exposed metrics is available in [METRICS.md](METRICS.md).

Performance-related metrics for the collection process are documented in the [performance analysis document](docs/performance-analysis.md).

## Getting Started

**Note**: No usage-metrics-collector container image is publicly hosted. You'll need to build and publish your own.

### Installation with Kind

**Important**: Requires cgroups v1
- For Docker on Mac: Configure using [these docs](https://docs.docker.com/desktop/release-notes/#for-mac-28)
- For GKE 1.26+ clusters: Enable cgroups v1

1. Create a kind cluster:
   ```
   kind create cluster
   ```

2. Build the image:
   ```
   docker build . -t usage-metrics-collector:v0.0.0
   ```

3. Load the image into kind:
   ```
   kind load docker-image usage-metrics-collector:v0.0.0
   ```

4. Install the configuration:
   ```
   kustomize build config | kubectl apply -f -
   ```

5. Set your context to the collector namespace:
   ```
   kubectl config set-context --current --namespace=usage-metrics-collector
   ```

### Verification Steps

1. Check pod health:
   ```
   kubectl get pods
   ```

2. Verify service endpoints:
   ```
   kubectl describe services
   ```

3. View collector metrics directly:
   ```
   kubectl exec -t -i $(kubectl get pods -o name -l app=metrics-prometheus-collector) -- curl localhost:8080/metrics
   ```
   
   Or using port forwarding:
   ```
   kubectl port-forward service/metrics-prometheus-collector 8080:8080
   ```
   Then visit `localhost:8080/metrics` in your browser.

4. Access Prometheus metrics:
   ```
   kubectl port-forward $(kubectl get pods -o name -l app=prometheus) 9090:9090
   ```
   Then visit `localhost:9090/` in your browser.

5. View metrics in Grafana:
   ```
   kubectl port-forward service/grafana 3000:3000
   ```
   Then:
   - Visit `localhost:3000` in your browser
   - Login with username/password: `admin`/`admin`
   - Go to "Explore"
   - Select "prometheus" as the source
   - Enter `kube_usage_` in the metric field
   - Remove any label filters
   - Click "Run Query"

### Customizing Aggregation Rules

1. Edit [config/metrics-prometheus-collector/configmaps/collector.yaml](config/metrics-prometheus-collector/configmaps/collector.yaml)
2. Run `make run-local`
3. View the updated metrics in Grafana

## Code of Conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
