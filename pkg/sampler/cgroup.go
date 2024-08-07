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
	"bufio"
	"bytes"
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/samplerserverv1alpha1"
)

// metricsReader reads cgroup metrics from pseudo files
type metricsReader struct {
	samplerserverv1alpha1.Reader `json:",inline" yaml:",inline"`

	ctx context.Context

	// fs is the filesystem used -- defaults to the OS fs
	fs fs.FS `json:"-" yaml:"-"`

	// cache cpu metric file names
	cpuFiles      map[ContainerKey]map[ContainerMetricType]metricFilepath
	cpuFilesMutex sync.RWMutex

	// cache memory metric file names
	memoryFiles      map[ContainerKey]map[ContainerMetricType]metricFilepath
	memoryFilesMutex sync.RWMutex

	// node level files
	nodeCPUFiles    map[samplerserverv1alpha1.NodeAggregationLevel]map[ContainerMetricType]metricFilepath
	nodeMemoryFiles map[samplerserverv1alpha1.NodeAggregationLevel]map[ContainerMetricType]metricFilepath

	// first time initialization
	once    sync.Once
	initErr error

	knownContainersSet atomic.Value

	readTimeFunc func(string) time.Time

	Stop context.CancelFunc
}

// cpuMetrics is a map of the cpuMetrics for each container running in a Pod
type cpuMetrics map[ContainerKey]containerCPUMetrics

// memoryMetrics is a map of the memoryMetrics for each container running in a Pod
type memoryMetrics map[ContainerKey]containerMemoryMetrics

// containerCPUMetrics are the cpu metrics for a container
type containerCPUMetrics struct {
	usage      containerCPUUsageMetrics
	throttling ContainerCPUThrottlingMetrics
}

type containerCPUUsageMetrics struct {
	// Time is the time that the sample was taken
	Time time.Time

	// UsageNanoSec is the cpu time in nano seconds (1e-9)
	UsageNanoSec uint64
}

// https://kernel.googlesource.com/pub/scm/linux/kernel/git/glommer/memcg/+/cpu_stat/Documentation/cgroups/cpu.txt
type ContainerCPUThrottlingMetrics struct {
	// Time is the time that the sample was taken
	Time             time.Time
	ThrottledNanoSec uint64
	// the number of cpu scheduling periods that the container has been throttled for
	// the container has been considered throttled for a period if it attempts to use the Cpu after exceeding Cpu quota.
	ThrottledPeriods uint64
	// the total number of Cpu scheduling period that have elapsed
	TotalPeriods uint64
}

// containerMemoryMetrics are the memory metrics for a container
type containerMemoryMetrics struct {
	// Time is the time that the sample was taken
	Time time.Time
	// RSS is the rss memory amount. Only set on cgroupv1
	RSS uint64
	// Cache is the cache memory amount. Only set on cgroupv1
	Cache uint64
	// Current is the total memory amount for the container. Only set on cgroupv2
	Current uint64
	// OOMKills is the number of times that the container has been killed by the oom killer
	OOMKills uint64
	OOMs     uint64
}

func (r *metricsReader) IsCgroupV2() bool {
	return r.CGroupVersion == samplerserverv1alpha1.CGroupV2
}

// GetContainerCPUMetrics reads the current cpu metrics for all cached containers
// It acts as a wrapper around GetLevelCPUMetrics but assumes that the targets are individual containers
func (r *metricsReader) GetContainerCPUMetrics() (cpuMetrics, error) {
	result := cpuMetrics{}
	if err := r.init(); err != nil {
		return result, err
	}
	r.cpuFilesMutex.RLock()
	defer r.cpuFilesMutex.RUnlock()

	knownPods := sets.NewString()
	for container, files := range r.cpuFiles {
		var metrics containerCPUMetrics
		var err error
		if r.IsCgroupV2() {
			metrics, err = r.GetLevelCPUMetricsV2(files)
		} else {
			metrics, err = r.GetLevelCPUMetricsV1(files)
		}
		if err != nil {
			return result, nil
		}
		result[container] = metrics
		knownPods.Insert(container.PodUID)
	}
	r.knownContainersSet.Store(knownPods)

	return result, nil
}

