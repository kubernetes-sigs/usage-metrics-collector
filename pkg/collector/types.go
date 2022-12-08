package collector

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
)

func (c *Collector) init() error {
	if c.MetricsPrometheusCollector == nil {
		return errors.Errorf("must specify MetricsPrometheusCollector")
	}
	c.defaultMetricsPrometheusCollector()

	// initialize internal IDs
	var labelId int
	for i := range c.MetricsPrometheusCollector.Extensions.Pods {
		c.MetricsPrometheusCollector.Extensions.Pods[i].ID = collectorcontrollerv1alpha1.LabelId(labelId)
		labelId++
	}
	for i := range c.MetricsPrometheusCollector.Extensions.Quota {
		c.MetricsPrometheusCollector.Extensions.Quota[i].ID = collectorcontrollerv1alpha1.LabelId(labelId)
		labelId++
	}
	for i := range c.MetricsPrometheusCollector.Extensions.Nodes {
		c.MetricsPrometheusCollector.Extensions.Nodes[i].ID = collectorcontrollerv1alpha1.LabelId(labelId)
		labelId++
	}
	for i := range c.MetricsPrometheusCollector.Extensions.Namespaces {
		c.MetricsPrometheusCollector.Extensions.Namespaces[i].ID = collectorcontrollerv1alpha1.LabelId(labelId)
		labelId++
	}
	for i := range c.MetricsPrometheusCollector.Extensions.NodeTaints {
		c.MetricsPrometheusCollector.Extensions.NodeTaints[i].ID = collectorcontrollerv1alpha1.LabelId(labelId)
		labelId++
	}
	for i := range c.MetricsPrometheusCollector.Extensions.PVCs {
		c.MetricsPrometheusCollector.Extensions.PVCs[i].ID = collectorcontrollerv1alpha1.LabelId(labelId)
		labelId++
	}
	for i := range c.MetricsPrometheusCollector.Extensions.PVs {
		c.MetricsPrometheusCollector.Extensions.PVs[i].ID = collectorcontrollerv1alpha1.LabelId(labelId)
		labelId++
	}
	var maskId int
	for i := range c.MetricsPrometheusCollector.Aggregations {
		for j := range c.MetricsPrometheusCollector.Aggregations[i].Levels {
			c.MetricsPrometheusCollector.Aggregations[i].Levels[j].Mask.ID = collectorcontrollerv1alpha1.LabelsMaskId(maskId)
			maskId++
		}
	}
	// setup indexes
	c.initExtensionLabelIndexes()
	for i := range c.Aggregations {
		for j := range c.Aggregations[i].Levels {
			c.initInternalLabelsMask(&c.Aggregations[i].Levels[j].Mask, c.Extensions)
		}
	}
	if c.SaveSamplesLocally != nil {
		for i := range c.SaveSamplesLocally.SampleSources {
			c.initInternalLabelsMask(&c.SaveSamplesLocally.SampleSources[i].Mask, c.Extensions)
		}
	}

	if err := c.validateMetricsPrometheusCollector(); err != nil {
		return err
	}

	c.labler.BuiltIn.UseQuotaNameForPriorityClass = c.BuiltIn.UseQuotaNameForPriorityClass
	c.labler.Extension.Extensions = c.Extensions
	return nil
}

func (c *Collector) defaultMetricsPrometheusCollector() {
	c.Kind = "MetricsPrometheusCollector"
	c.APIVersion = "v1alpha1"
	for i := range c.Aggregations {
		for j := range c.Aggregations[i].Levels {
			if c.Aggregations[i].Levels[j].Operation == "" {
				c.Aggregations[i].Levels[j].Operation = collectorcontrollerv1alpha1.SumOperation
			}
		}
	}
	if c.SaveSamplesLocally != nil {
		if c.SaveSamplesLocally.DirectoryPath == "" {
			c.SaveSamplesLocally.DirectoryPath = collectorcontrollerv1alpha1.DefaultSamplesLocalDirectoryPath
		}
		if c.SaveSamplesLocally.TimeFormat == "" {
			c.SaveSamplesLocally.TimeFormat = collectorcontrollerv1alpha1.DefaultSamplesTimeFormat
		}
	}
}

