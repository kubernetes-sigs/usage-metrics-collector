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

// Package testutil provides utilities for writing functional tests
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pkg/errors"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/yaml"
)

// TestCase contains a test case parsed from a testdata directory
type TestCase struct {
	T testing.TB

	// Name of the test cases
	Name string

	// Environment is a envtest.Environment for integration tests
	EnvironmentFn func(*TestCase) *envtest.Environment

	// Environment is populated by EnvironmentFn
	Environment *envtest.Environment

	// MaskExpectedMetadata will drop all metadata fields except
	// name, namespace, labels, and annotations when doing comparison.
	MaskExpectedMetadata bool

	// Inputs are the input files
	Inputs map[string]string

	// Expected is the contents of the expected output
	Expected string

	// Error is the contents of the expected error if one is expected
	Error string

	// ErrorFilepath is where the error was read from
	ErrorFilepath string

	// ExpectedFilepath is where the expected results were read from
	ExpectedFilepath string

	// ExpectedValues are the expected values read from files configured
	// through ExpectedFiles in the TestCaseParser
	ExpectedValues map[string]string

	// Actual is the actual results observed
	Actual string

	// ActualValues are the actual values to compare to the ExpectedValues read from
	// the ExpectedFiles configured in the TestCaseParser
	ActualValues map[string]string

	// ClientInputsSuffix is a suffix to append to files which
	// should be parsed as inputs for a fake client.Client.
	ClientInputsSuffix string

	// ClearMeta if set to true will clear metadata that shouldn't be set
	// at creation time
	ClearMeta bool

	// ExpectedObjects are the expected objects read from expected_objects.yaml if present
	ExpectedObjects string

	// ExpectedEvents are the expected events read from expected_events.yaml if present
	ExpectedEvents string

	// ExpectedMetrics are the expected metrics to read from expected_metrics.txt if present
	ExpectedMetrics string

	// CompareObjects is set to lists' of object types to read from the apiserver and compare
	// to ExpectedObjects
	CompareObjects []client.ObjectList

	// CompareMetrics is set to a list of metric names to compare the results of
	CompareMetrics []string

	// FakeRecorder is set when requesting a fake recorder and used to compare ExpectedEvents
	FakeRecorder *record.FakeRecorder

	// Client is set when requesting a fake client and used to compare ExpectedObjects
	Client client.Client
}

// TestCaseParser parses tests cases from testdata directories.
// Test cases are assumed to be 1 per-subdirectory under "testdata" with
// the input in a file called "input" and the expected value in a
// file called "expected".
// Errors are stored
type TestCaseParser struct {
	// EnvironmentFn if specified will create an integration test environment for the test
	EnvironmentFn func(*TestCase) *envtest.Environment

	// MaskExpectedMetadata will drop all metadata fields except
	// name, namespace, labels, and annotations when doing comparison.
	MaskExpectedMetadata bool

	// NameSuffix is appended to the test name
	NameSuffix string

	// Subdir is an optional subdirectory
	Subdir string

	// ExpectedFiles is a list of expected files.  The "expected_" prefix is applied to each
	// item when matching files. e.g. ExpectedFiles: [ "values.txt" ] will look for "expected_values.txt".
	// These are compared to the ActualValues map in the TestCase.  e.g. ActualValues[ "values.txt" ]
	// will be compared to the values read from the "expected_values.txt" file in the test directory
	// and is configured by setting ExpectedFiles to [ "values.txt" ] in the TestCaseParser.
	ExpectedFiles []string

	// InputSuffix is a suffix to append to the expected file
	ExpectedSuffix string

	// ClientInputsSuffix is a suffix to append to files which
	// should be parsed as inputs for a fake client.Client.
	ClientInputsSuffix string

	// DirPrefix is the directory prefix to filter on
	DirPrefix string

	// ClearMeta if set to true will clear metadata that shouldn't be set
	// at creation time
	ClearMeta bool

	// CompareObjects is set to a lists' of objects to read from the apiserver and
	// compare to the objects in expected_objects.yaml.
	CompareObjects []client.ObjectList

	// CompareListOptions are list options applied when comparing objects
	CompareListOptions []client.ListOption

	// CompareMetrics is set to a list of metrics to compare the results of
	CompareMetrics []string

	// CompareEvents if set to true will compare the events from the FakeRecorder to
	// the events in expected_events.yaml.
	CompareEvents bool
}

