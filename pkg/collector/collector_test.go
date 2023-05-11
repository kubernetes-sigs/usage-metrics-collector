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

package collector

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
)

// TestGetCGroupMetricSource tests the mapping of a cgroup to the metric source
func TestGetCGroupMetricSource(t *testing.T) {
	instance := Collector{
		MetricsPrometheusCollector: &collectorcontrollerv1alpha1.MetricsPrometheusCollector{
			CGroupMetrics: collectorcontrollerv1alpha1.CGroupMetrics{
				Sources: map[string]collectorcontrollerv1alpha1.CGroupMetric{
					"/system.slice": {
						Name: "system",
					},
					"/kubepods": {
						Name:    "kubelet",
						AvgName: "avg_kubelet",
					},
					"/": {
						Name: "node",
					},
				},
				RootSource: collectorcontrollerv1alpha1.CGroupMetric{
					Name: "root",
				},
			},
		}, IsLeaderElected: &atomic.Bool{},
	}
	instance.UtilizationServer.IsReadyResult.Store(true)
	instance.IsLeaderElected.Store(true)
	expected := map[string]collectorcontrollerv1alpha1.CGroupMetric{
		"system.slice/foo/bar": {Name: ""}, // not present
		"system.slice/foo":     {Name: "system"},
		"kubepods/besteffort":  {Name: "kubelet", AvgName: "avg_kubelet"},
		"kubepods/guaranteed":  {Name: "kubelet", AvgName: "avg_kubelet"},
		"system.slice":         {Name: "node"},
		"kubepods":             {Name: "node"},
		"":                     {Name: "root"},
	}
	for k, v := range expected {
		t.Run(k, func(t *testing.T) {
			require.Equal(t, v, instance.getCGroupMetricSource(k))
		})
	}
}

func TestSetLabelsForCGroup(t *testing.T) {
	instance := builtInLabler{}
	expected := map[string]string{
		"/system.slice/foo/bar": "bar",
		"/system.slice/foo":     "foo",
		"/kubepods/besteffort":  "besteffort",
		"/kubepods/guaranteed":  "guaranteed",
		"/system.slice":         "system.slice",
		"/kubepods/":            "kubepods",
		"/":                     "/",
	}
	for k, v := range expected {
		t.Run(k, func(t *testing.T) {
			l := &builtInLabelsValues{}
			m := &api.NodeAggregatedMetrics{AggregationLevel: v}
			instance.SetLabelsForCGroup(l, m)
			require.Equal(t, v, l.CGroup)
		})
	}
}
