package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/model"

	"sigs.k8s.io/usage-metrics-collector/pkg/cadvisor/client/testdata"
	commonlog "sigs.k8s.io/usage-metrics-collector/pkg/log"
)

func TestParseMetrics(t *testing.T) {
	type args struct {
		log  logr.Logger
		data string
	}
	log := commonlog.Log.WithName("TestParseMetrics")
	tests := map[string]struct {
		args    args
		want    model.Vector
		wantErr bool
	}{
		"Singe": {
			args: args{
				log:  log,
				data: testdata.SingleMetricString,
			},
			want: testdata.SingleNonPodMetricVector,
		},
		"Multiple": {
			args: args{
				log:  log,
				data: testdata.MultipleMetricsString,
			},
			want: testdata.MultipleMetricsVector,
		},
		"InvalidData": {
			args: args{
				log:  log,
				data: "Invalid metric text data",
			},
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ParseMetrics(tt.args.log.WithName(name), strings.NewReader(tt.args.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMetrics() error = %v, wantErr %v", err, tt.wantErr)
			}
			testdata.DiffVectors(t, "ParseMetrics()", got, tt.want)
		})
	}
}

func TestHttpMetricsClient_open(t *testing.T) {
	type fields struct {
		Client *http.Client
		URL    string
	}
	assertHeader := func(header http.Header) {
		for k, v := range headers {
			if got := header.Get(k); got != v {
				t.Errorf("open() header key(%s), got=%s, want %s", k, got, v)
			}
		}
	}
	tests := map[string]struct {
		fields             fields
		responseStatusCode int
		responseData       []byte
		want               io.ReadCloser
		wantErr            bool
	}{
		"Error_Connection": {
			fields: fields{
				Client: http.DefaultClient,
				URL:    "test",
			},
			responseStatusCode: http.StatusNotFound,
			wantErr:            true,
		},
		"Error_NotFound": {
			fields: fields{
				Client: http.DefaultClient,
			},
			responseStatusCode: http.StatusNotFound,
			wantErr:            true,
		},
		"PlainText": {
			fields: fields{
				Client: http.DefaultClient,
			},
			responseStatusCode: http.StatusOK,
			responseData:       []byte("hello"),
			wantErr:            false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			svr := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assertHeader(request.Header)
				writer.WriteHeader(tt.responseStatusCode)
				_, _ = writer.Write(tt.responseData)
			}))
			defer svr.Close()

			c := &HttpMetricsClient{
				Client: tt.fields.Client,
				URL:    tt.fields.URL,
			}
			if c.URL == "" {
				c.URL = svr.URL
			}

			reader, err := c.open()
			if err != nil {
				if !tt.wantErr {
					t.Errorf("open() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			got, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("open() unexpected read error: %v", err)
			}
			if diff := cmp.Diff(got, tt.responseData); diff != "" {
				t.Errorf("open() reader(-),want(+): %s", diff)
			}
		})
	}
}

func TestHttpMetricsClient_Metrics(t *testing.T) {
	type fields struct {
		Client *http.Client
		URL    string
	}
	log := commonlog.Log.WithName("TestHttpMetricsClient_Metrics")
	type args struct {
		log logr.Logger
	}
	tests := map[string]struct {
		fields  fields
		args    args
		data    []byte
		want    model.Vector
		wantErr bool
	}{
		"ErrorResponse": {
			fields: fields{
				Client: http.DefaultClient,
				URL:    "nonexistent",
			},
			args: args{
				log: log,
			},
			wantErr: true,
		},
		"EmptyResponse": {
			args: args{
				log: log,
			},
			fields: fields{
				Client: http.DefaultClient,
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			svr := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, _ = writer.Write(tt.data)
			}))
			defer svr.Close()

			c := &HttpMetricsClient{
				Client: tt.fields.Client,
				URL:    tt.fields.URL,
			}
			if c.URL == "" {
				c.URL = svr.URL
			}
			got, err := c.Metrics(tt.args.log.WithName(name))
			if (err != nil) != tt.wantErr {
				t.Errorf("Metrics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Metrics() got(-),want(+): %s", diff)
			}
		})
	}
}
