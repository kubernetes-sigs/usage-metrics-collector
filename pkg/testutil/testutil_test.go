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

package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodesAreNotTainted(t *testing.T) {
	suite := &IntegrationTestSuite{}
	suite.SetupTestSuite(t)
	defer suite.TearDownTestSuite(t)

	node := &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
	}

	ctx := context.Background()
	require.NoError(t, suite.Client.Create(ctx, node))
	require.Empty(t, node.Spec.Taints)

	taints := []v1.Taint{{Key: "testing-taint", Effect: v1.TaintEffectNoSchedule}}
	taintedNode := &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-2",
		},
		Spec: v1.NodeSpec{
			Taints: taints,
		},
	}
	require.NoError(t, suite.Client.Create(ctx, taintedNode))
	require.Equal(t, taints, taintedNode.Spec.Taints)
}
