package sampler

import (
	"time"

	"sigs.k8s.io/usage-metrics-collector/pkg/api/samplerserverv1alpha1"
)

// ContainerKey is the key to a container running in a Pod
type ContainerKey struct {
	// ContainerID is the id of the container, and corresponds to the pod.status.containerStatuses.containerID
	ContainerID string
	// PodUID is the uid of the pod the container is running in, and corresponds to the pod.metadata.uid, or for
	// mirror pods the config.mirror annotation.
	PodUID string
}

type sampleInstant struct {
	Time time.Time

	MemoryBytes                   uint64
	CumulativeCPUUsec             uint64
	CumulativeCPUThrottlingUsec   uint64
	CumulativeCPUPeriods          uint64
	CumulativeCPUThrottledPeriods uint64
	CumulativeMemoryOOM           uint64
	CumulativeMemoryOOMKill       uint64
	// CumulativeMemoryHigh        uint64

	// These values are derived from the last sample

	// CPUCores are the number of cores used
	HasCPUData                 bool
	CPUCoresNanoSec            uint64
	CPUThrottledUSec           uint64
	CPUPercentPeriodsThrottled float64

	// MemoryHighEvents uint64
	// MemoryLowEvents  uint64
	// OOMEvents        uint64
	// OOMKillEvents    uint64

	// MemoryUsageLifetimeMaxBytes uint64
	// MemoryLimitBytes            uint64
}

// ContainerMetricType identifies a type of metrics that corresponds to a specific cgroups file
type ContainerMetricType string

// sampleInstants are samples read from containerd
type sampleInstants struct {
	containers map[ContainerKey]sampleInstant
	node       map[samplerserverv1alpha1.NodeAggregationLevel]sampleInstant
}

type sampleInstantSlice []sampleInstant

// allSampleInstants are all the samples in the cache
type allSampleInstants struct {
	containers map[ContainerKey]sampleInstantSlice
	node       map[samplerserverv1alpha1.NodeAggregationLevel]sampleInstantSlice
}

const (
	MemoryUsageMetricType   ContainerMetricType = "memory-usage"
	MemoryOOMKillMetricType ContainerMetricType = "oom-kill"
	MemoryOMMMetricType     ContainerMetricType = "oom"
	CPUUsageMetricType      ContainerMetricType = "cpu-usage"
	CPUThrottlingMetricType ContainerMetricType = "cpu-throttling"
)
