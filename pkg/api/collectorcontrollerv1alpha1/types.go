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

package collectorcontrollerv1alpha1

import (
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// MetricsPrometheusCollector configures a metrics-prometheus-collector binary for exporting
// Kubernetes metrics to prometheus.
type MetricsPrometheusCollector struct {
	// Kind is the kind of this configuration
	//  "MetricsPrometheusCollector"
	Kind string `json:"kind" yaml:"kind"`
	// APIVersion is the API version of this configuration
	//  v1alpha1
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`

	// Prefix is the prefix applied to all exported metric names
	//  kube_usage
	Prefix string `json:"prefix" yaml:"prefix"`

	// CGroupMetrics configures how values collected from the node sampler are
	// transformed into metrics.
	CGroupMetrics CGroupMetrics `json:"cgroupMetrics,omitempty" yaml:"cgroupMetrics,omitempty"`

	// ClusterScopedMetrics configures how values in the cluster's control plane
	// are transformed into metrics.
	ClusterScopedMetrics ClusterScopedMetrics `json:"clusterScopedMetrics,omitempty" yaml:"clusterScopedMetrics,omitempty"`

	// Extensions defines user specified extensions for creating additional
	// prometheus metric labels derived from object metadata.
	Extensions Extensions `json:"extensions,omitempty" yaml:"extensions,omitempty"`

	// BuiltIn configures options for built in metric labels and metrics.
	BuiltIn BuiltIn `json:"builtIn,omitempty" yaml:"builtIn,omitempty"`

	// CacheOptions configures options for the internal informer cache of the
	// collector.
	CacheOptions CacheOptions `json:"cacheOptions,omitempty" yaml:"cacheOptions,omitempty"`

	// PreComputeMetrics configures options for when metrics are calculated
	// relative to the prometheus scrape.
	PreComputeMetrics PreComputeMetrics `json:"preComputeMetrics,omitempty" yaml:"preComputeMetrics,omitempty"`

	// Resources are the compute resources to provide metric data for from Pod
	// resources. The key is the value as it appears in
	// container.resources.requests. The value is how the resource appears in
	// the exported prometheus metric. e.g. the follow will export metrics for
	// "cpu" and have the metric name have "cpu_cores"
	//  {"cpu": "cpu_cores"}
	Resources map[ResourceName]ResourceAlias `json:"resources" yaml:"resources"`

	// Aggregations define how metrics are aggregated and exported.
	Aggregations Aggregations `json:"aggregations,omitempty" yaml:"aggregations,omitempty"`

	// UtilizationServer configures how metrics are pushed.
	UtilizationServer UtilizationServer `json:"utilizationServer" yaml:"utilizationServer"`

	MinResyncFrequencyMinutes float32 `json:"resyncFrequencyMinutes" yaml:"resyncFrequencyMinutes"`

	SaveSamplesLocally *SaveSamplesLocally `json:"saveSamplesLocally" yaml:"saveSamplesLocally"`

	ExitOnConfigChange bool `json:"exitOnConfigChange" yaml:"exitOnConfigChange"`

	// SideCarConfigDirectoryPaths are paths to directories with sidecar metric files.
	// SideCar metric
	SideCarConfigDirectoryPaths []string `json:"sideCarMetricPaths" yaml:"sideCarMetricPaths"`

	// SchedulerHealth if not empty configures exporting metrics on pod schedule time
	SchedulerHealth *SchedulerHealth `json:"schedulerHealth" yaml:"schedulerHealth"`

	// DefaultAggregationNames if set to true will default the aggregation name to the aggregation "type"
	DefaultAggregationNames bool `json:"defaultAggregationNames" yaml:"defaultAggregationNames"`
	// DefaultLevelNames if set to true will default the level name to the mask name
	DefaultLevelNames bool `json:"defaultLevelNames" yaml:"defaultLevelNames"`

	ResponseCacheOptions ResponseCacheOptions `json:"responseCacheOptions" yaml:"responseCacheOptions"`
}

type ResponseCacheOptions struct {
	RequestHeaders  map[string]string `json:"requestHeaders" yaml:"requestHeaders"`
	ResponseHeaders map[string]string `json:"responseHeaders" yaml:"responseHeaders"`
}

type SchedulerHealth struct {
	// MaxPodAgeMinutes disregards pods that are older than this many seconds.  Helps clean up noise from
	// pods that never schedule due to their specifications.
	MaxPodAgeMinutes uint64 `json:"maxPodAgeMinutes" yaml:"maxPodAgeMinutes"`

	// MinPodAgeMinutes disregards pods that are newer than this many seconds.  Helps clean up noise from
	// pods were just created.
	MinPodAgeMinutes uint64 `json:"minPodAgeMinutes" yaml:"minPodAgeMinutes"`
}

var (
	DefaultSamplesLocalDirectoryPath = os.TempDir() + "metrics-prometheus-collector-samples"
	DefaultSamplesTimeFormat         = "20060102-15:04:05"
)

type SaveSamplesLocally struct {
	DirectoryPath    string `json:"directoryPath" yaml:"directoryPath"`
	TimeFormat       string `json:"timeFormat" yaml:"timeFormat"`
	ExcludeTimestamp bool   `json:"excludeTimestamp" yaml:"excludeTimestamp"`
	SortValues       bool   `json:"sortValues" yaml:"sortValues"`

	// SaveProto if set to true will save local copies of the metrics as pb files
	// SaveProto defaults to true if SaveJSON is empty
	SaveProto *bool `json:"saveProto,omitempty" yaml:"saveProto,omitempty"`
	// SaveJSON if set to true will save local copies of the metrics as json files
	SaveJSON *bool `json:"saveJSON,omitempty" yaml:"saveJSON,omitempty"`

	SampleSources []SampleSources `json:"metrics" yaml:"metrics"`
}

type SampleSources struct {
	// Sources are the sources to save
	Sources Sources `json:"sources,omitempty" yaml:"sources,omitempty"`

	// Mask is the mask to apply when saving a sample locally
	Mask LabelsMask `json:"mask" yaml:"mask"`
}

type PreComputeMetrics struct {
	Enabled   bool          `json:"enabled" yaml:"enabled"`
	Frequency time.Duration `json:"frequency" yaml:"frequency"`
}

type CacheOptions struct {
	DropAnnotations       []string `json:"dropAnnotations" yaml:"dropAnnotations"`
	UnsafeDisableDeepCopy bool     `json:"unsafeDisableDeepCopy" yaml:"unsafeDisableDeepCopy"`
}

type UtilizationServer struct {
	// MetricsCollectorServerBindAddress is specified will create a server to receive
	// utilizaiton metrics pushed from metrics-node-sampler instances.
	ProtoBindPort int `json:"protoBindPort" yaml:"protoBindPort"`
	JSONBindPort  int `json:"jsonProtoBindPort" yaml:"jsonProtoBindPort"`

	// UnhealthyNodeConditions are conditions to check for when we aren't getting samples from a node to identify
	// the cause.  When samples are missing from a node matching any of these conditions, it will be logged as
	// the reason being the node is unhealthy.
	UnhealthyNodeConditions []Condition `json:"unhealthyNodeConditions" yaml:"unhealthyNodeConditions"`

	// TTLDuration defines the minimum age before utilization responses are expired from the cache.
	TTLDuration string `json:"responseTTLDuration" yaml:"responseTTLDuration"`

	// ExpireReponsesFrequencyDuration defines how frequently to check utilization response ages and
	// expire them if they are past their ttl.
	ExpireReponsesFrequencyDuration string `json:"expireReponsesFrequencyDuration" yaml:"expireReponsesFrequencyDuration"`

	// SamplerPodLabels defines the set of labels used to select samplers from
	// the pods in the sampler namespace.
	SamplerPodLabels map[string]string `json:"samplerPodLabels" yaml:"samplerPodLabels"`
	// SamplerNamespaceName is the name of the sampler namespace.
	SamplerNamespaceName string `json:"samplerNamespaceName" yaml:"samplerNamespaceName"`

	CollectorPodLabels map[string]string `json:"collectorPodLabels" yaml:"collectorPodLabels"`

	CollectorPodIPsIndex int `json:"collectorPodIPsIndex" yaml:"collectorPodIPsIndex"`

	SamplerPort int `json:"samplerPort" yaml:"samplerPort"`

	// MinResultPctBeforeReady if set will fail readiness checks until the collector has utilization results
	// from at least this percent of running samplers.
	MinResultPctBeforeReady int `json:"minResultPctBeforeReady" yaml:"minResultPctBeforeReady"`

	// WaitSamplerRegistrationsBeforeReady if set will wait this many additional registration cycles after reaching
	// MinResultPctBeforeReady before becoming ready.
	WaitSamplerRegistrationsBeforeReady int `json:"waitSamplerRegistrationsBeforeReady" yaml:"waitSamplerRegistrationsBeforeReady"`

	// WaitSamplerRegistrationsBeforeServe if set will wait this many additional registration cycles after reaching
	// MinResultPctBeforeReady before serving results.
	WaitSamplerRegistrationsBeforeServe int `json:"waitSamplerRegistrationsBeforeServe" yaml:"waitSamplerRegistrationsBeforeServe"`

	// DeleteUnregisteredPodsAfterAgeMinutes will delete node-sampler pods if the collector cannot register with them
	// and they are this many minutes old
	DeleteUnregisteredPodsAfterAgeMinutes int `json:"deleteUnregisteredPodsAfterAgeMinutes" yaml:"deleteUnregisteredPodsAfterAgeMinutes"`

	// DeleteUnregisteredPodsAfterCycles will delete node-sampler pods if the collector cannot register with them
	// after this many initial cycles.
	DeleteUnregisteredPodsAfterCycles int `json:"deleteUnregisteredPodsAfterCycles" yaml:"deleteUnregisteredPodsAfterCycles"`

	// RestrictMaxUtilizationToContainerLimits will replace any resource utilization sample values greater than the container's limits with
	// the container limit value.  This can be necessary as utilization values are not atomically sampled with the time -- leading to occassional
	// spurious values above the limits.  This is mostly an issue when using the max sample across many pods, where a single sample in 10K
	// being over the limits will cause the data to look incorrect.
	RestrictMaxUtilizationToContainerLimits bool `json:"restrictMaxUtilizationToContainerLimits" yaml:"restrictMaxUtilizationToContainerLimits"`

	RestrictMaxPeriods int `json:"restrictMaxPeriods" yaml:"restrictMaxPeriods"`
}

// Condition matches a node condition to determine whether a node is unhealthy.
type Condition struct {
	Type   string `json:"type" yaml:"type"`
	Status string `json:"status" yaml:"status"`
}

// SamplerEndpoint holds the Kubernetes coordinates of a pod exposing an
// endpoint for a node sampler.
type SamplerEndPoint struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	Name      string `json:"name" yaml:"name"`
}

const (
	// DefaultMetricNamePrefix is a prefix applied to all metrics if no other is specified.
	DefaultMetricNamePrefix = "kube_usage"
)

// ResourceName is the name of a type of resource.
type ResourceName = corev1.ResourceName

const (
	// ResourceCPU is the resource name for CPUs.
	ResourceCPU = corev1.ResourceCPU

	// ResourceMemory is the resource name for memory.
	ResourceMemory = corev1.ResourceMemory

	// ResourceStorage is the resource name for storage.
	ResourceStorage = corev1.ResourceStorage

	// ResourceItems is the resource name for object counts
	ResourceItems = ResourceName("items")

	// ResourcePeriods is the resource name for CPU periods.
	ResourcePeriods = ResourceName("periods")

	// ResourceTime is the resource name for time periods.
	ResourceTime = ResourceName("time")

	// These are the resources associated to network usage, we need to define both rx and tx
	// because they are used as key on maps to differentiate metrics.

	// ResourceNetworkRxBytes is the resource name for network bytes received.
	ResourceNetworkRxBytes = ResourceName("rx_bytes")
	// ResourceNetworkRxPackets is the resource name for network packets received.
	ResourceNetworkRxPackets = ResourceName("rx_packets")
	// ResourceNetworkRxErrors is the resource name for network errors at reception.
	ResourceNetworkRxErrors = ResourceName("rx_errors")
	// ResourceNetworkRxDropped is the resource name for network packets dropped at reception.
	ResourceNetworkRxDropped = ResourceName("rx_dropped")
	// ResourceNetworkTxBytes is the resource name for network bytes transferred.
	ResourceNetworkTxBytes = ResourceName("tx_bytes")
	// ResourceNetworkTxPackets is the resource name for network packets transferred.
	ResourceNetworkTxPackets = ResourceName("tx_packets")
	// ResourceNetworkTxErrors is the resource name for network errors during transmission.
	ResourceNetworkTxErrors = ResourceName("tx_errors")
	// ResourceNetworkTxDropped is the resource name for network packets dropped during transmission.
	ResourceNetworkTxDropped = ResourceName("tx_dropped")
)

// ResourceNames defines a set of the supported ResourceName values.
var ResourceNames = sets.New(ResourceCPU, ResourceMemory, ResourceStorage, ResourceItems, ResourceTime,
	ResourceNetworkRxBytes, ResourceNetworkRxPackets, ResourceNetworkRxErrors, ResourceNetworkRxDropped,
	ResourceNetworkTxBytes, ResourceNetworkTxPackets, ResourceNetworkTxErrors, ResourceNetworkTxDropped)

// ResourceAlias is an alias for a resource, it normally includes units (e.g. cpu_cores, memory_bytes)
type ResourceAlias string

func (a ResourceAlias) String() string { return string(a) }

const (
	// ResourceAliasCPU is the resource alias for CPUs.
	ResourceAliasCPU ResourceAlias = "cpu_cores"

	// ResourceAliasMemory is the resource alias for memory.
	ResourceAliasMemory ResourceAlias = "memory_bytes"

	// ResourceAliasStorage is the resource alias for storage.
	ResourceAliasStorage ResourceAlias = "storage_bytes"

	// ResourceAliasItems is the resource alias for object counts
	ResourceAliasItems ResourceAlias = "items"

	// ResourceAliasTime is the resource alias for time.
	ResourceAliasTime ResourceAlias = "time_seconds"

	// ResourceAliasNetworkRxBytes is the resource alias for network bytes received.
	ResourceAliasNetworkRxBytes = ResourceAlias("rx_bytes")
	// ResourceAliasNetworkRxPackets is the resource alias for network packets received.
	ResourceAliasNetworkRxPackets = ResourceAlias("rx_packets")
	// ResourceAliasNetworkRxErrors is the resource alias for network errors at reception.
	ResourceAliasNetworkRxErrors = ResourceAlias("rx_errors")
	// ResourceAliasNetworkRxDropped is the resource alias for network packets dropped at reception.
	ResourceAliasNetworkRxDropped = ResourceAlias("rx_dropped")
	// ResourceAliasNetworkTxBytes is the resource alias for network bytes transferred.
	ResourceAliasNetworkTxBytes = ResourceAlias("tx_bytes")
	// ResourceAliasNetworkTxPackets is the resource alias for network packets transferred.
	ResourceAliasNetworkTxPackets = ResourceAlias("tx_packets")
	// ResourceAliasNetworkTxErrors is the resource alias for network errors during transmission.
	ResourceAliasNetworkTxErrors = ResourceAlias("tx_errors")
	// ResourceAliasNetworkTxDropped is the resource alias for network packets dropped during transmission.
	ResourceAliasNetworkTxDropped = ResourceAlias("tx_dropped")
)

// Built in prometheus metric labels
const (
	// ExportedContainerLabel is the label name of the name for a container in a pod
	// Defined by the pod spec
	ExportedContainerLabel = "exported_container"

	// ContainerImageLabel is the label for the container image
	ContainerImageLabel = "container_image"

	// ExportedNamespaceLabel is the label name of the namespace for a pod
	// Defined by the pod namespace
	ExportedNamespaceLabel = "exported_namespace"

	// ExportedPodLabel is the label name of the name for a pod
	// Defined by the pod name
	ExportedPodLabel = "exported_pod"

	// ExportedNodeLabel is the label name of the name of a node for a pod
	// Defined by the pod spec
	ExportedNodeLabel = "exported_node"

	// NodeUnschedulableLabel is the label name corresponding to node.spec.unschedulable
	NodeUnschedulableLabel = "node_unschedulable"

	// WorkloadNameLabel is the label name of the workload a pod is owned by
	// Defined by pod owners references
	WorkloadNameLabel = "workload_name"

	// WorkloadKindLabel is the label kind of the workload a pod is owned by
	// Defined by pod owners references
	WorkloadKindLabel = "workload_kind"

	// WorkloadAPIGroupLabel is the label api group of the workload a pod is owned by
	// Defined by pod owners references
	WorkloadAPIGroupLabel = "workload_api_group"

	// WorkloadAPIVersionLabel is the label api version of the workload a pod is owned by
	// Defined by pod owners references
	WorkloadAPIVersionLabel = "workload_api_version"

	// AppLabel is the label name of the application a pod is part of
	// Defined by the pod `app` label
	AppLabel = "app"

	// QuotaLabel is the label name of the quota for a namespace
	QuotaLabel = "quota_name"

	// AllocationStrategyLabel is the label name of allocation strategy
	AllocationStrategyLabel = "allocation_strategy"

	// PriorityClassLabel is the label name of the name of the priority class for a pod
	// Defined by the pod spec
	PriorityClassLabel = "priority_class"

	// ScheduledLabel is the label that indicates of the capacity has been
	// scheduled to a Node.
	ScheduledLabel = "scheduled"

	// LevelLabel is the aggregation level name.  This is useful when querying to
	// find all metrics at a level.
	LevelLabel = "level"

	// PVCNameLabel is the label of the PersistentVolumeClaim name
	PVCNameLabel = "exported_pvc"

	// PVCNameLabel is the label of the PersistentVolumename
	PVNameLabel = "exported_pv"

	// StorageClassLabel is the label for the PV or PVC storage class
	StorageClassLabel = "storage_class"

	// CGroupLabel is the label for the cgroup
	CGroupLabel = "cgroup"

	// PhaseLabel is the label for the PV or PVC phase
	PhaseLabel = "phase"
)

// AggregationLevel represents the different levels at which metrics can be aggregated.
type AggregationLevel string

// String makes AggregationLevel a Stringer.
func (a AggregationLevel) String() string { return string(a) }

// Built in aggregation level names
const (
	// ContainerLevel is the lowest aggregation level for metrics pulled from containers
	// e.g. container requests, limits and utilization.
	ContainerLevel AggregationLevel = "container"

	// PodLevel is the lowest aggregation level for metrics pulled from pods.
	// e.g. count
	PodLevel AggregationLevel = "pod"

	// Namespacelevel is the lowest aggregation level for metrics pulled from quota.
	NamespaceLevel AggregationLevel = "namespace"

	// NodeLevel is the lowest aggregation level for metrics pulled from nodes.
	NodeLevel AggregationLevel = "node"

	// PVC level is the lowest aggregation level for metrics for PVCs
	PVCLevel AggregationLevel = "pvc"

	// PVC level is the lowest aggregation level for metrics for PVs
	PVLevel AggregationLevel = "pv"
)

// SourceType represents the different Kubernetes resources that metrics are sourced from.
type SourceType string

// String makes SourceType a Stringer.
func (s SourceType) String() string { return string(s) }

// Built in metric source types.  These define the types of Kubernetes resources
// that metrics are sourced from.
const (
	// QuotaType is the type for metrics pulled from resource quota objets
	QuotaType SourceType = "quota"

	// ContainerType is the type for metrics pulled from containers running in pods
	ContainerType SourceType = "container"

	// PodType is the type for metrics pulled from pods themselves
	PodType SourceType = "pod"

	// NodeType is the type for metrics pulled from nodes
	NodeType SourceType = "node"

	// PVCType is the type for metrics pulled from PersistentVolumeClaims
	PVCType SourceType = "pvc"

	// PVType is the type for metrics pulled from PersistentVolumes
	PVType SourceType = "pv"

	// NamespaceType is the type for metrics pulled from Namespace
	NamespaceType SourceType = "namespace"

	// CgroupType is the type for metrics pulled from a cgroup
	CGroupType SourceType = "cgroup"

	// ClusterScopedType is the type for metrics pulled from the cluster scope.
	ClusterScopedType SourceType = "cluster_scoped"

	// SchedulerHealthType is the type for metrics pulled for scheduler health.
	SchedulerHealthType SourceType = "scheduler_health"
)

// SourceTypes is a set of all of the valid SourceType values.
var SourceTypes = sets.New(
	QuotaType,
	PodType,
	CGroupType,
	NodeType,
	ContainerType,
	PVType,
	PVCType,
	NamespaceType,
	ClusterScopedType,
	SchedulerHealthType,
)

// Source is one of the sources of metrics for different types.
type Source string

// String makes Source a Stringer.
func (s Source) String() string { return string(s) }

// IsExtension returns true if the source name is a valid extension name, i.e. starts with ext_.
func (s Source) IsExtension() bool { return strings.HasPrefix(string(s), "ext_") }

// Built in metric sources.  These define the sources of metrics for different types.
const (
	// ContainerRequestsAllocatedSource is the source for container requests.  (type: container)
	ContainerRequestsAllocatedSource Source = "requests_allocated"

	// ContainerLimitsAllocatedSource is the source for container limits.  (type: container)
	ContainerLimitsAllocatedSource Source = "limits_allocated"

	// ContainerUtilizationSource is the source for container utilization.  (type: container)
	// Note: this source contains multiple samples over-time for each container
	// And must have a non-sum operation applied before being summed.
	ContainerUtilizationSource Source = "utilization"

	// AvgContainerUtilizationSource corresponds to the pre-computed mean for the `utilization` source.
	// The mean is computed by the node-samplers over the samples for a single container.
	AvgContainerUtilizationSource Source = "avg_utilization"

	// ContainerRequestsAllocatedMinusUtilizationSource is the source for
	// container requests allocated minus utilization. (type: container) Note:
	// this source contains multiple samples over-time for each container and
	// must have a non-sum operation applied before being summed.
	ContainerRequestsAllocatedMinusUtilizationSource Source = "requests_allocated_minus_utilization"

	// NRPeriodsSource is the source for nr_periods per sec
	// nr_periods is number of periods that any thread in the cgroup was runnable
	NRPeriodsSource Source = "nr_periods"

	// AvgNRPeriodsSource corresponds to the pre-computed mean for the `nr_periods` source.
	// The mean is computed by the node-samplers over the samples for a single container.
	AvgNRPeriodsSource Source = "avg_nr_periods"

	// NRThrottledSource is the source for nr_throttled per sec
	// throttled_time is the total time individual threads within the cgroup were throttled
	NRThrottledSource Source = "nr_throttled"

	// AvgNRThrottledSource corresponds to the pre-computed mean for the `nr_throttled` source.
	// The mean is computed by the node-samplers over the samples for a single container.
	AvgNRThrottledSource Source = "avg_nr_throttled"

	// OOMKillCountSource is the source for OOM Kill counter.  It is the total
	// oom kill events during the sampling window on the node-sampler.
	// This corresponds to the value read from the `memory.oom_control`` file.
	// This corresponds to the containerd v1.Metrics field `MemoryOomControl.OomKill`.
	OOMKillCountSource Source = "oom_kill"

	// OOMCountSource is the source for OOM counter.
	// It is the total oom events during the sampling window on the node-sampler.
	// This corresponds to the value read from the `memory.failcnt`` file.
	// This corresponds to the containerd v1.Metrics field `MemoryOomControl.UnderOom`.
	OOMCountSource Source = "oom"
)

const (
	// ContainerItemsSource the source for container object count
	ContainerItemsSource Source = "container"
)

// ContainerSources contains all of the Source values of SourceType ContainerType.
var ContainerSources = sets.New(
	ContainerItemsSource,
	ContainerRequestsAllocatedSource,
	ContainerLimitsAllocatedSource,
	AvgContainerUtilizationSource,
	ContainerUtilizationSource,
	ContainerRequestsAllocatedMinusUtilizationSource,
	AvgNRPeriodsSource,
	NRPeriodsSource,
	AvgNRThrottledSource,
	NRThrottledSource,
	OOMCountSource,
	OOMKillCountSource,
)

const (
	// PodItemsSource the source for pod object count
	PodItemsSource Source = "pod"
)

// PodSources contains all of the Source values of SourceType PodType.
var PodSources = sets.New(PodItemsSource)

const (
	// QuotaItemsSourceis the source for resource quota object count
	QuotaItemsSource Source = "quota"

	// QuotaRequestsHardSource is the source for resource quota requests (hard).  (type: quota)
	QuotaRequestsHardSource Source = "requests_quota_hard"

	// QuotaLimitsHardSource is the source for resource quota limits (hard).  (type: quota)
	QuotaLimitsHardSource Source = "limits_quota_hard"

	// QuotaRequestsHardSource is the source for resource quota requests (used).  (type: quota)
	QuotaRequestsUsedSource Source = "requests_quota_used"

	// QuotaLimitsHardSource is the source for resource quota limits (used).  (type: quota)
	QuotaLimitsUsedSource Source = "limits_quota_used"

	// QuotaRequestsHardMinusUsed is the source for resource quota requests - used (ie, unused requests). (type: quota)
	QuotaRequestsHardMinusUsed Source = "requests_quota_hard_minus_used"

	// QuotaLimitsHardMinusUsed is the source for resource quota requests - used (ie, unused limits). (type: quota)
	QuotaLimitsHardMinusUsed Source = "limits_quota_hard_minus_used"

	// PVCQuotaLimitsHardSource is the source for resource quota limits (hard).  (type: quota)
	PVCQuotaRequestsHardSource Source = "pvc_requests_quota_hard"

	// PVCQuotaRequestsUsedSource is the source for resource quota requests (used).  (type: quota)
	PVCQuotaRequestsUsedSource Source = "pvc_requests_quota_used"

	// QuotaDescriptorRequetsProposedSource is the source for proposed requests quota
	QuotaDescriptorRequestsProposedSource Source = "requests_quota_proposed"

	// QuotaDescriptorLimitsProposedSource is the source for proposed limits quota
	QuotaDescriptorLimitsProposedSource Source = "limits_quota_proposed"

	// QuotaDescriptorRequestsHardMinusProposedSource is the source for requests hard quota minus proposed (quota to be clawed back).
	QuotaDescriptorRequestsHardMinusProposedSource Source = "requests_quota_hard_minus_proposed"

	// QuotaDescriptorLimitsHardMinusProposedSource is the source for limits hard quota minus proposed (quota to be clawed back).
	QuotaDescriptorLimitsHardMinusProposedSource Source = "limits_quota_hard_minus_proposed"

	// QuotaDescriptorRequestsMaxObservedMinusHardSource is the source for max observed quota minus hard (net clawback applied).
	QuotaDescriptorRequestsMaxObservedMinusHardSource Source = "requests_quota_max_observed_minus_hard"

	// QuotaDescriptorLimitsMaxObservedMinusHardSource is the source for max observed quota minus hard (net clawback applied).
	QuotaDescriptorLimitsMaxObservedMinusHardSource Source = "limits_quota_max_observed_minus_hard"
)

// QuotaSources contains all of the Source values of SourceType QuotaType.
var QuotaSources = sets.New(
	QuotaItemsSource,
	QuotaLimitsHardMinusUsed,
	QuotaLimitsHardSource,
	QuotaLimitsUsedSource,
	QuotaRequestsHardMinusUsed,
	QuotaRequestsHardSource,
	QuotaRequestsUsedSource,
	PVCQuotaRequestsHardSource,
	PVCQuotaRequestsUsedSource,
	QuotaDescriptorLimitsHardMinusProposedSource,
	QuotaDescriptorLimitsMaxObservedMinusHardSource,
	QuotaDescriptorLimitsProposedSource,
	QuotaDescriptorRequestsHardMinusProposedSource,
	QuotaDescriptorRequestsMaxObservedMinusHardSource,
	QuotaDescriptorRequestsProposedSource,
)

// RQDSources contains all of the Source values related to resource quota descriptors.
var RQDSources = sets.New(
	QuotaDescriptorLimitsHardMinusProposedSource,
	QuotaDescriptorLimitsMaxObservedMinusHardSource,
	QuotaDescriptorLimitsProposedSource,
	QuotaDescriptorRequestsHardMinusProposedSource,
	QuotaDescriptorRequestsMaxObservedMinusHardSource,
	QuotaDescriptorRequestsProposedSource,
)

const (
	// NodeItemsSource the source for node object count
	NodeItemsSource Source = "node"

	// NodeAllocatableSource is the source for node allocatable.
	NodeAllocatableSource Source = "node_allocatable"

	// NodeCapacitySource is the source for node capacity.
	NodeCapacitySource Source = "node_capacity"

	// NodeRequestsSource is the source for node requests.
	NodeRequestsSource Source = "node_requests"

	// NodeLimitsSource is the source for node limits.
	NodeLimitsSource Source = "node_limits"

	// NodeAllocatableMinusRequests is a source that exposes metrics valued as (allocatable - requests).
	NodeAllocatableMinusRequests Source = "node_allocatable_minus_requests"
)

// NodeSources contains all of the Source values of SourceType NodeType.
var NodeSources = sets.New(
	NodeItemsSource,
	NodeAllocatableSource,
	NodeCapacitySource,
	NodeRequestsSource,
	NodeLimitsSource,
	NodeAllocatableMinusRequests,
)

const (
	// PVCRequestsSource is the source for PersistentVolumeClaim requests
	PVCRequestsSource Source = "pvc_requests_allocated"

	// PVCLimitsSource is the source for PersistentVolumeClaim limits
	PVCLimitsSource Source = "pvc_limits_allocated"

	// PVCCapacitySource is the source for PersistentVolumeClaim capacity
	PVCCapacitySource Source = "pvc_capacity"

	// PVCItemSource is the source for PersistentVolumeClaim object count
	PVCItemsSource Source = "pvc"
)

// PVCSources contains all of the Source values of SourceType PVCType.
var PVCSources = sets.New(
	PVCCapacitySource,
	PVCItemsSource,
	PVCLimitsSource,
	PVCRequestsSource,
)

const (
	// PVCapacitySource is the sources for PersistentVolume capacity
	PVCapacitySource Source = "pv_capacity"

	// PVItemsSource is the source for PersistentVolume object count
	PVItemsSource Source = "pv"
)

// PVSources contains all of the Source values of SourceType PVType.
var PVSources = sets.New(PVItemsSource, PVCapacitySource)

const (
	// NamespaceItemsSource is the source for the namespace count
	NamespaceItemsSource Source = "namespace"
)

// NamespaceSources all of the Source values of SourceType NamespaceType.
var NamespaceSources = sets.New(NamespaceItemsSource)

const (
	// SchedulerPodScheduleWait publishes all of the pods waiting to be scheduled or scheduled
	// that are less than MetricsPrometheusCollector.SchedulerHealth.MaxPodAgeMinutes old.
	// If a pod is waiting to be scheduled, metrics are only reported if it is
	// at least MetricsPrometheusCollector.SchedulerHealth.MinPodAgeMinutes old.
	// The value of this metric is the time (seconds) it took from pod creation to scheduling
	// for scheduled pods or the pod age for pending ones.
	SchedulerPodScheduleWait Source = "pod_schedule_wait"
)

// SchedulerHealthSources all of the Source values of SourceType SchedulerHealthType.
var SchedulerHealthSources = sets.New(SchedulerPodScheduleWait)

const (
	// LimitsResourcePrefix is the prefix for limits resource name
	LimitsResourcePrefix = "limits"
	// RequestsResourceName is the prefix for requests resource name
	RequestsResourcePrefix = "requests"
)

// AggregationOperation is used to combine metrics at a level.  Each aggregation
// level has a mask. The mask defines which labels form the keyspace for a
// level. When applied, the mask maps multiple metrics to the same key. Those
// metrics are combined into a single result through an operation.
//
// E.g. container level metrics have the "exported_container", "exported_pod",
// and "workload_name" labels. If a level defines a mask which removes the
// "exported_container" and "exported_pod" labels, the keyspace for this level
// will be formed from a single label - "workload_name". Applying a "sum"
// operation would add together all of the metrics for the containers and pods
// within that workload.
type AggregationOperation string

func (op AggregationOperation) String() string { return string(op) }

const (
	// SumOperation defines the sum operation for aggregating metrics to a level
	SumOperation AggregationOperation = "sum"

	// MaxOperation defines the max operation for aggregating metrics to a level
	MaxOperation AggregationOperation = "max"

	// P95Operation defines the p95 operation for aggregating metrics to a level
	P95Operation AggregationOperation = "p95"

	// AvgOperation defines the average operation for aggregating metrics to a level
	AvgOperation AggregationOperation = "avg"

	// MedianOperation defines the median operation for aggregating metrics to a level
	MedianOperation AggregationOperation = "median"

	// HistogramOperation defines the histogram operation for aggregating metrics to a level
	HistogramOperation AggregationOperation = "hist"
)

// AggregationOperation is a set of all of the known AggregationOperation values.
var AggregationOperations = sets.New(SumOperation, MaxOperation, P95Operation, AvgOperation, MedianOperation, HistogramOperation)

// Sources defines sources for metric data.
type Sources struct {
	// Type is the resource type the metrics are gotten from
	// See the *Type constants.
	//
	//  "type": "container"
	Type SourceType `json:"type" yaml:"type"`

	// Quota are sources from resource quota objects.
	Quota []Source `json:"quota,omitempty" yaml:"quota,omitempty"`

	// Node are sources from node objects.
	Node []Source `json:"node,omitempty" yaml:"node,omitempty"`

	// CGroup sources are from the cgroups on node objects.
	CGroup []Source `json:"cgroup,omitempty" yaml:"cgroup,omitempty"`

	// Container are sources from container objects. Each container source
	// exposes metrics "cpu_cores" and "memory_bytes".
	Container []Source `json:"container,omitempty" yaml:"container,omitempty"`

	// Pod are sources from pod objects.
	Pod []Source `json:"pod,omitempty" yaml:"pod,omitempty"`

	// PV are sources from persistent volume objects.
	PV []Source `json:"pv,omitempty" yaml:"pv,omitempty"`

	// PVC are sources from persistent volume claim objects.
	PVC []Source `json:"pvc,omitempty" yaml:"pvc,omitempty"`

	// Namespace are sources for namespace objects.
	Namespace []Source `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	// ClusterScoped are sources for cluster-scoped resources in the Kubernetes
	// control plane.
	ClusterScoped []Source `json:"cluster_scoped" yaml:"cluster_scoped"`

	// SchedulerHealth are sources for scheduler health objects.
	SchedulerHealth []Source `json:"scheduler_health,omitempty" yaml:"scheduler_health,omitempty"`

	// Alias maps sources to different names when publishing metrics
	Alias map[Source]string `json:"alias,omitempty" yaml:"alias,omitempty"`
}

func (s *Sources) GetSources() []Source {
	switch s.Type {
	case ContainerType:
		return s.Container
	case QuotaType:
		return s.Quota
	case NodeType:
		return s.Node
	case PodType:
		return s.Pod
	case PVType:
		return s.PV
	case PVCType:
		return s.PVC
	case NamespaceType:
		return s.Namespace
	case CGroupType:
		return s.CGroup
	case ClusterScopedType:
		return s.ClusterScoped
	case SchedulerHealthType:
		return s.SchedulerHealth
	}
	return nil
}

// CGroupMetrics configures how values collected from node samples are
// transformed into metrics.
type CGroupMetrics struct {
	// Sources defines metrics sources from cgroups.  The key is the parent
	// directory, and the value contains the name that is used for the source
	// part of each metric name.
	Sources map[string]CGroupMetric `json:"sources,omitempty" yaml:"sources,omitempty"`

	// RootSource defines the metrics source for the root cgroup. This is a
	// special case because there is no applicable parent directory for this
	// cgroup.
	RootSource CGroupMetric `json:"rootSource,omitempty" yaml:"rootSource,omitempty"`
}

type CGroupMetric struct {
	// Name is the source name that is used for CGroup metrics under a path
	// It uses the individual sample values provided by the node-sampler.
	Name Source `json:"name,omitempty" yaml:"name,omitempty"`

	// AvgName is the source name that is used for CGgroup metrics a path.
	// It uses the aggregate pre-computed average (mean) value provided by
	// the node-sampler.
	AvgName Source `json:"avgName,omitempty" yaml:"avgName,omitempty"`
}

// ClusterScopedMetrics configures how values collected from the cluster control
// plane are transformed into metrics.
type ClusterScopedMetrics struct {
	// AnnotatedCollectionSources define collections of API resources that hold
	// information about the cluster in annotations.
	AnnotatedCollectionSources []AnnotatedPriorityClassCollectionSource `json:"annotatedCollectionSources" yaml:"annotatedCollectionSources"`
}

// AnnotatedCollectionSource configures how information stored in annotations on
// an API resource collection should be transformed into metrics. The resource
// instances in the collection are expected to be annotated as follows:
//
// <annotation-prefix>/<priority-class-name>: <serialized resourceList>
//
// ASSUMPTIONS:
// - resource is name-aligned with priorityClass
//
// metrics driven:
// - vendable capacity by priority class (priority_class label)
// - cluster raw capacity?
type AnnotatedPriorityClassCollectionSource struct {
	// Name is the name for the source part of the metrics emitted for this
	// collection.
	Name Source `json:"name" yaml:"name"`

	// Group is the API group.
	Group string `json:"group" yaml:"group"`

	// Version is the API version of the resource to discover information
	// from.
	Version string `json:"version" yaml:"version"`

	// Kind is the name of the API resource to discover information from.
	Kind string `json:"kind" yaml:"kind"`

	// Annotation is annotation key to discover vendable capacity from.
	Annotation string `json:"annotation" yaml:"annotation"`
}

// Aggregations is a list of aggregations to perform
type Aggregations []Aggregation

func (a Aggregations) ByType(t SourceType) []*Aggregation {
	var result []*Aggregation
	for i := range a {
		if a[i].Sources.Type == t {
			result = append(result, &a[i])
		}
	}
	return result
}

// Aggregation defines a set of sources of metric data and how operations are
// applied to those sources to produce new metrics.
type Aggregation struct {
	// Name is used for only documentation purposes and whenever it is needed
	// to refer to a specific aggregation.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// Sources configure which metrics are available to the Levels associated
	// with the Aggregation.
	//
	// See the *Source constants for valid values.
	//  ["requests_allocated", "limits_allocated"]
	Sources Sources `json:"sources,omitempty" yaml:"sources,omitempty"`

	// Levels are the ordered list of exported aggregation levels
	// Each level is computed by aggregating the preceding level using
	// the defined operation.  Aggregation is performed by masking
	// label values and applying the operation to values that share the
	// same new key.
	Levels Levels `json:"levels" yaml:"levels"`
}

// Levels is a sequential set of aggregation levels applied to a source of data.
// Each level is exported as a metric.
type Levels []Level

// Level defines a single aggregation operation and label configuration for an
// exported metric.
//
// e.g. applying a mask to drop the exported_container, exported_pod and
// exported_node labels with a sum operation would sum the metric for all
// containers across all pods within the same workload (represented by the
// workload_name label).
type Level struct {
	// Name is an optional name that can be used for documentation purposes and
	// also to facilitate referring to a specific level in an aggregation.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// Mask is applied to retain only the labels that should appear at this level.
	Mask LabelsMask `json:"mask" yaml:"mask"`
	// Operation is applied to aggregate all metrics that have the same set of
	// labels after the mask is applied. If unspecified, this field defaults to
	// "sum".
	Operation AggregationOperation `json:"operation" yaml:"operation"`
	// Operations are a list of Operations applied to the same set of labels
	Operations []AggregationOperation `json:"operations" yaml:"operations"`
	// NoExport indicates that a level should not be exported as a metric.
	NoExport bool `json:"noExport,omitempty" yaml:"noExport,omitempty"`
	// HistogramBuckets describes the histograms and their associated buckets
	// for levels with a histogram operation. The keys are the names of the
	// histograms, and values are the buckets for that histogram.
	HistogramBuckets map[string][]float64 `json:"histogramBuckets,omitempty" yaml:"histogramBuckets,omitempty"`

	// RetentionName if specified indicates that the result should be included in aggregated metrics
	// written locally to a file for retention.
	// The will be set in the "name" field of the SampleList.
	RetentionName string `json:"retentionName,omitempty" yaml:"retentionName,omitempty"`

	// RetentionExponentialBuckets if RetentionName is specified and the current Operation is a histogram operation
	// then this defines an exponential histogram for each resource's raw samples .
	RetentionExponentialBuckets map[string]*ExponentialBuckets `json:"retentionExponentialBuckets,omitempty" yaml:"retentionExponentialBuckets,omitempty"`
}

// ExponentialBuckets histogram buckets for saving raw samples
type ExponentialBuckets struct {
	// MinimumBase is the smallest exponent base that can be used for any histogram
	MinimumBase float64 `json:"minimumBase" yaml:"minimumBase"`

	// Compression power-of-two level of compression at which buckets from the MinBase are merged together to form larger buckets
	// Compression = 0, base=2. This is the basic case.
	// Compression < 0, each base2 bucket is subdivided into 2^Compression log scale sub-buckets.
	// Compression > 0, every 2^Compression base2 buckets are merged.
	Compression int64 `json:"compression" yaml:"compression"`

	// ExponentOffset is offset to start bounds for exponential histogram
	// base = MinimumBase ^ (2 ^ Compression)
	// start bound = base ^ (index + ExponentOffset)
	// end bound = base ^ (index + 1 + ExponentOffset)
	ExponentOffset int64 `json:"exponentOffset" yaml:"exponentOffset"`

	// SaveMaxOnly indicates that only max value should be saved for resource instead of full histogram
	SaveMaxOnly bool `json:"saveMaxOnly,omitempty" yaml:"saveMaxOnly,omitempty"`
}

// LabelsMask defines which labels to keep at a level.
type LabelsMask struct {
	// Level is the name of the level.  Must be unique within the levels for the
	// enclosing Aggregation.
	Level AggregationLevel `json:"name" yaml:"name"`

	// BuiltIn defines the mask for built in labels -- e.g. exported_namespace
	BuiltIn BuiltInLabelsMask `json:"builtIn" yaml:"builtIn"`

	// Extensions defines the mask for user defined labels pulled from object metadata.
	Extensions ExtensionsLabelMask `json:"extensions" yaml:"extensions"`

	// Filters excludes metrics from aggregation based on some conditions.
	// If multiple filters that contradict each other then all metrics
	// will be excluded -- e.g. only include metrics with label foo, and
	// exclude all metrics with label foo.
	Filters []Filter `json:"filters" yaml:"filters"`

	// ID is internal
	ID LabelsMaskId `json:"-" yaml:"-"`
}

// Filter removes metrics based on their label values.
type Filter struct {
	// Present if set to true will only include metrics where the metric label is present (not empty).  If set
	// to false it will only include metrics where the metric is not present (empty).
	Present *bool `json:"present,omitempty" yaml:"present,omitempty"`
	// LabelNames are the names of the labels the filter applies to
	// LabelNames are the names of the labels the filter applies to
	LabelNames []string `json:"labelNames,omitempty" yaml:"labelNames,omitempty"`

	// Labels are the labels to match the values of
	Labels map[string][]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// ClusterName if set will filter matching the environment variable CLUSTER_NAME
	ClusterNames []string `json:"clusterNames,omitempty" yaml:"clusterNames,omitempty"`
}

// BuiltIn configures built in metrics and labels.
type BuiltIn struct {
	UseQuotaNameForPriorityClass bool `json:"useQuotaNameForPriorityClass,omitempty" yaml:"useQuotaNameForPriorityClass,omitempty"`

	// EnableResourceQuotaDescriptor enables features that require the
	// ResourceQuotaDescriptor (RQD) resource. If this flag is set, the
	// collector will:
	// - attempt to list and watch the RQD resource
	// - expose metrics that rely on RQD:
	//   - *_hard_minus_proposed_*
	//   - *_max_observed_*
	// - expose the "issue" label on quota-type metrics
	//
	// If this flag is not set, the collector will not attempt to use the RQD
	// resource and all associated features will be disabled.
	EnableResourceQuotaDescriptor bool `json:"enableResourceQuotaDescriptor,omitempty" yaml:"enableResourceQuotaDescriptor,omitempty"`
}

// BuiltInLabelsMask masks labels that are built in so that metrics may be
// aggregated. Fields set to 'true' indicate that the associated label should be
// retained.
type BuiltInLabelsMask struct {
	// ContainerName is the name of a container within a pod -- e.g. log-saver
	// It masks the MetricSetter.ContainerName field.
	ContainerName bool `json:"exported_container,omitempty" yaml:"exported_container,omitempty"`
	// PodName is the name of a pod.
	// It masks the MetricSetter.PodName field.
	PodName bool `json:"exported_pod,omitempty" yaml:"exported_pod,omitempty"`
	// NamespaceName is the name of a namespace.
	// It masks the MetricSetter.Namespace field.
	NamespaceName bool `json:"exported_namespace,omitempty" yaml:"exported_namespace,omitempty"`
	// NodeName is the name of a node.
	// It masks the MetricSetter.NodeName field.
	NodeName bool `json:"exported_node,omitempty" yaml:"exported_node,omitempty"`

	// NodeUnschedulable corresponds to node.spec.unschedulable
	NodeUnschedulable bool `json:"node_unschedulable,omitempty" yaml:"node_unschedulable,omitempty"`

	// WorkloadName is the name of a workload as defined by the
	// pod OwnerReferences.
	// It masks the MetricSetter.WorkloadName field.
	WorkloadName bool `json:"workload_name,omitempty" yaml:"workload_name,omitempty"`
	// WorkloadName is the kind of a workload as defined by the
	// pod OwnerReferences.
	// It masks the MetricSetter.WorkloadKind field.
	WorkloadKind bool `json:"workload_kind,omitempty" yaml:"workload_kind,omitempty"`
	// WorkloadAPIGroup is the APIGroup of a workload as defined by the
	// pod OwnerReferences.
	// It masks the MetricSetter.WorkloadKind field.
	WorkloadAPIGroup bool `json:"workload_api_group,omitempty" yaml:"workload_api_group,omitempty"`
	// WorkloadAPIVersion is the APIVersion of a workload as defined by the
	// pod OwnerReferences.
	// It masks the MetricSetter.WorkloadKind field.
	WorkloadAPIVersion bool `json:"workload_api_version,omitempty" yaml:"workload_api_version,omitempty"`

	// App is the name of a logical app a pod or workload belongs to as
	// defined by the app pod label.  An app may be composed of multiple
	// workloads -- e.g. 2 deployments compose a logical app.
	// It masks the MetricSetter.App field.
	App bool `json:"app,omitempty" yaml:"app,omitempty"`

	// QuotaName is the name of the quota object
	QuotaName bool `json:"quota_name,omitempty" yaml:"quota_name,omitempty"`

	// AllocationStrategy is the name of the allocation strategy
	AllocationStrategy bool `json:"allocation_strategy,omitempty" yaml:"allocation_strategy,omitempty"`

	// PriorityClass is the name of a priorityClass.
	// It masks the MetricSetter.PriorityClass field.
	PriorityClass bool `json:"priority_class,omitempty" yaml:"priority_class,omitempty"`

	// Scheduled is true if the capacity has been scheduled to a Node
	Scheduled bool `json:"scheduled,omitempty" yaml:"scheduled,omitempty"`

	// Level is the aggregation level name set as a label.  This is useful when querying to
	// find all metrics at a level.
	Level bool `json:"level,omitempty" yaml:"level,omitempty"`

	PVCName bool `json:"exported_pvc,omitempty" yaml:"exported_pvc,omitempty"`

	PVName bool `json:"exported_pv,omitempty" yaml:"exported_pv,omitempty"`

	StorageClass bool `json:"storage_class,omitempty" yaml:"storage_class,omitempty"`

	CGroup bool `json:"cgroup,omitempty" yaml:"cgroup,omitempty"`

	Phase bool `json:"phase,omitempty" yaml:"phase,omitempty"`

	PVType bool `json:"pv_type,omitempty" yaml:"pv_type,omitempty"`

	ContainerImage bool `json:"container_image,omitempty" yaml:"container_image,omitempty"`
}

// ExtensionsLabelMask is a mask for user defined metric labels.
type ExtensionsLabelMask map[LabelName]bool

// Extensions specifies user defined metric labels derived from Kubernetes object data.
type Extensions struct {
	// podLabels are labels applied to metrics by reading pod metadata
	Pods []ExtensionLabel `json:"podLabels,omitempty" yaml:"podLabels,omitempty"`

	// namespaceLabels are labels applied to metrics by reading namespace metadata
	Namespaces []ExtensionLabel `json:"namespaceLabels,omitempty" yaml:"namespaceLabels,omitempty"`

	// nodeLabels are labels applied to metrics by reading node metadata
	Nodes []ExtensionLabel `json:"nodeLabels,omitempty" yaml:"nodeLabels,omitempty"`

	// quotaLabels are labels applied to metrics by reading quota metadata
	Quota []ExtensionLabel `json:"quotaLabels,omitempty" yaml:"quotaLabels,omitempty"`

	// pvcLabels are the labels applied to PVC metrics
	PVCs []ExtensionLabel `json:"pvcLabels,omitempty" yaml:"pvcLabels,omitempty"`

	// pvLabels are the labels applied to PV metrics
	PVs []ExtensionLabel `json:"pvLabels,omitempty" yaml:"pvLabels,omitempty"`

	// nodeTaints are labels applied to metrics by reading node taints
	NodeTaints []NodeTaint `json:"nodeTaints,omitempty" yaml:"nodeTaints,omitempty"`
}

// ExtensionLabel configures a user defined label.
type ExtensionLabel struct {
	// LabelName is the name of the prometheus metric label
	LabelName LabelName `json:"name" yaml:"name"`

	// AnnotationKey is the name of Kubernetes object metadata annotation
	AnnotationKey AnnotationKey `json:"annotation,omitempty" yaml:"annotation,omitempty"`

	// AnnotationKeys are the names of Kubernetes object metadata annotations
	AnnotationKeys []AnnotationKey `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// LabelKey is the name of Kubernetes object metadata label
	LabelKey LabelKey `json:"label,omitempty" yaml:"label,omitempty"`

	// LabelKeys are names of Kubernetes object metadata labels
	LabelKeys []LabelKey `json:"labels,omitempty" yaml:"labels,omitempty"`

	// Value is the default value to use if the annotation or label is not present on
	// the object.
	Value string `json:"value,omitempty" yaml:"value,omitempty"`

	// ID is internal
	ID LabelId `json:"-" yaml:"-"`
}

// NodeTaint defines how extension labels are derived from node taints.
type NodeTaint struct {
	// LabelName is the name of the prometheus metric label
	LabelName LabelName `json:"name,omitempty" yaml:"name,omitempty"`

	// LabelValue is the value used if the requirements DO match a node.
	// If unspecified, the taint value is used.
	LabelValue string `json:"value,omitempty" yaml:"value,omitempty"`

	// LabelNegativeValue is the value used if the requirements DO NOT match a node
	LabelNegativeValue string `json:"negativeValue,omitempty" yaml:"negativeValue,omitempty"`

	// TaintKeys define requirements for matching node taint keys
	TaintKeys NodeTaintRequirements `json:"keys,omitempty" yaml:"keys,omitempty"`

	// TaintValues define requirements for matching node taint values
	TaintValues NodeTaintRequirements `json:"values,omitempty" yaml:"values,omitempty"`

	// TaintEffects define requirements for matching node taint effects
	TaintEffects NodeTaintRequirements `json:"effects,omitempty" yaml:"effects,omitempty"`

	// ID is internal
	ID LabelId `json:"-" yaml:"-"`
}

// NodeTaintRequirements defines a requirement for matching node taints.
type NodeTaintRequirements []NodeTaintRequirement

// NodeTaintRequirement defines requirements for matching part of a Taint on a node
type NodeTaintRequirement struct {
	NodeTaintOperator NodeTaintOperator `json:"operator,omitempty" yaml:"operator,omitempty"`

	Values []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// NodeTaintOperator defines an operator for match node taints
type NodeTaintOperator string

const (
	// NodeTaintOperatorOpIn will match if the value is found
	NodeTaintOperatorOpIn NodeTaintOperator = "In"
	// NodeTaintOperatorOpNotIn will match if the value is NOT found
	NodeTaintOperatorOpNotIn NodeTaintOperator = "NotIn"
)

// AnnotationKey is a kubernetes object metadata annotation name
type AnnotationKey string

// LabelKey is a kubernetes object metadata label name
type LabelKey string

// LabelName a prometheus metric label name
type LabelName string

// LabelsMaskId is internal
type LabelsMaskId int

// LabelId is internal
type LabelId int

// LabeledResources represents a ResourceList with associated labels.
type LabeledResources struct {
	Labels map[string]string   `json:"labels" yaml:"labels"`
	Values corev1.ResourceList `json:"values" yaml:"values"`
}

const SideCarConfigFileSuffix = "_sidecar_config.json"

// SideCarConfig stores metrics and labels written by sidecars
type SideCarConfig struct {
	SideCarMetrics []SideCarMetric `json:"metrics" yaml:"metrics"`

	Labels []ExtensionLabel `json:"labels" yaml:"labels"`
}

// SideCarMetric is an external metric that can be published by a side-car
type SideCarMetric struct {
	Name       string               `json:"name" yaml:"name"`
	Help       string               `json:"help" yaml:"help"`
	LabelNames []string             `json:"labelNames" yaml:"labelNames"`
	Values     []SideCarMetricValue `json:"values" yaml:"values"`
}

type SideCarMetricValue struct {
	MetricLabels []string `json:"labels" yaml:"labels"`
	Value        float64  `json:"value" yaml:"value"`
}

const MaxExtensionLabels = 100

func ValidateCollectorSpecAndApplyDefaults(spec *MetricsPrometheusCollector) error {
	// Set default names and validate there are no repetitions.
	for i := range spec.Aggregations {
		a := &spec.Aggregations[i]
		if a.Name == "" {
			if spec.DefaultAggregationNames {
				a.Name = string(a.Sources.Type)
			} else {
				a.Name = fmt.Sprintf("unnamed-aggregation-%d", i)
			}
		}
		for j := range a.Levels {
			l := &a.Levels[j]
			if l.Name == "" {
				if spec.DefaultLevelNames {
					l.Name = string(l.Mask.Level)
				} else {
					l.Name = fmt.Sprintf("unnamed-level-%d", j)
				}
			}
		}
	}

	names := sets.NewString()
	for _, a := range spec.Aggregations {
		for _, l := range a.Levels {
			for _, s := range a.Sources.GetSources() {
				n := fmt.Sprintf("%s %s %s %s", a.Name, s.String(), l.Name, l.Operation.String())
				if names.Has(n) {
					return fmt.Errorf("duplicate aggregation/source/level/operation combination found \"%s\"", n)
				}
				names.Insert(n)
				for _, o := range l.Operations {
					n := fmt.Sprintf("%s %s %s %s", a.Name, s.String(), l.Name, o)
					if names.Has(n) {
						return fmt.Errorf("duplicate aggregation/source/level/operation combination found \"%s\"", n)
					}
					names.Insert(n)
				}
			}
		}
	}

	// validate extension labels
	totalExtensionLabels := len(spec.Extensions.Pods)
	totalExtensionLabels += len(spec.Extensions.Namespaces)
	totalExtensionLabels += len(spec.Extensions.Nodes)
	totalExtensionLabels += len(spec.Extensions.Quota)
	totalExtensionLabels += len(spec.Extensions.PVCs)
	totalExtensionLabels += len(spec.Extensions.PVs)
	totalExtensionLabels += len(spec.Extensions.NodeTaints)

	if s, m := totalExtensionLabels, MaxExtensionLabels; s > m {
		return fmt.Errorf("collector config specifies %v extension labels which exceed the max (%v)", s, m)
	}

	if spec.BuiltIn.EnableResourceQuotaDescriptor {
		return nil
	}

	for ii, aggregation := range spec.Aggregations {
		if aggregation.Sources.Type != QuotaType {
			continue
		}

		for jj, source := range aggregation.Sources.GetSources() {
			if RQDSources.Has(source) {
				return fmt.Errorf("collector config specifies a source that requires rqd, but rqd is not enabled; aggregation[%v].sources[%v]", ii, jj)
			}
		}
	}

	if spec.UtilizationServer.DeleteUnregisteredPodsAfterAgeMinutes == 0 {
		spec.UtilizationServer.DeleteUnregisteredPodsAfterAgeMinutes = 30
	}
	if spec.UtilizationServer.DeleteUnregisteredPodsAfterCycles == 0 {
		spec.UtilizationServer.DeleteUnregisteredPodsAfterCycles = 10
	}

	return nil
}
