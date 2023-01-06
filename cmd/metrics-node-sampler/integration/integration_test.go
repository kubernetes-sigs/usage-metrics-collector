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

const (
	sampleSize = 5
)

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
			var samples samples
			tc.UnmarshalInputsStrict(map[string]interface{}{"input_samples.yaml": &samples})

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
				FS:      os.DirFS(testdataCopy),
				root:    testdataCopy,
				samples: samples,
				index:   make(map[string]int),
				time:    make(map[string]time.Time),
			}

			// get 2 free ports
			ports, err := testutil.GetFreePorts(2)
			require.NoError(t, err)

			server := sampler.Server{
				SortResults: true, // so the test results are consistent
				MetricsNodeSampler: samplerserverv1alpha1.MetricsNodeSampler{
					Buffer: samplerserverv1alpha1.Buffer{
						PollsPerMinute: 600, // poll frequently -- the clock is faked so this is just for the ticker
						Size:           5,   // store exactly 5 samples -- this must match the numer of samples in the testdata
					},
					Reader: samplerserverv1alpha1.Reader{
						CPUPaths: []samplerserverv1alpha1.MetricsFilepath{
							samplerserverv1alpha1.MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "cpu")),
							samplerserverv1alpha1.MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "cpuacct")),
						},
						MemoryPaths: []samplerserverv1alpha1.MetricsFilepath{
							samplerserverv1alpha1.MetricsFilepath(filepath.Join("sys", "fs", "cgroup", "memory"))},
						NodeAggregationLevelGlobs: append(samplerserverv1alpha1.DefaultNodeAggregationLevels,
							samplerserverv1alpha1.NodeAggregationLevel("system.slice/*"),
							samplerserverv1alpha1.NodeAggregationLevel("*"),
							samplerserverv1alpha1.NodeAggregationLevel(""),
						),
						ParentDirectories: []string{"burstable", "kubepods", "guaranteed"},
					},
					RestPort: ports[0],
					PBPort:   ports[1],
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
			require.Eventually(t, func() bool {
				tc.Actual = f(ports)
				// use protojson because regular json library isn't compatible with proto serialization of int64
				require.NoError(t, protojson.Unmarshal([]byte(tc.Actual), &result))
				if len(result.Containers) == 0 {
					return false
				}
				// poll until the server has populated its sample cache
				return len(result.Containers[0].CpuCoresNanoSec) >= sampleSize

			}, time.Second*10, time.Second)

			// TODO: test adding a new pod and getting the metrics within ~seconds with
			// a reason for the new pod

			// stop the server reading the metrics -- if we don't do this it will try to read
			// the tmp directories after they are deleted and create noise in error logs
			server.Stop()

			return nil
		})
}

type samples struct {
	MemorySamples        map[string][]MemorySample        `yaml:"memorySamples" json:"memorySamples"`
	MemoryOOMKillSamples map[string][]MemoryOOMKillSample `yaml:"oomSamples" json:"oomSamples"`
	MemoryOOMSamples     map[string][]MemoryOOMSample     `yaml:"oomKillSamples" json:"oomKillSamples"`
	CPUUsageSamples      map[string][]CPUUsageSample      `yaml:"cpuUsageSamples" json:"cpuUsageSamples"`
	CPUThrottlingSamples map[string][]CPUThrottlingSample `yaml:"cpuThrottlingSamples" json:"cpuThrottlingSamples"`
}

type fakeFS struct {
	samples
	index map[string]int
	time  map[string]time.Time
	root  string
	fs.FS
}

type MemorySample struct {
	RSS   int `yaml:"rss" json:"rss"`
	Cache int `yaml:"cache" json:"cache"`
}

type MemoryOOMKillSample struct {
	OOMKill int `yaml:"oom_kill" json:"oom_kill"`
}

type MemoryOOMSample int

type CPUUsageSample struct {
	Usage int `yaml:"usage" json:"usage"`
}

type CPUThrottlingSample struct {
	ThrottledTime    int `yaml:"throttled_time" json:"throttled_time"`
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
	if val, ok := fakeFS.MemorySamples[name]; ok {
		// update the file value by setting its value
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		newVal := val[index]
		b := fmt.Sprintf("cache %d\nrss %d\n", newVal.Cache, newVal.RSS)
		err := os.WriteFile(filepath.Join(fakeFS.root, name), []byte(b), 0600)
		if err != nil {
			return nil, err
		}
	} else if val, ok := fakeFS.CPUUsageSamples[name]; ok {
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
	} else if val, ok := fakeFS.CPUThrottlingSamples[name]; ok {
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
	} else if val, ok := fakeFS.MemoryOOMKillSamples[name]; ok {
		// update the file value by setting its value
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		newVal := val[index]
		b := fmt.Sprintf("oom_kill %d\n", newVal.OOMKill)
		err := os.WriteFile(filepath.Join(fakeFS.root, name), []byte(b), 0600)
		if err != nil {
			return nil, err
		}
	} else if val, ok := fakeFS.MemoryOOMSamples[name]; ok {
		// update the file value by setting its value
		index := fakeFS.index[name] % len(val)
		fakeFS.index[name] = (index + 1)
		newVal := val[index]
		b := fmt.Sprintf("%d", newVal)
		err := os.WriteFile(filepath.Join(fakeFS.root, name), []byte(b), 0600)
		if err != nil {
			return nil, err
		}
	}
	return fakeFS.FS.Open(name)
}
