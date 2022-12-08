package collector

import (
	"math"
	"sort"

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
)

// aggregate aggregates the ResourceLists
// If no ResourceName is specified, all resources are aggregated.
func aggregate(op collectorcontrollerv1alpha1.AggregationOperation, values quantities) []resource.Quantity {
	result := resource.Quantity{}
	// nolint: exhaustive
	switch op {
	case collectorcontrollerv1alpha1.SumOperation:
		for i := range values {
			result.Add(values[i])
		}
	case collectorcontrollerv1alpha1.MaxOperation:
		for i := range values {
			// result is less than next
			if result.Cmp(values[i]) < 0 {
				result = values[i]
			}
		}
	case collectorcontrollerv1alpha1.AvgOperation:
		for i := range values {
			result.Add(values[i])
		}
		result = divideQtyInt64(result, int64(len(values)), AveragePrecision)
	case collectorcontrollerv1alpha1.P95Operation:
		sort.Sort(values)
		percentileIndex := int(math.Floor(float64(len(values)-1) * 0.95))
		return []resource.Quantity{values[percentileIndex]}
	case collectorcontrollerv1alpha1.MedianOperation:
		sort.Sort(values)
		index := (len(values) - 1) / 2
		return []resource.Quantity{values[index]}
	case collectorcontrollerv1alpha1.HistogramOperation:
		// don't aggregate
		return values
	}
	return []resource.Quantity{result}
}

// aggregateMetric performs an aggregation on the metrics by mapping metrics to common keys using the provided mask.
// If resources is not defined then all ResourceNames are aggregated.
func (c *Collector) aggregateMetric(op collectorcontrollerv1alpha1.AggregationOperation, m Metric, mask collectorcontrollerv1alpha1.LabelsMask) Metric {
	result := Metric{
		Mask:   mask,
		Values: map[labelsValues][]resource.Quantity{},
	}

	// map the values so they are mappedValues by the new level rather than the old
	// e.g. map multiple "containers" in the same "pod" to that "pod" key
	indexed := map[labelsValues][]resource.Quantity{}
	for k, v := range m.Values {
		// apply the mask to get the new key of the aggregated value and add to that slice
		labels := c.mask(mask, k)
		indexed[labels] = append(indexed[labels], v...)
	}

	// reduce by applying the aggregation operation to each slice
	for k := range indexed {
		result.Values[k] = aggregate(op, indexed[k])
	}
	return result
}
