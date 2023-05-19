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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
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

	IsLeaderElected *atomic.Bool

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

	startTime          time.Time
	sideCarConfigs     []*collectorcontrollerv1alpha1.SideCarConfig
	saveLocalQueueSize atomic.Int32
	saveLocalDone      sync.WaitGroup
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
		IsLeaderElected:            &atomic.Bool{},
	}
	c.UtilizationServer.IsReadyResult.Store(false)
	c.IsLeaderElected.Store(false)

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
	c.registerWithSamplers(ctx)
}

// Describe returns all descriptions of the collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *Collector) saveMetricsToLocalFile(slc chan *collectorapi.SampleList) {
	defer c.saveLocalDone.Done()
	saveLocalQueueLength.Set(float64(c.saveLocalQueueSize.Add(1)))
	defer func() {
		saveLocalQueueLength.Set(float64(c.saveLocalQueueSize.Add(-1)))
	}()

	scrapeResult := collectorapi.ScrapeResult{}
	for sl := range slc { // read all the results from this scrape
		scrapeResult.Items = append(scrapeResult.Items, sl)
	}
	if c.SaveSamplesLocally == nil && len(scrapeResult.Items) == 0 {
		return
	}

	// preprocess the results for saving
	start := time.Now()
	if err := c.NormalizeForSave(&scrapeResult); err != nil {
		log.Error(err, "unable to save aggregated metrics locally", "reason", "normalize")
		return
	}
	saveLocalLatencySeconds.WithLabelValues("normalize").Set(time.Since(start).Seconds())

	// write the aggregated samples locally as a proto file
	start = time.Now()
	if c.SaveSamplesLocally != nil && pointer.BoolDeref(c.SaveSamplesLocally.SaveProto, false) && len(scrapeResult.Items) > 0 {
		if err := c.SaveScrapeResultToFile(&scrapeResult); err != nil {
			log.Error(err, "unable to save aggregated metrics locally", "reason", "proto")
			savedLocalMetric.WithLabelValues("fail", "proto").Add(1)
		} else {
			savedLocalMetric.WithLabelValues("success", "proto").Add(1)
		}
	}
	saveLocalLatencySeconds.WithLabelValues("proto").Set(time.Since(start).Seconds())

	// write the aggregated samples locally as a json file
	start = time.Now()
	if c.SaveSamplesLocally != nil && pointer.BoolDeref(c.SaveSamplesLocally.SaveJSON, false) && len(scrapeResult.Items) > 0 {
		if err := c.SaveScrapeResultToJSONFile(&scrapeResult); err != nil {
			log.Error(err, "unable to save aggregated metrics locally as json", "reason", "json")
			savedLocalMetric.WithLabelValues("fail", "json").Add(1)
		} else {
			savedLocalMetric.WithLabelValues("success", "json").Add(1)
		}
	}
	saveLocalLatencySeconds.WithLabelValues("json").Set(time.Since(start).Seconds())
}

