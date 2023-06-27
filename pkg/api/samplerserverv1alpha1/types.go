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

package samplerserverv1alpha1

import (
	"path/filepath"

	"k8s.io/utils/pointer"
)

// MetricsNodeSampler configures a metrics-node-sampler.
type MetricsNodeSampler struct {
	Buffer Buffer `json:"buffer" yaml:"buffer"`
	Reader Reader `json:"reader" yaml:"reader"`

	// PBPort is the port to bind for the protocol buffer endpoint
	PBPort int `json:"pbPort" yaml:"pbPort"`

	// RestPort is the port to bind for the REST endpoint
	RestPort int `json:"restPort" yaml:"restPort"`

	// Address is used by tests to bind to localhost
	Address string `json:"address,omitempty" yaml:"address,omitempty"`

	SendPushMetricsRetryCount int `json:"sendPushMetricsRetryCount" yaml:"sendPushMetricsRetryCount"`

	DEPRECATED_PushService   string `json:"pushAddress" yaml:"pushAddress"` // TODO: Remove this
	PushHeadlessService      string `json:"pushHeadlessService" yaml:"pushHeadlessService"`
	PushHeadlessServicePort  int    `json:"pushHeadlessServicePort" yaml:"pushHeadlessServicePort"`
	PushFrequency            string `json:"pushFrequencyDuration" yaml:"pushFrequencyDuration"`
	CheckCreatedPodFrequency string `json:"checkCreatedPodFrequencyDuration" yaml:"checkCreatedPodFrequencyDuration"`

	ExitOnConfigChange bool `json:"exitOnConfigChange" yaml:"exitOnConfigChange"`

	// UseCadvisorMonitor bool
	UseCadvisorMonitor bool `json:"useCadvisorMonitor" yaml:"useContainerMonitor"`
	// CadvisorEndpoint to retrieve cadvisor metrics.
	CadvisorEndpoint string `json:"cadvisorEndpoint" yaml:"cadvisorEndpoint"`

	// UseContainerMonitor enables container monitor metrics from containerd
	UseContainerMonitor bool `json:"useContainerMonitor" yaml:"useContainerMonitor"`

	// ContainerdAddress specifies the address for connecting to containerd
	// Default: /run/containerd/containerd.sock
	ContainerdAddress string `json:"containerdAddress" yaml:"containerdAddress"`

	// ContainerdNamespace specifies which containerd namespace to collect metrics for
	// Default: default
	ContainerdNamespace string `json:"containerdNamespace" yaml:"containerdNamespace"`

	DNSSpec DNSSpec `json:"dnsSpec" yaml:"dnsSpec"`
}

type DNSSpec struct {
	PollSeconds int `json:"pollSeconds" yaml:"pollSeconds"`
}

// Buffer configures the window of buffered metric samples.
type Buffer struct {
	// Size is the max number of containerd scrapes to store
	// Older samples will be expired when the max is reached.
	// +optional
	Size int `json:"size" yaml:"size"`

	// PollsPerMinute is how frequently to read container metrics from containerd
	// +optional
	PollsPerMinute int `json:"pollsPerMinute" yaml:"pollsPerMinute"`
}

