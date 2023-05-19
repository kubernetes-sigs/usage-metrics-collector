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
	"context"
	"errors"
	"time"

	v1 "github.com/containerd/containerd/metrics/types/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/usage-metrics-collector/pkg/ctrstats"
)

func (s *sampleCache) getContainerCPUAndMemoryCM() (cpuMetrics, memoryMetrics, NetworkMetrics, error) {
	containers, err := ctrstats.GetContainers(s.ContainerdClient)
	if err != nil {
		log.Error(err, "failed to list containers")
		return nil, nil, nil, err
	}

	if err := s.metricsReader.initCM(); err != nil {
		log.Error(err, "failed to initialize metrics reader")
		return nil, nil, nil, err
	}

	cpuResult := cpuMetrics{}
	memResult := memoryMetrics{}
	networkResult := NetworkMetrics{}
	knownPods := sets.NewString()

	for _, c := range containers {
		// TODO: is this a reasonable key for the metric read time?
		readTime := s.metricsReader.readTimeFunc(c.PodID + "/" + c.ContainerID)
		stats, err := ctrstats.GetContainerStats(context.Background(), c)
		if err != nil {
			log.V(10).WithValues(
				"container", c.ContainerID,
			).Info("failed to get container stats - likely an issue with non-running containers being tracked in containerd state", "err", err)
		} else if stats != nil {
			cpu, err := cmStatsToCPUResult(stats, readTime)
			if err != nil {
				log.Error(err, "no cpu stats available for container",
					"namespace", c.NamespaceName,
					"pod", c.PodName,
					"container", c.ContainerName,
				)
			}
			mem, err := cmStatsToMemoryResult(stats, readTime)
			if err != nil {
				log.Error(err, "no memory stats available for container",
					"namespace", c.NamespaceName,
					"pod", c.PodName,
					"container", c.ContainerName,
				)
			}
			ntwk, err := cmStatsToNetworkResult(stats, readTime)
			if err != nil {
				log.Error(err, "no network stats available for container",
					"namespace", c.NamespaceName,
					"pod", c.PodName,
					"container", c.ContainerName,
				)
			}

			var container ContainerKey
			container.ContainerID = c.ContainerID
			container.PodUID = c.PodID
			container.NamespaceName = c.NamespaceName
			container.PodName = c.PodName
			container.ContainerName = c.ContainerName

			cpuResult[container] = cpu
			memResult[container] = mem
			networkResult[container] = ntwk
			knownPods.Insert(c.PodID)
		}
	}

	s.metricsReader.knownContainersSet.Store(knownPods)
	return cpuResult, memResult, networkResult, nil
}

// cmStatsToCPUResult converts cpu stats read from containerd into a compatible type.
func cmStatsToCPUResult(stats *v1.Metrics, readTime time.Time) (containerCPUMetrics, error) {
	metrics := containerCPUMetrics{}
	if stats.CPU == nil {
		err := errors.New("no cpu stats available")
		return metrics, err
	}

	metrics.usage.Time = readTime
	// TODO: I assume we want usage.total but kernel/user breakouts are also available
	metrics.usage.UsageNanoSec = stats.CPU.Usage.Total

	metrics.throttling.Time = readTime
	metrics.throttling.ThrottledNanoSec = stats.CPU.Throttling.ThrottledTime
	metrics.throttling.ThrottledPeriods = stats.CPU.Throttling.ThrottledPeriods
	metrics.throttling.TotalPeriods = stats.CPU.Throttling.Periods

	return metrics, nil
}

// cmStatsToMemoryResult converts memory stats read from containerd into a compatible type.
func cmStatsToMemoryResult(stats *v1.Metrics, readTime time.Time) (containerMemoryMetrics, error) {
	metrics := containerMemoryMetrics{}
	if stats.Memory == nil {
		err := errors.New("no memory stats available")
		return metrics, err
	}

	metrics.Time = readTime
	// NOTE: RSSHuge, MappedFiles, pgfaults, (in)active anon, etc. also available
	// TODO: should this be Total{RSS,Cache} or is this fine?
	metrics.RSS = stats.Memory.RSS
	metrics.Cache = stats.Memory.Cache

	if stats.MemoryOomControl != nil {
		// TODO: not sure if these are the right metrics?
		metrics.OOMKills = stats.MemoryOomControl.OomKill
		metrics.OOMs = stats.MemoryOomControl.UnderOom
	} else {
		log.V(10).Info("no OOM stats available")
	}

	return metrics, nil
}

// cmStatsToMemoryResult converts memory stats read from containerd into a compatible type.
func cmStatsToNetworkResult(stats *v1.Metrics, readTime time.Time) (ContainerNetworkMetrics, error) {
	metrics := ContainerNetworkMetrics{
		Usage: make(map[string]ContainerNetworkUsageMetrics, len(stats.Network)),
	}
	if stats.Network == nil {
		err := errors.New("no network stats available")
		return metrics, err
	}

	for _, n := range stats.Network {
		metrics.Usage[n.Name] = ContainerNetworkUsageMetrics{
			CumulativeRXBytes:   n.RxBytes,
			CumulativeRXPackets: n.RxPackets,
			CumulativeRXErrors:  n.RxErrors,
			CumulativeRXDropped: n.RxDropped,
			CumulativeTXBytes:   n.TxBytes,
			CumulativeTXPackets: n.TxPackets,
			CumulativeTXErrors:  n.TxErrors,
			CumulativeTXDropped: n.TxDropped,
		}
	}

	return metrics, nil
}
