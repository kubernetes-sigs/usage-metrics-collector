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
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil/promlint"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	sigsyaml "sigs.k8s.io/yaml"

	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	quotamanagementv1alpha1 "sigs.k8s.io/usage-metrics-collector/pkg/api/quotamanagementv1alpha1"
	collectorapi "sigs.k8s.io/usage-metrics-collector/pkg/collector/api"
	"sigs.k8s.io/usage-metrics-collector/pkg/collector/utilization"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler"
	samplerapi "sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
	"sigs.k8s.io/usage-metrics-collector/pkg/scheme"
	"sigs.k8s.io/usage-metrics-collector/pkg/testutil"
)

func init() {
	// setup logging for tests
	ctrl.SetLogger(zap.New(zap.WriteTo(os.Stderr)))
	os.Setenv(utilization.FilterExpiredMetricsEnvName, "FALSE")
	now = func() time.Time {
		t := time.Time{}
		_ = t.UnmarshalText([]byte("2022-03-23T01:00:30Z"))
		return t
	}
}

// collectPhysicalCores is used to test collector function overriding, it publishes a new
// metric for physical_cpu_cores corresponding to half of virtual cpu cores.
func collectPhysicalCores(c *Collector, o *CapacityObjects, ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) error {
	name := MetricName{
		Source:        "ext_physical",
		ResourceAlias: collectorcontrollerv1alpha1.ResourceAliasCPU,
		Resource:      collectorcontrollerv1alpha1.ResourceCPU,
		SourceType:    collectorcontrollerv1alpha1.QuotaType,
	}

	metrics := map[MetricName]*Metric{}

	for i := range o.Quotas.Items {
		q := &o.Quotas.Items[i]

		// get the labels for this quota
		key := ResourceQuotaDescriptorKey{
			Name:      q.Name,
			Namespace: q.Namespace,
		}
		l := LabelsValues{}
		c.Labeler.SetLabelsForQuota(&l, q, o.RQDsByRQDKey[key], o.NamespacesByName[q.Namespace])
		if l.BuiltIn.PriorityClass == "" {
			continue
		}

		values := c.Reader.GetValuesForQuota(q, o.RQDsByRQDKey[key], c.BuiltIn.EnableResourceQuotaDescriptor)
		requests, ok := values[collectorcontrollerv1alpha1.QuotaRequestsHardSource]
		if !ok {
			continue
		}

		cpu := requests.ResourceList.Cpu()
		cpuValue, ok := cpu.AsInt64()
		if !ok {
			continue
		}
		cpu.Set(cpuValue / 2)

		// initialize the metric
		m, ok := metrics[name]
		if !ok {
			m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
			metrics[name] = m
		}

		m.Values[l] = []resource.Quantity{*cpu}
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.QuotaType) {
		c.AggregateAndCollect(a, metrics, ch, sCh)
	}
	return nil
}

