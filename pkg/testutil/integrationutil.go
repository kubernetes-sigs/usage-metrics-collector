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
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/usage-metrics-collector/pkg/scheme"
)

// IntegrationTestSuite is a test suite that configures a test environment
type IntegrationTestSuite struct {
	// Environment is the test control-plane environment
	Environment envtest.Environment

	// Config is a config for the control-plane
	Config *rest.Config
	// ConfigFilepath is set to the kubeconfig path to talk to the control-plane
	ConfigFilepath string
	// Client is a client to talk to the apiserver
	Client client.Client

	// TestCase is the test case to config
	TestCase TestCase
	// TestCaseObjects are the objects parsed for the test case
	TestCaseObjects []client.Object
}

// WriteKubeconfig writes the Config to a `kubeconfig` file
func (suite *IntegrationTestSuite) WriteKubeconfig(t *testing.T) error {
	// Create the directory to host the kubeconfig file
	f, err := os.CreateTemp("", "kube-metrics-kubeconfig")
	require.NoError(t, err)
	suite.ConfigFilepath = f.Name()
	user, err := suite.Environment.AddUser(
		envtest.User{Name: "metrics", Groups: []string{"system:masters"}}, suite.Config)
	require.NoError(t, err)
	cfg, err := user.KubeConfig()
	require.NoError(t, err)
	return os.WriteFile(suite.ConfigFilepath, cfg, 0500)
}

// PodMetricsCRD fakes the PodMetrics API as a CRD so we can manually set the values
var PodMetricsCRD = apiextensionsv1.CustomResourceDefinition{
	ObjectMeta: metav1.ObjectMeta{
		Name: "podmetrics.metrics.k8s.io",
		Annotations: map[string]string{
			"api-approved.kubernetes.io": "unapproved",
		},
	},
	Spec: apiextensionsv1.CustomResourceDefinitionSpec{
		Group: "metrics.k8s.io",
		Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
			{
				Name:    "v1beta1",
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					// Keep all the fields so we don't have to put the full schema here
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						XPreserveUnknownFields: pointer.BoolPtr(true),
						Type:                   "object",
					},
				},
			},
		},
		Names: apiextensionsv1.CustomResourceDefinitionNames{
			Singular: "podmetrics",
			Plural:   "podmetrics",
			Kind:     "PodMetrics",
		},
		Scope: apiextensionsv1.NamespaceScoped,
	},
}

// ResourceQuotaDescriptorCRD fakes the ResourceQuotaDescriptor API as a CRD so we can manually set the values
var ResourceQuotaDescriptorCRD = apiextensionsv1.CustomResourceDefinition{
	ObjectMeta: metav1.ObjectMeta{
		Name: "resourcequotadescriptors.quotamanagement.usagemetricscollector.sigs.k8s.io",
		Annotations: map[string]string{
			"api-approved.kubernetes.io": "unapproved",
		},
	},
	Spec: apiextensionsv1.CustomResourceDefinitionSpec{
		Group: "quotamanagement.usagemetricscollector.sigs.k8s.io",
		Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
			{
				Name:    "v1alpha1",
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					// Keep all the fields so we don't have to put the full schema here
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						XPreserveUnknownFields: pointer.BoolPtr(true),
						Type:                   "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"metadata": {
								Type: "object",
							},
						},
					},
				},
			},
		},
		Names: apiextensionsv1.CustomResourceDefinitionNames{
			Singular: "resourcequotadescriptor",
			Plural:   "resourcequotadescriptors",
			Kind:     "ResourceQuotaDescriptor",
			ListKind: "ResourceQuotaDescriptorList",
		},
		Scope: apiextensionsv1.NamespaceScoped,
	},
}

// SetupTest creates the objects in the apiserver
func (suite *IntegrationTestSuite) SetupTest(t *testing.T, tc TestCase) {
	// Parse the objects from strings
	suite.TestCase = tc
	objs, _, err := suite.TestCase.GetObjects(scheme.Scheme)
	require.NoError(t, err)
	suite.TestCaseObjects = objs

	// Create each object in the apiserver
	for i := range suite.TestCaseObjects {
		// save a copy because creating will clear the status
		copy := suite.TestCaseObjects[i].DeepCopyObject()

		// first, create the object
		require.NoError(t,
			suite.Client.Create(context.Background(), suite.TestCaseObjects[i]))

		// second, try to update the status using the copy -- will give an error for resource
		// types without a status endpoint, so ignore the result
		// this is necessary because the apiserver drops the status unless going through
		// the status endpoint
		switch v := copy.(type) {
		case client.Object:
			err := suite.Client.Status().Update(context.Background(), client.Object(v))
			// ignore the error if it is about the status endpoint missing (e.g. for Namespaces)
			if err != nil && (!strings.Contains(err.Error(), "the server could not find the requested resource") &&
				!strings.Contains(err.Error(), "not found")) {
				require.NoError(t, err)
			}
		}
	}
}

// TearDownTest implements TearDownTest
func (suite *IntegrationTestSuite) TearDownTest(t *testing.T) {
	// delete all of the objects in reverse
	policy := metav1.DeletePropagationForeground
	for _, o := range suite.TestCaseObjects {
		copy := o.DeepCopyObject().(client.Object)
		require.NoError(t,
			suite.Client.Delete(context.Background(), copy, &client.DeleteOptions{
				GracePeriodSeconds: pointer.Int64(0),
				PropagationPolicy:  &policy,
			}))
	}

	// for i := 0; i < 5; i++ {
	// 	if suite.CheckDeleted(t) {
	// 		break
	// 	}
	// 	fmt.Println("waiting on object deletion")
	// 	time.Sleep(1 * time.Second)
	// }

	suite.TestCase = TestCase{}
	suite.TestCaseObjects = nil
}