func (c *Collector) validateMetricsPrometheusCollector() error {
	resourceAlias := sets.NewString()
	for _, v := range c.Resources {
		resourceAlias.Insert(v)
	}

	if !c.PreComputeMetrics.Enabled && c.SaveSamplesLocally != nil {
		return errors.Errorf("must use preComputeMetrics with saveSamplesLocally")
	}

	for i, a := range c.Aggregations {
		for _, s := range a.Sources.Container {
			if !collectorcontrollerv1alpha1.ContainerSources.Has(s) {
				return errors.Errorf("invalid container source '%s'", s)
			}
		}
		for _, s := range a.Sources.Pod {
			if !collectorcontrollerv1alpha1.PodSources.Has(s) {
				return errors.Errorf("invalid pod source '%s'", s)
			}
		}
		for _, s := range a.Sources.Node {
			if !collectorcontrollerv1alpha1.NodeSources.Has(s) {
				return errors.Errorf("invalid node source '%s'", s)
			}
		}
		for _, s := range a.Sources.CGroup {
			found := func() bool {
				if c.CGroupMetrics.RootSource.Name == s {
					return true
				}
				for _, c := range c.CGroupMetrics.Sources {
					if c.Name == s {
						return true
					}
				}
				return false
			}()
			if !found {
				return errors.Errorf("invalid cgroup source '%s'", s)
			}
		}
		for _, s := range a.Sources.Quota {
			if !collectorcontrollerv1alpha1.QuotaSources.Has(s) {
				return errors.Errorf("invalid quota source '%s'", s)
			}
		}
		for _, s := range a.Sources.PV {
			if !collectorcontrollerv1alpha1.PVSources.Has(s) {
				return errors.Errorf("invalid pv source '%s'", s)
			}
		}
		for _, s := range a.Sources.PVC {
			if !collectorcontrollerv1alpha1.PVCSources.Has(s) {
				return errors.Errorf("invalid pvc source '%s'", s)
			}
		}
		for _, s := range a.Sources.Namespace {
			if !collectorcontrollerv1alpha1.NamespaceSources.Has(s) {
				return errors.Errorf("invalid namespace source '%s'", s)
			}
		}

		if !collectorcontrollerv1alpha1.Types.Has(a.Sources.Type) {
			return errors.Errorf("invalid aggregation type '%s'", a.Sources.Type)
		}
		if len(a.Levels) == 0 {
			return errors.Errorf("aggregation %v missing levels", i)
		}
		if a.Sources.Type == "container" {
			// the container utilization source has multiple values over time which should never be summed
			sources := sets.NewString(a.Sources.GetSources()...)
			if sources.Has(collectorcontrollerv1alpha1.ContainerUtilizationSource) &&
				InvalidContainerUtilizationInitialLevelOperations.Has(string(a.Levels[0].Operation)) {
				return errors.Errorf("initial operation for aggregation %v must not be 'sum' if utilization source is used", i)
			}
		}

		for _, l := range a.Levels {
			if !collectorcontrollerv1alpha1.Operations.Has(string(l.Operation)) {
				return errors.Errorf("invalid aggregation level operation '%s'", l.Operation)
			}
			if l.Mask.Level == "" {
				return errors.Errorf("level missing name %v", l)
			}
			for k := range l.HistogramBuckets {
				if !resourceAlias.Has(k) {
					return errors.Errorf("unknown histogramBucket key %v, must be one of %v", k, resourceAlias.List())
				}
			}
		}
	}

	for _, t := range c.Extensions.NodeTaints {
		if t.LabelName == "" {
			return errors.Errorf("nodeTaint extension missing name '%v'", t)
		}
		if len(t.TaintKeys) == 0 && len(t.TaintValues) == 0 && len(t.TaintEffects) == 0 {
			return errors.Errorf("nodeTaint extent requires at least one of keys, values, effects to be specified")
		}
	}
	return nil
}

var InvalidContainerUtilizationInitialLevelOperations = sets.NewString("sum", "")
