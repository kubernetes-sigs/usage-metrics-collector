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

package testutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	statsv1 "github.com/containerd/cgroups/stats/v1"
	statsv2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/typeurl"
)

type FakeContainerdClient struct {
	metrics   []types.Metric
	Labels    []map[string]string
	init      sync.Once
	MetricsV1 []*statsv1.Metrics
	MetricsV2 []*statsv2.Metrics
	time      time.Time
}

// Containers returns fake containers with populate metrics
func (c *FakeContainerdClient) Containers(ctx context.Context, filters ...string) ([]containerd.Container, error) {
	var err error
	c.init.Do(func() {
		c.time = time.Now()
	})
	if err != nil {
		return nil, err
	}
	return c.populateContainers()
}

func (c *FakeContainerdClient) Close() error {
	return nil
}

// generateMetrics populates container metrics with values from cgroup v1 and v2
func (c *FakeContainerdClient) generateMetrics() error {
	c.metrics = []types.Metric{}
	for i := range c.MetricsV1 {
		// add cgroups v1 metrics
		data, err := typeurl.MarshalAny(c.MetricsV1[i])
		if err != nil {
			return err
		}
		m := types.Metric{
			Data: data,
		}
		m.Timestamp = c.time
		c.metrics = append(c.metrics, m)

		// add cgroups v2 metrics
		data2, err := typeurl.MarshalAny(c.MetricsV2[i])
		if err != nil {
			return err
		}
		m2 := types.Metric{
			Data: data2,
		}
		m2.Timestamp = c.time
		c.metrics = append(c.metrics, m2)
	}

	return nil
}

// populateContainers() returns fake containers with newly generated metrics
func (c *FakeContainerdClient) populateContainers() ([]containerd.Container, error) {
	c.incrementValues()
	err := c.generateMetrics()
	if err != nil {
		return nil, err
	}

	allContainers := []containerd.Container{}
	for i := range c.metrics {
		task := FakeTask{
			FakeMetrics: c.metrics[i],
		}
		container := containers.Container{
			Labels: c.Labels[i],
		}
		fakeContainer := FakeContainer{
			FakeTask:            task,
			UnderlyingContainer: container,
			FakeID:              fmt.Sprintf("%d", i),
		}
		allContainers = append(allContainers, fakeContainer)
	}
	return allContainers, nil
}

// incrementValues increments container usage values by 10% for all metrics
func (c *FakeContainerdClient) incrementValues() {
	c.time = c.time.Add(time.Second)
	for i := range c.MetricsV1 {
		// cgroups v1
		c.MetricsV1[i].CPU.Usage.Total += c.MetricsV1[i].CPU.Usage.Total / 10
		c.MetricsV1[i].CPU.Throttling.ThrottledTime += c.MetricsV1[i].CPU.Throttling.ThrottledTime / 10
		c.MetricsV1[i].Memory.Usage.Usage += c.MetricsV1[i].Memory.Usage.Usage / 10
		c.MetricsV1[i].MemoryOomControl.OomKill += c.MetricsV1[i].MemoryOomControl.OomKill / 10

		// cgrups v2
		c.MetricsV2[i].CPU.UsageUsec += c.MetricsV2[i].CPU.UsageUsec / 10
		c.MetricsV2[i].CPU.ThrottledUsec += c.MetricsV2[i].CPU.ThrottledUsec / 10
		c.MetricsV2[i].Memory.Usage += c.MetricsV2[i].Memory.Usage / 10
		c.MetricsV2[i].MemoryEvents.OomKill += c.MetricsV2[i].MemoryEvents.OomKill / 10
		c.MetricsV2[i].MemoryEvents.Oom += c.MetricsV2[i].MemoryEvents.Oom / 10
		c.MetricsV2[i].MemoryEvents.High += c.MetricsV2[i].MemoryEvents.High / 10
		c.MetricsV2[i].MemoryEvents.Low += c.MetricsV2[i].MemoryEvents.Low / 10
	}
}

type FakeContainer struct {
	containerd.Container
	UnderlyingContainer containers.Container
	FakeTask            FakeTask
	FakeID              string
}

// ID returns container id
func (fc FakeContainer) ID() string {
	return fc.FakeID
}

// Info returns the underlying container for a fake container
func (fc FakeContainer) Info(context.Context, ...containerd.InfoOpts) (containers.Container, error) {
	return fc.UnderlyingContainer, nil
}

// Task returns the fake container task
func (fc FakeContainer) Task(context.Context, cio.Attach) (containerd.Task, error) {
	return &fc.FakeTask, nil
}

type FakeTask struct {
	containerd.Task
	FakeMetrics types.Metric
}

// Metrics returns metrics in a fake task
func (t *FakeTask) Metrics(ctx context.Context) (*types.Metric, error) {
	return &t.FakeMetrics, nil
}
