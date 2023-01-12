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
	"context"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	log = commonlog.Log.WithName("collector")
)

type Collector struct {
	// collector dependencies

	client.Client
	labler labler
	reader valueReader

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
	ch := make(chan prometheus.Metric)         // channgel for aggergation to send Metrics for exporting to prometheus
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

	// write the aggregated samples locally if configured to do so
	if c.SaveSamplesLocally != nil && len(scrapeResult.Items) > 0 {
		if err := c.SaveScrapeResultToFile(&scrapeResult); err != nil {
			log.Error(err, "unable to save aggregated metrics locally")
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

// collect returns the current state of all metrics of the collector.
func (c *Collector) collect(ch chan<- prometheus.Metric, sCh chan *collectorapi.SampleList) {
	start := time.Now()
	o, err := c.listCapacityObjects(ch)
	if err != nil {
		log.Error(err, "failed to list objects")
		return
	}

	err = c.wait(map[string]func() error{
		"collect_containers":     func() error { return c.collectContainers(o, ch, sCh) },
		"collect_quotas":         func() error { return c.collectQuota(o, ch, sCh) },
		"collect_nodes":          func() error { return c.collectNodes(o, ch, sCh) },
		"collect_pvs":            func() error { return c.collectPVs(o, ch, sCh) },
		"collect_pvcs":           func() error { return c.collectPVCs(o, ch, sCh) },
		"collect_namespace":      func() error { return c.collectNamespaces(o, ch, sCh) },
		"collect_cgroups":        func() error { return c.collectCGroups(o, ch, sCh) },
		"collect_cluster_scoped": func() error { return c.collectClusterScoped(o, ch, sCh) },
	}, ch)
	if err != nil {
		log.Error(err, "failed to collect container metrics")
	}

	log.V(1).Info("all collections complete", "seconds", time.Since(start).Seconds())
	latencyDesc := prometheus.NewDesc(
		c.Prefix+"_collection_latency_seconds", "collection latency", []string{}, nil)
	ch <- prometheus.MustNewConstMetric(
		latencyDesc, prometheus.GaugeValue, time.Since(start).Seconds())
}

func (c *Collector) collectPVs(o *capacityObjects, ch chan<- prometheus.Metric, sCh chan *collectorapi.SampleList) error {
	metrics := map[metricName]*Metric{}
	for i := range o.pvs.Items {
		pv := &o.pvs.Items[i]

		// find the pvc if the volume has been claimed
		var pvc *corev1.PersistentVolumeClaim
		if ref := pv.Spec.ClaimRef; ref != nil {
			pvc = o.pvcsByName[ref.Namespace+"/"+ref.Name]
		}

		// local volumes have a hostname -- try to lookup the node in this case
		var node *corev1.Node
		if nodeName, ok := pv.Annotations["kubernetes.io/hostname"]; ok {
			node = o.nodesByName[nodeName]
		}

		l := labelsValues{}
		c.labler.SetLabelsForPersistentVolume(&l, pv, pvc, node)
		values := c.reader.GetValuesForPV(pv)
		for src, v := range values {
			// each resource -- e.g. capacity
			for r, alias := range c.Resources {
				q, ok := v.ResourceList[corev1.ResourceName(r)]
				if !ok {
					// we aren't interested in this resource -- skip
					continue
				}

				// initialize the metric
				name := metricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "pv"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
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
		c.aggregateAndCollect(a, metrics, ch, sCh)
	}

	c.UtilizationServer.Collect(ch, o.utilizationByNode) // metrics on the utilization data health

	return nil
}

func (c *Collector) collectPVCs(o *capacityObjects, ch chan<- prometheus.Metric, sCh chan *collectorapi.SampleList) error {
	metrics := map[metricName]*Metric{}
	// each PVC
	for i := range o.pvcs.Items {
		pvc := &o.pvcs.Items[i]
		l := labelsValues{}
		n := o.namespacesByName[pvc.Namespace]
		pv := o.pvsByName[pvc.Spec.VolumeName]

		var p *corev1.Pod
		if pods := o.podsByPVC[pvc.Namespace+"/"+pvc.Name]; len(pods) == 1 {
			p = o.podsByPVC[pvc.Namespace+"/"+pvc.Name][0]
		}
		wl := workload{}
		var node *corev1.Node
		if p != nil {
			wl = getWorkloadForPod(c.Client, p)
			node = o.nodesByName[p.Spec.NodeName]
		}
		c.labler.SetLabelsForPersistentVolumeClaim(&l, pvc, pv, n, p, wl, node)

		values := c.reader.GetValuesForPVC(pvc)

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
				name := metricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "pvc"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
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
		c.aggregateAndCollect(a, metrics, ch, sCh)
	}
	return nil
}

func (c *Collector) collectQuota(o *capacityObjects, ch chan<- prometheus.Metric, sCh chan *collectorapi.SampleList) error {
	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.QuotaType) // For saving locally

	metrics := map[metricName]*Metric{}

	for i := range o.quotas.Items {
		q := &o.quotas.Items[i]

		// get the labels for this quota
		key := resourceQuotaDescriptorKey{
			name:      q.Name,
			namespace: q.Namespace,
		}
		l := labelsValues{}
		c.labler.SetLabelsForQuota(&l, q, o.rqdsByRQDKey[key], o.namespacesByName[q.Namespace])
		sample := sb.NewSample(l) // For saving locally

		values := c.reader.GetValuesForQuota(q, o.rqdsByRQDKey[key])

		// find the sources + resource we care about and add them to the metrics
		for src, v := range values {
			if src == collectorcontrollerv1alpha1.QuotaItemsSource {
				name := metricName{Source: collectorcontrollerv1alpha1.QuotaItemsSource,
					ResourceAlias: collectorcontrollerv1alpha1.ItemsResource,
					Resource:      collectorcontrollerv1alpha1.ItemsResource,
					SourceType:    "quota"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
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
					c.labler.SetLabelsForPVCQuota(&l, q, r)
				}
				// initialize the metric
				name := metricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "quota"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
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
		c.aggregateAndCollect(a, metrics, ch, sCh)
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

func (c *Collector) collectCGroups(o *capacityObjects, ch chan<- prometheus.Metric,
	sCh chan *collectorapi.SampleList) error {
	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.CGroupType) // Save locally

	resultMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_cgroup_usage_result",
		Help: "The number of containers missing usage information",
	}, []string{"exported_node", "sampler_pod", "sampler_phase", "found", "reason"})

	utilization := o.utilizationByNode
	metrics := map[metricName]*Metric{}

	// get metrics from each node
	for i := range o.nodes.Items {
		n := &o.nodes.Items[i]

		l := labelsValues{}
		c.labler.SetLabelsForNode(&l, n)

		var samplerPodName, samplerPodPhase string
		if p, ok := o.samplersByNode[n.Name]; ok {
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
			func(l labelsValues) {
				c.labler.SetLabelsFoCGroup(&l, m) // set the cgroup label to the base from the cgroup
				sample := sb.NewSample(l)         // For saving locally

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
				name := metricName{Source: src, ResourceAlias: alias, Resource: "cpu", SourceType: "cgroup"}
				metric, ok := metrics[name]
				if !ok {
					metric = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
					metrics[name] = metric
				}
				// get the values for this metric
				var cpuValues []resource.Quantity
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
				name = metricName{Source: src, ResourceAlias: alias, Resource: "memory", SourceType: "cgroup"}
				metric, ok = metrics[name]
				if !ok {
					metric = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
					metrics[name] = metric
				}
				// get the values for this metric
				var memoryValues []resource.Quantity
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
		c.aggregateAndCollect(a, metrics, ch, sCh)
	}
	resultMetric.Collect(ch)
	return nil
}

func (c *Collector) collectNodes(o *capacityObjects, ch chan<- prometheus.Metric, sCh chan *collectorapi.SampleList) error {
	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.NodeType) // For saving locally

	metrics := map[metricName]*Metric{}

	for i := range o.nodes.Items {
		n := &o.nodes.Items[i]

		// get the labels for this quota
		l := labelsValues{}
		c.labler.SetLabelsForNode(&l, n)
		sample := sb.NewSample(l) // For saving locally

		// get the values for this quota
		values := c.reader.GetValuesForNode(n, o.podsByNodeName[n.Name])

		// find the sources + resource we care about and add them to the metrics
		for src, v := range values {
			for r, alias := range c.Resources { // resource names are are interested in
				q, ok := v.ResourceList[corev1.ResourceName(r)]
				if !ok {
					// we aren't interested in this resource -- skip
					continue
				}

				// initialize the metric
				name := metricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "node"}
				m, ok := metrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
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
		c.aggregateAndCollect(a, metrics, ch, sCh)
	}
	return nil
}

func (c *Collector) collectNamespaces(o *capacityObjects, ch chan<- prometheus.Metric, sCh chan *collectorapi.SampleList) error {
	metrics := map[metricName]*Metric{}
	for i := range o.namespaces.Items {
		n := &o.namespaces.Items[i]

		// get the labels for this quota
		l := labelsValues{}
		c.labler.SetLabelsForNamespaces(&l, n)

		name := metricName{Source: collectorcontrollerv1alpha1.NamespaceItemsSource,
			ResourceAlias: collectorcontrollerv1alpha1.ItemsResource,
			Resource:      collectorcontrollerv1alpha1.ItemsResource,
			SourceType:    "namespace",
		}
		m, ok := metrics[name]
		if !ok {
			m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
			metrics[name] = m
		}
		m.Values[l] = []resource.Quantity{*resource.NewQuantity(1, resource.DecimalSI)}
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.NamespaceType) {
		c.aggregateAndCollect(a, metrics, ch, sCh)
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

func (c *Collector) collectContainers(o *capacityObjects, ch chan<- prometheus.Metric,
	sCh chan<- *collectorapi.SampleList) error {

	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.ContainerType) // Save locally

	start := time.Now()
	running := time.Since(c.startTime)

	log := log.WithValues("start", start.Local().Format("2006-01-02 15:04:05"))
	utilization := c.UtilizationServer.GetContainerUsageSummary(o.utilizationByNode)

	// metrics
	containerMetrics := map[metricName]*Metric{}
	podMetrics := map[metricName]*Metric{}

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

	for i := range o.pods.Items {
		pod := &o.pods.Items[i]

		// compute the metric labels for this pod
		podLabels := labelsValues{}

		wl := getWorkloadForPod(c.Client, pod)
		namespace := o.namespacesByName[pod.Namespace]
		node := o.nodesByName[pod.Spec.NodeName]
		c.labler.SetLabelsForPod(&podLabels, pod, wl, node, namespace)

		// collect pod values
		values := c.reader.GetValuesForPod(pod)

		for src, v := range values { // sources we are interested in
			for r, alias := range c.Resources { // resource names are are interested in
				q, ok := v.ResourceList[corev1.ResourceName(r)]
				if !ok {
					// container doesn't have values for this compute resource type -- skip it
					continue
				}

				// get the metric for this level + source + resource + operation
				name := metricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "pod"}
				m, ok := podMetrics[name]
				if !ok {
					m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
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
		if samplerPod, ok := o.samplersByNode[pod.Spec.NodeName]; ok {
			samplerPodName = samplerPod.Name
			samplerPodPhase = string(samplerPod.Status.Phase)
		}

		// Values are in the containers
		for i := range pod.Spec.Containers {
			// compute the labels based on the container -- copy the pod labels
			// and set the container values on the copy
			container := &pod.Spec.Containers[i]
			containerLabels := podLabels
			c.labler.SetLabelsForContainer(&containerLabels, container)
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
						"sampler_pod", o.samplersByNode[pod.Spec.NodeName],
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
			values := c.reader.GetValuesForContainer(container, pod, usage)

			for src, v := range values { // sources we are interested in
				for r, alias := range c.Resources { // resource names are are interested in
					if v.ResourceList != nil {
						q, ok := v.ResourceList[corev1.ResourceName(r)]
						if !ok {
							// container doesn't have values for this compute resource type -- skip it
							continue
						}

						// get the metric for this level + source + resource + operation
						name := metricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "container"}
						m, ok := containerMetrics[name]
						if !ok {
							m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
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
						name := metricName{Source: src, ResourceAlias: alias, Resource: r, SourceType: "container"}
						m, ok := containerMetrics[name]
						if !ok {
							m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
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
		c.aggregateAndCollect(a, containerMetrics, ch, sCh)
	}
	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.PodType) {
		c.aggregateAndCollect(a, podMetrics, ch, sCh)
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
			"pod-count", len(o.pods.Items),
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
	podCountMetric.Set(float64(len(o.pods.Items)))
	podCountMetric.Collect(ch)
	return nil
}

func (c *Collector) nodeMissingReason(node *corev1.Node, o *capacityObjects) string {
	if _, ok := o.utilizationByNode[node.Name]; ok {
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
	p, ok := o.samplersByNode[node.Name]
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
	id sampler.ContainerKey, o *capacityObjects) (string, bool) {

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

	if _, ok := o.utilizationByNode[pod.Spec.NodeName]; !ok {
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
func (c *Collector) listClusterScoped(o *capacityObjects) error {
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
		o.clusterScopedByName[source.Name] = list
	}

	return nil
}

// collectClusterScoped collects the configured cluster-scoped metrics.
func (c *Collector) collectClusterScoped(o *capacityObjects, ch chan<- prometheus.Metric, sCh chan *collectorapi.SampleList) error {
	sb := c.NewSampleListBuilder(collectorcontrollerv1alpha1.ClusterScopedType) // Save locally

	itemsMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_cluster_scoped_items",
		Help: "The number of collection items discovered for each cluster-scoped metric source",
	}, []string{"name", "reason"})

	resultMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_cluster_scoped_labeled_resources",
		Help: "The number of labeledResources discovered for each item in a cluster-scoped metric source collection",
	}, []string{"name", "resource_name", "reason"})

	metrics := map[metricName]*Metric{}

	for ii := range c.ClusterScopedMetrics.AnnotatedCollectionSources {
		source := c.ClusterScopedMetrics.AnnotatedCollectionSources[ii]
		log := log.WithValues("source", source.Name)

		list, ok := o.clusterScopedByName[source.Name]
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
					l := labelsValues{}
					c.labler.SetLabelsForClusterScoped(&l, entry.Labels)

					// iterate over the resource names we're interested in
					for r, alias := range c.Resources {
						qty, ok := entry.Values[corev1.ResourceName(r)]
						if !ok {
							log.V(2).Info("didn't find resource", "resource", r)
							// we didn't find this resource
							continue
						}

						log.V(2).Info("found resource", "resource", r, "value", qty)
						name := metricName{
							Source:        source.Name,
							ResourceAlias: alias,
							Resource:      r,
							SourceType:    collectorcontrollerv1alpha1.ClusterScopedType,
						}

						m, ok := metrics[name]
						if !ok {
							m = &Metric{Name: name, Values: map[labelsValues][]resource.Quantity{}}
							metrics[name] = m
						}

						m.Values[l] = []resource.Quantity{qty}
					}
				}
			}
		}
	}

	for _, a := range c.MetricsPrometheusCollector.Aggregations.ByType(collectorcontrollerv1alpha1.ClusterScopedType) {
		c.aggregateAndCollect(a, metrics, ch, sCh)
	}

	clusterScopedListResultMetric.Collect(ch)
	itemsMetric.Collect(ch)
	resultMetric.Collect(ch)

	if err := sb.SaveSamplesToFile(); err != nil {
		log.Error(err, "unable to save metrics locally")
	}

	return nil
}

// aggregateAndCollect aggregates the metrics at each level and then collects them
func (c *Collector) aggregateAndCollect(
	a *collectorcontrollerv1alpha1.Aggregation, m map[metricName]*Metric,
	ch chan<- prometheus.Metric, sCh chan<- *collectorapi.SampleList) {

	levels := a.Levels
	sources := sets.NewString(a.Sources.GetSources()...)

	// filter the sources we aggregate
	metrics := make(map[metricName]*Metric, len(m))
	for k, v := range m {
		if !sources.Has(k.Source) {
			continue
		}
		metrics[k] = v
	}

	wg := &sync.WaitGroup{}
	for k, v := range metrics {
		wg.Add(1)
		go func(name metricName, metric Metric) {
			// aggregate and collect the metric for each level
			for _, l := range levels {
				// aggregate at this level
				aggregatedName := metricName{
					Prefix:        c.Prefix,
					Level:         l.Mask.Level,
					Operation:     string(l.Operation),
					Source:        name.Source,
					ResourceAlias: name.ResourceAlias,
					Resource:      name.Resource,
					SourceType:    name.SourceType,
				}

				aggregatedMetric := c.aggregateMetric(l.Operation, metric, l.Mask)
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

// resourceQuotaDescriptorKey is used as a key for priority class -
// namespace ResourceQuotaDescriptor pair
type resourceQuotaDescriptorKey struct {
	name      string
	namespace string
}

// capacityObjects are the capacityObjects used for computing metrics
type capacityObjects struct {
	pods       corev1.PodList
	nodes      corev1.NodeList
	namespaces corev1.NamespaceList
	quotas     corev1.ResourceQuotaList
	rqds       quotamanagementv1alpha1.ResourceQuotaDescriptorList
	pvcs       corev1.PersistentVolumeClaimList
	pvs        corev1.PersistentVolumeList
	samplers   corev1.PodList

	namespacesByName map[string]*corev1.Namespace
	podsByNodeName   map[string][]*corev1.Pod
	podsByPVC        map[string][]*corev1.Pod
	pvsByName        map[string]*corev1.PersistentVolume
	pvcsByName       map[string]*corev1.PersistentVolumeClaim
	nodesByName      map[string]*corev1.Node
	rqdsByRQDKey     map[resourceQuotaDescriptorKey]*quotamanagementv1alpha1.ResourceQuotaDescriptor
	samplersByNode   map[string]*corev1.Pod

	clusterScopedByName map[string]*metav1.PartialObjectMetadataList

	utilizationByNode map[string]*samplerapi.ListMetricsResponse
}

// listCapacityObjects gets all the objects used for computing metrics
func (c *Collector) listCapacityObjects(ch chan<- prometheus.Metric) (*capacityObjects, error) {
	// read the objects in parallel

	var o capacityObjects
	o.clusterScopedByName = map[string]*metav1.PartialObjectMetadataList{}

	err := c.wait(map[string]func() error{
		"list_pods":           func() error { return c.Client.List(context.Background(), &o.pods) },
		"list_nodes":          func() error { return c.Client.List(context.Background(), &o.nodes) },
		"list_namespaces":     func() error { return c.Client.List(context.Background(), &o.namespaces) },
		"list_quotas":         func() error { return c.Client.List(context.Background(), &o.quotas) },
		"list_pvcs":           func() error { return c.Client.List(context.Background(), &o.pvcs) },
		"list_pvs":            func() error { return c.Client.List(context.Background(), &o.pvs) },
		"list_rqds":           func() error { return c.Client.List(context.Background(), &o.rqds) },
		"list_cluster_scoped": func() error { return c.listClusterScoped(&o) },
		"list_sampler_pods": func() error { // fetch the list of sampler pods
			return c.Client.List(
				context.Background(), &o.samplers,
				client.MatchingLabels(c.UtilizationServer.SamplerPodLabels),
				client.InNamespace(c.UtilizationServer.SamplerNamespaceName),
			)
		},
	}, ch)
	if err != nil {
		return nil, err
	}

	o.pvsByName = make(map[string]*corev1.PersistentVolume, len(o.pvs.Items))
	for i := range o.pvs.Items {
		pv := &o.pvs.Items[i]
		o.pvsByName[pv.Name] = pv
	}

	o.pvcsByName = make(map[string]*corev1.PersistentVolumeClaim, len(o.pvcs.Items))
	for i := range o.pvcs.Items {
		pvc := &o.pvcs.Items[i]
		o.pvcsByName[pvc.Namespace+"/"+pvc.Name] = pvc
	}

	// index the namespaces by name
	o.namespacesByName = make(map[string]*corev1.Namespace, len(o.namespaces.Items))
	for i := range o.namespaces.Items {
		o.namespacesByName[o.namespaces.Items[i].Name] = &o.namespaces.Items[i]
	}

	// index the nodes by name
	o.nodesByName = make(map[string]*corev1.Node, len(o.nodes.Items))
	for i := range o.nodes.Items {
		o.nodesByName[o.nodes.Items[i].Name] = &o.nodes.Items[i]
	}

	// index the resource quota descriptors by name
	o.rqdsByRQDKey = make(map[resourceQuotaDescriptorKey]*quotamanagementv1alpha1.ResourceQuotaDescriptor,
		len(o.rqds.Items))
	for i := range o.rqds.Items {
		key := resourceQuotaDescriptorKey{
			name:      o.rqds.Items[i].Name,
			namespace: o.rqds.Items[i].Namespace,
		}
		o.rqdsByRQDKey[key] = &o.rqds.Items[i]
	}

	// index the pods by node name
	o.podsByNodeName = make(map[string][]*corev1.Pod, len(o.nodes.Items))
	o.podsByPVC = make(map[string][]*corev1.Pod, len(o.pvcs.Items))
	for i := range o.pods.Items {
		p := &o.pods.Items[i]
		o.podsByNodeName[p.Spec.NodeName] = append(
			o.podsByNodeName[p.Spec.NodeName], p)
		for j := range p.Spec.Volumes {
			if p.Spec.Volumes[j].PersistentVolumeClaim == nil {
				continue
			}
			c := p.Namespace + "/" + p.Spec.Volumes[j].PersistentVolumeClaim.ClaimName
			o.podsByPVC[c] = append(o.podsByPVC[c], &o.pods.Items[i])
		}
	}

	// index the sampler pods
	o.samplersByNode = make(map[string]*corev1.Pod, len(o.samplers.Items))
	for i := range o.samplers.Items {
		p := &o.samplers.Items[i]
		if p.Spec.NodeName != "" {
			o.samplersByNode[p.Spec.NodeName] = p
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
		if _, ok := o.samplersByNode[nodeName]; ok && p.DeletionTimestamp != nil {
			continue // keep the non-deleted pod
		}
		o.samplersByNode[nodeName] = p
	}

	// get this once since it requires locking the cache
	o.utilizationByNode = c.UtilizationServer.GetMetrics()
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
