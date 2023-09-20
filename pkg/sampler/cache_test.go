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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/prometheus/common/model"

	"sigs.k8s.io/usage-metrics-collector/pkg/cadvisor/client/testdata"
)

func Test_populateCadvisorSummary(t *testing.T) {
	type args struct {
		sr              *sampleResult
		dropFirstRecord bool
	}
	now := time.Now()
	testNetworkStats := func(ts time.Time, increase uint64) *cadvisorNetworkStats {
		return &cadvisorNetworkStats{
			Timestamp: ts,
			RxBytes:   1 + increase,
			RxPackets: 2 + increase,
			RxDropped: 3 + increase,
			RxErrors:  4 + increase,
			TxBytes:   11 + increase,
			TxPackets: 12 + increase,
			TxDropped: 13 + increase,
			TxErrors:  14 + increase,
		}
	}
	tests := map[string]struct {
		args args
		want *sampleResult
	}{
		"NilResult": {},
		"EmptyResult": {
			args: args{
				sr: &sampleResult{},
			},
			want: &sampleResult{},
		},
		"EmptyResultValues": {
			args: args{
				sr: &sampleResult{
					values: sampleInstantSlice{},
				},
			},
			want: &sampleResult{
				values: sampleInstantSlice{},
			},
		},
		"SingleValue_WithoutNetworkStats": {
			args: args{
				sr: &sampleResult{
					values: sampleInstantSlice{
						{}, // default instance w/out network metrics.
					},
				},
			},
			want: &sampleResult{
				values: sampleInstantSlice{
					{},
				},
			},
		},
		"SingleValue_WithNetworkStats": {
			args: args{
				sr: &sampleResult{
					values: sampleInstantSlice{
						{
							CAdvisorNetworkStats: testNetworkStats(now, 0),
						},
					},
				},
			},
			want: &sampleResult{
				values: sampleInstantSlice{
					{
						CAdvisorNetworkStats: testNetworkStats(now, 0),
					},
				},
				avg: sampleInstant{
					CAdvisorNetworkStats: testNetworkStats(time.Time{}, 0),
				},
			},
		},
		"MultipleValues_AllWithoutNetworkMetrics": {
			args: args{
				sr: &sampleResult{
					values: sampleInstantSlice{
						{},
						{},
					},
				},
			},
			want: &sampleResult{
				values: sampleInstantSlice{
					{},
					{},
				},
			},
		},
		"MultipleValues_SomeWithoutNetworkMetrics": {
			args: args{
				sr: &sampleResult{
					values: sampleInstantSlice{
						{
							CAdvisorNetworkStats: testNetworkStats(now, 0),
						},
						{},
					},
				},
			},
			want: &sampleResult{
				values: sampleInstantSlice{
					{
						CAdvisorNetworkStats: testNetworkStats(now, 0),
					},
					{},
				},
				avg: sampleInstant{
					CAdvisorNetworkStats: testNetworkStats(time.Time{}, 0),
				},
			},
		},
		"MultipleValues_WithoutSkippingFirstRecord": {
			args: args{
				sr: &sampleResult{
					values: sampleInstantSlice{
						{
							CAdvisorNetworkStats: testNetworkStats(now, 0),
						},
						{
							CAdvisorNetworkStats: testNetworkStats(now, 2),
						},
					},
				},
			},
			want: &sampleResult{
				values: sampleInstantSlice{
					{
						CAdvisorNetworkStats: testNetworkStats(now, 0),
					},
					{
						CAdvisorNetworkStats: testNetworkStats(now, 2),
					},
				},
				avg: sampleInstant{
					CAdvisorNetworkStats: testNetworkStats(time.Time{}, 1),
				},
			},
		},
		"MultipleValues_SkippingFirstRecord": {
			args: args{
				sr: &sampleResult{
					values: sampleInstantSlice{
						{
							CAdvisorNetworkStats: testNetworkStats(now, 0),
						},
						{
							CAdvisorNetworkStats: testNetworkStats(now, 2),
						},
					},
				},
			},
			want: &sampleResult{
				values: sampleInstantSlice{
					{
						CAdvisorNetworkStats: testNetworkStats(now, 0),
					},
					{
						CAdvisorNetworkStats: testNetworkStats(now, 2),
					},
				},
				avg: sampleInstant{
					CAdvisorNetworkStats: testNetworkStats(time.Time{}, 1),
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			populateCadvisorSummary(tt.args.sr, tt.args.dropFirstRecord)
			if diff := cmp.Diff(tt.args.sr, tt.want, cmpopts.IgnoreUnexported(sampleResult{})); diff != "" {
				t.Errorf("populateCadvisorSummary() (-)got,(+)want: %s", diff)
			}
			if tt.args.sr == nil {
				return
			}
			if diff := cmp.Diff(tt.args.sr.values, tt.want.values); diff != "" {
				t.Errorf("populateCadvisorSummary() values (-)got,(+)want: %s", diff)
			}
			if diff := cmp.Diff(tt.args.sr.avg, tt.want.avg); diff != "" {
				t.Errorf("populateCadvisorSummary() avg (-)got,(+)want: %s", diff)
			}
		})
	}
}

func Test_containerKeyFromSampleMetric(t *testing.T) {
	type args struct {
		metric model.Metric
	}
	tests := map[string]struct {
		args args
		want ContainerKey
	}{
		"Empty": {
			args: args{
				metric: testdata.TestMetric("container_network_receive_bytes_total", "", "", "", "cni0", "", "", ""),
			},
		},
		"/": {
			args: args{
				metric: testdata.TestMetric("container_network_receive_bytes_total", "", "/", "", "cni0", "", "", ""),
			},
		},
		"Value": {
			args: args{
				metric: testdata.TestMetric("container_network_receive_bytes_total", "", "/kubepods/burstable/test-uid/container-id", "pause-amd64:3.1", "eth0", "test-name", "test-namespace", "test-pod-zvgxh"),
			},
			want: ContainerKey{
				ContainerID:   "test-name",
				PodUID:        "test-uid",
				NamespaceName: "test-namespace",
				PodName:       "test-pod-zvgxh",
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := containerKeyFromSampleMetric(tt.args.metric)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("containerKeyFromSampleMetric() got(-),want(+): %s", diff)
			}
		})
	}
}

