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

package samplerserverv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	sigsyaml "sigs.k8s.io/yaml"

	"sigs.k8s.io/usage-metrics-collector/pkg/testutil"
)

func TestDefault(t *testing.T) {
	p := testutil.TestCaseParser{
		Subdir:         "default",
		ExpectedSuffix: ".yaml",
	}
	p.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T
		var instance MetricsNodeSampler
		require.NoError(t, sigsyaml.UnmarshalStrict([]byte(tc.Inputs["input.yaml"]), &instance))
		instance.Default()
		b, err := sigsyaml.Marshal(instance)
		require.NoError(t, err)
		tc.Actual = string(b)
		return nil
	})
}
