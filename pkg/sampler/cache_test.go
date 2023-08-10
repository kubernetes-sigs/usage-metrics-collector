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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/cadvisor/client"
	cadvisorv1 "github.com/google/cadvisor/info/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"

	"sigs.k8s.io/usage-metrics-collector/pkg/cadvisor"
)

func equateSampleResult(a, b sampleResult) bool {
	return cmp.Equal(a.values, b.values) &&
		cmp.Equal(a.avg, b.avg) &&
		a.totalOOM == b.totalOOM &&
		a.totalOOMKill == b.totalOOMKill
}

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

func cadvisorTestClient(path string, expectedPostObj *cadvisorv1.ContainerInfoRequest, replyObj interface{}, t *testing.T) (*client.Client, *httptest.Server, error) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			if expectedPostObj != nil {
				expectedPostObjEmpty := new(cadvisorv1.ContainerInfoRequest)
				decoder := json.NewDecoder(r.Body)
				if err := decoder.Decode(expectedPostObjEmpty); err != nil {
					t.Errorf("Received invalid object: %v", err)
				}
				if expectedPostObj.NumStats != expectedPostObjEmpty.NumStats ||
					expectedPostObj.Start.Unix() != expectedPostObjEmpty.Start.Unix() ||
					expectedPostObj.End.Unix() != expectedPostObjEmpty.End.Unix() {
					t.Errorf("Received unexpected object: %+v, expected: %+v", expectedPostObjEmpty, expectedPostObj)
				}
			}
			encoder := json.NewEncoder(w)
			err := encoder.Encode(replyObj)
			assert.NoError(t, err)
		} else {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "Page not found: %s", path)
		}
	}))
	client, err := client.NewClient(ts.URL)
	if err != nil {
		ts.Close()
		return nil, nil, err
	}
	return client, ts, err
}

func equateSampleInstants(a, b sampleInstants) bool {
	return cmp.Equal(a.containers, b.containers) &&
		cmp.Equal(a.node, b.node)
}

var sampleInstantsComparer = cmp.Comparer(equateSampleInstants)

func Test_fetchCAdvisorSample(t *testing.T) {
	type testClientBuilder func() (*client.Client, *httptest.Server)
	type args struct {
		samples *sampleInstants
	}
	now := time.Now()
	tests := map[string]struct {
		builder     testClientBuilder
		args        args
		wantErr     bool
		wantSamples *sampleInstants
	}{
		"NilClient": {
			builder: func() (*client.Client, *httptest.Server) {
				return nil, nil
			},
		},
		"NilSampleInstant": {
			builder: func() (*client.Client, *httptest.Server) {
				client, server, _ := cadvisorTestClient("", nil, nil, t)
				return client, server
			},
		},
		"FailureToRetrieveContainersInfo": {
			builder: func() (*client.Client, *httptest.Server) {
				client, server, _ := cadvisorTestClient("", nil, nil, t)
				return client, server
			},
			args: args{
				samples: &sampleInstants{},
			},
			wantErr:     true,
			wantSamples: &sampleInstants{},
		},
		"NoContainersInfo": {
			builder: func() (*client.Client, *httptest.Server) {
				client, server, _ := cadvisorTestClient("/api/v1.3/subcontainers/kubelet", &cadvisorv1.ContainerInfoRequest{}, nil, t)
				return client, server
			},
			args: args{
				samples: &sampleInstants{},
			},
			wantSamples: &sampleInstants{},
		},
		"ContainersInfo": {
			builder: func() (*client.Client, *httptest.Server) {
				client, server, _ := cadvisorTestClient("/api/v1.3/subcontainers/kubelet", &cadvisorv1.ContainerInfoRequest{}, []cadvisorv1.ContainerInfo{
					{}, // <-- skipped, doesn't have pod identifier (pod.UID)
					{
						Spec: cadvisorv1.ContainerSpec{
							Labels: map[string]string{
								cadvisor.ContainerLabelPodUID: "not-found",
							},
						},
					}, // <-- skipped, not found in provided samples.
					{
						Spec: cadvisorv1.ContainerSpec{
							Labels: map[string]string{
								cadvisor.ContainerLabelPodUID: "test-container-1",
							},
						},
					}, // <-- skipped, doesn't have container stats.
					{
						Spec: cadvisorv1.ContainerSpec{
							Labels: map[string]string{
								cadvisor.ContainerLabelPodUID: "test-container-2",
							},
						},
						Stats: []*cadvisorv1.ContainerStats{
							{
								Timestamp: now, // <-- only first stats value is kept.
							},
							{
								Timestamp: now.Add(-time.Minute),
							},
						},
					}, // <-- kept, has containers stats.
				}, t)
				return client, server
			},
			args: args{
				samples: &sampleInstants{
					containers: map[ContainerKey]sampleInstant{
						{PodUID: "test-container-1"}: {},
						{PodUID: "test-container-2"}: {},
					},
				},
			},
			wantSamples: &sampleInstants{
				containers: map[ContainerKey]sampleInstant{
					{PodUID: "test-container-1"}: {},
					{PodUID: "test-container-2"}: {
						CAdvisorNetworkStats: &cadvisorNetworkStats{
							Timestamp: now,
						},
					},
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client, server := tt.builder()
			defer func() {
				if server != nil {
					server.Close()
				}
			}()

			if err := fetchCAdvisorSample(client, tt.args.samples); (err != nil) != tt.wantErr {
				t.Errorf("fetchCAdvisorSample() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.args.samples, tt.wantSamples, sampleInstantsComparer); diff != "" {
				t.Errorf("fetchCAdvisorSample() got(-),want(+): %s", diff)
			}
		})
	}
}
