package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	sigsyaml "sigs.k8s.io/yaml"

	commonlog "sigs.k8s.io/usage-metrics-collector/pkg/log"
	"sigs.k8s.io/usage-metrics-collector/pkg/testutil"
)

func TestParseMetrics_integration(t *testing.T) {
	p := testutil.TestCaseParser{
		Subdir: "parse",
	}
	log := commonlog.Log.WithName("TestParseMetrics")
	p.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T
		input := tc.Inputs["input"]

		got, err := ParseMetrics(log.WithName(t.Name()), strings.NewReader(input))
		if err != nil {
			return err // This error is expected and is a part of the output.
		}
		tc.Actual = got.String()
		return nil
	})
}

func TestHttpMetricsClient_open_integration(t *testing.T) {
	p := testutil.TestCaseParser{
		Subdir: "open",
	}

	assertHeader := func(t testing.TB, header http.Header) {
		for k, v := range headers {
			if got := header.Get(k); got != v {
				t.Errorf("open() header key(%s), got=%s, want %s", k, got, v)
			}
		}
	}

	type testCaseData struct {
		InputURL            string `yaml:"inputURL,omitempty"`
		UninitializedClient bool   `yaml:"uninitializedClient,omitempty"`
		ExpectClientHeader  string `yaml:"expectClientHeader,omitempty"`
		ResponseStatusCode  int    `yaml:"responseStatusCode,omitempty"`
		ResponseData        string `yaml:"responseData,omitempty"`
	}

	p.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T
		tt := &testCaseData{}

		require.NoError(t, sigsyaml.UnmarshalStrict([]byte(tc.Inputs["input.yaml"]), tt))

		svr := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			assertHeader(t, request.Header)
			writer.WriteHeader(tt.ResponseStatusCode)
			_, _ = writer.Write([]byte(tt.ResponseData))
		}))
		defer svr.Close()

		c := &HttpMetricsClient{
			Client: func() *http.Client {
				if !tt.UninitializedClient {
					return http.DefaultClient
				}
				return nil
			}(),
			URL: tt.InputURL,
		}
		if c.URL == "" {
			c.URL = svr.URL
		}

		reader, err := c.open()
		if err != nil {
			return err // This error is expected and is a part of the output.
		}
		got, err := io.ReadAll(reader)
		if err != nil {
			require.NoError(t, err) // This error is unexpected.
		}
		tc.Actual = string(got)
		return nil
	})
}

func TestHttpMetricsClient_Metrics_integration(t *testing.T) {
	p := testutil.TestCaseParser{
		Subdir: "metrics",
	}

	assertHeader := func(t testing.TB, header http.Header) {
		for k, v := range headers {
			if got := header.Get(k); got != v {
				t.Errorf("open() header key(%s), got=%s, want %s", k, got, v)
			}
		}
	}

	type testCaseData struct {
		ResponseStatusCode int    `yaml:"responseStatusCode,omitempty"`
		ResponseData       string `yaml:"responseData,omitempty"`
	}

	p.TestDir(t, func(tc *testutil.TestCase) error {
		t := tc.T
		tt := &testCaseData{}

		require.NoError(t, sigsyaml.UnmarshalStrict([]byte(tc.Inputs["input.yaml"]), tt))

		svr := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			assertHeader(t, request.Header)
			writer.WriteHeader(tt.ResponseStatusCode)
			_, _ = writer.Write([]byte(tt.ResponseData))
		}))
		defer svr.Close()

		c := &HttpMetricsClient{
			Client: http.DefaultClient,
		}
		c.URL = svr.URL

		reader, err := c.open()
		if err != nil {
			return err // This error is expected and is a part of the output.
		}
		got, err := io.ReadAll(reader)
		if err != nil {
			require.NoError(t, err) // This error is unexpected.
		}
		tc.Actual = string(got)
		return nil
	})
}