// SetupEnvTest will setup and run and envtest environment (apiserver and etcd)
// for integration tests.Returns a function to stop the envtest.
// Credentials and coordinates will be injected into each webhook.
func (p TestCaseParser) SetupEnvTest(t *testing.T, tc *TestCase) func() {
	ctx, cancel := context.WithCancel(context.Background())
	if tc.EnvironmentFn == nil {
		return cancel
	}

	// helpful error message for if envtest dependencies aren't installed
	require.NotEmptyf(t, os.Getenv("KUBEBUILDER_ASSETS"),
		"must set KUBEBUILDER_ASSETS using export 'KUBEBUILDER_ASSETS="+
			"`go run sigs.k8s.io/controller-runtime/tools/setup-envtest use -p path`'")

	// create a new test environment
	et := tc.EnvironmentFn(tc)
	tc.Environment = et

	if len(et.WebhookInstallOptions.MutatingWebhooks) > 0 ||
		len(et.WebhookInstallOptions.ValidatingWebhooks) > 0 {
		// setup the creds and coords for the server
		opt := &et.WebhookInstallOptions
		opt.PrepWithoutInstalling()

		// inject the creds and cords into the mutating webhook configs
		for i := range et.WebhookInstallOptions.MutatingWebhooks {
			wh := et.WebhookInstallOptions.MutatingWebhooks[i]
			for i := range wh.Webhooks {
				url := *wh.Webhooks[i].ClientConfig.URL
				if !strings.HasPrefix(url, "https://") {
					url = fmt.Sprintf("https://%s:%d/%s",
						opt.LocalServingHost, opt.LocalServingPort, url)
				}
				wh.Webhooks[i].ClientConfig = admissionv1.WebhookClientConfig{
					URL:      &url,
					CABundle: opt.LocalServingCAData,
				}
			}
		}
		// inject the creds and cords into the validating webhook configs
		for i := range et.WebhookInstallOptions.ValidatingWebhooks {
			wh := et.WebhookInstallOptions.ValidatingWebhooks[i]
			for i := range wh.Webhooks {
				url := *wh.Webhooks[i].ClientConfig.URL
				if !strings.HasPrefix(url, "https://") {
					url = fmt.Sprintf("https://%s:%d/%s",
						opt.LocalServingHost, opt.LocalServingPort, url)
				}
				wh.Webhooks[i].ClientConfig = admissionv1.WebhookClientConfig{
					URL:      &url,
					CABundle: opt.LocalServingCAData,
				}
			}
		}
	}

	// start the envtest -- apiserver and etcd
	_, err := et.Start()
	require.NoError(t, err)

	go func() {
		// stop the server when the test is complete
		<-ctx.Done()
		require.NoError(t, et.Stop())
	}()

	// setup the client
	client, err := client.New(et.Config, client.Options{})
	require.NoError(t, err)
	tc.Client = client

	return cancel
}