// Update test data by running with `TESTUTIL_UPDATE_EXPECTED=true`
func TestCollector(t *testing.T) {
	parser := testutil.TestCaseParser{
		Subdir:         "collector",
		ExpectedSuffix: ".txt",
	}

	parser.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T

		if name := t.Name(); name != "TestCollector/collector/items" {
			t.SkipNow()
		}

		// get the client initialized with the test data
		c, err := tc.GetFakeClient(scheme.Scheme)
		require.NoError(t, err, "unable to create Kubernetes client")

		// filter metric names if an allow list exists
		var ignoreMetricNames []string
		var spec collectorcontrollerv1alpha1.MetricsPrometheusCollector

		tc.UnmarshalInputsStrict(map[string]interface{}{"input_collector_spec.yaml": &spec})
		spec.SideCarConfigDirectoryPaths = []string{filepath.Dir(tc.ExpectedFilepath)}

		if tc.Inputs["input_ignore_metric_names.txt"] != "" {
			ignoreMetricNames = strings.Split(tc.Inputs["input_ignore_metric_names.txt"], "\n")
		}

		// Read env.yaml, set environment variables accordingly, and revert changes later.
		if _, ok := tc.Inputs["input_env.yaml"]; ok {
			var env struct {
				Env []struct {
					Name  string `yaml:"name"`
					Value string `yaml:"value"`
				} `yaml:"env"`
			}
			tc.UnmarshalInputsStrict(map[string]interface{}{"input_env.yaml": &env})

			oldEnv := make(map[string]string)
			for _, e := range env.Env {
				oldEnv[e.Name] = os.Getenv(e.Name)
				os.Setenv(e.Name, e.Value)
			}

			defer func() {
				for _, e := range env.Env {
					os.Setenv(e.Name, oldEnv[e.Name])
				}
			}()
		}

		// This single metric exists as a package var due to it existing outside the normal collection flow.
		clusterScopedListResultMetric.Reset()

		instance, err := NewCollector(context.Background(), c, &spec)
		require.NoError(t, err)
		tc.UnmarshalInputsStrict(map[string]interface{}{"input_usage.yaml": &instance.UtilizationServer})

		if os.Getenv("TEST_COLLECTOR_FUNCS_OVERRIDE") == "true" {
			fns := CollectorFuncs()
			fns["collect_physical_cores"] = collectPhysicalCores
			SetCollectorFuncs(fns)

			defer func() {
				delete(fns, "collect_physical_cores")
				SetCollectorFuncs(fns)
			}()
		}

		reg := prometheus.NewPedanticRegistry()
		require.NoError(t, reg.Register(instance))

		// collect the metrics
		got, err := reg.Gather()
		require.NoError(t, err)

		// lint for errors
		problem, err := promlint.NewWithMetricFamilies(got).Lint()
		require.Empty(t, problem)
		require.NoError(t, err)

		// set actual results for verification
		got = filterMetrics(got, ignoreMetricNames)
		b := bytes.Buffer{}
		for _, a := range got {
			_, err := expfmt.MetricFamilyToText(&b, a)
			require.NoError(t, err)
		}
		tc.Actual = b.String()

		return nil
	})
}

// Update test data by running with `TESTUTIL_UPDATE_EXPECTED=true`
func TestCollectorOverride(t *testing.T) {
	// Test override functionality
	LabelOverrides = append(LabelOverrides, LabelOverriderFn(func(m map[string]string) map[string]string {
		o := map[string]string{}
		if v, ok := m["exported_namespace"]; ok {
			o["exported_namespace"] = v + "-overridden"
		}
		return o
	}))
	defer func() {
		LabelOverrides = nil
	}()

	parser := testutil.TestCaseParser{
		Subdir:         "collector-override",
		ExpectedSuffix: ".txt",
	}
	parser.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T

		// get the client initialized with the test data
		c, err := tc.GetFakeClient(scheme.Scheme)
		require.NoError(t, err, "unable to create Kubernetes client")

		// filter metric names if an allow list exists
		var ignoreMetricNames []string
		var spec collectorcontrollerv1alpha1.MetricsPrometheusCollector

		tc.UnmarshalInputsStrict(map[string]interface{}{"input_collector_spec.yaml": &spec})
		if tc.Inputs["input_ignore_metric_names.txt"] != "" {
			ignoreMetricNames = strings.Split(tc.Inputs["input_ignore_metric_names.txt"], "\n")
		}

		instance, err := NewCollector(context.Background(), c, &spec)
		require.NoError(t, err)
		tc.UnmarshalInputsStrict(map[string]interface{}{"input_usage.yaml": &instance.UtilizationServer})

		reg := prometheus.NewPedanticRegistry()
		require.NoError(t, reg.Register(instance))

		// collect the metrics
		got, err := reg.Gather()
		require.NoError(t, err)

		// lint for errors
		problem, err := promlint.NewWithMetricFamilies(got).Lint()
		require.Empty(t, problem)
		require.NoError(t, err)

		// set actual results for verification
		got = filterMetrics(got, ignoreMetricNames)
		b := bytes.Buffer{}
		for _, a := range got {
			_, err := expfmt.MetricFamilyToText(&b, a)
			require.NoError(t, err)
		}
		tc.Actual = b.String()

		return nil
	})
}

