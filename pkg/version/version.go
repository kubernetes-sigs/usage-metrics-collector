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