// GetLevelCPUMetricsV1 returns a set of cpu metrics given a specific set of cgroup metric filepaths for cgroupv1
func (r *metricsReader) GetLevelCPUMetricsV1(metricFilepaths map[ContainerMetricType]metricFilepath) (containerCPUMetrics, error) {
	metrics := containerCPUMetrics{}
	for metricType, filepath := range metricFilepaths {
		readTime := r.readTimeFunc(string(filepath))
		b, err := r.readFile(filepath)
		if err != nil {
			// cgroup may have been deleted since we populated the cache
			continue
		}

		switch metricType {
		case CPUUsageMetricType:
			metrics.usage.Time = readTime
			value, err := strconv.ParseUint(strings.TrimSpace(b.String()), 10, 64)
			if err != nil {
				return metrics, err
			}
			metrics.usage.UsageNanoSec = value
		case CPUThrottlingMetricTypeV1:
			scanner := bufio.NewScanner(b)
			for scanner.Scan() {
				fields := strings.Fields(scanner.Text())
				value, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					return metrics, err
				}

				switch fields[0] {
				case "throttled_time":
					metrics.throttling.ThrottledNanoSec = value
				case "nr_periods":
					metrics.throttling.TotalPeriods = value
				case "nr_throttled":
					metrics.throttling.ThrottledPeriods = value
				}
			}
		}
	}

	return metrics, nil
}

// GetLevelCPUMetricsV2 returns a set of cpu metrics given a specific set of cgroup metric filepaths for cgroupv2
func (r *metricsReader) GetLevelCPUMetricsV2(metricFilepaths map[ContainerMetricType]metricFilepath) (containerCPUMetrics, error) {
	metrics := containerCPUMetrics{}
	for metricType, filepath := range metricFilepaths {
		readTime := r.readTimeFunc(string(filepath))
		b, err := r.readFile(filepath)
		if err != nil {
			// cgroup may have been deleted since we populated the cache
			continue
		}

		switch metricType { // only one CPU metric type on v2
		case CPUUsageMetricType:
			metrics.usage.Time = readTime
			scanner := bufio.NewScanner(b)
			for scanner.Scan() {
				fields := strings.Fields(scanner.Text())
				value, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					return metrics, err
				}

				switch fields[0] {
				case "usage_usec":
					metrics.usage.UsageNanoSec = value * 1000 // cgroupv2 values are in microseconds
				case "throttled_usec":
					metrics.throttling.ThrottledNanoSec = value * 1000 // cgroupv2 values are in microseconds
				case "nr_periods":
					metrics.throttling.TotalPeriods = value
				case "nr_throttled":
					metrics.throttling.ThrottledPeriods = value
				}
			}
		}
	}

	return metrics, nil
}

// GetMemoryMetrics reads the current memory metrics for all cached containers
func (r *metricsReader) GetContainerMemoryMetrics() (memoryMetrics, error) {
	result := memoryMetrics{}
	if err := r.init(); err != nil {
		return result, err
	}
	r.memoryFilesMutex.RLock()
	defer r.memoryFilesMutex.RUnlock()

	for container, files := range r.memoryFiles {
		var metrics containerMemoryMetrics
		var err error
		if r.IsCgroupV2() {
			metrics, err = r.GetLevelMemoryMetricsV2(files)
		} else {
			metrics, err = r.GetLevelMemoryMetricsV1(files)
		}
		if err != nil {
			return result, err
		}
		result[container] = metrics
	}

	return result, nil
}

func (r *metricsReader) GetLevelMemoryMetricsV1(metricFilepaths map[ContainerMetricType]metricFilepath) (containerMemoryMetrics, error) {
	metrics := containerMemoryMetrics{}

	for metricType, filepath := range metricFilepaths {
		metrics.Time = r.readTimeFunc(string(filepath)) // this is slightly inaccurate in the case of reading multiple files to gather stats for a single container

		b, err := r.readFile(filepath)
		if err != nil {
			// cgroup may have been deleted since we populated the cache
			continue
		}

		// parse the file contents into a struct
		scanner := bufio.NewScanner(b)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())

			// files that contain only numbers
			if len(fields) == 1 {
				value, err := strconv.ParseUint(fields[0], 10, 64)
				if err != nil {
					return metrics, err
				}

				switch metricType {
				case MemoryOOMMetricType:
					metrics.OOMs = value
				}

			} else if len(fields) == 2 { // files that contain key value pairs
				value, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					return metrics, err
				}

				switch metricType {
				case MemoryUsageMetricType:
					switch fields[0] {
					case "total_rss":
						metrics.RSS = value
					case "total_cache":
						metrics.Cache = value
					}
				case MemoryOOMKillMetricTypeV1:
					switch fields[0] {
					case "oom_kill":
						metrics.OOMKills = value
					}
				}
			} else {
				continue
			}

		}
	}
	return metrics, nil
}

