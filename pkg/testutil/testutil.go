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
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

// TestCase contains a test case parsed from a testdata directory
type TestCase struct {
	T *testing.T

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

	// Actual is the actual results observed
	Actual string

	// ClientInputsSuffix is a suffix to append to files which
	// should be parsed as inputs for a fake client.Client.
	ClientInputsSuffix string

	// ClearMeta if set to true will clear metadata that shouldn't be set
	// at creation time
	ClearMeta bool
}

// TestCaseParser parses tests cases from testdata directories.
// Test cases are assumed to be 1 per-subdirectory under "testdata" with
// the input in a file called "input" and the expected value in a
// file called "expected".
// Errors are stored
type TestCaseParser struct {
	// Subdir is an optional subdirectory
	Subdir string

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
		t.Run(tc.Name, func(t *testing.T) {
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
		})
	}
}

// UpdateExpectedDir invokes fn for each TestCase for in a "testdata" directory
// and updates the expected data file with the actual observed value
func (p TestCaseParser) UpdateExpectedDir(t *testing.T, cases []TestCase, fn func(*TestCase) error) {
	for i := range cases {
		tc := cases[i]
		t.Run(tc.Name, func(t *testing.T) {
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
		})
	}
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
func (tc TestCase) GetFakeClient(s *runtime.Scheme) (client.Client, error) {
	objs, objLists, err := tc.GetObjects(s)
	if err != nil {
		return nil, err
	}
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithLists(objLists...).Build(), nil
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
		})
		return nil
	})
	return testCases, err
}