// TestDir invokes fn for each TestCase for in a "testdata" directory
// and verifies that the actual observed value matches the expected value
func (p TestCaseParser) TestDir(t *testing.T, fn func(*TestCase) error) {
	cases, err := p.GetTestCases()
	require.NoError(t, err)

	// update test directory
	if os.Getenv("TESTUTIL_UPDATE_EXPECTED") == "true" {
		p.UpdateExpectedDir(t, cases, fn)
		return
	}

	for i := range cases {
		tc := cases[i]
		t.Run(tc.Name+p.NameSuffix, func(t *testing.T) {
			env := tc.Inputs["input_env.env"]
			for _, s := range strings.Split(env, "\n") {
				if !strings.Contains(s, "=") {
					continue
				}
				kv := strings.SplitN(s, "=", 2)
				os.Setenv(kv[0], kv[1])
			}

			tc.T = t
			cancel := p.SetupEnvTest(t, &tc)
			defer cancel()
			if tc.Error != "" {
				err := fn(&tc)
				require.Error(t, err, "expected an error to be returned")
				require.Contains(t, err.Error(), tc.Error)
			} else {
				require.NoError(t, fn(&tc))
				require.Equal(t,
					strings.TrimSpace(tc.Expected),
					strings.TrimSpace(tc.Actual),
				)
			}

			for k, v := range tc.ExpectedValues {
				require.Equal(t,
					strings.TrimSpace(v),
					strings.TrimSpace(tc.ActualValues[k]),
				)
			}

			if len(tc.ExpectedObjects) > 0 {
				objects := p.GetActualObjects(t, &tc)
				require.Equal(t, tc.ExpectedObjects, objects, "actual objects do not match expected")
			}
			if len(tc.ExpectedEvents) > 0 {
				events := p.GetActualEvents(t, &tc)
				require.Equal(t, tc.ExpectedEvents, events, "actual events do not match expected")
			}
			if len(tc.ExpectedMetrics) > 0 {
				metrics := p.GetActualMetrics(t, p.CompareMetrics)
				require.Equal(t, tc.ExpectedMetrics, metrics, "actual metrics do not match expected")
			}
		})
	}
}

// UpdateExpectedDir invokes fn for each TestCase for in a "testdata" directory
// and updates the expected data file with the actual observed value
func (p TestCaseParser) UpdateExpectedDir(t *testing.T, cases []TestCase, fn func(*TestCase) error) {
	for i := range cases {
		tc := cases[i]
		t.Run(tc.Name+p.NameSuffix, func(t *testing.T) {
			env := tc.Inputs["input_env.env"]
			for _, s := range strings.Split(env, "\n") {
				if !strings.Contains(s, "=") {
					continue
				}
				kv := strings.SplitN(s, "=", 2)
				os.Setenv(kv[0], kv[1])
			}

			tc.T = t
			cancel := p.SetupEnvTest(t, &tc)
			defer cancel()

			err := fn(&tc)
			if err != nil {
				require.NoError(t,
					os.WriteFile(tc.ErrorFilepath, []byte(err.Error()), 0600))
			} else {
				_ = os.Remove(tc.ErrorFilepath)
			}
			if tc.Actual != "" {
				require.NoError(t,
					os.WriteFile(tc.ExpectedFilepath, []byte(tc.Actual), 0600))
			} else {
				_ = os.Remove(tc.ExpectedFilepath)
			}

			for k, v := range tc.ActualValues {
				if v != "" {
					require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(tc.ExpectedFilepath), "expected_"+k), []byte(v), 0600))
				} else {
					_ = os.Remove(k)
				}
			}

			if p.CompareObjects != nil {
				objects := p.GetActualObjects(t, &tc)
				objectsFilepath := filepath.Join(filepath.Dir(tc.ExpectedFilepath), "expected_objects.yaml")
				require.NoError(t,
					os.WriteFile(objectsFilepath, []byte(objects), 0600))
			}
			if p.CompareEvents {
				events := p.GetActualEvents(t, &tc)
				eventsFilepath := filepath.Join(filepath.Dir(tc.ExpectedFilepath), "expected_events.yaml")
				require.NoError(t,
					os.WriteFile(eventsFilepath, []byte(events), 0600))
			}
			if p.CompareMetrics != nil {
				metrics := p.GetActualMetrics(t, p.CompareMetrics)
				metricsFilepath := filepath.Join(filepath.Dir(tc.ExpectedFilepath), "expected_metrics.txt")
				require.NoError(t,
					os.WriteFile(metricsFilepath, []byte(metrics), 0600))
			}
		})
	}
}

