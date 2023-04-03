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
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/quotamanagementv1alpha1"
	collectorapi "sigs.k8s.io/usage-metrics-collector/pkg/collector/api"
	"sigs.k8s.io/usage-metrics-collector/pkg/collector/utilization"
	commonlog "sigs.k8s.io/usage-metrics-collector/pkg/log"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler"
	samplerapi "sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	log = commonlog.Log.WithName("collector")
)

type Collector struct {
	// collector dependencies

	client.Client
	Labeler Labeler
	Reader  ValueReader

	UtilizationServer utilization.Server
	*collectorcontrollerv1alpha1.MetricsPrometheusCollector

	nextId                 int
	labelNamesByIds        map[collectorcontrollerv1alpha1.LabelId]collectorcontrollerv1alpha1.LabelName
	labelIdsByNames        map[collectorcontrollerv1alpha1.LabelName]collectorcontrollerv1alpha1.LabelId
	labelsById             map[collectorcontrollerv1alpha1.LabelId]*collectorcontrollerv1alpha1.ExtensionLabel
	taintLabelsById        map[collectorcontrollerv1alpha1.LabelId]*collectorcontrollerv1alpha1.NodeTaint
	extensionLabelMaskById map[collectorcontrollerv1alpha1.LabelsMaskId]*extensionLabelsMask

	startTime time.Time
	metrics   chan *cachedMetrics

	sideCarConfigs []*collectorcontrollerv1alpha1.SideCarConfig
}

type cachedMetrics struct {
	startTime time.Time
	endTime   time.Time
	metrics   []prometheus.Metric
}

// NewCollector returns a collector which publishes capacity and utilisation metrics.
func NewCollector(
	ctx context.Context,
	client client.Client,
	config *collectorcontrollerv1alpha1.MetricsPrometheusCollector) (*Collector, error) {

	c := &Collector{
		Client:                     client,
		MetricsPrometheusCollector: config,
		UtilizationServer:          utilization.Server{UtilizationServer: config.UtilizationServer},
		startTime:                  time.Now(),
		metrics:                    make(chan *cachedMetrics),
	}

	// initialize the extensions data
	if err := c.init(); err != nil {
		return nil, err
	}

	if c.SaveSamplesLocally != nil {
		log.Info("ensuring directory for local samples", "directory",
			c.SaveSamplesLocally.DirectoryPath)
		err := os.MkdirAll(c.SaveSamplesLocally.DirectoryPath, 0700)
		if err != nil {
			log.Error(err, "unable to create directory for saving metrics",
				"directory", c.SaveSamplesLocally.DirectoryPath)
		}
	}

	return c, nil
}

// InitInformers initializes the informer caches by listing all the objects
func (c *Collector) InitInformers(ctx context.Context) error {
	_, err := c.listCapacityObjects(nil)
	return err
}

// Start runs the collector
func (c *Collector) Run(ctx context.Context) {
	if c.PreComputeMetrics.Enabled {
		c.continuouslyCollect(ctx)
	}
}

// Describe returns all descriptions of the collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *Collector) continuouslyCollect(ctx context.Context) {
	for {
		m := c.cacheMetrics()
		select {
		case <-ctx.Done():
			return // shutdown
		case c.metrics <- m:
			// write the cached metrics
		}
	}
}

func (c *Collector) cacheMetrics() *cachedMetrics {
	sCh := make(chan *collectorapi.SampleList) // channel for aggregation to send SampleLists for saving locally
	ch := make(chan prometheus.Metric)         // channel for aggregation to send Metrics for exporting to prometheus
	cm := cachedMetrics{startTime: time.Now()}

	// read metrics from the collector and add to a slice
	done := sync.WaitGroup{}
	done.Add(1) // wait until we've gotten the Metrics
	go func() {
		// read metrics as they are aggregated -- required for aggregation not to block
		for m := range ch {
			cm.metrics = append(cm.metrics, m)
		}
		done.Done()
	}()

	scrapeResult := collectorapi.ScrapeResult{}
	if c.SaveSamplesLocally != nil {
		done.Add(1) // wait until we've gotten the SampleLists

		// save aggregated metrics locally
		go func() {
			// read scrape results as they are aggregated -- required for aggregation not to block
			for m := range sCh {
				scrapeResult.Items = append(scrapeResult.Items, m)
			}
			done.Done()
		}()
	}

	c.collect(ch, sCh) // compute the metrics
	close(ch)          // all metrics written, stop reading them
	close(sCh)         // all scrape results written, stop reading them
	done.Wait()        // wait until all the metrics and scrapes have been read

	cm.endTime = time.Now()

	// Save local copies of the metrics
	savedLocalSuccessMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_save_local_success",
		Help: "The number of local metrics saved as json.",
	})
	savedLocalFailMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_save_local_fail",
		Help: "The number of local metrics saved as json.",
	})
	savedJSONLocalSuccessMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_save_json_local_success",
		Help: "The number of local metrics saved as json.",
	})
	savedJSONLocalFailMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_save_json_local_fail",
		Help: "The number of local metrics saved as json.",
	})
	defer func() {
		cm.metrics = append(cm.metrics,
			savedLocalSuccessMetric, savedLocalFailMetric,
			savedJSONLocalSuccessMetric, savedJSONLocalFailMetric)
	}()
	if c.SaveSamplesLocally != nil && len(scrapeResult.Items) > 0 {
		if err := c.NormalizeForSave(&scrapeResult); err != nil {
			log.Error(err, "unable to save aggregated metrics locally")
			savedLocalFailMetric.Inc()
			savedJSONLocalFailMetric.Inc()
		} else {
			// write the aggregated samples locally if configured to do so
			if c.SaveSamplesLocally != nil && pointer.BoolDeref(c.SaveSamplesLocally.SaveProto, false) && len(scrapeResult.Items) > 0 {
				if err := c.SaveScrapeResultToFile(&scrapeResult); err != nil {
					log.Error(err, "unable to save aggregated metrics locally")
					savedLocalFailMetric.Inc()
				} else {
					savedLocalSuccessMetric.Inc()
				}
			}

			// write the aggregated samples locally if configured to do so
			if c.SaveSamplesLocally != nil && pointer.BoolDeref(c.SaveSamplesLocally.SaveJSON, false) && len(scrapeResult.Items) > 0 {
				if err := c.SaveScrapeResultToJSONFile(&scrapeResult); err != nil {
					log.Error(err, "unable to save aggregated metrics locally as json")
					savedJSONLocalFailMetric.Inc()
				} else {
					savedJSONLocalSuccessMetric.Inc()
				}
			}
		}
	}

	return &cm
}