func TestCollectorSave(t *testing.T) {
	for _, saveJSON := range []bool{true, false} {
		for _, saveProto := range []bool{true, false} {
			if !saveJSON && !saveProto {
				continue
			}
			testSave(t, saveJSON, saveProto)
		}
	}
}

func testSave(t *testing.T, saveJSON, saveProto bool) {
	parser := testutil.TestCaseParser{Subdir: "collector-save",
		NameSuffix: fmt.Sprintf("/json-%v-proto-%v", saveJSON, saveProto)}
	if saveJSON {
		parser.ExpectedSuffix = ".json"
	} else {
		parser.ExpectedSuffix = ".txt"
	}
	parser.TestDir(t, func(tc *testutil.TestCase) error {
		if saveJSON && !strings.Contains(tc.Name, "aggregatedsamples") {
			// we don't do JSON saves for non-aggregated samples
			return nil
		}

		os.Setenv("CLUSTER_NAME", "unit-test")
		t := tc.T

		// get the client initialized with the test data
		c, err := tc.GetFakeClient(scheme.Scheme)
		require.NoError(t, err, "unable to create Kubernetes client")

		// create the directory to store the samples in
		tmpDir, err := os.MkdirTemp("", "metrics-prometheus-collector-save")
		require.NoError(t, err)
		if l, ok := os.LookupEnv("SAVE_COLLECTOR_SAMPLES"); !ok || strings.ToLower(l) != "true" {
			defer os.RemoveAll(tmpDir)
		}

		// create the collector
		var spec collectorcontrollerv1alpha1.MetricsPrometheusCollector
		tc.UnmarshalInputsStrict(map[string]interface{}{"input_collector_spec.yaml": &spec})
		spec.SaveSamplesLocally.DirectoryPath = tmpDir  // override the directory for saving samples
		spec.SaveSamplesLocally.ExcludeTimestamp = true // don't include the timestamp in the tests
		spec.SaveSamplesLocally.SortValues = true       // sort that values so the output is stable
		spec.SaveSamplesLocally.SaveJSON = &saveJSON
		spec.SaveSamplesLocally.SaveProto = &saveProto

		// create the collector
		instance, err := NewCollector(context.Background(), c, &spec)
		require.NoError(t, err)
		tc.UnmarshalInputsStrict(map[string]interface{}{"input_usage.yaml": &instance.UtilizationServer})

		ctx, cancelCtx := context.WithCancel(context.Background())
		go instance.Run(ctx)
		defer cancelCtx()

		// collect the metrics
		reg := prometheus.NewPedanticRegistry()
		require.NoError(t, reg.Register(instance))
		_, err = reg.Gather()
		require.NoError(t, err)

		contains := strings.TrimSpace(tc.Inputs["input_outputfile.txt"])

		// read the save resul file
		subDir := path.Base(tc.Name)
		subDir = strings.Split(subDir, "_")[0]
		if contains != "" {
			subDir = contains
		}
		list, err := os.ReadDir(filepath.Join(tmpDir, subDir))
		require.NoError(t, err)

		var jsonFileName, protoFilename string
		for i := range list {
			if contains != "" && !strings.Contains(list[i].Name(), contains) {
				continue
			}
			if strings.HasSuffix(list[i].Name(), ".json") {
				jsonFileName = list[i].Name()
			}
			if strings.HasSuffix(list[i].Name(), ".pb") {
				protoFilename = list[i].Name()
			}
		}
		if saveJSON {
			require.NotEmpty(t, jsonFileName)
		}
		if saveProto {
			require.NotEmpty(t, protoFilename)
		}

		if saveProto {
			b, err := os.ReadFile(filepath.Join(tmpDir, subDir, protoFilename))
			require.NoError(t, err)
			var m proto.Message
			if strings.Contains(protoFilename, "samplelist") {
				m = &collectorapi.SampleList{}
			} else if strings.Contains(protoFilename, "scraperesult") {
				m = &collectorapi.ScrapeResult{}
			} else {
				require.Fail(t, "unrecognized result type", "filename", protoFilename)
			}

			err = proto.Unmarshal(b, m)
			if err != nil {
				return err
			}

			// convert the results to json
			marshaler := protojson.MarshalOptions{
				Indent:    " ",
				Multiline: true,
			}
			jsn, err := marshaler.Marshal(m)
			if err != nil {
				return err
			}
			if !saveJSON {
				// Check the proto results as the actual
				tc.Actual = strings.ReplaceAll(string(jsn), ":  ", ": ")
			} else {
				// Verify that the proto and json outputs are equivalent
				// But check the json results as the actual
				jsonBytes, err := os.ReadFile(filepath.Join(tmpDir, subDir, jsonFileName))
				require.NoError(t, err)

				sr := m.(*collectorapi.ScrapeResult)
				b := &bytes.Buffer{}
				for _, s := range sr.Items {
					require.NoError(t, ProtoToJSON(s, b))
				}
				require.Equal(t, string(jsonBytes), b.String())
			}
		}

		if saveJSON {
			b, err := os.ReadFile(filepath.Join(tmpDir, subDir, jsonFileName))
			require.NoError(t, err)
			tc.Actual = string(b)
		}

		return nil
	})
}