// GetActualMetrics gathers the metrics from the registry and returns a string value
func (p TestCaseParser) GetActualMetrics(t *testing.T, names []string) string {
	metrics, err := metrics.Registry.Gather()
	require.NoError(t, err)
	var filtered []*dto.MetricFamily
	keep := sets.NewString(names...)

	for _, m := range metrics {
		if !keep.Has(m.GetName()) {
			continue
		}
		filtered = append(filtered, m)
	}
	var b bytes.Buffer
	for _, a := range filtered {
		_, err := expfmt.MetricFamilyToText(&b, a)
		require.NoError(t, err)
	}
	return b.String()
}

// GetActualObjects reads objects from the apiserver and returns them as yaml
func (p TestCaseParser) GetActualObjects(t *testing.T, tc *TestCase) string {
	var actual string
	for _, o := range tc.CompareObjects {
		o = o.DeepCopyObject().(client.ObjectList)
		require.NoError(t, tc.Client.List(context.Background(), o, p.CompareListOptions...))

		if p.MaskExpectedMetadata {
			// objects created through an integration test environment
			// need to have the metadata masked for the test output to
			// be consistent between runs
			items, err := meta.ExtractList(o)
			require.NoError(t, err, "error masking yaml for object")
			for i := range items {
				obj := items[i].(metav1.Object)
				obj.SetUID("")
				obj.SetManagedFields(nil)
				obj.SetCreationTimestamp(metav1.Time{})
				obj.SetResourceVersion("")
			}
			obj := o.(metav1.ListInterface)
			obj.SetResourceVersion("")
		}

		b, err := yaml.Marshal(o)
		require.NoError(t, err, "error marshaling yaml for object")
		actual += "---\n" + string(b)
	}
	return actual
}

// GetActualEvents reads objects from the FakeRecorder and returns them as yaml
func (p TestCaseParser) GetActualEvents(t *testing.T, tc *TestCase) string {
	if tc.FakeRecorder == nil {
		return ""
	}
	var actual string
	func() {
		for {
			select {
			case e := <-tc.FakeRecorder.Events:
				b, err := yaml.Marshal(&e)
				require.NoError(t, err, "error marshaling yaml for event")
				actual = actual + "---\n" + string(b)
			default:
				return
			}
		}
	}()
	return actual
}

// UnmarshalInputsStrict Unmarshals the TestCase Inputs (i.e. by filename) into
// the corresponding objects (e.g. pointers to structs)
func (tc TestCase) UnmarshalInputsStrict(into map[string]interface{}) {
	for k, v := range into {
		if _, ok := tc.Inputs[k]; !ok {
			continue
		}
		require.NoError(
			tc.T, yaml.UnmarshalStrict([]byte(tc.Inputs[k]), v))
	}
}

func (tc TestCase) CreateObjects() {
	objs, _, err := tc.GetObjects(tc.Client.Scheme())
	require.NoError(tc.T, err)
	for _, o := range objs {
		require.NoError(tc.T, tc.Client.Create(context.Background(), o))
	}
}

// UpdateObjects will update objects from files prefixed with "input_client_objects_updates"
// After each set of updates fn is called.  Updates are performed in alphanumberic order by filename.
func (tc TestCase) UpdateObjects(s *runtime.Scheme, fn func()) error {
	js := sjson.NewSerializer(sjson.DefaultMetaFactory, s, s, false)

	// sort the updates
	var files []string
	for k := range tc.Inputs {
		if !strings.HasPrefix(k, "input_client_objects_updates") {
			continue
		}
		files = append(files, k)
	}
	sort.Strings(files)

	// for each set of updates, update the objects then call fn
	for _, filename := range files {
		// parse the objects from the file data
		data := tc.Inputs[filename]
		o, ol, err := tc.GetObjectsFromFile(js, s, filename, data)
		if err != nil {
			return err
		}

		// update objects
		for _, obj := range o {
			err := tc.Client.Update(context.Background(), obj)
			if err != nil {
				return err
			}
		}
		for _, objL := range ol {
			// update objets read from lists
			o, err := meta.ExtractList(objL)
			if err != nil {
				return err
			}
			for _, obj := range o {
				err := tc.Client.Update(context.Background(), obj.(client.Object))
				if err != nil {
					return err
				}
			}
		}

		fn() // call the function for the test to do work
	}
	return nil
}