// Collect returns the current state of all metrics of the collector.
// This there are cached metrics, Collect will return the cached metrics.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	if !c.PreComputeMetrics.Enabled {
		c.collect(ch, nil)
		return
	}

	// write the cached metrics out as a response
	metrics := <-c.metrics
	for i := range metrics.metrics {
		ch <- metrics.metrics[i]
	}
	metrics.metrics = nil

	metricsAge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_metrics_age_seconds",
		Help: "The time since the metrics were computed.",
	})
	metricsAge.Set(float64(time.Since(metrics.endTime).Seconds()))
	metricsAge.Collect(ch)

	objectsAge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_object_age_seconds",
		Help: "The time since the objects were read to compute the metrics.",
	})
	objectsAge.Set(float64(time.Since(metrics.startTime).Seconds()))
	objectsAge.Collect(ch)

	cacheCollectTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_cache_collect_time_seconds",
		Help: "The time to collect the cached metrics.",
	})
	cacheCollectTime.Set(float64(time.Since(start).Seconds()))
	cacheCollectTime.Collect(ch)
}

func (c *Collector) getSideCarConfigs() ([]*collectorcontrollerv1alpha1.SideCarConfig, error) {
	var results []*collectorcontrollerv1alpha1.SideCarConfig
	for _, p := range c.SideCarConfigDirectoryPaths {
		entries, err := os.ReadDir(p)
		if err != nil {
			log.Error(err, "unable to read side car metrics directory", "directory", p)
			return nil, err
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), collectorcontrollerv1alpha1.SideCarConfigFileSuffix) {
				continue
			}
			b, err := os.ReadFile(filepath.Join(p, e.Name()))
			if err != nil {
				log.Error(err, "unable to read side car metrics file", "filename", e.Name())
				return nil, err
			}
			d := json.NewDecoder(bytes.NewBuffer(b))
			sc := collectorcontrollerv1alpha1.SideCarConfig{}
			err = d.Decode(&sc)
			if err != nil {
				log.Error(err, "unable to parse side car metrics file", "filename", e.Name())
				return nil, err
			}
			results = append(results, &sc)
		}
	}
	return results, nil
}

// CollectorFunc is the type of the metric collector functions accepted for collector overrides.
type CollectorFunc func(c *Collector, o *CapacityObjects, ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) error

// CollectorFuncs returns a copy of the map holding all the collector functions.
// This function is safe to be used concurrently.
func CollectorFuncs() map[string]CollectorFunc {
	collectorFuncs.RLock()
	defer collectorFuncs.RUnlock()

	m := make(map[string]CollectorFunc)
	for k, v := range collectorFuncs.values {
		m[k] = v
	}
	return m
}

// SetCollectorFuncs copies the given map and sets the current collector functions.
// This function is safe to be used concurrently.
func SetCollectorFuncs(fns map[string]CollectorFunc) {
	collectorFuncs.Lock()
	defer collectorFuncs.Unlock()

	collectorFuncs.values = make(map[string]CollectorFunc)
	for k, v := range fns {
		collectorFuncs.values[k] = v
	}
}

// collectorFuncs enables the modification of metric collection by adding new metrics or overriding existing ones.
var collectorFuncs = struct {
	sync.RWMutex
	values map[string]CollectorFunc
}{
	values: map[string]CollectorFunc{
		"collect_containers":     (*Collector).collectContainers,
		"collect_quotas":         (*Collector).collectQuota,
		"collect_nodes":          (*Collector).collectNodes,
		"collect_pvs":            (*Collector).collectPVs,
		"collect_pvcs":           (*Collector).collectPVCs,
		"collect_namespace":      (*Collector).collectNamespaces,
		"collect_cgroups":        (*Collector).collectCGroups,
		"collect_cluster_scoped": (*Collector).collectClusterScoped,
	},
}

// bindCollectorFuncs returns a map containing the collector functions in CollectorFuncs
// bound to the parameters given to this method.
func (c *Collector) bindCollectorFuncs(o *CapacityObjects, ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) map[string]func() error {
	m := make(map[string]func() error)

	fns := CollectorFuncs()
	for name := range fns {
		// This is necessary since `name` escapes this context via the lambda below.
		fn := fns[name]
		m[name] = func() error { return fn(c, o, ch, sCh) }
	}

	return m
}

// collect returns the current state of all metrics of the collector.
func (c *Collector) collect(ch chan<- prometheus.Metric, sCh chan *collectorapi.SampleList) {
	start := time.Now()
	o, err := c.listCapacityObjects(ch)
	if err != nil {
		log.Error(err, "failed to list objects")
		return
	}

	err = c.wait(c.bindCollectorFuncs(o, ch, sCh), ch)
	if err != nil {
		log.Error(err, "failed to collect container metrics")
	}

	for _, sc := range c.sideCarConfigs {
		for _, m := range sc.SideCarMetrics {
			if m.Help == "" {
				m.Help = c.Prefix + "_" + m.Name
			}
			d := prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: c.Prefix + "_" + m.Name,
				Help: m.Help,
			}, m.LabelNames)
			for _, v := range m.Values {
				d.WithLabelValues(v.MetricLabels...).Set(v.Value)
			}
			d.Collect(ch)
		}
	}

	log.V(1).Info("all collections complete", "seconds", time.Since(start).Seconds())
	latencyDesc := prometheus.NewDesc(
		c.Prefix+"_collection_latency_seconds", "collection latency", []string{}, nil)
	ch <- prometheus.MustNewConstMetric(
		latencyDesc, prometheus.GaugeValue, time.Since(start).Seconds())
}

func (c *Collector) collectPVs(o *CapacityObjects, ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) error {
	metrics := map[MetricName]*Metric{}
	for i := range o.PVs.Items {
		pv := &o.PVs.Items[i]

		// find the pvc if the volume has been claimed
		var pvc *corev1.PersistentVolumeClaim
		if ref := pv.Spec.ClaimRef; ref != nil {
			pvc = o.PVCsByName[ref.Namespace+"/"+ref.Name]
		}

		// local volumes have a hostname -- try to lookup the node in this case
		var node *corev1.Node
		if nodeName, ok := pv.Annotations["kubernetes.io/hostname"]; ok {
			node = o.NodesByName[nodeName]
		}

		l := LabelsValues{}
		c.Labeler.SetLabelsForPersistentVolume(&l, pv, pvc, node)
		values := c.Reader.GetValuesForPV(pv)
		for src, v := range values {
			// each resource -- e.g. capacity
			for r, alias := range c.Resources {
				q, ok := v.ResourceList[corev1.ResourceName(r)]
				if !ok {
					// we aren't interested in this resource -- skip
					continue
				}

				// initialize the metric
				name := MetricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "pv"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
					metrics[name] = m
				}

				// set the value for this set of labels
				if _, ok := m.Values[l]; ok {
					// already found an item here
					log.Info("duplicate value for pvs", "labels", l)
				}
				m.Values[l] = []resource.Quantity{q}
			}
		}
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.PVType) {
		c.AggregateAndCollect(a, metrics, ch, sCh)
	}

	c.UtilizationServer.Collect(ch, o.UtilizationByNode) // metrics on the utilization data health

	return nil
}

