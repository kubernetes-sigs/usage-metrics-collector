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