func (tc TestCase) GetObjects(s *runtime.Scheme) ([]client.Object, []client.ObjectList, error) {
	var objs []client.Object
	var objList []client.ObjectList

	js := sjson.NewSerializer(sjson.DefaultMetaFactory, s, s, false)
	if len(tc.ClientInputsSuffix) == 0 {
		tc.ClientInputsSuffix = "_client_objects.yaml"
	}
	for k, v := range tc.Inputs {
		if !strings.HasSuffix(k, tc.ClientInputsSuffix) {
			continue
		}
		o, ol, err := tc.GetObjectsFromFile(js, s, k, v)
		if err != nil {
			return nil, nil, err
		}
		objs = append(objs, o...)
		objList = append(objList, ol...)
	}

	return objs, objList, nil
}

// GetObjects parses the input objects from the test case
func (tc TestCase) GetObjectsFromFile(js *sjson.Serializer, s *runtime.Scheme, filename, data string) ([]client.Object, []client.ObjectList, error) {
	var objs []client.Object
	var objLists []client.ObjectList

	items := strings.Split(data, "\n---\n")
	for _, i := range items {
		// skip empty items
		i = strings.TrimSpace(i)
		if len(i) == 0 {
			continue
		}
		b := []byte(i)

		// convert to json
		if strings.HasSuffix(filename, ".yaml") {
			o := map[string]interface{}{}
			err := yaml.Unmarshal([]byte(i), &o)
			if err != nil {
				return nil, nil, err
			}
			b, err = json.Marshal(o)
			if err != nil {
				return nil, nil, err
			}
		}

		// decode the object to a struct
		out, gvk, err := js.Decode(b, nil, nil)
		if err != nil {
			return nil, nil, err
		}

		if _, ok := out.(*unstructured.Unstructured); ok {
			// special case unstructured because they implement both Object and ObjectList
			if strings.HasSuffix(gvk.Kind, "List") {
				// list object -- e.g. PodList
				if objL, ok := out.(client.ObjectList); ok {
					objLists = append(objLists, objL)
				}
				continue
			}
			if obj, ok := out.(client.Object); ok {
				objs = append(objs, obj)
				continue
			}
		}

		// list object -- e.g. PodList
		if objL, ok := out.(client.ObjectList); ok {
			objLists = append(objLists, objL)
			continue
		}

		// individual object -- e.g. Pod
		if obj, ok := out.(client.Object); ok {
			objs = append(objs, obj)
			continue
		}
	}
	if len(objs) == 0 && len(objLists) == 0 {
		return nil, nil, errors.Errorf("no input objects found")
	}

	if tc.ClearMeta {
		for i := range objLists {
			o, err := meta.ExtractList(objLists[i])
			if err != nil {
				return nil, nil, err
			}
			for i := range o {
				objs = append(objs, o[i].(client.Object))
			}
		}
		objLists = nil

		// clear this data if it is set
		for i := range objs {
			objs[i].SetResourceVersion("")
			objs[i].SetSelfLink("")
			objs[i].SetGeneration(0)
		}
	}
	return objs, objLists, nil
}

// TestCaseFakeClientBuilder is a function that can be used to modify the fake client builder
// before building the client.
type TestCaseFakeClientBuilder func(*fake.ClientBuilder) *fake.ClientBuilder