func (c *Collector) collectPVCs(o *CapacityObjects, ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) error {
	metrics := map[MetricName]*Metric{}
	// each PVC
	for i := range o.PVCs.Items {
		pvc := &o.PVCs.Items[i]
		l := LabelsValues{}
		n := o.NamespacesByName[pvc.Namespace]
		pv := o.PVsByName[pvc.Spec.VolumeName]

		var p *corev1.Pod
		if pods := o.PodsByPVC[pvc.Namespace+"/"+pvc.Name]; len(pods) == 1 {
			p = o.PodsByPVC[pvc.Namespace+"/"+pvc.Name][0]
		}
		wl := workload{}
		var node *corev1.Node
		if p != nil {
			wl = getWorkloadForPod(c.Client, p)
			node = o.NodesByName[p.Spec.NodeName]
		}
		c.Labeler.SetLabelsForPersistentVolumeClaim(&l, pvc, pv, n, p, wl, node)

		values := c.Reader.GetValuesForPVC(pvc)

		// each source -- e.g. pvc_requests, pvc_limits, pvc_capacity
		for src, v := range values {
			// each resource -- e.g. capacity
			for r, alias := range c.Resources {
				q, ok := v.ResourceList[corev1.ResourceName(r)]
				if !ok {
					// we aren't interested in this resource -- skip
					continue
				}

				// initialize the metric
				name := MetricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "pvc"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
					metrics[name] = m
				}

				// set the value for this set of labels
				if _, ok := m.Values[l]; ok {
					// already found an item here
					log.Info("duplicate value for pvcs", "labels", l)
				}
				m.Values[l] = []resource.Quantity{q}
			}
		}
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.PVCType) {
		c.AggregateAndCollect(a, metrics, ch, sCh)
	}
	return nil
}

func (c *Collector) collectQuota(o *CapacityObjects, ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) error {
	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.QuotaType) // For saving locally

	metrics := map[MetricName]*Metric{}

	for i := range o.Quotas.Items {
		q := &o.Quotas.Items[i]

		// get the labels for this quota
		key := ResourceQuotaDescriptorKey{
			Name:      q.Name,
			Namespace: q.Namespace,
		}
		l := LabelsValues{}
		c.Labeler.SetLabelsForQuota(&l, q, o.RQDsByRQDKey[key], o.NamespacesByName[q.Namespace])
		sample := sb.NewSample(l) // For saving locally

		values := c.Reader.GetValuesForQuota(q, o.RQDsByRQDKey[key], c.BuiltIn.EnableResourceQuotaDescriptor)

		// find the sources + resource we care about and add them to the metrics
		for src, v := range values {
			if src == collectorcontrollerv1alpha1.QuotaItemsSource {
				name := MetricName{Source: collectorcontrollerv1alpha1.QuotaItemsSource,
					ResourceAlias: collectorcontrollerv1alpha1.ItemsResource,
					Resource:      collectorcontrollerv1alpha1.ItemsResource,
					SourceType:    "quota"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
					metrics[name] = m
				}
				m.Values[l] = []resource.Quantity{*resource.NewQuantity(1, resource.DecimalSI)}
				continue
			}
			for r, alias := range c.Resources { // resource names are are interested in
				qty, ok := v.ResourceList[corev1.ResourceName(r)]
				if !ok {
					// we didn't find this resource -- skip
					continue
				}
				// if this is a storage resource, set pvc storage class label here
				if strings.Contains(r, "storageclass.storage.k8s.io") {
					c.Labeler.SetLabelsForPVCQuota(&l, q, r)
				}
				// initialize the metric
				name := MetricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "quota"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
					metrics[name] = m
				}

				// set the value for this set of labels
				if _, ok := m.Values[l]; ok {
					// already found an item here
					log.Info("duplicate value for quota", "labels", l)
				}
				m.Values[l] = []resource.Quantity{qty}
				sb.AddQuantityValues(sample, r, src, qty) // For saving locally
			}
		}
	}

	if err := sb.SaveSamplesToFile(); err != nil {
		log.Error(err, "unable to save metrics locally")
		// don't return
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.QuotaType) {
		c.AggregateAndCollect(a, metrics, ch, sCh)
	}
	return nil
}

// getCGroupMetricSource find the name of the metric for this level
func (c *Collector) getCGroupMetricSource(cgroup string) string {
	if cgroup == "" {
		// special case root cgroup -- there is no parent directory
		return c.CGroupMetrics.RootSource.Name
	}
	return c.CGroupMetrics.Sources[filepath.Dir("/"+cgroup)].Name
}

