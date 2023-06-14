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
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/testutil"
)

// Update test data by running with `TESTUTIL_UPDATE_EXPECTED=true`
func TestMetricsPrometheusCollector(t *testing.T) {
	// Create all the test objects
	parser := &testutil.TestCaseParser{ExpectedSuffix: ".txt"}
	parser.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T

		suite := &testutil.IntegrationTestSuite{}
		suite.SetupTestSuite(t)
		defer suite.TearDownTestSuite(t)

		suite.SetupTest(t, *tc)
		defer suite.TearDownTest(t)

		ports, err := testutil.GetFreePorts(2)
		require.NoError(t, err)
		port0 := fmt.Sprintf("%v", ports[0])
		port1 := fmt.Sprintf("%v", ports[1])

		// build the instance first so it doesn't have to compile much when we run it
		b, err := exec.Command("go", "build",
			"sigs.k8s.io/usage-metrics-collector/cmd/metrics-prometheus-collector").CombinedOutput()
		require.NoError(t, err, string(b))

		// Run the instance
		c, buff, cmdErr := suite.RunCommand(t, "go", "run",
			"sigs.k8s.io/usage-metrics-collector/cmd/metrics-prometheus-collector",
			"--kubeconfig", suite.ConfigFilepath,
			"--leader-election=false",
			"--internal-http-addr", "localhost:"+port0,
			"--http-addr", "localhost:"+port1,
			"--collector-config-filepath", filepath.Join(filepath.Dir(tc.ExpectedFilepath), "input_collector.yaml"),
		)
		defer suite.StopCommand(t, c)

		// Allow a moment for the collector to crash
		select {
		case err := <-cmdErr:
			return err
		case <-time.After(10 * time.Second):
		}

		// Get the metrics
		out, headers := suite.GetMetrics(t, "http://localhost:"+port1+"/metrics", buff)

		config := &collectorcontrollerv1alpha1.MetricsPrometheusCollector{}
		require.NoError(t, yaml.Unmarshal([]byte(tc.Inputs["input_collector.yaml"]), config))

		if strings.Contains(config.ResponseCacheOptions.RequestHeaders["Accept"], "application/vnd.google.protobuf") {
			reader := bytes.NewBufferString(out)
			results := []string{}
			var ierr error
			for ierr != io.EOF {
				mf := &dto.MetricFamily{}
				if _, ierr := pbutil.ReadDelimited(reader, mf); ierr != nil {
					if ierr == io.EOF {
						break
					}
					require.NoError(t, err)
				}
				b, err := json.Marshal(mf)
				require.NoError(t, err)
				results = append(results, string(b))
			}
			out = strings.Join(results, "\n") + "\n"
		}

		tc.Actual = filterMetrics(out,
			"go_", "rest_client_", "_latency_seconds", "prometheus_",
			"process_", "promhttp_", "net_", "certwatcher",
			"kube_usage_version",
			"kube_usage_leader_elected",
			"kube_usage_collect_cache_time",
			"kube_usage_metric_aggregation_",
		) + "---\n" + headers + "\n"
		return nil
	})
}

func filterMetrics(s string, remove ...string) string {
	var lines []string
	for _, s := range strings.Split(s, "\n") {
		var match bool
		for _, r := range remove {
			if strings.Contains(s, r) {
				match = true
			}
		}
		if !match {
			lines = append(lines, s)
		}
	}
	return strings.Join(lines, "\n")
}
