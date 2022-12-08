package collector

import (
	"bytes"
	"context"
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

// Update test data by running with `TESTUTIL_UPDATE_EXPECTED=true`
func TestCollector(t *testing.T) {
	parser := testutil.TestCaseParser{
		Subdir:         "collector",
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

		// This single metric exists as a package var due to it existing outside the normal collection flow.
		clusterScopedListResultMetric.Reset()

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
	parser := testutil.TestCaseParser{
		Subdir:         "collector-save",
		ExpectedSuffix: ".txt",
	}
	parser.TestDir(t, func(tc *testutil.TestCase) error {
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

		var filename string
		for i := range list {
			// get the matching file if specified
			if contains != "" && !strings.Contains(list[i].Name(), contains) {
				continue
			}
			filename = list[i].Name()
			break
		}
		require.NotEmpty(t, filename)

		require.NotEmpty(t, filename)
		b, err := os.ReadFile(filepath.Join(tmpDir, subDir, filename))
		require.NoError(t, err)

		var m proto.Message
		if strings.Contains(filename, "samplelist") {
			m = &collectorapi.SampleList{}
		} else if strings.Contains(filename, "scraperesult") {
			m = &collectorapi.ScrapeResult{}
		} else {
			require.Fail(t, "unrecognized result type", "filename", filename)
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
		tc.Actual = strings.ReplaceAll(string(jsn), ":  ", ": ")

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
		var l labelsValues
		var inputs LabelInputs

		tc.UnmarshalInputsStrict(map[string]interface{}{
			"input.yaml": &inputs,
		})

		c := Collector{MetricsPrometheusCollector: &inputs.Spec}
		require.NoError(t, c.init())

		instance := labler{
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

		var instance valueReader
		var values map[string]value
		switch {
		case strings.Contains(tc.ExpectedFilepath, "container"):
			pod := inputs.Pods[0]
			usage := scraper.GetContainerUsageSummary(scraper.GetMetrics())[sampler.ContainerKey{
				ContainerID: strings.TrimPrefix(pod.Status.ContainerStatuses[0].ContainerID, "containerd://"),
				PodUID:      string(pod.UID)}]
			values = instance.GetValuesForContainer(&pod.Spec.Containers[0], pod, usage)
		case strings.Contains(tc.ExpectedFilepath, "pod"):
			values = instance.GetValuesForPod(inputs.Pods[0])
		case strings.Contains(tc.ExpectedFilepath, "node"):
			values = instance.GetValuesForNode(&inputs.Node, inputs.Pods)
		case strings.Contains(tc.ExpectedFilepath, "quota"):
			values = instance.GetValuesForQuota(&inputs.Quota, &inputs.ResourceQuotaDescriptor)
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