func (c *Collector) collectCGroups(o *CapacityObjects, ch chan<- prometheus.Metric,
	sCh chan<- *collectorapi.SampleList) error {
	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.CGroupType) // Save locally

	resultMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_cgroup_usage_result",
		Help: "The number of containers missing usage information",
	}, []string{"exported_node", "sampler_pod", "sampler_phase", "found", "reason"})

	utilization := o.UtilizationByNode
	metrics := map[MetricName]*Metric{}

	// get metrics from each node
	for i := range o.Nodes.Items {
		n := &o.Nodes.Items[i]

		l := LabelsValues{}
		c.Labeler.SetLabelsForNode(&l, n)

		var samplerPodName, samplerPodPhase string
		if p, ok := o.SamplersByNode[n.Name]; ok {
			samplerPodName = p.Name
			samplerPodPhase = string(p.Status.Phase)
		} else {
			samplerPodName = "unknown"
			samplerPodPhase = "unknown"
		}

		u, ok := utilization[n.Name]
		if !ok {
			reason := c.nodeMissingReason(n, o)
			resultMetric.WithLabelValues(n.Name, samplerPodName, samplerPodPhase, "false", reason).Add(1)
			continue
		}
		resultMetric.WithLabelValues(n.Name, samplerPodName, samplerPodPhase, "true", "").Add(1)

		for _, m := range u.Node.AggregatedMetrics { // each of the cgroups we aggregate at
			func(l LabelsValues) {
				c.Labeler.SetLabelsFoCGroup(&l, m) // set the cgroup label to the base from the cgroup
				sample := sb.NewSample(l)          // For saving locally

				// get the source name for this level in the hiearchy
				src := c.getCGroupMetricSource(m.AggregationLevel)
				if src == "" {
					// this cgroup isn't one we are tracking
					return
				}

				// record cpu
				alias := c.Resources["cpu"]
				if alias == "" {
					alias = "cpu_cores"
				}
				// find the metric, initialize if necessary
				// the metric source is a function of the cgroup parent directory (filepath.Dir) while
				// the metric cgroup label is a function of the cgroup base directory (filepath.Base)
				name := MetricName{Source: src, ResourceAlias: alias, Resource: "cpu", SourceType: "cgroup"}
				metric, ok := metrics[name]
				if !ok {
					metric = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
					metrics[name] = metric
				}
				// get the values for this metric
				cpuValues := make([]resource.Quantity, 0, len(m.CpuCoresNanoSec))
				for _, v := range m.CpuCoresNanoSec {
					cpuValues = append(cpuValues, *resource.NewScaledQuantity(v, resource.Nano))
				}
				metric.Values[l] = cpuValues

				// record memory
				alias = c.Resources["memory"]
				if alias == "" {
					alias = "memory_bytes"
				}
				// find the metric, initialize if necessary
				name = MetricName{Source: src, ResourceAlias: alias, Resource: "memory", SourceType: "cgroup"}
				metric, ok = metrics[name]
				if !ok {
					metric = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
					metrics[name] = metric
				}
				// get the values for this metric
				memoryValues := make([]resource.Quantity, 0, len(m.MemoryBytes))
				for _, v := range m.MemoryBytes {
					memoryValues = append(memoryValues, *resource.NewQuantity(v, resource.DecimalSI))
				}
				metric.Values[l] = memoryValues

				sb.AddIntValues(sample, "memory", src, m.MemoryBytes...)  // For saving locally
				sb.AddIntValues(sample, "cpu", src, m.CpuCoresNanoSec...) // For saving locally
			}(l)
		}
	}

	if err := sb.SaveSamplesToFile(); err != nil {
		log.Error(err, "unable to save metrics locally")
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.CGroupType) {
		c.AggregateAndCollect(a, metrics, ch, sCh)
	}

	resultMetric.Collect(ch)
	return nil
}

func (c *Collector) collectNodes(o *CapacityObjects, ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) error {
	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.NodeType) // For saving locally

	metrics := map[MetricName]*Metric{}

	for i := range o.Nodes.Items {
		n := &o.Nodes.Items[i]

		// get the labels for this quota
		l := LabelsValues{}
		c.Labeler.SetLabelsForNode(&l, n)
		sample := sb.NewSample(l) // For saving locally

		// get the values for this quota
		values := c.Reader.GetValuesForNode(n, o.PodsByNodeName[n.Name])

		// find the sources + resource we care about and add them to the metrics
		for src, v := range values {
			for r, alias := range c.Resources { // resource names are are interested in
				q, ok := v.ResourceList[corev1.ResourceName(r)]
				if !ok {
					// we aren't interested in this resource -- skip
					continue
				}

				// initialize the metric
				name := MetricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "node"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
					metrics[name] = m
				}

				// set the value for this set of labels
				if _, ok := m.Values[l]; ok {
					// already found an item here
					log.Info("duplicate value for nodes", "labels", l)
				}
				m.Values[l] = []resource.Quantity{q}
				sb.AddQuantityValues(sample, r, src, q) // For saving locally
			}
		}
	}

	if err := sb.SaveSamplesToFile(); err != nil {
		log.Error(err, "unable to save metrics locally")
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.NodeType) {
		c.AggregateAndCollect(a, metrics, ch, sCh)
	}
	return nil
}

func (c *Collector) collectNamespaces(o *CapacityObjects, ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) error {
	metrics := map[MetricName]*Metric{}
	for i := range o.Namespaces.Items {
		n := &o.Namespaces.Items[i]

		// get the labels for this quota
		l := LabelsValues{}
		c.Labeler.SetLabelsForNamespaces(&l, n)

		name := MetricName{Source: collectorcontrollerv1alpha1.NamespaceItemsSource,
			ResourceAlias: collectorcontrollerv1alpha1.ItemsResource,
			Resource:      collectorcontrollerv1alpha1.ItemsResource,
			SourceType:    "namespace",
		}
		m, ok := metrics[name]
		if !ok {
			m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
			metrics[name] = m
		}
		m.Values[l] = []resource.Quantity{*resource.NewQuantity(1, resource.DecimalSI)}
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.NamespaceType) {
		c.AggregateAndCollect(a, metrics, ch, sCh)
	}
	return nil
}

func getContainerNameToID(pod *corev1.Pod) (map[string]string, string) {
	containerNameToID := map[string]string{}
	sort.Slice(pod.Status.ContainerStatuses, func(i, j int) bool {
		if pod.Status.ContainerStatuses[i].State.Running != nil && pod.Status.ContainerStatuses[j].State.Running == nil {
			return true
		}
		if pod.Status.ContainerStatuses[i].State.Running == nil && pod.Status.ContainerStatuses[j].State.Running != nil {
			return false
		}
		if pod.Status.ContainerStatuses[i].State.Running != nil && pod.Status.ContainerStatuses[j].State.Running != nil {
			return pod.Status.ContainerStatuses[i].State.Running.StartedAt.After(
				pod.Status.ContainerStatuses[j].State.Running.StartedAt.Time)
		}
		return false
	})
	for i := range pod.Status.ContainerStatuses {
		c := pod.Status.ContainerStatuses[i]

		// strip the non-id portion of the prefix -- e.g. containerd://
		id := c.ContainerID
		if index := strings.Index(c.ContainerID, "://"); index >= 0 {
			id = id[index+3:]
		}
		if _, ok := containerNameToID[c.Name]; !ok {
			containerNameToID[c.Name] = id
			continue
		}
		if c.State.Running != nil {
			log.Info("found duplicate container status",
				"namespace", pod.Namespace, "pod", pod.Name, "container", c.Name,
				"old-id", containerNameToID[c.Name], "new-id", id,
			)
			containerNameToID[c.Name] = id
		}
	}
	podUID := string(pod.UID)
	if mirror, ok := pod.Annotations[corev1.MirrorPodAnnotationKey]; ok {
		podUID = mirror
	}
	return containerNameToID, podUID
}