// Collect returns the current state of all metrics of the collector.
// This there are cached metrics, Collect will return the cached metrics.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	if !c.IsLeaderElected.Load() || !c.UtilizationServer.IsReadyResult.Load() {
		// Never export metrics if we aren't the leader or aren't ready to report them.
		// e.g. have sufficient utilization metrics
		log.Info("skipping collection")
		return
	}

	sCh := make(chan *collectorapi.SampleList) // channel for aggregation to send SampleLists for saving locally
	defer close(sCh)                           // all scrape results written, stop reading them
	c.saveLocalDone.Add(1)
	go c.saveMetricsToLocalFile(sCh) // write the saved metrics to a file, but don't block
	c.collect(ch, sCh)               // compute the metrics

	cacheCollectTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collect_time_seconds",
		Help: "The time to collect the cached metrics.",
	})
	cacheCollectTime.Set(float64(time.Since(start).Seconds()))
	cacheCollectTime.Collect(ch)
	log.Info("finished collection", "seconds", time.Since(start).Seconds())
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

		pvLabels := LabelsValues{}
		c.Labeler.SetLabelsForPersistentVolume(&pvLabels, pv, pvc, node)
		values := c.Reader.GetValuesForPV(pv)
		c.addValuesToMetrics(values, pvLabels, metrics, collectorcontrollerv1alpha1.PVType, nil)
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
		c.addValuesToMetrics(values, l, metrics, collectorcontrollerv1alpha1.PVCType, nil)
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
					ResourceAlias: collectorcontrollerv1alpha1.ResourceAliasItems,
					Resource:      collectorcontrollerv1alpha1.ResourceItems,
					SourceType:    collectorcontrollerv1alpha1.QuotaType}
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
				if strings.Contains(string(r), "storageclass.storage.k8s.io") {
					c.Labeler.SetLabelsForPVCQuota(&l, q, r)
				}
				// initialize the metric
				name := MetricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: collectorcontrollerv1alpha1.QuotaType}
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
func (c *Collector) getCGroupMetricSource(cgroup string) collectorcontrollerv1alpha1.CGroupMetric {
	if cgroup == "" {
		// special case root cgroup -- there is no parent directory
		return c.CGroupMetrics.RootSource
	}
	return c.CGroupMetrics.Sources[filepath.Dir("/"+cgroup)]
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

				cpuAlias := c.Resources[collectorcontrollerv1alpha1.ResourceCPU]
				if cpuAlias == "" {
					cpuAlias = collectorcontrollerv1alpha1.ResourceAliasCPU
				}
				memoryAlias := c.Resources[collectorcontrollerv1alpha1.ResourceMemory]
				if memoryAlias == "" {
					memoryAlias = collectorcontrollerv1alpha1.ResourceAliasMemory
				}

				// get the source name for this level in the hiearchy
				src := c.getCGroupMetricSource(m.AggregationLevel)
				if src.Name != "" {
					// find the metric, initialize if necessary
					// the metric source is a function of the cgroup parent directory (filepath.Dir) while
					// the metric cgroup label is a function of the cgroup base directory (filepath.Base)
					name := MetricName{Source: src.Name, ResourceAlias: cpuAlias, Resource: collectorcontrollerv1alpha1.ResourceCPU, SourceType: collectorcontrollerv1alpha1.CGroupType}
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

					// find the metric, initialize if necessary
					name = MetricName{Source: src.Name, ResourceAlias: memoryAlias, Resource: collectorcontrollerv1alpha1.ResourceMemory, SourceType: collectorcontrollerv1alpha1.CGroupType}
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

					sb.AddIntValues(sample, collectorcontrollerv1alpha1.ResourceCPU, src.Name, m.CpuCoresNanoSec...) // For saving locally
					sb.AddIntValues(sample, collectorcontrollerv1alpha1.ResourceMemory, src.Name, m.MemoryBytes...)  // For saving locally
				}
				if src.AvgName != "" {
					// get the pre-computed average
					name := MetricName{
						Source:        src.AvgName,
						ResourceAlias: cpuAlias,
						Resource:      collectorcontrollerv1alpha1.ResourceCPU,
						SourceType:    collectorcontrollerv1alpha1.CGroupType,
					}
					metric, ok := metrics[name]
					if !ok {
						metric = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
						metrics[name] = metric
					}
					metric.Values[l] = []resource.Quantity{*resource.NewScaledQuantity(m.AvgCPUCoresNanoSec, resource.Nano)}

					// record memory

					// get the pre-computed average
					name = MetricName{
						Source:        src.AvgName,
						ResourceAlias: memoryAlias,
						Resource:      collectorcontrollerv1alpha1.ResourceMemory,
						SourceType:    collectorcontrollerv1alpha1.CGroupType,
					}
					metric, ok = metrics[name]
					if !ok {
						metric = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
						metrics[name] = metric
					}
					metric.Values[l] = []resource.Quantity{*resource.NewQuantity(m.AvgMemoryBytes, resource.DecimalSI)}

					sb.AddIntValues(sample, collectorcontrollerv1alpha1.ResourceCPU, src.AvgName, m.AvgCPUCoresNanoSec) // For saving locally
					sb.AddIntValues(sample, collectorcontrollerv1alpha1.ResourceMemory, src.AvgName, m.AvgMemoryBytes)  // For saving locally
				}
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
		nodeLabels := LabelsValues{}
		c.Labeler.SetLabelsForNode(&nodeLabels, n)

		// get the values for this quota
		values := c.Reader.GetValuesForNode(n, o.PodsByNodeName[n.Name])
		c.addValuesToMetrics(values, nodeLabels, metrics, collectorcontrollerv1alpha1.NodeType, sb)
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
			ResourceAlias: collectorcontrollerv1alpha1.ResourceAliasItems,
			Resource:      collectorcontrollerv1alpha1.ResourceItems,
			SourceType:    collectorcontrollerv1alpha1.NamespaceType,
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
	log.Info("found utilization metrics", "container-count", len(utilization))

	// metrics
	containerMetrics := map[MetricName]*Metric{}
	podMetrics := map[MetricName]*Metric{}
	schedulerMetrics := map[MetricName]*Metric{}

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
		c.addValuesToMetrics(values, podLabels, podMetrics, collectorcontrollerv1alpha1.PodType, nil)

		// collect scheduler health values
		values = c.Reader.GetValuesForSchedulerHealth(pod, c.SchedulerHealth)
		c.addValuesToMetrics(values, podLabels, schedulerMetrics, collectorcontrollerv1alpha1.SchedulerHealthType, nil)

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

			// first try to get the metrics based on the container name and namespace
			id := sampler.ContainerKey{ContainerName: container.Name, PodName: pod.Name, NamespaceName: pod.Namespace}
			allContainers.Insert(string(pod.UID) + "/" + string(containerNameToID[container.Name]))
			usage, ok := utilization[id]
			if !ok {
				// check for metrics based on uid and id if not present by name
				id := sampler.ContainerKey{ContainerID: containerNameToID[container.Name], PodUID: podUID}
				usage = utilization[id]
			}

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
			c.addValuesToMetrics(values, containerLabels, containerMetrics, collectorcontrollerv1alpha1.ContainerType, sb)
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

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.SchedulerHealthType) {
		c.AggregateAndCollect(a, schedulerMetrics, ch, sCh)
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

func (c *Collector) addValuesToMetrics(values map[collectorcontrollerv1alpha1.Source]value, labels LabelsValues,
	metrics map[MetricName]*Metric, sourceType collectorcontrollerv1alpha1.SourceType, sb *SampleListBuilder) {
	sample := sb.NewSample(labels)

	for src, v := range values { // sources we are interested in
		for r, alias := range c.Resources { // resource names are are interested in
			var qs []resource.Quantity
			if v.ResourceList != nil {
				if q, ok := v.ResourceList[corev1.ResourceName(r)]; ok {
					qs = []resource.Quantity{q}
				}
			} else if v.MultiResourceList != nil {
				if q, ok := v.MultiResourceList[corev1.ResourceName(r)]; ok {
					qs = q
				}
			}
			if qs == nil {
				continue
			}

			// get the metric for this level + source + resource + operation
			name := MetricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: sourceType}
			m, ok := metrics[name]
			if !ok {
				m = &Metric{Name: name, Values: map[LabelsValues][]resource.Quantity{}}
				metrics[name] = m
			}
			// set the value
			if _, ok := m.Values[labels]; ok {
				// already found an item here
				log.V(1).Info("duplicate value for", "sourceType", sourceType, "labels", labels)
			}
			m.Values[labels] = qs

			sb.AddQuantityValues(sample, r, src, qs...) // For saving locally
		}
	}
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
			clusterScopedListResultMetric.WithLabelValues(source.Name.String(), err.Error()).Add(1)
			continue
		}

		clusterScopedListResultMetric.WithLabelValues(source.Name.String(), "").Add(1)
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
			itemsMetric.WithLabelValues(source.Name.String(), "objects not found").Add(1)
			continue
		}
		log.V(2).Info("list items", "items", len(list.Items))
		itemsMetric.WithLabelValues(source.Name.String(), "").Add(float64(len(list.Items)))

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
					resultMetric.WithLabelValues(source.Name.String(), obj.GetName(), err.Error()).Add(1)
					continue
				}
				resultMetric.WithLabelValues(source.Name.String(), obj.GetName(), "").Add(1)

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
	sources := sets.New(a.Sources.GetSources()...)

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
				if len(l.Operations) > 0 && i != len(levels)-1 {
					log.Error(errors.Errorf("too many operations for level"),
						"only terminal levels are supported with multiple operations")
					continue
				}

				if l.Operation != "" {
					l.Operations = append(l.Operations, l.Operation)
				}

				// create an operations key
				opsStrings := make([]string, 0, len(l.Operations))
				for _, op := range l.Operations {
					opsStrings = append(opsStrings, op.String())
				}
				operationsKey := strings.Join(opsStrings, ",")

				aggregatedMetricOps := c.aggregateMetric(operationsKey, l.Operations, metric, l.Mask, ch, name.String(), a.Name, l.Name)

				start := time.Now()
				for op, aggregatedMetric := range aggregatedMetricOps {
					// aggregate at this level for all operations
					aggregatedName := MetricName{
						Prefix:        c.Prefix,
						Level:         l.Mask.Level,
						Operation:     op,
						SourceAlias:   a.Sources.Alias[name.Source],
						Source:        name.Source,
						ResourceAlias: name.ResourceAlias,
						Resource:      name.Resource,
						SourceType:    name.SourceType,
					}
					aggregatedMetric.Name = aggregatedName
					metric = *aggregatedMetric

					if l.RetentionName != "" && c.SaveSamplesLocally != nil { // export this metric to a local file for retention
						start := time.Now()
						slb := c.NewAggregatedSampleListBuilder(aggregatedName.SourceType)
						slb.SampleList.MetricName = aggregatedName.String()
						slb.SampleList.Name = l.RetentionName
						slb.Mask = l.Mask

						for mk, mv := range metric.Values {
							s := slb.NewSample(mk)
							s.Operation = aggregatedName.Operation.String()
							s.Level = aggregatedName.Level.String()
							// save samples in histogram format
							if aggregatedName.Operation == collectorcontrollerv1alpha1.HistogramOperation && l.RetentionExponentialBuckets != nil {
								slb.AddHistogramValues(s,
									aggregatedName.Resource,
									aggregatedName.Source,
									l.RetentionExponentialBuckets[aggregatedName.Resource.String()],
									mv...)
							} else {
								slb.AddQuantityValues(s,
									aggregatedName.Resource,
									aggregatedName.Source, mv...)
							}
						}

						sCh <- slb.SampleList
						c.publishTimer("metric_aggregation_per_aggregated_metric", ch, start, "local_save", name.String(), a.Name, l.Name, "aggregated_metric", aggregatedName.String())
					}

					if l.NoExport {
						// aggregate the metric, but don't export it
						continue
					}

					// export the metric
					start := time.Now()
					if op == collectorcontrollerv1alpha1.HistogramOperation {
						metric.Buckets = l.HistogramBuckets
						c.collectHistogramMetric(metric, ch)
					} else {
						c.collectMetric(metric, ch)
					}
					c.publishTimer("metric_aggregation_per_aggregated_metric", ch, start, "collection", name.String(), a.Name, l.Name, "aggregated_metric", aggregatedName.String())
				}
				c.publishTimer("metric_aggregation", ch, start, "collection_total", name.String(), a.Name, l.Name, "ops", operationsKey)
			}
			wg.Done()
		}(k, *v)
	}
	wg.Wait()
}

