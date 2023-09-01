# Usage Metrics Collector

The usage-metrics-collector is a Prometheus metrics collector optimized for collecting kube usage and
capacity metrics.

## Motivation

Why not just use promql and recording rules?

- Scale
  - Aggregate at collection time to reduce prometheus work
  - Export aggregated metrics without the raw metrics to reduce prometheus storage
- Insight
  - Join labels at collection time (e.g. set the priority class on pod metrics)
  - Set hard to resolve labels (e.g. set the workload kind on pod metrics)
  - View node-level cgroup utilization (e.g. kubepods vs system.slice metrics)
- Fidelity
  - Scrape utilization at 1s intervals as raw metrics
  - Perform aggregations on the 1s interval metrics (e.g. get the p95 1s utilization sample for all replicas of a workload)

### Example

[collector.yaml](config/metrics-prometheus-collector/configmaps/collector.yaml)

## Architecture

### considerations

- Metrics must be highly configurable
  - The metrics labels derived from objects
  - The aggregations (sum, average, median, max, p95, histograms, etc) must be configurable
- Metrics should be able to be pushed to additional sources such as cloud storage buckets, BigQuery, etc
- Metric computation must scale to large clusters with lots of churn.
  - Run aggregations in parallel
  - Re-use previous results as much as possible
- Scrapes should always immediately get a result, even when complex aggregations in large clusters take minutes to compute.
- Utilization metrics should not be published until the data is present from a sufficient number of nodes.
  This is to prevent showing "low" utilization numbers before all nodes have sent results.
- There should be no graphs in data.  There must always be at least 1 ready and healthy replica which can be scraped by prometheus.
- Sampler pods which become unhealthy due to issues on a node should be continuously recreated until they are functional again.
- All cluster objects and utilization samples are cached in the collector so memory must be optimized.
- It is difficult to horizontally scale the collector.  Offloading computations to the samplers is preferred.

### metrics-node-sampler

- Runs as a DaemonSet on each Node
- Periodically reads utilization data for cpu and memory and stores in ring buffer
  - Period and number of samples is configurable
- Reads host metrics directly from cgroups psuedo filesystem (e.g. cpu usage for all of kubepods cgroup)
- Reads container metrics from containerd (e.g. cpu usage for an individual container)
- Periodically pushes metrics to collectors
  - After time period
  - After new pod starts running
  - Before shutting down
- Finds collectors to push to via DNS
- Collectors can register manually with each sampler
- Performs some precomputations such as averages.

### metrics-prometheus-collector

- Runs as a Deployment with multiple replicas for HA
- Highly configurable metrics
  - Metric labels may be derived from annotations / labels on other objects (e.g. pod metrics should have metric labels pulled from node conditions)
  - Metrics may be pre-aggregated prior to being scraped by prometheus (e.g. reduce cardinality, produce quantiles and histograms)
- Periodically get the metrics and cache them to be scraped (i.e. minimize scrape time by eagerly computing results)
- Registers itself and all collector replicas with each sampler
- Recieves metrics from node-samplers as a utilization source
- Waits until has sufficient samples before providing results
- Waits until results have been scraped before marking self as Ready
- Can write additional metrics to local files

### collector side-cars

- May expose additional metrics read from external sources
- May write local files to persistent storage for futher analysis

## Exposed Metrics

A sample of the exposed metrics is available in [METRICS.md](METRICS.md).

In addition to these metrics, a series of performance related metrics are published for the collection process.
These metrics are documented in [performance analysis document](docs/performance-analysis.md).

## Getting started

**Note**: No usage-metrics-collector container image is publicly hosted.  Folks will need to build and publish
this own until this is resolved.

### Installing into a cluster

#### Kind cluster

**Important**: requires using cgroups v1.

- Must set for Docker on Mac using [these docs](https://docs.docker.com/desktop/release-notes/#for-mac-28)
- Must set for GKE for 1.26+ clusters

1. Create a kind cluster
  - `kind create cluster`
2. Build the image
  - `docker build . -t usage-metrics-collector:v0.0.0`
3. Load the image into kind
  - `kind load docker-image usage-metrics-collector:v0.0.0`
4. Install the config
  - `kustomize build config | kubectl apply -f -`
5. Update your context to use the usage-metrics-collector namespace by default
  - `kubectl config set-context --current --namespace=usage-metrics-collector`

### Kicking the tires

1. Make sure the pods are healthy
  - `kubectl get pods`
2. Make sure the services have endpoints
  - `kubectl describe services`
3. Get the metrics from the collector itself
  - `kubectl exec -t -i $(kubectl get pods -o name -l app=metrics-prometheus-collector) -- curl localhost:8080/metrics`
  - wait for service to be ready
  - `kubectl port-forward service/metrics-prometheus-collector 8080:8080`
  - visit `localhost:8080/metrics` in your browser
4. Get the metrics from prometheus
  - `kubectl port-forward $(kubectl get pods -o name -l app=prometheus) 9090:9090`
  - visit `localhost:9090/` in your browser
5. View the metrics in Grafana
  - `kubectl port-forward service/grafana 3000:3000`
  - visit `localhost:3000` in your browser
  - enter `admin` for the username and password
  - go to "Explore"
  - change the source to "prometheus"
  - enter `kube_usage_` into the metric field
  - remove the label filters
  - click "Run Query"

### Specifying aggregation rules

1. Edit [config/metrics-prometheus-collector/configmaps/collector.yaml](config/metrics-prometheus-collector/configmaps/collector.yaml)
2. Run `make run-local`
3. View the updated metrics in grafana

## Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