func Test_sampleInstantsFromMetricVector(t *testing.T) {
	type args struct {
		vector     model.Vector
		containers map[ContainerKey]sampleInstant
	}
	// test "constants"
	testMetric := func(name model.LabelValue) model.Metric {
		return testdata.TestMetric(name, "", "/kubepods/burstable/test-uid/container-id", "pause-amd64:3.1", "eth0", "test-name", "test-namespace", "test-pod-zvgxh")
	}
	testContainerKey := ContainerKey{
		ContainerID:   "test-name",
		PodUID:        "test-uid",
		NamespaceName: "test-namespace",
		PodName:       "test-pod-zvgxh",
	}
	tests := map[string]struct {
		args args
		want map[ContainerKey]sampleInstant
	}{
		"EmptyVector_EmptyResults": {},
		"EmptyVector": {
			args: args{
				containers: map[ContainerKey]sampleInstant{
					ContainerKey{ContainerID: "foo"}: {},
				},
			},
			want: map[ContainerKey]sampleInstant{
				ContainerKey{ContainerID: "foo"}: {},
			},
		},
		"SkipPodWithoutUID": {
			args: args{
				containers: map[ContainerKey]sampleInstant{
					ContainerKey{ContainerID: "foo"}: {},
				},
				vector: testdata.SingleNonPodMetricVector,
			},
			want: map[ContainerKey]sampleInstant{
				ContainerKey{ContainerID: "foo"}: {},
			},
		},
		"SkipAddWithZeroMetricValue": {
			args: args{
				containers: map[ContainerKey]sampleInstant{
					ContainerKey{ContainerID: "foo"}: {},
				},
				vector: testdata.SinglePodMetricVectorWithZeroValue,
			},
			want: map[ContainerKey]sampleInstant{
				ContainerKey{ContainerID: "foo"}: {},
			},
		},
		"UpdateMetric_WithLaterTimestamp": {
			args: args{
				containers: map[ContainerKey]sampleInstant{
					testContainerKey: {
						Time: time.UnixMilli(1695160365070), // <-- earlier timestamp will be replaced.
						CAdvisorNetworkStats: &cadvisorNetworkStats{
							RxBytes: 1,
						},
					},
				},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_receive_bytes_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				testContainerKey: {
					Time: time.UnixMilli(1695160365071),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						RxBytes: 2,
					},
				},
			},
		},
		"UpdateMetric_WithEarlierTimestamp": {
			args: args{
				containers: map[ContainerKey]sampleInstant{
					testContainerKey: {
						Time: time.UnixMilli(1695160365072), // <-- later timestamp will be retained.
						CAdvisorNetworkStats: &cadvisorNetworkStats{
							RxBytes: 1,
						},
					},
				},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_receive_bytes_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				testContainerKey: {
					Time: time.UnixMilli(1695160365072),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						RxBytes: 2,
					},
				},
			},
		},
		"Add_RxBytes": {
			args: args{
				containers: map[ContainerKey]sampleInstant{
					ContainerKey{ContainerID: "foo"}: {},
				},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_receive_bytes_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				ContainerKey{ContainerID: "foo"}: {},
				testContainerKey: {
					Time: time.UnixMilli(1695160365071),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						RxBytes: 1,
					},
				},
			},
		},
		"Add_RxPackets": {
			args: args{
				containers: map[ContainerKey]sampleInstant{},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_receive_packets_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				testContainerKey: {
					Time: time.UnixMilli(1695160365071),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						RxPackets: 1,
					},
				},
			},
		},
		"Add_RxDropped": {
			args: args{
				containers: map[ContainerKey]sampleInstant{},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_receive_packets_dropped_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				testContainerKey: {
					Time: time.UnixMilli(1695160365071),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						RxDropped: 1,
					},
				},
			},
		},
		"Add_RxErrors": {
			args: args{
				containers: map[ContainerKey]sampleInstant{},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_receive_errors_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				testContainerKey: {
					Time: time.UnixMilli(1695160365071),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						RxErrors: 1,
					},
				},
			},
		},

		"Add_TxBytes": {
			args: args{
				containers: map[ContainerKey]sampleInstant{
					ContainerKey{ContainerID: "foo"}: {},
				},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_transmit_bytes_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				ContainerKey{ContainerID: "foo"}: {},
				testContainerKey: {
					Time: time.UnixMilli(1695160365071),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						TxBytes: 1,
					},
				},
			},
		},
		"Add_TxPackets": {
			args: args{
				containers: map[ContainerKey]sampleInstant{},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_transmit_packets_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				testContainerKey: {
					Time: time.UnixMilli(1695160365071),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						TxPackets: 1,
					},
				},
			},
		},
		"Add_TxDropped": {
			args: args{
				containers: map[ContainerKey]sampleInstant{},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_transmit_packets_dropped_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				testContainerKey: {
					Time: time.UnixMilli(1695160365071),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						TxDropped: 1,
					},
				},
			},
		},
		"Add_TxErrors": {
			args: args{
				containers: map[ContainerKey]sampleInstant{},
				vector: model.Vector{
					{
						Metric:    testMetric("container_network_transmit_errors_total"),
						Value:     1,
						Timestamp: 1695160365071,
					},
				},
			},
			want: map[ContainerKey]sampleInstant{
				testContainerKey: {
					Time: time.UnixMilli(1695160365071),
					CAdvisorNetworkStats: &cadvisorNetworkStats{
						TxErrors: 1,
					},
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			sampleInstantsFromMetricVector(tt.args.vector, tt.args.containers)
			if diff := cmp.Diff(tt.args.containers, tt.want); diff != "" {
				t.Errorf("sampleInstantsFromMetricVector, got(-),want(+): %s", diff)
			}
		})
	}
}
