package collector

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
)

// Metric contains all the values for a metric
type Metric struct {
	// Mask contains the set of labels that apply to this metric
	Mask collectorcontrollerv1alpha1.LabelsMask

	Name metricName

	Buckets map[string][]float64

	// Values contains the metric values for each unique set of labels
	Values map[labelsValues][]resource.Quantity
}

type metricName struct {
	Prefix string

	Level string // e.g. cluster

	Operation string // e.g. sum

	Source string // e.g. requests_quota_hard

	ResourceAlias string // e.g. cpu_cores

	Resource string // e.g. cpu

	SourceType string // e.g. quota
}

func (m metricName) String() string {
	return strings.Join([]string{m.Prefix, m.Level, m.Operation, m.Source, m.ResourceAlias}, "_")
}

// quantities is a list of quantities
type quantities []resource.Quantity

func (v quantities) Less(i, j int) bool {
	return v[i].Cmp(v[j]) < 0
}

func (v quantities) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v quantities) Len() int {
	return len(v)
}

// Collect implements prometheus.Collector
func (c *Collector) collectMetric(m Metric, ch chan<- prometheus.Metric) {
	names := c.getLabelNames(m.Mask)
	for k, v := range m.Values {
		labels := c.getLabelValues(m.Mask, k, names)
		desc := prometheus.NewDesc(m.Name.String(), m.Name.String(), names, nil)
		ch <- prometheus.MustNewConstMetric(
			desc, prometheus.GaugeValue,
			v[0].AsApproximateFloat64(), labels...)
	}
}

func (c *Collector) collectHistogramMetric(m Metric, ch chan<- prometheus.Metric) {
	names := c.getLabelNames(m.Mask)

	// Create the histogram with the label names
	hv := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Buckets: m.Buckets[m.Name.ResourceAlias],
		Help:    m.Name.String(),
		Name:    m.Name.String(),
	}, names)

	// Record the observed values to the histogram
	for k, v := range m.Values {
		labels := c.getLabelValues(m.Mask, k, names)
		for _, val := range v {
			hv.WithLabelValues(labels...).Observe(val.AsApproximateFloat64())
		}
	}

	// Collect the histogram metric values
	hv.Collect(ch)
}

// workload represents the highest level pod template level or controller.
// All pods running in the same workload converge to the same pod spec.
type workload struct {
	Name       string `json:"name" yaml:"name"`
	Kind       string `json:"kind" yaml:"kind"`
	APIGroup   string `json:"apiGroup" yaml:"apiGroup"`
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
}
