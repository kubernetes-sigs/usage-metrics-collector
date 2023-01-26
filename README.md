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

## Exposed Metrics

A sample of the exposed metrics is available in [METRICS.md](METRICS.md).

## Getting started

### Installing into a cluster

#### Kind cluster

Note only cgroups v1 are currently supported.

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

#### Remote cluster

1. Build the image
  - `docker build . -t my-org/usage-metrics-collector:v0.0.0`
2. Push the image to a container repo
  - `docker push my-org/usage-metrics-collector:v0.0.0`
3. Update the sampler config to reflect your cluster cgroup layout
  - edit [config/metrics-node-sampler/configmaps/sampler.yaml](config/metrics-node-sampler/configmaps/sampler.yaml)
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
4. View the metrics in Grafana
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