func normalizeContainerID(id string) string {
	if index := strings.Index(id, "://"); index >= 0 {
		id = id[index+3:]
	}
	return id
}

const maxWaitTimeForUtilization = time.Minute

func (c *Collector) collectContainers(o *CapacityObjects, ch chan<- prometheus.Metric,
	sCh chan<- *collectorapi.SampleList) error {
	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.ContainerType) // Save locally

	start := time.Now()
	running := time.Since(c.startTime)

	log := log.WithValues("start", start.Local().Format("2006-01-02 15:04:05"))
	utilization := c.UtilizationServer.GetContainerUsageSummary(o.UtilizationByNode)

	// metrics
	containerMetrics := map[MetricName]*Metric{}
	podMetrics := map[MetricName]*Metric{}

	// debug state
	results := map[string]int{}
	nodeMissing := map[string]int{}
	haveUsageForContainers := sets.NewString()
	allContainers := sets.NewString()

	resultMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_container_usage_result",
		Help: "The number of containers missing usage information",
	}, []string{"exported_node", "sampler_pod", "sampler_phase", "reason", "found"})

	nodeUtilizationMissing := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_node_utilization_missing",
		Help: "The nodes that we are missing utilization data from.",
	}, []string{"exported_node", "sampler_pod", "sampler_phase"})

	for i := range o.Pods.Items {
		pod := &o.Pods.Items[i]

		// compute the metric labels for this pod
		podLabels := LabelsValues{}

		wl := getWorkloadForPod(c.Client, pod)
		namespace := o.NamespacesByName[pod.Namespace]
		node := o.NodesByName[pod.Spec.NodeName]
		c.Labeler.SetLabelsForPod(&podLabels, pod, wl, node, namespace)

		// collect pod values
		values := c.Reader.GetValuesForPod(pod)

		for src, v := range values { // sources we are interested in
			for r, alias := range c.Resources { // resource names are are interested in
				q, ok := v.ResourceList[corev1.ResourceName(r)]
				if !ok {
					// container doesn't have values for this compute resource type -- skip it
					continue
				}

				// get the metric for this level + source + resource + operation
				name := MetricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "pod"}
				m, ok := podMetrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
					podMetrics[name] = m
				}
				// set the value
				if _, ok := m.Values[podLabels]; ok {
					// already found an item here
					log.Info("duplicate value for pods", "labels", podLabels)
				}
				m.Values[podLabels] = []resource.Quantity{q}
			}
		}

		// the utilization values on give the containerID, not the container name
		// use the Pod containerStatuses field to map the container name to a containerID.
		containerNameToID, podUID := getContainerNameToID(pod)

		var samplerPodName, samplerPodPhase string
		if samplerPod, ok := o.SamplersByNode[pod.Spec.NodeName]; ok {
			samplerPodName = samplerPod.Name
			samplerPodPhase = string(samplerPod.Status.Phase)
		}

		// Values are in the containers
		for i := range pod.Spec.Containers {
			// compute the labels based on the container -- copy the pod labels
			// and set the container values on the copy
			container := &pod.Spec.Containers[i]
			containerLabels := podLabels
			c.Labeler.SetLabelsForContainer(&containerLabels, container)
			sample := sb.NewSample(containerLabels) // For saving locally

			id := sampler.ContainerKey{ContainerID: containerNameToID[container.Name], PodUID: podUID}
			allContainers.Insert(string(id.PodUID) + "/" + string(id.ContainerID))

			usage := utilization[id]
			if usage != nil {
				log.V(2).Info("found usage for container", "namespace", pod.Namespace,
					"pod", pod.Name, "container", container.Name, "id", id,
					"nodeName", pod.Spec.NodeName, "nodeID", usage.NodeName,
					"cpu", usage.CpuCoresNanoSec, "memory", usage.MemoryBytes,
					"throttling", usage.CpuPercentPeriodsThrottled)
				resultMetric.WithLabelValues(pod.Spec.NodeName, samplerPodName, samplerPodPhase, "", "true").Add(1)
				results["found"]++
			} else if running > maxWaitTimeForUtilization {
				reason, ok := c.containerMissingReason(pod, container, id, o)
				results[reason]++

				resultMetric.WithLabelValues(pod.Spec.NodeName, samplerPodName, samplerPodPhase, reason, "false").Add(1)

				if !ok {
					nodeUtilizationMissing.WithLabelValues(pod.Spec.NodeName, samplerPodName, samplerPodPhase).Inc()
					nodeMissing[pod.Spec.NodeName]++
				}

				// if we can't find utilization information for a container that should have samples
				// log it so we can follow up to figure out what is going on
				if reason == "unknown" {
					log.Info("unable to find usage for container",
						"reason", reason,
						"node", pod.Spec.NodeName,
						"sampler_pod", o.SamplersByNode[pod.Spec.NodeName],
						"id", id,
						"namespace", pod.Namespace,
						"pod", pod.Name,
						"container", container.Name,
						"created", pod.CreationTimestamp,
					)
				}
			}

			// get the quantities from the container -- there will be 1 value for
			// each unique source
			values := c.Reader.GetValuesForContainer(container, pod, usage)

			for src, v := range values { // sources we are interested in
				for r, alias := range c.Resources { // resource names are are interested in
					if v.ResourceList != nil {
						q, ok := v.ResourceList[corev1.ResourceName(r)]
						if !ok {
							// container doesn't have values for this compute resource type -- skip it
							continue
						}

						// get the metric for this level + source + resource + operation
						name := MetricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "container"}
						m, ok := containerMetrics[name]
						if !ok {
							m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
							containerMetrics[name] = m
						}
						// set the value
						if _, ok := m.Values[containerLabels]; ok {
							// already found an item here
							log.Info("duplicate value for containers", "labels", containerLabels)
						}
						m.Values[containerLabels] = []resource.Quantity{q}
						sb.AddQuantityValues(sample, r, src, q) // For saving locally
					} else if v.MultiResourceList != nil {
						// TODO: figure out a way to factor this so it is clean with less duplication across the file
						q, ok := v.MultiResourceList[corev1.ResourceName(r)]
						if !ok {
							// container doesn't have values for this compute resource type -- skip it
							continue
						}

						// get the metric for this level + source + resource + operation
						name := MetricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "container"}
						m, ok := containerMetrics[name]
						if !ok {
							m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
							containerMetrics[name] = m
						}
						// set the value
						if _, ok := m.Values[containerLabels]; ok {
							// already found an item here
							log.Info("duplicate value for containers", "labels", containerLabels)
						}
						m.Values[containerLabels] = q
						sb.AddQuantityValues(sample, r, src, q...) // For saving locally
					}
				}
			}
		}
	}

	if err := sb.SaveSamplesToFile(); err != nil {
		log.Error(err, "unable to save metrics locally")
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.ContainerType) {
		c.AggregateAndCollect(a, containerMetrics, ch, sCh)
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.PodType) {
		c.AggregateAndCollect(a, podMetrics, ch, sCh)
	}
	for id := range utilization {
		haveUsageForContainers.Insert(id.PodUID + "/" + id.ContainerID)
	}

	if log.V(1).Enabled() {
		containersWithoutUsage := allContainers.Difference(haveUsageForContainers).List()
		usageWithoutContainers := haveUsageForContainers.Difference(allContainers).List()
		log.V(2).Info("containers without usage", "containers", containersWithoutUsage)
		log.V(2).Info("usage without containers", "containers", usageWithoutContainers)

		log.V(1).Info("finished container collection",
			"latency-seconds", time.Since(start).Seconds(),
			"pod-count", len(o.Pods.Items),
			"container-count", allContainers.Len(),
			"containers-missing-usage-count", len(containersWithoutUsage),
			"usage-without-containers-count", len(usageWithoutContainers),
			"container-results", results,
			"node-missing", nodeMissing,
		)
	}

	// Report results for monitoring health of usage collection
	resultMetric.Collect(ch)
	nodeUtilizationMissing.Collect(ch)

	latencyMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_collect_containers_latency_seconds",
		Help: "The number of seconds to collect containers",
	})
	latencyMetric.Set(time.Since(start).Seconds())
	latencyMetric.Collect(ch)

	podCountMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_pods_collected",
		Help: "The number of pods collected during the last collect",
	})
	podCountMetric.Set(float64(len(o.Pods.Items)))
	podCountMetric.Collect(ch)
	return nil
}