// Reader configures how a single sample of cgroup metrics are read
// from the filesystem.
type Reader struct {
	// CGroupVersion defines which version of cgroups to use
	CGroupVersion CGroupVersion `json:"cgroupVersion" yaml:"cgroupVersion"`

	// CPUPath defines where to read cpu metrics from.  Defaults to cpuacct/kubepods.
	CPUPaths []MetricsFilepath `json:"cpuPaths" yaml:"cpuPaths"`

	// MemoryPath defines where to read memory metrics from.  Defaults to memory/kubepods.
	MemoryPaths []MetricsFilepath `json:"memoryPaths" yaml:"memoryPaths"`

	PodPrefix             []string          `json:"podPrefix,omitempty" yaml:"podPrefix,omitempty"`
	PodSuffix             []string          `json:"podSuffix,omitempty" yaml:"podSuffix,omitempty"`
	ContainerPrefix       []string          `json:"containerPrefix,omitempty" yaml:"containerPrefix,omitempty"`
	ContainerSuffix       []string          `json:"containerSuffix,omitempty" yaml:"containerSuffix,omitempty"`
	PodReplacements       map[string]string `json:"podReplacements,omitempty" yaml:"podReplacements,omitempty"`
	ContainerReplacements map[string]string `json:"containerReplacements,omitempty" yaml:"containerReplacements,omitempty"`
	ParentDirectories     []string          `json:"parents,omitempty" yaml:"parents,omitempty"`

	NodeAggregationLevelGlobs []NodeAggregationLevel `json:"nodeAggregationLevelGlobs" yaml:"nodeAggregationLevelGlobs"`

	// ContainerCacheSyncInterval defines how frequently to sync the list of containers.  Defaults to 1 minute.
	ContainerCacheSyncIntervalSeconds *float32 `json:"containerCacheSyncIntervalSeconds" yaml:"containerCacheSyncIntervalSeconds"`

	// MinCPUCoresSampleValue is the minimum number of cpu cores / sec a sample must have to be considered valid.
	// Invalid samples are dropped from responses.
	MinCPUCoresSampleValue float64 `json:"minCPUCoresSampleValue" yaml:"minCPUCoresSampleValue"`
	// MaxCPUCoresSampleValue is the maximum number of cpu cores / sec a sample may have to be considered valid.
	// Invalid samples are dropped from responses.
	MaxCPUCoresSampleValue float64 `json:"maxCPUCoresSampleValue" yaml:"maxCPUCoresSampleValue"`

	// MinCPUCoresNanoSec is defaulted from MinCPUCoresSampleValue
	MinCPUCoresNanoSec int64 `json:"minCPUCoresNanoSec" yaml:"minCPUCoresNanoSec"`
	// MaxCPUCoresNanoSec is defaulted from MaxCPUCoresSampleValue
	MaxCPUCoresNanoSec uint64 `json:"maxCPUCoresNanoSec" yaml:"maxCPUCoresNanoSec"`

	// DropFirstValue if true will drop the first sample value so the average
	// by computing the last-first values more closely matches the average
	// by computing the average of the individual samples.
	DropFirstValue *bool `json:"dropFirstValue,omitempty" yaml:"dropFirstValue,omitempty"`
}

type NodeAggregationLevel string

const (
	NodeAggregationLevelPodsAll        NodeAggregationLevel = "kubelet/kubepods"
	NodeAggregationLevelPodsBestEffort NodeAggregationLevel = "kubelet/kubepods/besteffort"
	NodeAggregationLevelPodsBurstable  NodeAggregationLevel = "kubelet/kubepods/burstable"
	NodeAggregationLevelPodsGuaranteed NodeAggregationLevel = "kubelet/kubepods/guaranteed"
)

var DefaultNodeAggregationLevels = []NodeAggregationLevel{
	NodeAggregationLevelPodsAll,
	NodeAggregationLevelPodsBestEffort,
	NodeAggregationLevelPodsBurstable,
	NodeAggregationLevelPodsGuaranteed,
}

// MetricsFilepath is a filepath to walk for metrics -- e.g. /sys/fs/cgroup/cpuacct/kubelet/kubepods
type MetricsFilepath string

// CGroupVersion is the version of cgroups being used -- either v1 or v2
type CGroupVersion string

var (
	// DefaultMetricsReaderCPUPaths is the default paths for reading cpu metrics
	DefaultMetricsReaderCPUPaths = []MetricsFilepath{
		MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "cpu,cpuacct")),
		MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "cpuacct")),
		MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "cpu")),
	}

	// DefaultCPUPaths is the default paths for reading memory metrics
	DefaultMetricsReaderMemoryPaths = []MetricsFilepath{
		MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "memory")),
	}

	// DefaultContainerCacheSyncInterval is how frequently to sync the list of known containers.
	DefaultContainerCacheSyncIntervalSeconds = pointer.Float32(10)
)

const (
	// CGroupV1 is set for cgroups v1.
	CGroupV1 CGroupVersion = "v1"

	// DefaultPollsPerMinute is the default number polls to perform per minute.
	DefaultPollsPerMinute = 60

	// DefaultSampleBufferSize is the default number of samples to keep.  The cache will have samples
	// over a duration of DefaultPollSeconds*DefaultSampleBufferSize (in seconds).
	DefaultSampleBufferSize = 900

	DefaultPBPort   = 8080
	DefaultRestPort = 8090

	DefaultPushFrequency            = "1m"
	DefaultCheckCreatedPodFrequency = "6s"

	CPUUsageSourceFilename      = "cpuacct.usage"
	CPUThrottlingSourceFilename = "cpu.stat"
	MemoryUsageSourceFilename   = "memory.stat"
	MemoryOOMKillFilename       = "memory.oom_control"
	MemoryOOMFilename           = "memory.failcnt"
)

