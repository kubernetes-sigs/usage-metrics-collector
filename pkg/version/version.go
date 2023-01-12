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

// Package version exports the build version for a component
package version

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
)

// Collector exports the build version information as a metric
// These values should be set at build time using `-ldflags`
type Collector struct {
	Name    string
	Desc    *prometheus.Desc
	Version string
	Commit  string
	Date    string
}

var _ prometheus.Collector = &Collector{}

// Collect implements Collector
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	if c.Desc == nil {
		c.Desc = prometheus.NewDesc(
			c.Name, "version",
			[]string{"version", "commit", "date", "collector_instance"}, nil)
	}
	ch <- prometheus.MustNewConstMetric(
		c.Desc, prometheus.GaugeValue, 1.0,
		c.Version, c.Commit, c.Date, os.Getenv("POD_NAME"))
}

// Describe implements Collector
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	if c.Desc == nil {
		c.Desc = prometheus.NewDesc(
			c.Name, "version",
			[]string{"version", "commit", "date", "collector_instance"}, nil)
	}
	ch <- c.Desc
}
