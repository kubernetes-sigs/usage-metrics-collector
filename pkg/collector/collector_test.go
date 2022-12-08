package collector

import (
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
						Name: "kubelet",
					},
					"/": {
						Name: "node",
					},
				},
				RootSource: collectorcontrollerv1alpha1.CGroupMetric{
					Name: "root",
				},
			},
		},
	}
	expected := map[string]string{
		"system.slice/foo/bar": "", // not present
		"system.slice/foo":     "system",
		"kubepods/besteffort":  "kubelet",
		"kubepods/guaranteed":  "kubelet",
		"system.slice":         "node",
		"kubepods":             "node",
		"":                     "root",
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
