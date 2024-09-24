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

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/samplerserverv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
	"sigs.k8s.io/usage-metrics-collector/pkg/testutil"
)

type testConfig struct {
	SampleSize          int                                      `json:"sampleSize" yaml:"sampleSize"`
	MinContainerSamples int                                      `json:"minContainerSamples" yaml:"minContainerSamples"`
	MinNodeSamples      int                                      `json:"minNodeSamples" yaml:"minNodeSamples"`
	Config              samplerserverv1alpha1.MetricsNodeSampler `json:"config" yaml:"config"`
}

func TestMetricsNodeSamplerGRPC(t *testing.T) {
	setupTests(t, func(ports []int) string {
		address := "localhost:" + strconv.Itoa(ports[1])
		// create connection
		conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)

		defer conn.Close()
		metricsClient := api.NewMetricsClient(conn)
		resp, err := metricsClient.ListMetrics(context.Background(), &api.ListMetricsRequest{})
		require.NoError(t, err)

		out, err := protojson.MarshalOptions{
			EmitUnpopulated: true,
		}.Marshal(proto.Message(resp))
		require.NoError(t, err)
		var formatted bytes.Buffer
		require.NoError(t, json.Indent(&formatted, out, "", "\t"))
		return formatted.String()
	})
}

func TestMetricsNodeSamplerRestEndpoint(t *testing.T) {
	setupTests(t, func(ports []int) string {
		endpoint := "http://localhost:" + strconv.Itoa(ports[0]) + "/v1/metrics"
		// Read the metrics from the metrics endpoint
		// nolint: gosec,noctx
		resp, err := http.Get(endpoint)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, resp.Body.Close())
		}()
		out, err := io.ReadAll(resp.Body)

		// Verify the result is OK
		require.NoError(t, err)
		require.Equal(t,
			http.StatusOK, resp.StatusCode, string(out))
		var formatted bytes.Buffer
		require.NoError(t, json.Indent(&formatted, out, "", "\t"))
		return formatted.String()
	})
}