func (c *Collector) publishTimer(metricName string, ch chan<- prometheus.Metric, start time.Time, phase, metric, agg, level string, fields ...string) {
	if len(fields)%2 != 0 {
		panic(fmt.Sprintf("fields should contain an even number of strings, got %v", fields))
	}

	metric = strings.TrimLeft(metric, "_")

	names := []string{"phase", "metric_name", "aggregation_name", "level_name"}
	values := []string{phase, metric, agg, level}
	for i := 0; i < len(fields); i += 2 {
		names = append(names, fields[i])
		values = append(values, fields[i+1])
	}

	log := log
	for i, name := range names {
		log = log.WithValues(name, values[i])
	}
	log.V(2).Info("metric aggregation complete", "seconds", time.Since(start).Seconds())

	if ch != nil {
		// record how long it took
		desc := prometheus.NewDesc(c.Prefix+"_"+metricName+"_latency_seconds", "operation latency", names, nil)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, time.Since(start).Seconds(), values...)
	}
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

	ClusterScopedByName map[collectorcontrollerv1alpha1.Source]*metav1.PartialObjectMetadataList

	UtilizationByNode map[string]*samplerapi.ListMetricsResponse
}

// listCapacityObjects gets all the objects used for computing metrics
func (c *Collector) listCapacityObjects(ch chan<- prometheus.Metric) (*CapacityObjects, error) {
	// read the objects in parallel

	var o CapacityObjects
	o.ClusterScopedByName = map[collectorcontrollerv1alpha1.Source]*metav1.PartialObjectMetadataList{}

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
	log.Info("found utilization metrics", "node-count", len(o.UtilizationByNode))
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

// registerWithSamplers manually registers the collector with node samplers
// so that it can start getting metrics before it marks itself as "Ready"
// and before it appears in the DNS ips for the service.
func (c *Collector) registerWithSamplers(ctx context.Context) {
	if c.UtilizationServer.MinResultPctBeforeReady == 0 {
		// don't register with samplers
		c.UtilizationServer.IsReadyResult.Store(true)
		log.Info("not using min utilization results for ready check")
	}
	if len(c.UtilizationServer.CollectorPodLabels) == 0 {
		// don't know the collector pods, we can't register
		log.Info("not registering collector with samplers, no collector pod labels were specified")
		return
	}

	// continuously register with node samplers that we don't have results from
	go func() {
		tick := time.NewTicker(time.Minute)
		waitReadyCyclesCount := c.UtilizationServer.WaitSamplerRegistrationsBeforeReady
		for {
			log.V(1).Info("ensuring collector is registered with samplers")
			// since we can't get the status.ips list from the downward api,
			// get the collector status.ips lists from the collector pods
			// and register all collectors.
			collectors := &corev1.PodList{}
			err := c.Client.List(ctx, collectors,
				client.InNamespace(c.UtilizationServer.SamplerNamespaceName),
				client.MatchingLabels(c.UtilizationServer.CollectorPodLabels),
			)
			if err != nil || len(collectors.Items) == 0 {
				log.Error(err, "unable to register collector with node samplers -- failed to list collector pods.")
			}
			// calculate the collector IPs to register with the samplers
			req := &samplerapi.RegisterCollectorsRequest{
				Collectors: make([]*samplerapi.Collector, 0, len(collectors.Items)),
				Source:     "Collector",           // the sampler also registers collectors from DNS
				FromPod:    os.Getenv("POD_NAME"), // used for debugging in the sampler
			}
			for _, col := range collectors.Items {
				if col.DeletionTimestamp != nil {
					// only register running collectors
					continue
				}
				if col.Status.Phase != corev1.PodRunning {
					// only register running collectors
					continue
				}
				if len(col.Status.PodIPs) <= c.UtilizationServer.CollectorPodIPsIndex {
					// only register collectors with ip addresses
					continue
				}
				ip := col.Status.PodIPs[c.UtilizationServer.CollectorPodIPsIndex] // pick the ip to register
				req.Collectors = append(req.Collectors, &samplerapi.Collector{
					IpAddress: ip.IP,
					PodName:   col.Name, // used for debugging in the sampler
				})
			}

			// list all sampler pods we may need to register with.
			samplers := &corev1.PodList{}
			err = c.Client.List(ctx, samplers,
				client.InNamespace(c.UtilizationServer.SamplerNamespaceName),
				client.MatchingLabels(c.UtilizationServer.SamplerPodLabels),
			)
			if err != nil {
				log.Error(err, "unable to register collector with node samplers.  failed to list samplers.")
			}

			// don't re-register with samplers if we are already registered
			nodesWithResults := c.UtilizationServer.GetNodeNames()

			// Check if we have gotten enough node results to consider ourselves ready.
			// We don't want Kubernetes to terminate the old replica during a rollout
			// until we are able to publish utilization metrics to prometheus, and
			// accomplish this by not being ready.
			if c.UtilizationServer.MinResultPctBeforeReady > 0 && !c.UtilizationServer.IsReadyResult.Load() {
				nodesWithRunningSamplers := sets.NewString()
				nodesWithSamplers := sets.NewString() // used for logging
				for i := range samplers.Items {
					nodesWithSamplers.Insert(samplers.Items[i].Spec.NodeName)
					if samplers.Items[i].Status.Phase != corev1.PodRunning {
						// only consider running pods -- we don't expect results from non-running pods
						continue
					}
					nodesWithRunningSamplers.Insert(samplers.Items[i].Spec.NodeName)
				}
				nodesMissingResults := nodesWithRunningSamplers.Difference(nodesWithResults)

				readyPct := (nodesWithResults.Len() * 100 / nodesWithSamplers.Len())
				if nodesWithSamplers.Len() > 0 && readyPct > c.UtilizationServer.MinResultPctBeforeReady {
					if waitReadyCyclesCount <= 0 {
						// Have enough utilization results to say we are ready
						log.Info("collector ready",
							"running-minutes", time.Since(c.startTime).Minutes(),
							"nodes-with-results-count", nodesWithResults.Len(),
							"nodes-with-samplers-count", nodesWithSamplers.Len(),
							"nodes-with-running-samplers-count", nodesWithRunningSamplers.Len(),
							"nodes-missing-results", nodesMissingResults.List(),
							"ready-pct", readyPct,
							"min-ready-pct", c.UtilizationServer.MinResultPctBeforeReady,
							"remaining-cycles", waitReadyCyclesCount,
						)
						c.UtilizationServer.IsReadyResult.Store(true)
					} else {
						log.Info("collector has required sampler results, waiting on WaitSamplerRegistrationsBeforeReady to be ready",
							"running-minutes", time.Since(c.startTime).Minutes(),
							"nodes-with-results-count", nodesWithResults.Len(),
							"nodes-with-samplers-count", nodesWithSamplers.Len(),
							"nodes-with-running-samplers-count", nodesWithRunningSamplers.Len(),
							"nodes-missing-results", nodesMissingResults.List(),
							"ready-pct", readyPct,
							"min-ready-pct", c.UtilizationServer.MinResultPctBeforeReady,
							"remaining-cycles", waitReadyCyclesCount,
						)
						waitReadyCyclesCount--
					}
				} else {
					// Don't have enough utilization results to say we are ready
					log.Info("collector not-ready",
						"running-minutes", time.Since(c.startTime).Minutes(),
						"nodes-with-results-count", nodesWithResults.Len(),
						"nodes-with-samplers-count", nodesWithSamplers.Len(),
						"nodes-with-running-samplers-count", nodesWithRunningSamplers.Len(),
						"nodes-missing-results", nodesMissingResults.List(),
						"ready-pct", readyPct,
						"min-ready-pct", c.UtilizationServer.MinResultPctBeforeReady,
						"remaining-cycles", waitReadyCyclesCount,
					)
				}
			}

			// identify the sampler pods to register with
			wg := sync.WaitGroup{}
			var samplerCount, samplerResultsCount, notRunningCount int
			var errorCount, registeredCount atomic.Int32
			for i := range samplers.Items {
				pod := samplers.Items[i]
				switch {
				case pod.Status.Phase != corev1.PodRunning:
					// sampler isn't running yet
					notRunningCount++
					continue
				case pod.Spec.NodeName == "":
					// defensive: shouldn't hit this
					notRunningCount++
					continue
				case pod.Status.PodIP == "":
					// defensive: pod not running
					notRunningCount++
					continue
				case nodesWithResults.Has(pod.Spec.NodeName):
					// already registered.
					samplerResultsCount++
					samplerCount++
					continue
				}
				samplerCount++

				// register the collector with the each node sampler
				wg.Add(1)
				go func() {
					defer wg.Done()
					// build the address for us to register with the sampler
					address := pod.Status.PodIP
					if strings.Contains(address, ":") {
						// format IPv6 correctly
						address = "[" + address + "]"
					}
					address = fmt.Sprintf("%s:%v", address, c.UtilizationServer.SamplerPort)

					// create the grpc connection
					var conn *grpc.ClientConn
					for i := 0; i < 3; i++ { // retry connection
						conn, err = grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
						if err == nil {
							// connection established
							break
						}
					}
					if err != nil {
						log.Error(err, "unable to connect to node sampler",
							"node", pod.Spec.NodeName,
							"pod", pod.Name,
						)
						errorCount.Add(1)
						return
					}

					// register ourself with this node sampler
					c := samplerapi.NewMetricsClient(conn)
					resp, err := c.RegisterCollectors(ctx, req)
					if err != nil {
						errorCount.Add(1)
						log.Error(err, "failed to register with node sampler",
							"node", pod.Spec.NodeName,
							"pod", pod.Name,
						)
					} else {
						registeredCount.Add(1)
						log.Info("registered with node sampler",
							"node", pod.Spec.NodeName,
							"pod", pod.Name,
							"sent", req,
							"got", resp.IpAddresses)
					}
				}()
			}
			wg.Wait()

			log.Info("finished registering with node-samplers",
				"results-pct", samplerResultsCount*100/samplerCount,
				"results-count", samplerResultsCount,
				"running-sampler-count", samplerCount,
				"not-running-sampler-count", notRunningCount,
				"register-success-count", registeredCount.Load(),
				"register-fail-count", errorCount.Load(),
			)
			registeredWithSamplers.Reset()
			registeredWithSamplers.WithLabelValues("node-registered").Set(float64(samplerResultsCount))
			registeredWithSamplers.WithLabelValues("node-registration-error").Set(float64(errorCount.Load()))
			registeredWithSamplers.WithLabelValues("node-registration-success").Set(float64(registeredCount.Load()))
			registeredWithSamplers.WithLabelValues("sampler-not-running").Set(float64(notRunningCount))
			registeredWithSamplers.WithLabelValues("node-not-registered").Set(float64(
				samplerCount - samplerResultsCount - int(errorCount.Load()) - int(registeredCount.Load())))

			<-tick.C // wait before registering again
		}
	}()
}

var saveLocalQueueLength = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "metrics_prometheus_collector_save_local_queue_length",
	Help: "The number of scrapes being saved locally.",
})

var saveLocalLatencySeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "metrics_prometheus_collector_save_local_latency_seconds",
	Help: "The time in seconds to save the results.",
}, []string{"type"})

var savedLocalMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "metrics_prometheus_collector_save_local_result",
	Help: "The results from saving metrics to local files.",
}, []string{"result", "type"})

var registeredWithSamplers = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "metrics_prometheus_collector_registered_count",
	Help: "Nodes the collector has registered with.",
}, []string{"status"})

func init() {
	ctrlmetrics.Registry.MustRegister(registeredWithSamplers)
	ctrlmetrics.Registry.MustRegister(savedLocalMetric)
	ctrlmetrics.Registry.MustRegister(saveLocalQueueLength)
	ctrlmetrics.Registry.MustRegister(saveLocalLatencySeconds)
}