func (r *metricsReader) GetLevelMemoryMetricsV2(metricFilepaths map[ContainerMetricType]metricFilepath) (containerMemoryMetrics, error) {
	metrics := containerMemoryMetrics{}

	for metricType, filepath := range metricFilepaths {
		metrics.Time = r.readTimeFunc(string(filepath)) // this is slightly inaccurate in the case of reading multiple files to gather stats for a single container

		b, err := r.readFile(filepath)
		if err != nil {
			// cgroup may have been deleted since we populated the cache
			continue
		}

		// parse the file contents into a struct
		scanner := bufio.NewScanner(b)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())

			// files that contain only numbers
			if len(fields) == 1 {
				value, err := strconv.ParseUint(fields[0], 10, 64)
				if err != nil {
					return metrics, err
				}

				switch metricType {
				case MemoryCurrentMetricsTypeV2:
					metrics.Current = value
				}

			} else if len(fields) == 2 { // files that contain key value pairs
				value, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					return metrics, err
				}

				switch metricType {
				case MemoryOOMMetricType:
					switch fields[0] {
					case "oom_kill":
						metrics.OOMKills = value
					case "oom":
						metrics.OOMs = value
					}
				}
			} else {
				continue
			}

		}
	}
	return metrics, nil
}

// metricFilepath is the path to a cgroup metrics file
type metricFilepath string

func (r *metricsReader) readFile(path metricFilepath) (*bytes.Buffer, error) {
	b, err := fs.ReadFile(r.fs, string(path))
	return bytes.NewBuffer(b), err
}

// nolint:unused
func (r *metricsReader) initCM() error {
	r.once.Do(func() {
		if r.readTimeFunc == nil {
			r.readTimeFunc = func(s string) time.Time {
				return time.Now()
			}
		}
		if r.fs == nil {
			r.fs = os.DirFS("/")
		}

		if err := r.loadNodeLevelMetricFilePaths(); err != nil {
			r.initErr = err
			return
		}

		// No need to sync the cache of container paths
	})
	return r.initErr
}

func (r *metricsReader) init() error {
	r.once.Do(func() {
		if r.readTimeFunc == nil {
			r.readTimeFunc = func(s string) time.Time {
				return time.Now()
			}
		}
		if r.fs == nil {
			r.fs = os.DirFS("/")
		}

		if err := r.loadNodeLevelMetricFilePaths(); err != nil {
			r.initErr = err
			return
		}

		// start syncing the cache of containers
		r.initErr = r.syncContainerCache()
		go func() {
			ticker := time.NewTicker(time.Duration(*r.ContainerCacheSyncIntervalSeconds * float32(time.Second)))
			defer ticker.Stop()
			for {
				select {
				case <-r.ctx.Done():
					log.Info("stopping sync container cache")
					return
				case <-ticker.C:
					if err := r.syncContainerCache(); err != nil {
						log.Error(err, "failed to sync container cache")
					}
				}
			}
		}()
	})
	return r.initErr
}

func (r *metricsReader) loadNodeLevelMetricFilePaths() error {
	var cpuFilenames map[ContainerMetricType]string
	var memoryFilenames map[ContainerMetricType]string

	if r.IsCgroupV2() {
		cpuFilenames = map[ContainerMetricType]string{
			CPUUsageMetricType: samplerserverv1alpha1.CPUUsageSourceFilenameV2,
		}
	} else {
		cpuFilenames = map[ContainerMetricType]string{
			CPUUsageMetricType: samplerserverv1alpha1.CPUUsageSourceFilenameV1,
		}
	}

	if r.IsCgroupV2() {
		memoryFilenames = map[ContainerMetricType]string{
			MemoryCurrentMetricsTypeV2: samplerserverv1alpha1.MemoryCurrentFilenameV2,
		}
	} else {
		memoryFilenames = map[ContainerMetricType]string{
			MemoryUsageMetricType: samplerserverv1alpha1.MemoryUsageSourceFilenameV1,
		}
	}

	cpuPaths, err := r.getNodeMetricFilePaths(r.NodeAggregationLevelGlobs, r.CPUPaths, cpuFilenames)
	if err != nil {
		return err
	}
	r.nodeCPUFiles = cpuPaths

	memoryPaths, err := r.getNodeMetricFilePaths(r.NodeAggregationLevelGlobs, r.MemoryPaths, memoryFilenames)
	if err != nil {
		return err
	}
	r.nodeMemoryFiles = memoryPaths

	return err
}