// setupTests runs a container metrics test cases
func setupTests(t *testing.T, f func([]int) string) {
	testutil.TestCaseParser{
		ExpectedSuffix: ".json",
	}.TestDir(t,
		func(tc *testutil.TestCase) error {
			t := tc.T
			// create a new container client
			var v1samples samplesV1
			var v2samples samplesV2
			var cfg testConfig
			tc.UnmarshalInputsStrict(map[string]interface{}{
				"input_samples.yaml":    &v1samples,
				"input_samples_v2.yaml": &v2samples,
				"input_test.yaml":       &cfg,
			})

			// setup the testdata by copying it to a tmp directory
			// this is so the test doesn't modify the testdata directory, and so multiple copies
			// of the test can be run in parallel
			testdataRoot := filepath.Dir(tc.ExpectedFilepath)
			testdataCopy, err := os.MkdirTemp("", "metrics-node-sampler-test")
			require.NoError(t, err)
			defer os.RemoveAll(testdataCopy)
			_ = filepath.WalkDir(testdataRoot, func(path string, d fs.DirEntry, err error) error {
				require.NoError(t, err)
				rel, err := filepath.Rel(testdataRoot, path)
				require.NoError(t, err)
				if rel == "." {
					return nil
				}
				if d.IsDir() {
					require.NoError(t, os.Mkdir(filepath.Join(testdataCopy, rel), 0700))
					return nil
				}
				b, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(testdataCopy, rel), b, 0600))
				return nil
			})

			fs := &fakeFS{
				FS:        os.DirFS(testdataCopy),
				root:      testdataCopy,
				samplesV1: v1samples,
				samplesV2: v2samples,
				index:     make(map[string]int),
				time:      make(map[string]time.Time),
			}

			// get 2 free ports
			ports, err := testutil.GetFreePorts(2)
			require.NoError(t, err)

			var cpuPaths []samplerserverv1alpha1.MetricsFilepath
			var memoryPaths []samplerserverv1alpha1.MetricsFilepath
			if cfg.Config.Reader.CGroupVersion == samplerserverv1alpha1.CGroupV2 {
				cpuPaths = []samplerserverv1alpha1.MetricsFilepath{
					samplerserverv1alpha1.MetricsFilepath(filepath.Join("sys", "fs", "cgroup"))}
				memoryPaths = []samplerserverv1alpha1.MetricsFilepath{
					samplerserverv1alpha1.MetricsFilepath(filepath.Join("sys", "fs", "cgroup"))}
			} else {
				cpuPaths = []samplerserverv1alpha1.MetricsFilepath{
					samplerserverv1alpha1.MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "cpu")),
					samplerserverv1alpha1.MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "cpuacct")),
				}
				memoryPaths = []samplerserverv1alpha1.MetricsFilepath{
					samplerserverv1alpha1.MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "memory"))}
			}

			server := sampler.Server{
				SortResults: true, // so the test results are consistent
				MetricsNodeSampler: samplerserverv1alpha1.MetricsNodeSampler{
					Buffer: samplerserverv1alpha1.Buffer{
						PollsPerMinute: 600, // poll frequently -- the clock is faked so this is just for the ticker
						Size:           cfg.SampleSize,
					},
					Reader: samplerserverv1alpha1.Reader{
						CGroupVersion: cfg.Config.Reader.CGroupVersion,
						CPUPaths:      cpuPaths,
						MemoryPaths:   memoryPaths,
						NodeAggregationLevelGlobs: append(samplerserverv1alpha1.DefaultNodeAggregationLevels,
							samplerserverv1alpha1.NodeAggregationLevel("system.slice/*"),
							samplerserverv1alpha1.NodeAggregationLevel("*"),
							samplerserverv1alpha1.NodeAggregationLevel(""),
						),
						ParentDirectories:  []string{"burstable", "kubepods", "guaranteed"},
						MaxCPUCoresNanoSec: cfg.Config.Reader.MaxCPUCoresNanoSec,
						MinCPUCoresNanoSec: cfg.Config.Reader.MinCPUCoresNanoSec,
						DropFirstValue:     cfg.Config.Reader.DropFirstValue,
					},
					RestPort: ports[0],
					PBPort:   ports[1],
					Address:  "localhost",
				},
				FS:       fs,
				TimeFunc: fs.Time,
			}

			go func() {
				if err := server.Start(context.Background(), func() {}); err != nil {
					fmt.Fprintf(os.Stdout, "%s\n", err)
					os.Exit(1)
				}
			}()

			var result api.ListMetricsResponse
			var missMatchCount int
			require.Eventually(t, func() bool {
				tc.Actual = f(ports)
				// use protojson because regular json library isn't compatible with proto serialization of int64
				require.NoError(t, protojson.Unmarshal([]byte(tc.Actual), &result))
				if len(result.Containers) == 0 {
					return false
				}
				// poll until the server has populated its sample cache
				for _, c := range result.Containers {
					if len(c.CpuCoresNanoSec) < cfg.MinContainerSamples {
						return false
					}
				}
				for _, c := range result.Node.AggregatedMetrics {
					if len(c.CpuCoresNanoSec) < cfg.MinNodeSamples {
						return false
					}
				}

				if tc.Expected != tc.Actual && missMatchCount < 5 {
					// address flakey tests by retrying if the results don't match
					missMatchCount++
					return false
				}

				return true
			}, time.Minute*2, 2*time.Second)

			// TODO: test adding a new pod and getting the metrics within ~seconds with
			// a reason for the new pod

			// stop the server reading the metrics -- if we don't do this it will try to read
			// the tmp directories after they are deleted and create noise in error logs
			server.Stop()

			return nil
		})
}

type samplesV1 struct {
	MemorySamplesV1        map[string][]MemorySampleV1        `yaml:"memorySamples" json:"memorySamples"`
	MemoryOOMKillSamplesV1 map[string][]MemoryOOMKillSampleV1 `yaml:"oomSamples" json:"oomSamples"`
	MemoryOOMSamplesV1     map[string][]MemoryOOMSampleV1     `yaml:"oomKillSamples" json:"oomKillSamples"`
	CPUUsageSamplesV1      map[string][]CPUUsageSampleV1      `yaml:"cpuUsageSamples" json:"cpuUsageSamples"`
	CPUThrottlingSamplesV1 map[string][]CPUThrottlingSampleV1 `yaml:"cpuThrottlingSamples" json:"cpuThrottlingSamples"`
}

