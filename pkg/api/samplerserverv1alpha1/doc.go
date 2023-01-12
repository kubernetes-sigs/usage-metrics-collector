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

// Package samplerserverv1alpha1 defines the MetricsNodeSampler API.
//
// The sampler scrapes cgroup metrics from the node filesystem and serves an
// aggregated view over a GRPC interface.
//
// Example MetricsNodeSampler
//
//	requiredSamples: 30 # start serving aggregated metrics after we have this many samplers for a container
//	pbPort: 8080 # bind this port for protocol-buffer GRPC requests
//	restPort: 8090 # bind this port for rest GRPC requests
//	buffer: # store 15 minutes worth of samples in the buffer
//	  size: 900 # store 900 samples
//	  pollsPerMinute: 60 # sample every 1 seconds
//	reader:
//	  containerCacheSyncIntervalSeconds: 10 # look for new containers every 10 seconds
//	  cpuPaths: # look for cpu metrics under these paths
//	  - sys/fs/cgroup/cpuacct
//	  - sys/fs/cgroup/cpu,cpuacct
//	  memoryPaths: # look for memory metrics under these paths
//	  - sys/fs/cgroup/memory
//
// The sampler reads the following metrics from cgroupfs
//
//	cpuacct.usage: reports the total CPU time (in nanoseconds) consumed by all tasks in this cgroup
//	cpu.stat:
//	  nr_periods – number of periods that any thread in the cgroup was runnable
//	  nr_throttled – number of runnable periods in which the application used its entire quota and was throttled
//	  throttled_time – sum total amount of time individual threads within the cgroup were throttled
//	memory.stat:
//	  cache : The amount of memory used by the processes of this control group that can be associated precisely with a block on a block device.
//	  rss : The amount of memory that doesn’t correspond to anything on disk: stacks, heaps, and anonymous memory maps.
//	oom_control:
//	  oom-kill : OOM Kill counter
//	  memory.failcnt: reports the number of times that the memory limit has reached the value set in memory.limit_in_bytes.
package samplerserverv1alpha1
