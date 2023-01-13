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
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/usage-metrics-collector/pkg/testutil"
)

// Update test data by running with `TESTUTIL_UPDATE_EXPECTED=true`
func TestMetricsPrometheusCollector(t *testing.T) {
	suite := &testutil.IntegrationTestSuite{}
	suite.SetupTestSuite(t)
	defer suite.TearDownTestSuite(t)

	// Create all the test objects
	parser := &testutil.TestCaseParser{ExpectedSuffix: ".txt"}
	parser.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T

		suite.SetupTest(t, *tc)
		defer suite.TearDownTest(t)

		ports, err := testutil.GetFreePorts(1)
		require.NoError(t, err)
		port := fmt.Sprintf("%v", ports[0])

		// Run the instance
		c, buff, cmdErr := suite.RunCommand(t, "go", "run",
			"sigs.k8s.io/usage-metrics-collector/cmd/metrics-prometheus-collector",
			"--kubeconfig", suite.ConfigFilepath,
			"--leader-election",
			"--leader-election-namespace", "default",
			"--http-addr", "127.0.0.1:"+port,
			"--collector-config-filepath", filepath.Join(filepath.Dir(tc.ExpectedFilepath), "input_collector.yaml"),
		)
		defer suite.StopCommand(t, c)

		// Allow a moment for the collector to crash
		select {
		case err := <-cmdErr:
			return err
		case <-time.After(5 * time.Second):
		}

		// Get the metrics
		out := suite.GetMetrics(t, "http://localhost:"+port+"/metrics", buff)

		tc.Actual = filterMetrics(out,
			"go_", "rest_client_", "_latency_seconds", "prometheus_",
			"process_", "promhttp_", "net_", "certwatcher", "kube_usage_version", "leader_elected")
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