type samplesV2 struct {
	MemorySamplesV2    map[string][]MemorySampleV2    `yaml:"memorySamples" json:"memorySamplesV2"`
	MemoryOOMSamplesV2 map[string][]MemoryOOMSampleV2 `yaml:"memorySamples" json:"oomSamplesV2"`
	CPUSamplesV2       map[string][]CPUSampleV2       `yaml:"memorySamples" json:"cpuSamplesV2"`
}

type fakeFS struct {
	samplesV1
	samplesV2
	index map[string]int
	time  map[string]time.Time
	root  string
	fs.FS
}

type MemorySampleV1 struct {
	RSS   int `yaml:"total_rss" json:"total_rss"`
	Cache int `yaml:"total_cache" json:"total_cache"`
}

type MemoryOOMKillSampleV1 struct {
	OOMKill int `yaml:"oom_kill" json:"oom_kill"`
}

type MemoryOOMSampleV1 int

type CPUUsageSampleV1 struct {
	Usage int `yaml:"usage" json:"usage"`
}

type CPUThrottlingSampleV1 struct {
	ThrottledTime    int `yaml:"throttled_time" json:"throttled_time"`
	Periods          int `yaml:"nr_periods" json:"nr_periods"`
	ThrottledPeriods int `yaml:"nr_throttled" json:"nr_throttled"`
}

type MemorySampleV2 struct {
	Current int `yaml:"total_rss" json:"usage"`
}

type MemoryOOMSampleV2 struct {
	OOMKill int `yaml:"oom_kill" json:"oom_kill"`
	OOM     int `yaml:"oom_kill" json:"oom"`
}

type CPUSampleV2 struct {
	Usage            int `yaml:"usage" json:"usage_usec"`
	ThrottledTime    int `yaml:"throttled_time" json:"throttled_usec"`
	Periods          int `yaml:"nr_periods" json:"nr_periods"`
	ThrottledPeriods int `yaml:"nr_throttled" json:"nr_throttled"`
}

func (fakeFS *fakeFS) Time(name string) time.Time {
	t, ok := fakeFS.time[name]
	if !ok {
		fakeFS.time[name] = time.Now()
		return fakeFS.time[name]
	}
	t = t.Add(time.Second)
	fakeFS.time[name] = t
	return t
}

