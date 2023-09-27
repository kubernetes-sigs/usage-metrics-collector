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

package client

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// MetricsClient defines operations to retrieve cadvisor metrics.
type MetricsClient interface {
	// Metrics read into model.Vector.
	Metrics(logr.Logger) (model.Vector, error)
}

// HttpMetricsClient is a http implementation of the Metrics client.
type HttpMetricsClient struct {
	// Http client.
	*http.Client
	// URL enp-point.
	URL string
}

func NewHttpMetricsClient(client *http.Client, url string) *HttpMetricsClient {
	return &HttpMetricsClient{
		Client: client,
		URL:    url,
	}
}

var headers = map[string]string{
	"Accept":                              `application/openmetrics-text;version=1.0.0,application/openmetrics-text;version=0.0.1;q=0.75,text/plain;version=0.0.4;q=0.5,*/*;q=0.1`,
	"User-Agent":                          "sampler",
	"X-Prometheus-Scrape-Timeout-Seconds": strconv.FormatFloat(time.Minute.Seconds(), 'f', -1, 64),
}

// open GET connection and return reader to process the response.
func (c *HttpMetricsClient) open() (io.ReadCloser, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("uninitialized http client")
	}
	req, err := http.NewRequest("GET", c.URL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	return resp.Body, nil
}

func ParseMetrics(log logr.Logger, reader io.Reader) (model.Vector, error) {
	TextVersion := "0.0.4"
	Format := `text/plain; version=` + TextVersion + `; charset=utf-8`
	decoder := expfmt.NewDecoder(reader, expfmt.Format(Format))
	dec := &expfmt.SampleDecoder{
		Dec: decoder,
		Opts: &expfmt.DecodeOptions{
			Timestamp: model.Now(),
		},
	}
	var all model.Vector
	for {
		var samples model.Vector
		err := dec.Decode(&samples)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		all = append(all, samples...)
	}
	log.V(1).Info("fetched", "count", len(all))
	return all, nil
}

func (c *HttpMetricsClient) Metrics(log logr.Logger) (model.Vector, error) {
	reader, err := c.open()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()
	return ParseMetrics(log, reader)
}

func MetricName(sample *model.Sample) string {
	return string(sample.Metric[model.MetricNameLabel])
}
