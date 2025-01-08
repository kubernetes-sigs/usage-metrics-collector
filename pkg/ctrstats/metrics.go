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

//go:build linux && amd64

package ctrstats

import (
	"fmt"
	"time"

	// nolint: typecheck
	v2 "github.com/containerd/cgroups/v2/stats"
	metrics "github.com/docker/go-metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type Metric struct {
	name   string
	help   string
	unit   metrics.Unit
	Vt     prometheus.ValueType
	labels []string
	// getvalues returns the value and labels for the data
	GetValues func(stats *v2.Metrics) value
}

type value struct {
	V float64
	L []string
}

func (m *Metric) Desc() *prometheus.Desc {
	name := m.name
	if m.unit != "" {
		name = fmt.Sprintf("%s_%s", m.name, m.unit)
	}
	return prometheus.NewDesc(name, m.help, append([]string{"container", "id", "image", "namespace", "pod"}, m.labels...), make(map[string]string))
}

var CpuMetrics = []*Metric{
	{
		name: "container_cpu_usage_seconds_total",
		help: "Cumulative cpu time consumed in seconds.",
		Vt:   prometheus.CounterValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.CPU == nil {
				return value{}
			}

			return value{
				V: (float64(stats.CPU.UsageUsec) * float64(time.Microsecond)) / float64(time.Second),
			}
		},
	},
	{
		name: "container_cpu_user_seconds_total",
		help: "Cumulative user cpu time consumed in seconds.",
		Vt:   prometheus.CounterValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.CPU == nil {
				return value{}
			}

			return value{
				V: (float64(stats.CPU.UserUsec) * float64(time.Microsecond)) / float64(time.Second),
			}
		},
	},
	{
		name: "container_cpu_system_seconds_total",
		help: "Cumulative system cpu time consumed in seconds.",
		Vt:   prometheus.CounterValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.CPU == nil {
				return value{}
			}

			return value{
				V: (float64(stats.CPU.SystemUsec) * float64(time.Microsecond)) / float64(time.Second),
			}
		},
	},
	{
		name: "container_cpu_cfs_periods_total",
		help: "Number of throttled period intervals.",
		Vt:   prometheus.CounterValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.CPU == nil {
				return value{}
			}

			return value{
				V: float64(stats.CPU.NrThrottled),
			}
		},
	},
	{
		name: "container_cpu_cfs_throttled_seconds_total",
		help: "Total time duration the container has been throttled.",
		Vt:   prometheus.CounterValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.CPU == nil {
				return value{}
			}

			return value{
				V: (float64(stats.CPU.ThrottledUsec) * float64(time.Microsecond)) / float64(time.Second),
			}
		},
	},
}

var MemoryMetrics = []*Metric{

	{
		name: "container_memory_cache",
		help: "Number of bytes of page cache memory.",
		Vt:   prometheus.GaugeValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.Memory == nil {
				return value{}
			}
			return value{
				V: float64(stats.Memory.File),
			}
		},
	},
	{
		name: "container_memory_rss",
		help: "Size of RSS in bytes.",
		Vt:   prometheus.GaugeValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.Memory == nil {
				return value{}
			}
			// add anon + file = rss
			return value{
				V: float64(stats.Memory.Anon + stats.Memory.File),
			}

		},
	},
	{
		name: "container_memory_swap",
		help: "Container swap usage in bytes.",
		Vt:   prometheus.GaugeValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.Memory == nil {
				return value{}
			}
			return value{
				V: float64(stats.Memory.SwapUsage),
			}
		},
	},
	{
		name: "container_memory_usage_bytes",
		help: "Current memory usage in bytes, including all memory regardless of when it was accessed",
		Vt:   prometheus.GaugeValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.Memory == nil {
				return value{}
			}
			return value{
				V: float64(stats.Memory.Usage),
			}
		},
	},
	{
		name: "container_memory_working_set_bytes",
		help: "Current working set in bytes.",
		Vt:   prometheus.GaugeValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.Memory == nil {
				return value{}
			}
			var workingSet uint64
			if stats.Memory.Usage > stats.Memory.InactiveFile {
				workingSet = stats.Memory.Usage - stats.Memory.InactiveFile
			} else {
				workingSet = 0
			}

			return value{
				V: float64(workingSet),
			}
		},
	},
}

var FileSystemMetrics = []*Metric{

	/*container_fs_limit_bytes
	{

		name: "container_fs_limit_bytes",
		help: "Number of bytes that can be consumed by the container on this filesystem.",
		Vt:   prometheus.GaugeValue,
		GetValues: func(stats *v2.Metrics) value {
			if stats.Filesystem == nil {
				return value{}
			}
			return value{
				V: float64(stats.Filesystem.Limit),
			}

		},
	},
	*/
}