type nodeLevelFile struct {
	path string
	name string
}

func (r *metricsReader) getNodeMetricFilePaths(
	levels []samplerserverv1alpha1.NodeAggregationLevel,
	parentPaths []samplerserverv1alpha1.MetricsFilepath,
	metricFilenames map[ContainerMetricType]string,
) (map[samplerserverv1alpha1.NodeAggregationLevel]map[ContainerMetricType]metricFilepath, error) {
	metricFilePaths := map[samplerserverv1alpha1.NodeAggregationLevel]map[ContainerMetricType]metricFilepath{}

	var paths []nodeLevelFile
	for _, level := range levels {
		for _, p := range parentPaths {
			matches, err := fs.Glob(r.fs, path.Join(string(p), string(level)))
			if err != nil {
				return nil, err
			}
			for _, m := range matches {
				paths = append(paths, nodeLevelFile{
					path: m,
					name: strings.TrimLeft(strings.TrimPrefix(m, string(p)), "/"),
				})
			}
		}
	}

	for _, p := range paths {
		contents, err := fs.ReadDir(r.fs, p.path)
		if err != nil {
			continue // ignore directories that don't exist
		}

		for _, dirEntry := range contents {
			for metricType, filename := range metricFilenames {
				if dirEntry.Name() == filename {
					if _, found := metricFilePaths[samplerserverv1alpha1.NodeAggregationLevel(p.name)]; !found {
						metricFilePaths[samplerserverv1alpha1.NodeAggregationLevel(p.name)] = map[ContainerMetricType]metricFilepath{}
					}
					metricFilePaths[samplerserverv1alpha1.NodeAggregationLevel(p.name)][metricType] = metricFilepath(path.Join(p.path, filename))
				}
			}
		}
	}
	return metricFilePaths, nil
}

// syncContainerCache caches all the containers -- the hierachy is cached to avoid having
// to do a filepath.Walk on every sampling
func (r *metricsReader) syncContainerCache() error {

	// get list of containers with cpu metrics
	if err := func() error {
		filenames := map[ContainerMetricType]string{}
		if r.IsCgroupV2() {
			filenames[CPUUsageMetricType] = samplerserverv1alpha1.CPUUsageSourceFilenameV2
		} else {
			filenames[CPUUsageMetricType] = samplerserverv1alpha1.CPUUsageSourceFilenameV1
			filenames[CPUThrottlingMetricTypeV1] = samplerserverv1alpha1.CPUThrottlingSourceFilenameV1
		}

		r.cpuFilesMutex.Lock()
		defer r.cpuFilesMutex.Unlock()
		cpuFiles, err := r.getContainers(r.CPUPaths, filenames)
		if err != nil {
			return err
		}
		r.cpuFiles = cpuFiles
		return nil
	}(); err != nil {
		return err
	}

	// get list of containers with memory metrics.
	if err := func() error {
		filenames := map[ContainerMetricType]string{}
		if r.IsCgroupV2() {
			filenames[MemoryOOMMetricType] = samplerserverv1alpha1.MemoryOOMEventsFilenameV2
			filenames[MemoryCurrentMetricsTypeV2] = samplerserverv1alpha1.MemoryCurrentFilenameV2
		} else {
			filenames[MemoryOOMMetricType] = samplerserverv1alpha1.MemoryOOMFilenameV1
			filenames[MemoryOOMKillMetricTypeV1] = samplerserverv1alpha1.MemoryOOMKillFilenameV1
			filenames[MemoryUsageMetricType] = samplerserverv1alpha1.MemoryUsageSourceFilenameV1
		}
		r.memoryFilesMutex.Lock()
		defer r.memoryFilesMutex.Unlock()
		memoryFiles, err := r.getContainers(r.MemoryPaths, filenames)
		if err != nil {
			return err
		}
		r.memoryFiles = memoryFiles
		return nil
	}(); err != nil {
		return err
	}

	return nil
}