// filterMetrics removes metrics matching ignore from the slice and returns the result
func filterMetrics(metrics []*dto.MetricFamily, ignore []string) []*dto.MetricFamily {
	var filtered []*dto.MetricFamily
	ignoreSet := sets.NewString(ignore...)

	for _, m := range metrics {
		if ignoreSet.Has(m.GetName()) {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}

type LabelInputs struct {
	Pod                     corev1.Pod                                             `json:"pod"`
	Container               corev1.Container                                       `json:"container"`
	Node                    corev1.Node                                            `json:"node"`
	Namespace               corev1.Namespace                                       `json:"namespace"`
	Quota                   corev1.ResourceQuota                                   `json:"quota"`
	ResourceQuotaDescriptor quotamanagementv1alpha1.ResourceQuotaDescriptor        `json:"resourceQuotaDescriptor"`
	Workload                workload                                               `json:"workload"`
	Spec                    collectorcontrollerv1alpha1.MetricsPrometheusCollector `json:"spec"`
}

// Update test data by running with `TESTUTIL_UPDATE_EXPECTED=true`
func TestLabels(t *testing.T) {
	p := testutil.TestCaseParser{
		Subdir:         "labels",
		ExpectedSuffix: ".yaml",
	}
	p.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T
		var l LabelsValues
		var inputs LabelInputs

		tc.UnmarshalInputsStrict(map[string]interface{}{
			"input.yaml": &inputs,
		})

		c := Collector{MetricsPrometheusCollector: &inputs.Spec}
		require.NoError(t, c.init())

		instance := Labeler{
			BuiltIn: builtInLabler{
				UseQuotaNameForPriorityClass: inputs.Spec.BuiltIn.UseQuotaNameForPriorityClass},
			Extension: extensionLabler{Extensions: inputs.Spec.Extensions},
		}

		switch {
		case strings.Contains(tc.ExpectedFilepath, "container"):
			instance.SetLabelsForPod(&l, &inputs.Pod, inputs.Workload, &inputs.Node, &inputs.Namespace)
			instance.SetLabelsForContainer(&l, &inputs.Container)
		case strings.Contains(tc.ExpectedFilepath, "node"):
			instance.SetLabelsForNode(&l, &inputs.Node)
		case strings.Contains(tc.ExpectedFilepath, "quota"):
			instance.SetLabelsForQuota(&l, &inputs.Quota, &inputs.ResourceQuotaDescriptor, &inputs.Namespace)
		}

		require.Len(t, inputs.Spec.Aggregations, 1)
		require.Len(t, inputs.Spec.Aggregations[0].Levels, 1)
		mask := inputs.Spec.Aggregations[0].Levels[0].Mask

		labels := map[string]string{}
		lNames := c.getLabelNames(mask)
		lValues := c.getLabelValues(mask, l, lNames)
		require.Len(t, lNames, len(lValues), "names and values have different length")
		for i := range lNames {
			labels[lNames[i]] = lValues[i]
		}

		result, err := yaml.Marshal(labels)
		require.NoError(t, err)
		tc.Actual = string(result)
		return nil
	})
}

