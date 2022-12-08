// Package collectorcontrollerv1alpha1 defines the MetricsPrometheusCollector API.
//
// The collector is highly configurable and works with Kubernetes extension types (e.g. CRDs)
// and arbitrary compute resource types.  The metrics-prometheus-collector binary will print
// its configuration to stdout on startup.
//
// The configuration is provided to the binary via the --config-filepath.
//
//	# run the binary
//	metrics-prometheus-collector --config-filepath collector.yaml
//
// Example MetricsPrometheusCollector
//
//		apiVersion: v1alpha1
//		kind: MetricsPrometheusCollector
//		prefix: kube_usage # all exported metrics share this prefix
//		resources: # specify which compute resources to export metrics for
//		  cpu: cpu_cores # get cpu from the container resources and name the metric cpu_cores
//		  memory: memory_bytes
//		cgroupMetrics: # specify collecting cgroup-level metrics from nodes
//		  sources: # parent directory mapped to source name
//		    "/": {name: "root_utilization"}
//		    "/kubepods": {name: "kubepods_utilization"}
//		    "/system.slice": {name: "system_utilization"}
//		  rootSource: {name: "utilization"}
//		workloadUtilization:
//		  cpuBucketSizes: [0.5, 1, 2, 4, 6]
//		  memoryBucketSizes: [10000000, 15000000, 20000000, 25000000, 35000000]
//		  workloadContainerMask:
//		    name: "workloadcontainer"
//		    builtIn:
//		      exported_container: true
//		      exported_namespace: true
//		      workload_name: true
//		      workload_kind: true
//		      workload_api_group: true
//		      workload_api_version: true
//		aggregations: # define which metrics are exported
//		- sources: # specify which sources to use for this aggregation
//		    type: container # aggregate container sources
//		    container: # select container sources to aggregate
//		    - p95_utilization
//		    - requests_allocated
//		  levels:
//		  - operation: sum # sum all metrics that have the same labels (i.e. pods in a workload)
//		    mask:
//		      name: workload # user defined name for this level
//		      builtIn: # define which labels are present at this level
//		        exported_namespace: true
//		        workload_api_group: true
//		        workload_api_version: true
//		        workload_kind: true
//		        workload_name: true
//		  - operation: sum # sum all metrics that have the same labels (i.e. workloads in a namespace)
//		    mask:
//		      name: namespace # user defined name for this level
//		      builtIn: # define which labels are present at this level
//		        exported_namespace: true
//		  - operation: sum # sum all namespaces in the cluster
//		    mask:
//		      name: cluster # sum all metrics that have the same labels (i.e. namespaces in a cluster)
//		      builtIn: {}
//	  - sources: # use these sources (see the API documentation for sources)
//	    type: "node" # use container source
//	    node:
//	  - "utilization"
//	  - "root_utilization"
//	  - "kubepods_utilization"
//	  - "system_utilization"
//	    levels:
//	  - mask:
//	    name: "node" # sum all containers / pods into workload metrics
//	    builtIn:
//	    exported_node: true
//	    cgroup: true
//	    operation: "hist"
//	    histogramBuckets:
//	    cpu_cores: [0.0000011, 0.0000021, 0.0000031, 0.0000041, 0.0000051]
//	    memory_bytes: [50, 100, 200, 400, 800, 1600]
//	  - mask:
//	    name: "node" # sum all containers / pods into workload metrics
//	    builtIn:
//	    exported_node: true
//	    cgroup: true
//	    operation: "p95"
//	  - group: apps
//	    kind: Deployment
//	    version: v1
package collectorcontrollerv1alpha1