func (fakeFS *fakeFS) Open(name string) (fs.File, error) {
	if val, ok := fakeFS.MemorySamplesV1[name]; ok {
		// update the file value by setting its value
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		newVal := val[index]
		b := fmt.Sprintf("total_cache %d\ntotal_rss %d\n", newVal.Cache, newVal.RSS)
		err := os.WriteFile(filepath.Join(fakeFS.root, name), []byte(b), 0600)
		if err != nil {
			return nil, err
		}
	} else if val, ok := fakeFS.CPUUsageSamplesV1[name]; ok {
		var i int
		// update the file value by incrementing it
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		inc := val[index]

		b, err := os.ReadFile(filepath.Join(fakeFS.root, name))
		if err != nil {
			return nil, err
		}
		i, err = strconv.Atoi(string(b))
		if err != nil {
			return nil, err
		}
		i += inc.Usage
		err = os.WriteFile(filepath.Join(fakeFS.root, name), []byte(fmt.Sprintf("%d", i)), 0600)
		if err != nil {
			return nil, err
		}
	} else if val, ok := fakeFS.CPUThrottlingSamplesV1[name]; ok {
		var throttledTime, periods, periodsThrottled int
		// update the file value by incrementing it
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		inc := val[index]

		b, err := os.ReadFile(filepath.Join(fakeFS.root, name))
		if err != nil {
			return nil, err
		}

		// parse the value out
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return nil, err
			}

			switch fields[0] {
			case "throttled_time":
				throttledTime = int(value)
			case "nr_periods":
				periods = int(value)
			case "nr_throttled":
				periodsThrottled = int(value)
			}
		}

		throttledTime += inc.ThrottledTime
		periods += inc.Periods
		periodsThrottled += inc.ThrottledPeriods

		err = os.WriteFile(filepath.Join(fakeFS.root, name), []byte(fmt.Sprintf(
			"throttled_time %d\nnr_periods %d\nnr_throttled %d", throttledTime, periods, periodsThrottled)), 0600)
		if err != nil {
			return nil, err
		}
	} else if val, ok := fakeFS.MemoryOOMKillSamplesV1[name]; ok {
		// update the file value by setting its value
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		newVal := val[index]

		b, err := os.ReadFile(filepath.Join(fakeFS.root, name))
		if err != nil {
			return nil, err
		}
		old := strings.Split(string(b), " ")[1]
		i, err := strconv.Atoi(old)
		if err != nil {
			return nil, err
		}
		i += newVal.OOMKill
		err = os.WriteFile(filepath.Join(fakeFS.root, name), []byte(fmt.Sprintf("oom_kill %d", i)), 0600)
		if err != nil {
			return nil, err
		}
	} else if val, ok := fakeFS.MemoryOOMSamplesV1[name]; ok {
		// update the file value by setting its value
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		newVal := val[index]
		b, err := os.ReadFile(filepath.Join(fakeFS.root, name))
		if err != nil {
			return nil, err
		}
		i, err := strconv.Atoi(string(b))
		if err != nil {
			return nil, err
		}
		i += int(newVal)
		err = os.WriteFile(filepath.Join(fakeFS.root, name), []byte(fmt.Sprintf("%d", i)), 0600)
		if err != nil {
			return nil, err
		}
	} else if val, ok := fakeFS.MemorySamplesV2[name]; ok {
		// update the file value by setting its value
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		newVal := val[index]
		b := fmt.Sprintf("%d\n", newVal.Current)
		err := os.WriteFile(filepath.Join(fakeFS.root, name), []byte(b), 0600)
		if err != nil {
			return nil, err
		}
	} else if val, ok := fakeFS.CPUSamplesV2[name]; ok {
		var usage, throttledTime, periods, periodsThrottled int
		// update the file value by incrementing it
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		inc := val[index]

		b, err := os.ReadFile(filepath.Join(fakeFS.root, name))
		if err != nil {
			return nil, err
		}
		// parse the value out
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return nil, err
			}

			switch fields[0] {
			case "usage_usec":
				usage = int(value)
			case "throttled_usec":
				throttledTime = int(value)
			case "nr_periods":
				periods = int(value)
			case "nr_throttled":
				periodsThrottled = int(value)
			}
		}

		usage += inc.Usage
		throttledTime += inc.ThrottledTime
		periods += inc.Periods
		periodsThrottled += inc.ThrottledPeriods

		err = os.WriteFile(filepath.Join(fakeFS.root, name), []byte(fmt.Sprintf(
			"usage_usec %d\nthrottled_usec %d\nnr_periods %d\nnr_throttled %d", usage, throttledTime, periods, periodsThrottled)), 0600)
		if err != nil {
			return nil, err
		}
	} else if val, ok := fakeFS.MemoryOOMSamplesV2[name]; ok {
		var oom, oomKill int
		// update the file value by setting its value
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		newVal := val[index]

		b, err := os.ReadFile(filepath.Join(fakeFS.root, name))
		if err != nil {
			return nil, err
		}
		// parse the value out
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return nil, err
			}

			switch fields[0] {
			case "oom":
				oom = int(value)
			case "oom_kill":
				oomKill = int(value)
			}
		}

		oom += newVal.OOM
		oomKill += newVal.OOMKill

		err = os.WriteFile(filepath.Join(fakeFS.root, name), []byte(fmt.Sprintf("oom %d\noom_kill %d", oom, oomKill)), 0600)
		if err != nil {
			return nil, err
		}
	}

	return fakeFS.FS.Open(name)
}