type ValuelInputs struct {
	Pods                    []*corev1.Pod                                   `json:"pods"`
	Node                    corev1.Node                                     `json:"node"`
	Quota                   corev1.ResourceQuota                            `json:"quota"`
	ResourceQuotaDescriptor quotamanagementv1alpha1.ResourceQuotaDescriptor `json:"resourceQuotaDescriptor"`
	UsageResponses          map[string]*samplerapi.ListMetricsResponse      `json:"responses"`
}

func TestValues(t *testing.T) {
	p := testutil.TestCaseParser{
		Subdir:         "values",
		ExpectedSuffix: ".yaml",
	}
	p.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T
		var inputs ValuelInputs

		if b, ok := tc.Inputs["input.yaml"]; ok {
			require.NoError(t, sigsyaml.UnmarshalStrict([]byte(b), &inputs))
		}

		scraper := utilization.Server{
			Responses: make(map[string]*samplerapi.ListMetricsResponse),
		}
		for k, v := range inputs.UsageResponses {
			v.NodeName = k
			scraper.CacheMetrics(v)
		}

		var instance ValueReader
		var values map[collectorcontrollerv1alpha1.Source]value
		switch {
		case strings.Contains(tc.ExpectedFilepath, "container"):
			pod := inputs.Pods[0]
			usage := scraper.GetContainerUsageSummary(scraper.GetMetrics())[sampler.ContainerKey{
				ContainerID: strings.TrimPrefix(pod.Status.ContainerStatuses[0].ContainerID, "containerd://"),
				PodUID:      string(pod.UID)}]
			values = instance.GetValuesForContainer(&pod.Spec.Containers[0], pod, usage)
		case strings.Contains(tc.ExpectedFilepath, "pod"):
			values = instance.GetValuesForPod(inputs.Pods[0])
		case strings.Contains(tc.ExpectedFilepath, "scheduler"):
			values = instance.GetValuesForSchedulerHealth(inputs.Pods[0], time.Minute)
		case strings.Contains(tc.ExpectedFilepath, "node"):
			values = instance.GetValuesForNode(&inputs.Node, inputs.Pods)
		case strings.Contains(tc.ExpectedFilepath, "quota"):
			values = instance.GetValuesForQuota(&inputs.Quota, &inputs.ResourceQuotaDescriptor, true)
		}

		result, err := sigsyaml.Marshal(values)
		require.NoError(t, err)
		tc.Actual = string(result)
		return nil
	})
}

// Update test data by running with `TESTUTIL_UPDATE_EXPECTED=true`
func TestGetWorkloadForPod(t *testing.T) {
	p := testutil.TestCaseParser{
		Subdir:         filepath.Join("util", "getworkloadforpod"),
		ExpectedSuffix: ".yaml",
	}
	p.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T
		c, err := tc.GetFakeClient(scheme.Scheme)

		// there should be exactly 1 pod in the test data
		assert.NoError(t, err)
		pods := &corev1.PodList{}
		assert.NoError(t, c.List(context.Background(), pods))
		assert.Len(t, pods.Items, 1)

		// run the test
		wl := getWorkloadForPod(c, &pods.Items[0])
		b, err := yaml.Marshal(wl)
		assert.NoError(t, err)
		tc.Actual = string(b)
		return nil
	})
}
