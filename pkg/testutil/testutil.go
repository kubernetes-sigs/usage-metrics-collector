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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/yaml"
)

// TestCase contains a test case parsed from a testdata directory
type TestCase struct {
	T testing.TB

	// Name of the test cases
	Name string

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

	// CompareMetrics is set to a list of metrics to compare the results of
	CompareMetrics []string

	// CompareEvents if set to true will compare the events from the FakeRecorder to
	// the events in expected_events.yaml.
	CompareEvents bool
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
		require.NoError(t, tc.Client.List(context.Background(), o))
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

// GetObjects parses the input objects from the test case
func (tc TestCase) GetObjects(s *runtime.Scheme) ([]client.Object, []client.ObjectList, error) {
	var objs []client.Object
	var objLists []client.ObjectList

	if len(tc.ClientInputsSuffix) == 0 {
		tc.ClientInputsSuffix = "_client_objects.yaml"
	}

	ser := sjson.NewSerializer(sjson.DefaultMetaFactory, s, s, false)
	// dec := serializer.NewCodecFactory(s).UniversalDecoder()
	for k, v := range tc.Inputs {
		if !strings.HasSuffix(k, tc.ClientInputsSuffix) {
			continue
		}
		items := strings.Split(v, "\n---\n")
		for _, i := range items {
			// skip empty items
			i = strings.TrimSpace(i)
			if len(i) == 0 {
				continue
			}
			b := []byte(i)

			// convert to json
			if strings.HasSuffix(k, ".yaml") {
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
			out, _, err := ser.Decode(b, nil, nil)
			if err != nil {
				return nil, nil, err
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

// GetFakeClient returns a new client.Client populated with Kubernetes
// parsed from the testdata input files.
// Defaults to parsing objects from files with suffix
// `_client_runtime_objects.yaml`
//
//nolint:gocognit
func (tc *TestCase) GetFakeClient(s *runtime.Scheme) (client.Client, error) {
	if tc.Client != nil {
		return tc.Client, nil
	}
	objs, objLists, err := tc.GetObjects(s)
	if err != nil {
		return nil, err
	}
	tc.Client = fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithLists(objLists...).Build()
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
			Name:               name,
			Inputs:             inputs,
			Error:              string(errorValue),
			ErrorFilepath:      errorFilepath,
			Expected:           string(expected),
			ExpectedFilepath:   expectedFilepath,
			ClientInputsSuffix: p.ClientInputsSuffix,
			ClearMeta:          p.ClearMeta,
			ExpectedObjects:    string(expectedObjects),
			ExpectedEvents:     string(expectedEvents),
			ExpectedMetrics:    string(expectedMetrics),
			CompareObjects:     p.CompareObjects,
			CompareMetrics:     p.CompareMetrics,
			ExpectedValues:     expectedValues,
		})
		return nil
	})
	return testCases, err
}
