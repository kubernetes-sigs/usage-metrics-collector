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

	Name MetricName

	Buckets map[string][]float64

	// Values contains the metric values for each unique set of labels
	Values map[LabelsValues][]resource.Quantity
}

type MetricName struct {
	Prefix string

	Level collectorcontrollerv1alpha1.AggregationLevel // e.g. cluster

	Operation collectorcontrollerv1alpha1.AggregationOperation // e.g. sum

	Source collectorcontrollerv1alpha1.Source // e.g. requests_quota_hard

	SourceAlias string

	ResourceAlias collectorcontrollerv1alpha1.ResourceAlias // e.g. cpu_cores

	Resource collectorcontrollerv1alpha1.ResourceName // e.g. cpu

	SourceType collectorcontrollerv1alpha1.SourceType // e.g. quota
}

func (m MetricName) String() string {
	if m.SourceAlias != "" {
		return strings.Join([]string{m.Prefix, string(m.Level), string(m.Operation), string(m.SourceAlias), string(m.ResourceAlias)}, "_")
	}
	return strings.Join([]string{m.Prefix, string(m.Level), string(m.Operation), string(m.Source), string(m.ResourceAlias)}, "_")
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
		Buckets: m.Buckets[string(m.Name.ResourceAlias)],
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