func (c *Collector) nodeMissingReason(node *corev1.Node, o *CapacityObjects) string {
	if _, ok := o.UtilizationByNode[node.Name]; ok {
		// make sure it is actually missing
		return "found"
	}
	// check if the node is unhealthy before looking at the pod
	for _, condition := range c.UtilizationServer.UnhealthyNodeConditions { // conditions are in outer loop so we prioritize their ordering
		for _, v := range node.Status.Conditions {
			if condition.Type == string(v.Type) && condition.Status == string(v.Status) {
				return "node-" + strings.ToLower(string(v.Type)+"-"+string(v.Status))
			}
		}
	}

	// check if the sampler pod is healthy
	p, ok := o.SamplersByNode[node.Name]
	if !ok {
		// no sampler pod for this node found -- unexpected
		return "sampler-pod-missing"
	}
	if p.DeletionTimestamp != nil {
		return "sampler-pod-status-" + strings.ToLower(string(p.Status.Phase)) + "-terminated"
	}

	for _, i := range p.Status.ContainerStatuses {
		if i.Name != "metrics-node-sampler" {
			continue
		}
		if i.State.Running != nil && time.Since(i.State.Running.StartedAt.Time) < time.Minute*5 {
			// we may not have gotten a sample yet since it just started
			return "sampler-container-recently-running"
		}
		if i.State.Terminated != nil {
			return "sampler-container-terminated"
		}
		if i.State.Waiting != nil {
			return "sampler-container-waiting"
		}
		if time.Since(i.State.Running.StartedAt.Time) < time.Minute*5 {
			return "sampler-container-recently-started"
		}
		break
	}

	// we don't have the metrics, and the node is healthy and the pod is running
	// we don't know why it isn't giving us
	return "unknown"

}

// containerMissingReason returns the string reason a container values are missing,
// and the bool value for if the metrics are found for the node the pod is running
// on (true if found)
func (c *Collector) containerMissingReason(
	pod *corev1.Pod, container *corev1.Container,
	id sampler.ContainerKey, o *CapacityObjects) (string, bool) {

	// microvms aren't supported yet
	if pod.Spec.RuntimeClassName != nil && *pod.Spec.RuntimeClassName == "microvm" {
		return "container-pod-microvm", true
	}

	// pod isn't running -- don't expect metrics
	if pod.Status.Phase != corev1.PodRunning {
		return "container-pod-phase-" + strings.ToLower(string(pod.Status.Phase)), true
	}

	// check if container is running
	for _, i := range pod.Status.ContainerStatuses {
		// find the container status for this container
		if normalizeContainerID(i.ContainerID) != id.ContainerID {
			continue
		}
		if i.State.Terminated != nil {
			return "container-terminated", true
		}
		if i.State.Waiting != nil {
			return "container-waiting", true
		}
		if i.State.Running == nil {
			return "container-not-running", true
		}
		if time.Since(i.State.Running.StartedAt.Time) < maxWaitTimeForUtilization {
			return "container-recently-started", true
		}
		break
	}

	if _, ok := o.UtilizationByNode[pod.Spec.NodeName]; !ok {
		// no metrics found for the node this container is on
		return "no-metrics-for-node", false
	}

	return "node-missing-container", false
}

type MissingContainerID struct {
	PodNamespace  string
	PodName       string
	ContainerName string
	NodeName      string
	ID            sampler.ContainerKey
}

var clusterScopedListResultMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "metrics_prometheus_collector_cluster_scoped_list_result",
	Help: "The number of list operations performed for cluster-scoped metric sources",
}, []string{"name", "reason"})

// listClusterScoped lists the configured cluster-scoped collections into the
// given capacityObjects or returns an error.
func (c *Collector) listClusterScoped(o *CapacityObjects) error {
	for ii := range c.ClusterScopedMetrics.AnnotatedCollectionSources {
		source := c.ClusterScopedMetrics.AnnotatedCollectionSources[ii]

		// construct partial object metadata and list, map back to name of source
		var list = &metav1.PartialObjectMetadataList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   source.Group,
			Version: source.Version,
			Kind:    source.Kind,
		})

		err := c.Client.List(context.Background(), list)
		if err != nil {
			log.Error(err, "error listing cluster-scoped resource", "group", source.Group, "kind", source.Kind)
			clusterScopedListResultMetric.WithLabelValues(source.Name, err.Error()).Add(1)
			continue
		}

		clusterScopedListResultMetric.WithLabelValues(source.Name, "").Add(1)
		o.ClusterScopedByName[source.Name] = list
	}

	return nil
}

