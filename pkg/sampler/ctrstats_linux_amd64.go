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
	v2 "github.com/containerd/containerd/metrics/types/v2"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/usage-metrics-collector/pkg/ctrstats"
)

func (s *sampleCache) getContainerCPUAndMemoryCM() (cpuMetrics, memoryMetrics, error) {
	log.V(9).Info("fetching cpu and memory for containers")

	containers, err := ctrstats.GetContainers(s.ContainerdClient)
	if err != nil {
		log.Error(err, "failed to list containers")
		return nil, nil, err
	}

	if err := s.metricsReader.initCM(); err != nil {
		log.Error(err, "failed to initialize metrics reader")
		return nil, nil, err
	}

	cpuResult := cpuMetrics{}
	memResult := memoryMetrics{}
	knownPods := sets.NewString()

	log.V(9).Info("found containers", "count", len(containers))

	for _, c := range containers {
		readTime := s.metricsReader.readTimeFunc(c.PodID + "/" + c.ContainerID)
		var statsV1 *v1.Metrics
		var statsV2 *v2.Metrics
		var err error
		if s.metricsReader.IsCgroupV2() {
			statsV2, err = ctrstats.GetContainerStatsV2(context.Background(), c)
		} else {
			statsV1, err = ctrstats.GetContainerStatsV1(context.Background(), c)
		}

		if err != nil {
			log.V(10).WithValues(
				"container", c.ContainerID,
			).Info("failed to get container stats - likely an issue with non-running containers being tracked in containerd state", "err", err)
		} else if statsV1 != nil || statsV2 != nil {
			var cpu containerCPUMetrics
			if statsV1 != nil {
				cpu, err = cmStatsToCPUResultV1(statsV1, readTime)
			} else if statsV2 != nil {
				cpu, err = cmStatsToCPUResultV2(statsV2, readTime)
			}
			if err != nil {
				log.Error(err, "no cpu stats available for container",
					"namespace", c.NamespaceName,
					"pod", c.PodName,
					"container", c.ContainerName,
				)
			}

			var mem containerMemoryMetrics
			if statsV1 != nil {
				mem, err = cmStatsToMemoryResultV1(statsV1, readTime)

			} else if statsV2 != nil {
				mem, err = cmStatsToMemoryResultV2(statsV2, readTime)
			}
			if err != nil {
				log.Error(err, "no memory stats available for container",
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
			knownPods.Insert(c.PodID)
		}
	}

	s.metricsReader.knownContainersSet.Store(knownPods)
	return cpuResult, memResult, nil
}

// cmStatsToCPUResultV1 converts cpu stats read from containerd into a compatible type.
func cmStatsToCPUResultV1(stats *v1.Metrics, readTime time.Time) (containerCPUMetrics, error) {
	metrics := containerCPUMetrics{}
	if stats.CPU == nil {
		err := errors.New("no cpu stats available")
		return metrics, err
	}

	metrics.usage.Time = readTime
	metrics.usage.UsageNanoSec = stats.CPU.Usage.Total

	metrics.throttling.Time = readTime
	metrics.throttling.ThrottledNanoSec = stats.CPU.Throttling.ThrottledTime
	metrics.throttling.ThrottledPeriods = stats.CPU.Throttling.ThrottledPeriods
	metrics.throttling.TotalPeriods = stats.CPU.Throttling.Periods

	return metrics, nil
}

// cmStatsToCPUResultV2 converts cpu stats read from containerd into a compatible type.
func cmStatsToCPUResultV2(stats *v2.Metrics, readTime time.Time) (containerCPUMetrics, error) {
	metrics := containerCPUMetrics{}
	if stats.CPU == nil {
		err := errors.New("no cpu stats available")
		return metrics, err
	}

	metrics.usage.Time = readTime
	metrics.usage.UsageNanoSec = stats.CPU.UsageUsec * 1000 // convert usec to nanosec

	metrics.throttling.Time = readTime
	metrics.throttling.ThrottledNanoSec = stats.CPU.ThrottledUsec * 1000 // convert usec to nanosec
	metrics.throttling.ThrottledPeriods = stats.CPU.NrThrottled
	metrics.throttling.TotalPeriods = stats.CPU.NrPeriods

	return metrics, nil
}

// cmStatsToMemoryResultV1 converts memory stats read from containerd into a compatible type.
func cmStatsToMemoryResultV1(stats *v1.Metrics, readTime time.Time) (containerMemoryMetrics, error) {
	metrics := containerMemoryMetrics{}
	if stats.Memory == nil {
		err := errors.New("no memory stats available")
		return metrics, err
	}

	metrics.Time = readTime
	metrics.RSS = stats.Memory.RSS
	metrics.Cache = stats.Memory.Cache

	if stats.MemoryOomControl != nil {
		metrics.OOMKills = stats.MemoryOomControl.OomKill
		metrics.OOMs = stats.MemoryOomControl.UnderOom
	} else {
		log.V(10).Info("no OOM stats available")
	}

	return metrics, nil
}

// cmStatsToMemoryResultV2 converts memory stats read from containerd into a compatible type.
func cmStatsToMemoryResultV2(stats *v2.Metrics, readTime time.Time) (containerMemoryMetrics, error) {
	metrics := containerMemoryMetrics{}
	if stats.Memory == nil {
		err := errors.New("no memory stats available")
		return metrics, err
	}

	metrics.Time = readTime
	metrics.Current = stats.Memory.Usage

	if stats.MemoryEvents != nil {
		metrics.OOMKills = stats.MemoryEvents.OomKill
		metrics.OOMs = stats.MemoryEvents.Oom
	} else {
		log.V(10).Info("no OOM stats available")
	}

	return metrics, nil
}