func (suite *IntegrationTestSuite) CheckDeleted(t *testing.T) bool {
	done := true
	policy := metav1.DeletePropagationForeground
	for _, o := range suite.TestCaseObjects {
		copy := o.DeepCopyObject().(client.Object)
		err := suite.Client.Get(
			context.Background(),
			client.ObjectKey{Namespace: o.GetNamespace(), Name: o.GetName()},
			copy,
		)
		if err == nil {
			// we want it to be gone which would give an error -- no error means it's still around
			require.NoError(t,
				suite.Client.Delete(context.Background(), copy, &client.DeleteOptions{
					GracePeriodSeconds: pointer.Int64(0),
					PropagationPolicy:  &policy,
				}))
			done = false
		}
	}
	return done
}

// GetMetrics reads the metrics from the metrics endpoint
// nolint: gosec
func (suite *IntegrationTestSuite) GetMetrics(t *testing.T, url string, cmdOut *bytes.Buffer, prefixes ...string) string {

	var out []byte
	require.Eventually(t, func() bool {
		// Read the metrics from the metrics endpoint
		// nolint: gosec,noctx
		resp, err := http.Get(url)
		if err != nil {
			return false
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		out, err = io.ReadAll(resp.Body)

		// Verify the result is OK
		if err != nil {
			return false
		}
		if resp.StatusCode != http.StatusOK {
			return false
		}

		return true
	}, time.Second*60, time.Second*5, cmdOut)

	if len(prefixes) == 0 {
		return string(out)
	}

	var lines []string
	for _, s := range strings.Split(string(out), "\n") {
		for _, prefix := range prefixes {
			if strings.HasPrefix(s, prefix) || strings.HasPrefix(s, "# HELP "+prefix) ||
				strings.HasPrefix(s, "# TYPE "+prefix) {
				lines = append(lines, s)
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

// SetupTestSuite implements SetupSuite
func (suite *IntegrationTestSuite) SetupTestSuite(t *testing.T) {
	// Fake out the PodMetrics APIs -- this is actually an aggregated API in a real cluster
	suite.Environment.CRDs = []*apiextensionsv1.CustomResourceDefinition{&PodMetricsCRD, &ResourceQuotaDescriptorCRD}
	cfg, err := suite.Environment.Start()
	require.NoError(t, err)
	suite.Config = cfg

	c, err := client.New(suite.Config, client.Options{
		// Use the Scheme with all the registered types
		Scheme: scheme.Scheme,
	})
	require.NoError(t, err)
	suite.Client = c

	// Write a kubeconfig file
	require.NoError(t, suite.WriteKubeconfig(t))
}

// TearDownTestSuite implements TearDownSuite
func (suite *IntegrationTestSuite) TearDownTestSuite(t *testing.T) {
	// Stop the control-plane
	require.NoError(t, suite.Environment.Stop())

	// Delete the kubeconfig file
	assert.NoError(t, os.Remove(suite.ConfigFilepath))
}

// RunCommand runs the command asynchronously, returning the command + the output buffer
func (suite *IntegrationTestSuite) RunCommand(t *testing.T, cmd string, args ...string) (*exec.Cmd, *bytes.Buffer, chan error) {
	// nolint: gosec
	command := exec.Command(cmd, args...)
	command.SysProcAttr = &syscall.SysProcAttr{
		// Ensure child processes are killed
		Setpgid: true,
	}
	// Run the process in the background
	ch := make(chan error)

	var buff bytes.Buffer
	command.Stdout = &buff
	command.Stdout = &buff
	go func() {
		err := command.Run()
		err = errors.Wrap(err, buff.String())
		defer func() { ch <- err }()
	}()
	return command, &buff, ch
}

// StopCommand kills the command and it's child processes
func (suite *IntegrationTestSuite) StopCommand(t *testing.T, c *exec.Cmd) {
	// Kill all the commands and their child processes
	if c.Process == nil || c.Process.Pid == 0 {
		return
	}
	gid, err := syscall.Getpgid(c.Process.Pid)
	if err != nil && strings.Contains(err.Error(), "no such process") {
		// don't try to kill the process if it already exited
		// expected case for testing error conditions
		return
	}
	require.NoError(t, err)
	if err != nil {
		return
	}
	err = syscall.Kill(-gid, syscall.SIGKILL)
	require.NoError(t, err)
	if err != nil {
		return
	}
}

// CheckHealth ensures the localhost:8080/healthz is healthy
// nolint: godox
// TODO: make this more generic
func (suite *IntegrationTestSuite) CheckHealth(t *testing.T, url string, out *bytes.Buffer) {
	// Poll the healthz endpoint until it is healthy
	require.Eventually(t, func() bool {
		// nolint: gosec,noctx
		resp, err := http.Get(url)
		if err != nil {
			return false
		}
		defer func() {
			assert.NoError(t, resp.Body.Close())
		}()
		// check the response is ok
		if resp.StatusCode != http.StatusOK {
			return false
		}
		out, err := io.ReadAll(resp.Body)
		if err != nil {
			return false
		}
		if string(out) != "ok" {
			return false
		}
		return true
	}, time.Second*5, time.Second, out)
}

// GetFreePorts request n free ready to use ports from the kernel
func GetFreePorts(n int) ([]int, error) {
	ports := make([]int, n)
	for i := range ports {
		addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			return nil, err
		}

		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return nil, err
		}
		defer l.Close()
		ports[i] = l.Addr().(*net.TCPAddr).Port
	}
	return ports, nil
}