// collectClusterScoped collects the configured cluster-scoped metrics.
func (c *Collector) collectClusterScoped(o *CapacityObjects, ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) error {
	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.ClusterScopedType) // Save locally

	itemsMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_cluster_scoped_items",
		Help: "The number of collection items discovered for each cluster-scoped metric source",
	}, []string{"name", "reason"})

	resultMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_cluster_scoped_labeled_resources",
		Help: "The number of labeledResources discovered for each item in a cluster-scoped metric source collection",
	}, []string{"name", "resource_name", "reason"})

	metrics := map[MetricName]*Metric{}

	for ii := range c.ClusterScopedMetrics.AnnotatedCollectionSources {
		source := c.ClusterScopedMetrics.AnnotatedCollectionSources[ii]
		log := log.WithValues("source", source.Name)

		list, ok := o.ClusterScopedByName[source.Name]
		if !ok {
			log.V(2).Info("list items", "items", 0)
			itemsMetric.WithLabelValues(source.Name, "objects not found").Add(1)
			continue
		}
		log.V(2).Info("list items", "items", len(list.Items))
		itemsMetric.WithLabelValues(source.Name, "").Add(float64(len(list.Items)))

		for jj := range list.Items {
			obj := list.Items[jj]
			for k, v := range obj.GetAnnotations() {
				if source.Annotation != k {
					continue
				}

				log := log.WithValues("item", obj.Name)

				log.V(2).Info("found annotation", "annotation", source.Annotation)
				labeledResources := []collectorcontrollerv1alpha1.LabeledResources{}
				err := yaml.UnmarshalStrict([]byte(v), &labeledResources)
				if err != nil {
					log.V(2).Error(err, "error unmarshalling annotation")
					resultMetric.WithLabelValues(source.Name, obj.GetName(), err.Error()).Add(1)
					continue
				}
				resultMetric.WithLabelValues(source.Name, obj.GetName(), "").Add(1)

				log.V(2).Info("examining labeled resources", "count", len(labeledResources))
				for ii := range labeledResources {
					entry := labeledResources[ii]
					l := LabelsValues{}
					c.Labeler.SetLabelsForClusterScoped(&l, entry.Labels)

					// iterate over the resource names we're interested in
					for r, alias := range c.Resources {
						qty, ok := entry.Values[corev1.ResourceName(r)]
						if !ok {
							log.V(2).Info("didn't find resource", "resource", r)
							// we didn't find this resource
							continue
						}

						log.V(2).Info("found resource", "resource", r, "value", qty)
						name := MetricName{
							Source:        source.Name,
							ResourceAlias: alias,
							Resource:      r,
							SourceType:    collectorcontrollerv1alpha1.ClusterScopedType,
						}

						m, ok := metrics[name]
						if !ok {
							m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
							metrics[name] = m
						}

						m.Values[l] = []resource.Quantity{qty}
					}
				}
			}
		}
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.ClusterScopedType) {
		c.AggregateAndCollect(a, metrics, ch, sCh)
	}

	clusterScopedListResultMetric.Collect(ch)
	itemsMetric.Collect(ch)
	resultMetric.Collect(ch)

	if err := sb.SaveSamplesToFile(); err != nil {
		log.Error(err, "unable to save metrics locally")
	}

	return nil
}

// AggregateAndCollect aggregates the metrics at each level and then collects them
func (c *Collector) AggregateAndCollect(
	a *collectorcontrollerv1alpha1.Aggregation, m map[MetricName]*Metric,
	ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) {

	levels := a.Levels
	sources := sets.NewString(a.Sources.GetSources()...)

	// filter the sources we aggregate
	metrics := make(map[MetricName]*Metric, len(m))
	for k, v := range m {
		if !sources.Has(k.Source) {
			continue
		}
		metrics[k] = v
	}

	wg := &sync.WaitGroup{}
	for k, v := range metrics {
		wg.Add(1)
		go func(name MetricName, metric Metric) {
			// aggregate and collect the metric for each level
			for i, l := range levels {
				// aggregate at this level
				aggregatedName := MetricName{
					Prefix:        c.Prefix,
					Level:         l.Mask.Level,
					Operation:     string(l.Operation),
					Source:        name.Source,
					ResourceAlias: name.ResourceAlias,
					Resource:      name.Resource,
					SourceType:    name.SourceType,
				}

				if len(l.Operations) > 0 && i == len(levels)-1 {
					// Error -- only support terminal levels with operations
				}

				aggregatedMetric := c.aggregateMetric(append(l.Operations, l.Operation), metric, l.Mask)
				aggregatedMetric.Name = aggregatedName
				metric = aggregatedMetric

				if l.RetentionName != "" && c.SaveSamplesLocally != nil { // export this metric to a local file for retention
					slb := c.NewAggregatedSampleListBuilder(aggregatedName.SourceType)
					slb.SampleList.MetricName = aggregatedName.String()
					slb.SampleList.Name = l.RetentionName
					slb.Mask = l.Mask
					for mk, mv := range metric.Values {
						s := slb.NewSample(mk)
						s.Operation = aggregatedName.Operation
						s.Level = aggregatedName.Level
						slb.AddQuantityValues(s,
							aggregatedName.Resource,
							aggregatedName.Source, mv...)
					}
					sCh <- slb.SampleList
				}

				if l.NoExport {
					// aggregate the metric, but don't export it
					continue
				}

				// export the metric
				if l.Operation == collectorcontrollerv1alpha1.HistogramOperation {
					metric.Buckets = l.HistogramBuckets
					c.collectHistogramMetric(metric, ch)
				} else {
					c.collectMetric(metric, ch)
				}
			}
			wg.Done()
		}(k, *v)
	}
	wg.Wait()
}

// ResourceQuotaDescriptorKey is used as a key for priority class -
// namespace ResourceQuotaDescriptor pair
type ResourceQuotaDescriptorKey struct {
	Name      string
	Namespace string
}

// CapacityObjects are the CapacityObjects used for computing metrics
type CapacityObjects struct {
	Pods       corev1.PodList
	Nodes      corev1.NodeList
	Namespaces corev1.NamespaceList
	Quotas     corev1.ResourceQuotaList
	RQDs       quotamanagementv1alpha1.ResourceQuotaDescriptorList
	PVCs       corev1.PersistentVolumeClaimList
	PVs        corev1.PersistentVolumeList
	Samplers   corev1.PodList

	NamespacesByName map[string]*corev1.Namespace
	PodsByNodeName   map[string][]*corev1.Pod
	PodsByPVC        map[string][]*corev1.Pod
	PVsByName        map[string]*corev1.PersistentVolume
	PVCsByName       map[string]*corev1.PersistentVolumeClaim
	NodesByName      map[string]*corev1.Node
	RQDsByRQDKey     map[ResourceQuotaDescriptorKey]*quotamanagementv1alpha1.ResourceQuotaDescriptor
	SamplersByNode   map[string]*corev1.Pod

	ClusterScopedByName map[string]*metav1.PartialObjectMetadataList

	UtilizationByNode map[string]*samplerapi.ListMetricsResponse
}

