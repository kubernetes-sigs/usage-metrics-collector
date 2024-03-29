Usage-metrics-collector will expose the following metrics:

This sample is taking from a kind cluster.

```
# HELP kube_usage_cluster_sum_limits_allocated_cpu_cores kube_usage_cluster_sum_limits_allocated_cpu_cores
# TYPE kube_usage_cluster_sum_limits_allocated_cpu_cores gauge
kube_usage_cluster_sum_limits_allocated_cpu_cores{priority_class=""} 3.1
# HELP kube_usage_cluster_sum_limits_allocated_memory_bytes kube_usage_cluster_sum_limits_allocated_memory_bytes
# TYPE kube_usage_cluster_sum_limits_allocated_memory_bytes gauge
kube_usage_cluster_sum_limits_allocated_memory_bytes{priority_class=""} 3.542089728e+09
# HELP kube_usage_cluster_sum_requests_allocated_cpu_cores kube_usage_cluster_sum_requests_allocated_cpu_cores
# TYPE kube_usage_cluster_sum_requests_allocated_cpu_cores gauge
kube_usage_cluster_sum_requests_allocated_cpu_cores{priority_class=""} 1.7
# HELP kube_usage_cluster_sum_requests_allocated_memory_bytes kube_usage_cluster_sum_requests_allocated_memory_bytes
# TYPE kube_usage_cluster_sum_requests_allocated_memory_bytes gauge
kube_usage_cluster_sum_requests_allocated_memory_bytes{priority_class=""} 2.39460608e+09
# HELP kube_usage_collection_latency_seconds collection latency
# TYPE kube_usage_collection_latency_seconds gauge
kube_usage_collection_latency_seconds 0.061926565
# HELP kube_usage_namespace_sum_limits_allocated_cpu_cores kube_usage_namespace_sum_limits_allocated_cpu_cores
# TYPE kube_usage_namespace_sum_limits_allocated_cpu_cores gauge
kube_usage_namespace_sum_limits_allocated_cpu_cores{exported_namespace="kube-system",priority_class=""} 0.1
# HELP kube_usage_namespace_sum_limits_allocated_memory_bytes kube_usage_namespace_sum_limits_allocated_memory_bytes
# TYPE kube_usage_namespace_sum_limits_allocated_memory_bytes gauge
kube_usage_namespace_sum_limits_allocated_memory_bytes{exported_namespace="kube-system",priority_class=""} 5.24288e+07
# HELP kube_usage_namespace_sum_requests_allocated_cpu_cores kube_usage_namespace_sum_requests_allocated_cpu_cores
# TYPE kube_usage_namespace_sum_requests_allocated_cpu_cores gauge
kube_usage_namespace_sum_requests_allocated_cpu_cores{exported_namespace="kube-system",priority_class=""} 0.1
# HELP kube_usage_namespace_sum_requests_allocated_memory_bytes kube_usage_namespace_sum_requests_allocated_memory_bytes
# TYPE kube_usage_namespace_sum_requests_allocated_memory_bytes gauge
kube_usage_namespace_sum_requests_allocated_memory_bytes{exported_namespace="kube-system",priority_class=""} 5.24288e+07
# HELP kube_usage_node_sum_node_allocatable_cpu_cores kube_usage_node_sum_node_allocatable_cpu_cores
# TYPE kube_usage_node_sum_node_allocatable_cpu_cores gauge
kube_usage_node_sum_node_allocatable_cpu_cores{exported_node="kind-control-plane",node_no_schedule="false"} 4
# HELP kube_usage_node_sum_node_allocatable_memory_bytes kube_usage_node_sum_node_allocatable_memory_bytes
# TYPE kube_usage_node_sum_node_allocatable_memory_bytes gauge
kube_usage_node_sum_node_allocatable_memory_bytes{exported_node="kind-control-plane",node_no_schedule="false"} 1.6628338688e+10
# HELP kube_usage_node_sum_node_capacity_cpu_cores kube_usage_node_sum_node_capacity_cpu_cores
# TYPE kube_usage_node_sum_node_capacity_cpu_cores gauge
kube_usage_node_sum_node_capacity_cpu_cores{exported_node="kind-control-plane",node_no_schedule="false"} 4
# HELP kube_usage_node_sum_node_capacity_memory_bytes kube_usage_node_sum_node_capacity_memory_bytes
# TYPE kube_usage_node_sum_node_capacity_memory_bytes gauge
kube_usage_node_sum_node_capacity_memory_bytes{exported_node="kind-control-plane",node_no_schedule="false"} 1.6628338688e+10
# HELP kube_usage_node_sum_node_limits_cpu_cores kube_usage_node_sum_node_limits_cpu_cores
# TYPE kube_usage_node_sum_node_limits_cpu_cores gauge
kube_usage_node_sum_node_limits_cpu_cores{exported_node="kind-control-plane",node_no_schedule="false"} 3.1
# HELP kube_usage_node_sum_node_limits_memory_bytes kube_usage_node_sum_node_limits_memory_bytes
# TYPE kube_usage_node_sum_node_limits_memory_bytes gauge
kube_usage_node_sum_node_limits_memory_bytes{exported_node="kind-control-plane",node_no_schedule="false"} 3.898605568e+09
# HELP kube_usage_node_sum_node_requests_cpu_cores kube_usage_node_sum_node_requests_cpu_cores
# TYPE kube_usage_node_sum_node_requests_cpu_cores gauge
kube_usage_node_sum_node_requests_cpu_cores{exported_node="kind-control-plane",node_no_schedule="false"} 2.5500000000000003
# HELP kube_usage_node_sum_node_requests_memory_bytes kube_usage_node_sum_node_requests_memory_bytes
# TYPE kube_usage_node_sum_node_requests_memory_bytes gauge
kube_usage_node_sum_node_requests_memory_bytes{exported_node="kind-control-plane",node_no_schedule="false"} 2.64626432e+09
# HELP kube_usage_nodecluster_hist_node_allocatable_cpu_cores kube_usage_nodecluster_hist_node_allocatable_cpu_cores
# TYPE kube_usage_nodecluster_hist_node_allocatable_cpu_cores histogram
kube_usage_nodecluster_hist_node_allocatable_cpu_cores_bucket{node_no_schedule="false",le="0.1"} 0
# HELP kube_usage_nodecluster_hist_node_allocatable_memory_bytes kube_usage_nodecluster_hist_node_allocatable_memory_bytes
# TYPE kube_usage_nodecluster_hist_node_allocatable_memory_bytes histogram
kube_usage_nodecluster_hist_node_allocatable_memory_bytes_bucket{node_no_schedule="false",le="4e+09"} 0
# HELP kube_usage_nodecluster_hist_node_capacity_cpu_cores kube_usage_nodecluster_hist_node_capacity_cpu_cores
# TYPE kube_usage_nodecluster_hist_node_capacity_cpu_cores histogram
kube_usage_nodecluster_hist_node_capacity_cpu_cores_bucket{node_no_schedule="false",le="0.1"} 0
# HELP kube_usage_nodecluster_hist_node_capacity_memory_bytes kube_usage_nodecluster_hist_node_capacity_memory_bytes
# TYPE kube_usage_nodecluster_hist_node_capacity_memory_bytes histogram
kube_usage_nodecluster_hist_node_capacity_memory_bytes_bucket{node_no_schedule="false",le="4e+09"} 0
# HELP kube_usage_nodecluster_hist_node_limits_cpu_cores kube_usage_nodecluster_hist_node_limits_cpu_cores
# TYPE kube_usage_nodecluster_hist_node_limits_cpu_cores histogram
kube_usage_nodecluster_hist_node_limits_cpu_cores_bucket{node_no_schedule="false",le="0.1"} 0
# HELP kube_usage_nodecluster_hist_node_limits_memory_bytes kube_usage_nodecluster_hist_node_limits_memory_bytes
# TYPE kube_usage_nodecluster_hist_node_limits_memory_bytes histogram
kube_usage_nodecluster_hist_node_limits_memory_bytes_bucket{node_no_schedule="false",le="4e+09"} 1
# HELP kube_usage_nodecluster_hist_node_requests_cpu_cores kube_usage_nodecluster_hist_node_requests_cpu_cores
# TYPE kube_usage_nodecluster_hist_node_requests_cpu_cores histogram
kube_usage_nodecluster_hist_node_requests_cpu_cores_bucket{node_no_schedule="false",le="0.1"} 0
# HELP kube_usage_nodecluster_hist_node_requests_memory_bytes kube_usage_nodecluster_hist_node_requests_memory_bytes
# TYPE kube_usage_nodecluster_hist_node_requests_memory_bytes histogram
kube_usage_nodecluster_hist_node_requests_memory_bytes_bucket{node_no_schedule="false",le="4e+09"} 1
# HELP kube_usage_nodecluster_sum_node_allocatable_cpu_cores kube_usage_nodecluster_sum_node_allocatable_cpu_cores
# TYPE kube_usage_nodecluster_sum_node_allocatable_cpu_cores gauge
kube_usage_nodecluster_sum_node_allocatable_cpu_cores{node_no_schedule="false"} 4
# HELP kube_usage_nodecluster_sum_node_allocatable_memory_bytes kube_usage_nodecluster_sum_node_allocatable_memory_bytes
# TYPE kube_usage_nodecluster_sum_node_allocatable_memory_bytes gauge
kube_usage_nodecluster_sum_node_allocatable_memory_bytes{node_no_schedule="false"} 1.6628338688e+10
# HELP kube_usage_nodecluster_sum_node_capacity_cpu_cores kube_usage_nodecluster_sum_node_capacity_cpu_cores
# TYPE kube_usage_nodecluster_sum_node_capacity_cpu_cores gauge
kube_usage_nodecluster_sum_node_capacity_cpu_cores{node_no_schedule="false"} 4
# HELP kube_usage_nodecluster_sum_node_capacity_memory_bytes kube_usage_nodecluster_sum_node_capacity_memory_bytes
# TYPE kube_usage_nodecluster_sum_node_capacity_memory_bytes gauge
kube_usage_nodecluster_sum_node_capacity_memory_bytes{node_no_schedule="false"} 1.6628338688e+10
# HELP kube_usage_nodecluster_sum_node_limits_cpu_cores kube_usage_nodecluster_sum_node_limits_cpu_cores
# TYPE kube_usage_nodecluster_sum_node_limits_cpu_cores gauge
kube_usage_nodecluster_sum_node_limits_cpu_cores{node_no_schedule="false"} 3.1
# HELP kube_usage_nodecluster_sum_node_limits_memory_bytes kube_usage_nodecluster_sum_node_limits_memory_bytes
# TYPE kube_usage_nodecluster_sum_node_limits_memory_bytes gauge
kube_usage_nodecluster_sum_node_limits_memory_bytes{node_no_schedule="false"} 3.898605568e+09
# HELP kube_usage_nodecluster_sum_node_requests_cpu_cores kube_usage_nodecluster_sum_node_requests_cpu_cores
# TYPE kube_usage_nodecluster_sum_node_requests_cpu_cores gauge
kube_usage_nodecluster_sum_node_requests_cpu_cores{node_no_schedule="false"} 2.5500000000000003
# HELP kube_usage_nodecluster_sum_node_requests_memory_bytes kube_usage_nodecluster_sum_node_requests_memory_bytes
# TYPE kube_usage_nodecluster_sum_node_requests_memory_bytes gauge
kube_usage_nodecluster_sum_node_requests_memory_bytes{node_no_schedule="false"} 2.64626432e+09
# HELP kube_usage_operation_latency_seconds operation latency
# TYPE kube_usage_operation_latency_seconds gauge
kube_usage_operation_latency_seconds{operation="collect_cgroups"} 0.002698614
# HELP kube_usage_version version
# TYPE kube_usage_version gauge
kube_usage_version{collector_instance="metrics-prometheus-collector-7fd567ff6f-m9m24",commit="8519347",date="2023-01-26_12:54:40PM",version=""} 1
# HELP kube_usage_workload_median_limits_allocated_cpu_cores kube_usage_workload_median_limits_allocated_cpu_cores
# TYPE kube_usage_workload_median_limits_allocated_cpu_cores gauge
kube_usage_workload_median_limits_allocated_cpu_cores{app="grafana",exported_container="grafana",exported_namespace="usage-metrics-collector",priority_class="",workload_api_group="apps",workload_api_version="v1",workload_kind="deployment",workload_name="grafana"} 1
# HELP kube_usage_workload_median_limits_allocated_memory_bytes kube_usage_workload_median_limits_allocated_memory_bytes
# TYPE kube_usage_workload_median_limits_allocated_memory_bytes gauge
kube_usage_workload_median_limits_allocated_memory_bytes{app="",exported_container="coredns",exported_namespace="kube-system",priority_class="system-cluster-critical",workload_api_group="apps",workload_api_version="v1",workload_kind="deployment",workload_name="coredns"} 1.7825792e+08
# HELP kube_usage_workload_median_requests_allocated_cpu_cores kube_usage_workload_median_requests_allocated_cpu_cores
# TYPE kube_usage_workload_median_requests_allocated_cpu_cores gauge
kube_usage_workload_median_requests_allocated_cpu_cores{app="",exported_container="coredns",exported_namespace="kube-system",priority_class="system-cluster-critical",workload_api_group="apps",workload_api_version="v1",workload_kind="deployment",workload_name="coredns"} 0.1
# HELP kube_usage_workload_median_requests_allocated_memory_bytes kube_usage_workload_median_requests_allocated_memory_bytes
# TYPE kube_usage_workload_median_requests_allocated_memory_bytes gauge
kube_usage_workload_median_requests_allocated_memory_bytes{app="",exported_container="coredns",exported_namespace="kube-system",priority_class="system-cluster-critical",workload_api_group="apps",workload_api_version="v1",workload_kind="deployment",workload_name="coredns"} 7.340032e+07
# HELP kube_usage_workload_sum_limits_allocated_cpu_cores kube_usage_workload_sum_limits_allocated_cpu_cores
# TYPE kube_usage_workload_sum_limits_allocated_cpu_cores gauge
kube_usage_workload_sum_limits_allocated_cpu_cores{app="grafana",exported_container="grafana",exported_namespace="usage-metrics-collector",priority_class="",workload_api_group="apps",workload_api_version="v1",workload_kind="deployment",workload_name="grafana"} 1
# HELP kube_usage_workload_sum_limits_allocated_memory_bytes kube_usage_workload_sum_limits_allocated_memory_bytes
# TYPE kube_usage_workload_sum_limits_allocated_memory_bytes gauge
kube_usage_workload_sum_limits_allocated_memory_bytes{app="",exported_container="coredns",exported_namespace="kube-system",priority_class="system-cluster-critical",workload_api_group="apps",workload_api_version="v1",workload_kind="deployment",workload_name="coredns"} 3.5651584e+08
# HELP kube_usage_workload_sum_requests_allocated_cpu_cores kube_usage_workload_sum_requests_allocated_cpu_cores
# TYPE kube_usage_workload_sum_requests_allocated_cpu_cores gauge
kube_usage_workload_sum_requests_allocated_cpu_cores{app="",exported_container="coredns",exported_namespace="kube-system",priority_class="system-cluster-critical",workload_api_group="apps",workload_api_version="v1",workload_kind="deployment",workload_name="coredns"} 0.2
# HELP kube_usage_workload_sum_requests_allocated_memory_bytes kube_usage_workload_sum_requests_allocated_memory_bytes
# TYPE kube_usage_workload_sum_requests_allocated_memory_bytes gauge
kube_usage_workload_sum_requests_allocated_memory_bytes{app="",exported_container="coredns",exported_namespace="kube-system",priority_class="system-cluster-critical",workload_api_group="apps",workload_api_version="v1",workload_kind="deployment",workload_name="coredns"} 1.4680064e+08
```
