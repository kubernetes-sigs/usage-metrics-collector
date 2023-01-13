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
	"github.com/containerd/containerd"
	"github.com/containerd/typeurl"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

type Container struct {
	SandboxID        string
	ContainerID      string
	PodID            string
	PodName          string
	SandboxNamespace string
	ContainerName    string
	Container        containerd.Container
}

var ctrStatsLog = logrus.WithField("source", "ctrstats")

func NewContainerdClient(address, namespace string) (*containerd.Client, error) {
	return containerd.New(address, containerd.WithDefaultNamespace(namespace))
}

func GetContainers(client *containerd.Client) ([]Container, error) {
	var cids []Container

	ctx := gocontext.Background()

	containers, err := client.Containers(ctx)
	if err != nil {
		ctrStatsLog.WithError(err).Errorf("client containers request failure")

		return cids, err
	}

	for _, c := range containers {
		var sandboxID, podName, sandboxNamespace, containerName string

		container, err := c.Info(ctx)
		if err != nil {
			ctrStatsLog.WithField("container-id", c.ID()).Warn("could not get container info")
			continue
		}

		if container.Spec != nil && container.Spec.GetValue() != nil {
			v, err := typeurl.UnmarshalAny(container.Spec)
			if err != nil {
				ctrStatsLog.WithField("container-id", c.ID()).Warn("could not get container Spec")
				continue
			}

			spec, ok := v.(*specs.Spec)
			if !ok {
				ctrStatsLog.WithField("container-id", c.ID()).Warn("spec type assertion failure")
				continue
			}

			sandboxID = spec.Annotations["io.kubernetes.cri.sandbox-id"]
			podName = spec.Annotations["io.kubernetes.cri.sandbox-name"]
			sandboxNamespace = spec.Annotations["io.kubernetes.cri.sandbox-namespace"]
			containerName = spec.Annotations["io.kubernetes.cri.container-name"]
		}

		containerInfo := Container{
			ContainerID:      c.ID(),
			SandboxID:        sandboxID,
			PodID:            container.Labels["io.kubernetes.pod.uid"],
			PodName:          podName,
			SandboxNamespace: sandboxNamespace,
			ContainerName:    containerName,
			Container:        c,
		}

		// If there isn't a SandboxID, then we're probably dealing with
		// standard container (not a pod - launched with ctr)-- set SandboxID to ContainerID
		if containerInfo.SandboxID == "" {
			containerInfo.SandboxID = containerInfo.ContainerID
		}

		cids = append(cids, containerInfo)
	}

	return cids, nil
}

func GetContainerStats(ctx context.Context, c Container) (*v1.Metrics, error) {
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
