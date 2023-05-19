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

	// NamespaceName is the namespace of the pod
	NamespaceName string

	// ContainerName is the name of the container
	ContainerName string

	// PodName is the name of the pod
	PodName string
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
	MemoryOOM                     uint64
	MemoryOOMKill                 uint64
	// CumulativeMemoryHigh        uint64

	// These values are derived from the last sample

	// CPUCores are the number of cores used
	HasCPUData                 bool
	CPUCoresNanoSec            uint64
	CPUThrottledUSec           uint64
	CPUPercentPeriodsThrottled float64
	CPUPeriodsSec              uint64
	CPUThrottledPeriodsSec     uint64

	Network map[string]ContainerNetworkUsageMetrics

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

type sampleResult struct {
	values sampleInstantSlice
	avg    sampleInstant
}

// allSampleInstants are all the samples in the cache
type allSampleInstants struct {
	containers map[ContainerKey]*sampleResult
	node       map[samplerserverv1alpha1.NodeAggregationLevel]*sampleResult
}

const (
	MemoryUsageMetricType   ContainerMetricType = "memory-usage"
	MemoryOOMKillMetricType ContainerMetricType = "oom-kill"
	MemoryOMMMetricType     ContainerMetricType = "oom"
	CPUUsageMetricType      ContainerMetricType = "cpu-usage"
	CPUThrottlingMetricType ContainerMetricType = "cpu-throttling"
)

// networkMetrics is a map of the networkMetrics for each container running in a Pod
type NetworkMetrics map[ContainerKey]ContainerNetworkMetrics

type ContainerNetworkMetrics struct {
	Time time.Time

	Usage map[string]ContainerNetworkUsageMetrics
}

type ContainerNetworkUsageMetrics struct {
	RXBytes   uint64
	RXPackets uint64
	RXErrors  uint64
	RXDropped uint64
	TXBytes   uint64
	TXPackets uint64
	TXErrors  uint64
	TXDropped uint64

	CumulativeRXBytes   uint64
	CumulativeRXPackets uint64
	CumulativeRXErrors  uint64
	CumulativeRXDropped uint64
	CumulativeTXBytes   uint64
	CumulativeTXPackets uint64
	CumulativeTXErrors  uint64
	CumulativeTXDropped uint64
}