// GetFakeClient returns a new client.Client populated with Kubernetes
// parsed from the testdata input files.
// Defaults to parsing objects from files with suffix
// `_client_runtime_objects.yaml`
//
//nolint:gocognit
func (tc *TestCase) GetFakeClient(s *runtime.Scheme, buildOpts ...TestCaseFakeClientBuilder) (client.Client, error) {
	if tc.Client != nil {
		return tc.Client, nil
	}
	objs, objLists, err := tc.GetObjects(s)
	if err != nil {
		return nil, err
	}

	builder := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithLists(objLists...)

	for _, buildOpt := range buildOpts {
		builder = buildOpt(builder)
	}

	tc.Client = builder.Build()
	return tc.Client, nil
}

// GetFakeRecorder returns a fake recorder which can be used to compare events
func (tc *TestCase) GetFakeRecorder() *record.FakeRecorder {
	if tc.FakeRecorder != nil {
		return tc.FakeRecorder
	}
	tc.FakeRecorder = record.NewFakeRecorder(100)
	return tc.FakeRecorder
}

// GetTestCases parses the test cases from the testdata dir
//
//nolint:gocognit
func (p TestCaseParser) GetTestCases() ([]TestCase, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "testdata", p.Subdir)

	var testCases []TestCase
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if p.DirPrefix != "" && !strings.HasPrefix(d.Name(), p.DirPrefix) {
			return nil
		}

		files, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		inputs := map[string]string{}
		for i := range files {
			if files[i].IsDir() {
				continue
			}
			if strings.HasPrefix(files[i].Name(), "input") {
				input, err := os.ReadFile(filepath.Join(path, files[i].Name()))
				if err != nil {
					return err
				}
				inputs[files[i].Name()] = string(input)
			}
		}
		if len(inputs) == 0 {
			return nil
		}

		var expectedObjects []byte
		if len(p.CompareObjects) > 0 {
			expectedFilepath := filepath.Clean(filepath.Join(path, "expected_objects.yaml"))
			expectedObjects, err = os.ReadFile(expectedFilepath)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}

		var expectedEvents []byte
		if p.CompareEvents {
			expectedFilepath := filepath.Clean(filepath.Join(path, "expected_events.yaml"))
			expectedEvents, err = os.ReadFile(expectedFilepath)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}

		var expectedMetrics []byte
		if len(p.CompareMetrics) > 0 {
			expectedFilepath := filepath.Clean(filepath.Join(path, "expected_metrics.txt"))
			expectedMetrics, err = os.ReadFile(expectedFilepath)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}

		// additional expected files
		expectedValues := map[string]string{}
		for i := range p.ExpectedFiles {
			expectedFilepath := filepath.Clean(filepath.Join(path, "expected_"+p.ExpectedFiles[i]))
			expected, err := os.ReadFile(expectedFilepath)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			expectedValues[p.ExpectedFiles[i]] = string(expected)
		}

		// expected is optional
		expectedFilepath := filepath.Clean(filepath.Join(path, "expected"+p.ExpectedSuffix))
		expected, err := os.ReadFile(expectedFilepath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		// error is optional
		errorFilepath := filepath.Clean(filepath.Join(path, "error"))
		errorValue, err := os.ReadFile(errorFilepath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		name, err := filepath.Rel(dir, path)
		name = filepath.Join(p.Subdir, name)
		if err != nil {
			return err
		}
		testCases = append(testCases, TestCase{
			EnvironmentFn:        p.EnvironmentFn,
			MaskExpectedMetadata: p.MaskExpectedMetadata,
			Name:                 name,
			Inputs:               inputs,
			Error:                string(errorValue),
			ErrorFilepath:        errorFilepath,
			Expected:             string(expected),
			ExpectedFilepath:     expectedFilepath,
			ClientInputsSuffix:   p.ClientInputsSuffix,
			ClearMeta:            p.ClearMeta,
			ExpectedObjects:      string(expectedObjects),
			ExpectedEvents:       string(expectedEvents),
			ExpectedMetrics:      string(expectedMetrics),
			CompareObjects:       p.CompareObjects,
			CompareMetrics:       p.CompareMetrics,
			ExpectedValues:       expectedValues,
		})
		return nil
	})
	return testCases, err
}