var (
	// Transformations to cgroup -> pod mapping
	DefaultContainerSuffix = []string{".slice", ".scope"}
	DefaultContainerPrefix = []string{"cri-containerd-"}
	DefaultPodSuffix       = []string{".slice", ".scope"}
	DefaultPodPrefix       = []string{"kubelet-kubepods-burstable-pod", "kubelet-kubepods-besteffort-pod", "kubelet-kubepods-pod", "pod"}
	DefaultReplacements    = map[string]string{
		"_": "-",
	}
	DefaultParentDirectories = []string{
		"kubelet-kubepods-besteffort.slice", "kubelet-kubepods-burstable.slice", "kubelet-kubepods.slice", "kubelet.slice",
	}
)

func (s *MetricsNodeSampler) Default() {
	if s.Reader.ContainerPrefix == nil {
		s.Reader.ContainerPrefix = DefaultContainerPrefix
	}
	if s.Reader.ContainerSuffix == nil {
		s.Reader.ContainerSuffix = DefaultContainerSuffix
	}
	if s.Reader.PodPrefix == nil {
		s.Reader.PodPrefix = DefaultPodPrefix
	}
	if s.Reader.PodSuffix == nil {
		s.Reader.PodSuffix = DefaultPodSuffix
	}
	if s.Reader.ContainerReplacements == nil {
		s.Reader.ContainerReplacements = DefaultReplacements
	}
	if s.Reader.PodReplacements == nil {
		s.Reader.PodReplacements = DefaultReplacements
	}
	if s.Reader.ParentDirectories == nil {
		s.Reader.ParentDirectories = DefaultParentDirectories
	}

	if s.PBPort == 0 {
		s.PBPort = DefaultPBPort
	}
	if s.RestPort == 0 {
		s.RestPort = DefaultRestPort
	}

	if s.Buffer.PollsPerMinute == 0 {
		s.Buffer.PollsPerMinute = DefaultPollsPerMinute
	}
	if s.Buffer.Size == 0 {
		s.Buffer.Size = DefaultSampleBufferSize
	}

	if s.Reader.ContainerCacheSyncIntervalSeconds == nil {
		s.Reader.ContainerCacheSyncIntervalSeconds = pointer.Float32(1)
	}
	if s.Reader.CPUPaths == nil {
		s.Reader.CPUPaths = DefaultMetricsReaderCPUPaths
	}
	if s.Reader.MemoryPaths == nil {
		s.Reader.MemoryPaths = DefaultMetricsReaderMemoryPaths
	}
	if s.Reader.CGroupVersion == "" {
		s.Reader.CGroupVersion = CGroupV1
	}

	if s.Reader.ContainerCacheSyncIntervalSeconds == nil {
		s.Reader.ContainerCacheSyncIntervalSeconds = DefaultContainerCacheSyncIntervalSeconds
	}

	if s.Reader.NodeAggregationLevelGlobs == nil {
		s.Reader.NodeAggregationLevelGlobs = DefaultNodeAggregationLevels
	}

	if s.PushHeadlessServicePort == 0 {
		s.PushHeadlessServicePort = 9090
	}
	if s.PushFrequency == "" {
		s.PushFrequency = DefaultPushFrequency
	}
	if s.CheckCreatedPodFrequency == "" {
		s.CheckCreatedPodFrequency = DefaultCheckCreatedPodFrequency
	}
	if s.SendPushMetricsRetryCount < 0 {
		s.SendPushMetricsRetryCount = 0 // make sure we send metrics at least once
	}

	if s.Reader.MaxCPUCoresSampleValue <= 0 {
		s.Reader.MaxCPUCoresSampleValue = 512.
	}
	if s.Reader.MaxCPUCoresNanoSec == 0 {
		s.Reader.MaxCPUCoresNanoSec = uint64(1e9 * s.Reader.MaxCPUCoresSampleValue) // Convert the CPU value to CPU nano-seconds
	}
	if s.Reader.MinCPUCoresNanoSec == 0 {
		s.Reader.MinCPUCoresNanoSec = int64(1e9 * s.Reader.MinCPUCoresSampleValue) // Convert the CPU value to CPU nano-seconds
	}

	if s.ContainerdAddress == "" {
		s.ContainerdAddress = "/run/containerd/containerd.sock"
	}

	if s.ContainerdNamespace == "" {
		s.ContainerdNamespace = "default"
	}
}
