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

package testdata

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/model"
)

func TestMetric(_name, container, id, image, _interface, name, namespace, pod model.LabelValue) model.Metric {
	return model.Metric{
		model.MetricNameLabel: _name,
		"container":           container,
		"id":                  id,
		"image":               image,
		"interface":           _interface,
		"name":                name,
		"namespace":           namespace,
		"pod":                 pod,
	}
}

const (
	SingleMetricString = `# HELP container_network_receive_bytes_total Cumulative count of bytes received
# TYPE container_network_receive_bytes_total counter
container_network_receive_bytes_total{container="",id="/",image="",interface="cni0",name="",namespace="",pod=""} 1.40827091146e+11 1695160365071
`

	MultipleMetricsString = `# HELP container_network_receive_bytes_total Cumulative count of bytes received
# TYPE container_network_receive_bytes_total counter
container_network_receive_bytes_total{container="",id="/",image="",interface="cni0",name="",namespace="",pod=""} 1.40827091146e+11 1695160365071
container_network_receive_bytes_total{container="",id="/",image="",interface="dummy0",name="",namespace="",pod=""} 0 1695160365071
container_network_receive_bytes_total{container="",id="/",image="",interface="enp24s0",name="",namespace="",pod=""} 5.19363e+07 1695160365071
container_network_receive_bytes_total{container="",id="/kubepods/burstable/podUIDValue/containerIDValue",image="pause-amd64:3.1",interface="eth0",name="containerID",namespace="test-namespace",pod="test-pod-zvgxh"} 1.2898191e+07 1695160356349
`
)

var (
	// SingleNonPodMetricVector that matches SingleMetricString.
	SingleNonPodMetricVector = model.Vector{
		&model.Sample{
			Metric:    TestMetric("container_network_receive_bytes_total", "", "/", "", "cni0", "", "", ""),
			Value:     1.40827091146e+11,
			Timestamp: 1695160365071,
		},
	}
	// SinglePodMetricVector that matchers pod container from MultipleMetricsString.
	SinglePodMetricVector = model.Vector{
		&model.Sample{
			Metric:    TestMetric("container_network_receive_bytes_total", "", "/kubepods/burstable/podUIDValue/containerIDValue", "pause-amd64:3.1", "eth0", "containerID", "test-namespace", "test-pod-zvgxh"),
			Value:     1.2898191e+07,
			Timestamp: 1695160356349,
		},
	}
	// SinglePodMetricVectorWithZeroValue same as above but with 0 value.
	SinglePodMetricVectorWithZeroValue = model.Vector{
		&model.Sample{
			Metric:    TestMetric("container_network_receive_bytes_total", "", "/kubepods/burstable/podUIDValue/containerIDValue", "pause-amd64:3.1", "eth0", "containerID", "test-namespace", "test-pod-zvgxh"),
			Value:     0,
			Timestamp: 1695160356349,
		},
	}

	// MultipleMetricsVector that matches MultipleMetricsString.
	MultipleMetricsVector = model.Vector{
		SingleNonPodMetricVector[0],
		&model.Sample{
			Metric:    TestMetric("container_network_receive_bytes_total", "", "/", "", "dummy0", "", "", ""),
			Value:     0,
			Timestamp: 1695160365071,
		},
		&model.Sample{
			Metric:    TestMetric("container_network_receive_bytes_total", "", "/", "", "enp24s0", "", "", ""),
			Value:     5.19363e+07,
			Timestamp: 1695160365071,
		},
		SinglePodMetricVector[0],
	}
)

func DiffVectors(t *testing.T, unitName string, got, want model.Vector) {
	t.Helper()
	if diff := cmp.Diff([]*model.Sample(got), []*model.Sample(want)); diff != "" {
		t.Errorf("%s got(-),want(+): %s", unitName, diff)
		for i := range got {
			gm, wm := got[i], want[i]
			if gm.Value != wm.Value {
				t.Errorf("%s index(%d).Value got(%v), want(%v)", unitName, i, gm.Value, wm.Value)
				continue
			}
			if gm.Timestamp != wm.Timestamp {
				t.Errorf("%s index(%d).Timestamp got(%v), want(%v)", unitName, i, gm.Timestamp, wm.Timestamp)
				continue
			}
			if diff := cmp.Diff(map[model.LabelName]model.LabelValue(gm.Metric), map[model.LabelName]model.LabelValue(wm.Metric)); diff != "" {
				t.Errorf("%s index(%d).Metric got(-),want(+): %s", unitName, i, diff)
			}
		}
	}
}