// getContainers returns a map of containers to individual metric filepaths.
func (r *metricsReader) getContainers(
	paths []samplerserverv1alpha1.MetricsFilepath, filenames map[ContainerMetricType]string,
) (map[ContainerKey]map[ContainerMetricType]metricFilepath, error) {
	files := make(map[ContainerKey]map[ContainerMetricType]metricFilepath)
	for _, p := range paths {
		// look for containers
		err := fs.WalkDir(r.fs, string(p), func(path string, d fs.DirEntry, err error) error {
			log := log.WithValues("path", path)
			log.V(1).Info("walking cgroup file", "err", err)
			if err != nil && strings.Contains(err.Error(), "no such file or directory") {
				// just skip the directory, don't consider this an error
				// this can happen if the directory is deleted after we start walking
				log.V(1).Error(err, "failed to walk directory (does not exist)")
				return fs.SkipDir
			}
			if err != nil {
				log.Error(err, "failed to walk directory")
				return err
			}
			// containerID and podUID are in the directory path
			dir := filepath.Dir(path)
			podID := filepath.Base(filepath.Dir(dir))                 // get pod directory name
			containerID := filepath.Base(dir)                         // get the container directory name
			kubeDir := filepath.Base(filepath.Dir(filepath.Dir(dir))) // should be a parent

			// verify this is a is in an allowed parent
			var hasParent bool
			if len(r.ParentDirectories) == 0 {
				hasParent = true
			}
			for _, parent := range r.ParentDirectories {
				if podID == parent {
					log.V(1).Info("skipping cgroup file", "reason", "is parent directory")
					return nil // ignore this directory, it is a parent directory
				}
				if kubeDir == parent {
					hasParent = true
				}
			}
			if !hasParent {
				log.V(1).Info("skipping cgroup file", "reason", "missing parent directory")
				return nil // not in one of the allowed parent directories
			}

			// check if this file matches the filenames we care about
			var filename string
			var metricType ContainerMetricType
			for mt, fn := range filenames {
				if d.Name() == fn {
					metricType = mt
					filename = fn
					break
				}
			}
			if filename == "" {
				// file doesn't match any of the files we care about
				log.V(1).Info("skipping cgroup file", "reason", "no matching metric for filename")
				return nil
			}
			log = log.WithValues("filename", filename, "metricType", metricType)

			// Get the Pod UID for the path
			var hasPodPrefix bool
			for _, prefix := range r.PodPrefix { // strip prefixes
				id := strings.TrimPrefix(podID, prefix)
				if id == "" {
					return nil // contains multiple pods
				}
				if id != podID {
					podID = id
					hasPodPrefix = true
					break
				}
			}
			if !hasPodPrefix {
				log.V(1).Info("skipping cgroup file", "reason", "missing pod prefix", "file", podID, "dir", dir)
				return nil
			}
			for _, suffix := range r.PodSuffix { // strip suffixes
				id := strings.TrimSuffix(podID, suffix)
				if id != podID {
					podID = id
					break
				}
			}

			for old, new := range r.PodReplacements { // do replacements
				podID = strings.ReplaceAll(podID, old, new)
			}

			// Get the container ID for the path
			for _, prefix := range r.ContainerPrefix { // strip prefixes
				id := strings.TrimPrefix(containerID, prefix)
				if id != containerID {
					containerID = id
					break
				}
			}
			for _, suffix := range r.ContainerSuffix { // strip suffixes
				id := strings.TrimSuffix(containerID, suffix)
				if id != containerID {
					containerID = id
					break
				}
			}
			for old, new := range r.ContainerReplacements { // replace characters
				containerID = strings.ReplaceAll(containerID, old, new)
			}
			log = log.WithValues("pod-id", podID, "container-id", containerID)

			// record this container in the cache
			if podID == "" || containerID == "" {
				log.V(1).Info("skipping cgroup file", "reason", "missing pod or container id")
				return nil
			}

			k := ContainerKey{ContainerID: containerID, PodUID: podID}
			if _, found := files[k]; !found {
				files[k] = map[ContainerMetricType]metricFilepath{}
			}
			files[k][metricType] = metricFilepath(path)
			log.V(1).Info("added cgroup file")
			return nil
		})
		if err != nil {
			log.Error(err, "unable to walk cgroup directory")
		}
	}

	return files, nil
}