// listCapacityObjects gets all the objects used for computing metrics
func (c *Collector) listCapacityObjects(ch chan<- prometheus.Metric) (*CapacityObjects, error) {
	// read the objects in parallel

	var o CapacityObjects
	o.ClusterScopedByName = map[string]*metav1.PartialObjectMetadataList{}

	waitFor := map[string]func() error{
		"list_pods":           func() error { return c.Client.List(context.Background(), &o.Pods) },
		"list_nodes":          func() error { return c.Client.List(context.Background(), &o.Nodes) },
		"list_namespaces":     func() error { return c.Client.List(context.Background(), &o.Namespaces) },
		"list_quotas":         func() error { return c.Client.List(context.Background(), &o.Quotas) },
		"list_pvcs":           func() error { return c.Client.List(context.Background(), &o.PVCs) },
		"list_pvs":            func() error { return c.Client.List(context.Background(), &o.PVs) },
		"list_cluster_scoped": func() error { return c.listClusterScoped(&o) },
		"list_sampler_pods": func() error { // fetch the list of sampler pods
			return c.Client.List(
				context.Background(), &o.Samplers,
				client.MatchingLabels(c.UtilizationServer.SamplerPodLabels),
				client.InNamespace(c.UtilizationServer.SamplerNamespaceName),
			)
		},
	}

	if c.MetricsPrometheusCollector.BuiltIn.EnableResourceQuotaDescriptor {
		waitFor["list_rqds"] = func() error { return c.Client.List(context.Background(), &o.RQDs) }
	}

	err := c.wait(waitFor, ch)
	if err != nil {
		return nil, err
	}

	o.PVsByName = make(map[string]*corev1.PersistentVolume, len(o.PVs.Items))
	for i := range o.PVs.Items {
		pv := &o.PVs.Items[i]
		o.PVsByName[pv.Name] = pv
	}

	o.PVCsByName = make(map[string]*corev1.PersistentVolumeClaim, len(o.PVCs.Items))
	for i := range o.PVCs.Items {
		pvc := &o.PVCs.Items[i]
		o.PVCsByName[pvc.Namespace+"/"+pvc.Name] = pvc
	}

	// index the namespaces by name
	o.NamespacesByName = make(map[string]*corev1.Namespace, len(o.Namespaces.Items))
	for i := range o.Namespaces.Items {
		o.NamespacesByName[o.Namespaces.Items[i].Name] = &o.Namespaces.Items[i]
	}

	// index the nodes by name
	o.NodesByName = make(map[string]*corev1.Node, len(o.Nodes.Items))
	for i := range o.Nodes.Items {
		o.NodesByName[o.Nodes.Items[i].Name] = &o.Nodes.Items[i]
	}

	// index the resource quota descriptors by name
	o.RQDsByRQDKey = make(map[ResourceQuotaDescriptorKey]*quotamanagementv1alpha1.ResourceQuotaDescriptor,
		len(o.RQDs.Items))
	for i := range o.RQDs.Items {
		key := ResourceQuotaDescriptorKey{
			Name:      o.RQDs.Items[i].Name,
			Namespace: o.RQDs.Items[i].Namespace,
		}
		o.RQDsByRQDKey[key] = &o.RQDs.Items[i]
	}

	// index the pods by node name
	o.PodsByNodeName = make(map[string][]*corev1.Pod, len(o.Nodes.Items))
	o.PodsByPVC = make(map[string][]*corev1.Pod, len(o.PVCs.Items))
	for i := range o.Pods.Items {
		p := &o.Pods.Items[i]
		o.PodsByNodeName[p.Spec.NodeName] = append(
			o.PodsByNodeName[p.Spec.NodeName], p)
		for j := range p.Spec.Volumes {
			if p.Spec.Volumes[j].PersistentVolumeClaim == nil {
				continue
			}
			c := p.Namespace + "/" + p.Spec.Volumes[j].PersistentVolumeClaim.ClaimName
			o.PodsByPVC[c] = append(o.PodsByPVC[c], &o.Pods.Items[i])
		}
	}

	// index the sampler pods
	o.SamplersByNode = make(map[string]*corev1.Pod, len(o.Samplers.Items))
	for i := range o.Samplers.Items {
		p := &o.Samplers.Items[i]
		if p.Spec.NodeName != "" {
			o.SamplersByNode[p.Spec.NodeName] = p
			continue
		}

		// pod isn't scheduled yet -- look at it's affinity to find the node
		if p.Spec.Affinity == nil || p.Spec.Affinity.NodeAffinity == nil ||
			p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil ||
			len(p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) != 1 ||
			len(p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchFields) != 1 ||
			len(p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchFields[0].Values) != 1 {
			log.Info("missing node affinity for pod", "pod", p.Name)
			continue
		}
		nodeName := p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchFields[0].Values[0]
		if _, ok := o.SamplersByNode[nodeName]; ok && p.DeletionTimestamp != nil {
			continue // keep the non-deleted pod
		}
		o.SamplersByNode[nodeName] = p
	}

	// get this once since it requires locking the cache
	o.UtilizationByNode = c.UtilizationServer.GetMetrics()
	return &o, nil
}

// wait runs all of the fns in go routines and waits for them to complete.
// returns the first error encountered, or nil if no errors
func (c *Collector) wait(fns map[string]func() error, metricCh chan<- prometheus.Metric) error {
	ch := make(chan error, len(fns))
	wg := &sync.WaitGroup{}
	for k, v := range fns {
		// run each function
		name := k
		fn := v
		wg.Add(1)
		go func() {
			start := time.Now()
			// write the result to a channel
			ch <- fn()

			if metricCh != nil {
				// record how long it took
				log.V(2).Info("async operation complete", "name", name, "seconds", time.Since(start).Seconds())
				opLatencyDesc := prometheus.NewDesc(
					c.Prefix+"_operation_latency_seconds", "operation latency", []string{"operation"}, nil)
				metricCh <- prometheus.MustNewConstMetric(
					opLatencyDesc, prometheus.GaugeValue, time.Since(start).Seconds(), name)
			}
			wg.Done()
		}()
	}
	// wait for all functions to complete
	wg.Wait()

	// return the first error encountered
	select {
	case err := <-ch:
		return err
	default:
	}
	return nil
}
