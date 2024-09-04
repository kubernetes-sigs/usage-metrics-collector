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

package ctrstats

import (
	"context"
	gocontext "context"
	"fmt"

	v1 "github.com/containerd/cgroups/stats/v1"
	v2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/containerd"
	"github.com/containerd/typeurl"
	"github.com/sirupsen/logrus"
)

type Container struct {
	ContainerID      string
	PodID            string
	PodName          string
	SandboxNamespace string
	ContainerName    string
	Container        containerd.Container
	NamespaceName    string
}

var ctrStatsLog = logrus.WithField("source", "ctrstats")

func NewContainerdClient(address, namespace string) (*containerd.Client, error) {
	return containerd.New(address, containerd.WithDefaultNamespace(namespace))
}

func GetContainers(client *containerd.Client) ([]Container, error) {
	ctx := gocontext.Background()

	containers, err := client.Containers(ctx)
	if err != nil {
		ctrStatsLog.WithError(err).Errorf("client containers request failure")

		return nil, err
	}

	cids := make([]Container, 0, len(containers))
	for _, c := range containers {
		container, err := c.Info(ctx)
		if err != nil {
			ctrStatsLog.WithField("container-id", c.ID()).Warn("could not get container info")
			continue
		}

		containerInfo := Container{
			ContainerID:      c.ID(),
			PodID:            container.Labels["io.kubernetes.pod.uid"],
			PodName:          container.Labels["io.kubernetes.pod.name"],
			SandboxNamespace: container.Labels["io.kubernetes.pod.namespace"],
			ContainerName:    container.Labels["io.kubernetes.container.name"],
			NamespaceName:    container.Labels["io.kubernetes.pod.namespace"],
			Container:        c,
		}

		cids = append(cids, containerInfo)
	}

	return cids, nil
}

func GetContainerStatsV1(ctx context.Context, c Container) (*v1.Metrics, error) {
	task, err := c.Container.Task(ctx, nil)
	if err != nil {
		return nil, err
	}

	r, err := task.Metrics(ctx)
	if err != nil {
		return nil, err
	}

	if r.Data != nil {
		s, err := typeurl.UnmarshalAny(r.Data)
		if err != nil {
			return nil, err
		}

		stats, ok := s.(*v1.Metrics)
		if !ok {
			return nil, fmt.Errorf("type assertion failure for task.Metrics' Data field")
		}
		return stats, nil
	}

	return nil, fmt.Errorf("no stats obtained for container: %s", c.ContainerID)
}

func GetContainerStatsV2(ctx context.Context, c Container) (*v2.Metrics, error) {
	task, err := c.Container.Task(ctx, nil)
	if err != nil {
		return nil, err
	}

	r, err := task.Metrics(ctx)
	if err != nil {
		return nil, err
	}

	if r.Data != nil {
		s, err := typeurl.UnmarshalAny(r.Data)
		if err != nil {
			return nil, err
		}

		stats, ok := s.(*v2.Metrics)
		if !ok {
			return nil, fmt.Errorf("type assertion failure for task.Metrics' Data field")
		}
		return stats, nil
	}

	return nil, fmt.Errorf("no stats obtained for container: %s", c.ContainerID)
}
