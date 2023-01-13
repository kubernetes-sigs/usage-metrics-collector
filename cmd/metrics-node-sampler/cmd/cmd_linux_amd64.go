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

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/containerd/containerd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"sigs.k8s.io/usage-metrics-collector/pkg/ctrstats"
)

var (
	cmAddress    string
	cmNamespace  string
	cmServerPort string
	cmLogLevel   string

	containerMonitorCmd = &cobra.Command{
		Use:   "container-monitor",
		Short: "tool for exporting container stats in prometheus format from containers using task service API",
		RunE:  cmRunE,
	}
)

var (
	monitorLog = logrus.WithField("source", "container-monitor")

	promNamespaceMonitor = "ctr_stats_gatherer"

	scrapeCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: promNamespaceMonitor,
		Name:      "scrape_count_total",
		Help:      "total scape count.",
	})

	scrapeDurationsHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: promNamespaceMonitor,
		Name:      "scrape_durations_milliseconds",
		Help:      "Time used to scrape from shims",
		Buckets:   prometheus.ExponentialBuckets(1, 2, 10),
	})
)

type Collector struct {
	metrics []*ctrstats.Metric
	client  *containerd.Client
}

func newCollector(address, namespace string) (*Collector, error) {
	monitorLog.WithFields(logrus.Fields{
		"containerd address": address,
		"namespace":          namespace,
	}).Info("creating new collector")

	client, err := ctrstats.NewContainerdClient(address, namespace)
	if err != nil {
		monitorLog.WithError(err).Errorf("failure to create new containerd client")
		return nil, err
	}
	c := &Collector{
		client: client,
	}

	// TODO: scrub the metric names, usage so it matches what we have in CAdvisor*
	c.metrics = append(c.metrics, ctrstats.CpuMetrics...)
	c.metrics = append(c.metrics, ctrstats.MemoryMetrics...)

	return c, nil
}

func (collector *Collector) cleanup() {
	if collector.client != nil {
		collector.client.Close()
	}
}

func (collector *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range collector.metrics {
		ch <- m.Desc()
	}
}

func (collector *Collector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	scrapeCount.Inc()

	defer func() {
		scrapeDurationsHistogram.Observe(float64(time.Since(start).Nanoseconds() / int64(time.Millisecond)))
	}()

	// Get list of containers, and then from these, get the actual stats
	// that we'll want to send back
	containers, err := ctrstats.GetContainers(collector.client)
	if err != nil {
		monitorLog.WithError(err).Error("failed to get list of containers from containerd")
		return
	}

	wg := &sync.WaitGroup{}
	for _, c := range containers {

		// If ContainerID and SandboxID are the same, we are either dealing with a
		// single container (non pod), or a pause container
		if c.ContainerID == c.SandboxID {

			// If a Pod ID is available, we know that this is a pod and should skip
			// stats gathering:
			if c.PodID != "" {
				monitorLog.WithField("container id", c.ContainerID).Trace("skipping stats collection for pause container")
				continue
			}

			monitorLog.WithField("container id", c.ContainerID).Trace("standlone container observed")
			c.ContainerName = c.ContainerID
		}

		wg.Add(1)

		go func(c ctrstats.Container, results chan<- prometheus.Metric) {
			stats, err := ctrstats.GetContainerStats(context.Background(), c)
			if err != nil {
				monitorLog.WithFields(logrus.Fields{
					"sandbox":   c.SandboxID,
					"container": c.ContainerID,
				}).WithError(err).Info("failed to get container stats - likely an issue with non-running containers being tracked in containerd state")
			} else if stats != nil {
				for _, m := range collector.metrics {
					metric := m.GetValues(stats)
					results <- prometheus.MustNewConstMetric(
						m.Desc(), m.Vt, metric.V, append([]string{c.ContainerName, c.SandboxNamespace, c.PodName}, metric.L...)...)
				}
			}
			wg.Done()
		}(c, ch)
	}

	wg.Wait()
}

func initLog(level string) {
	containerMonitorLog := logrus.WithFields(logrus.Fields{
		"name": "container-monitor",
		"pid":  os.Getpid(),
	})

	// set log level, default to warn
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.WarnLevel
	}

	containerMonitorLog.Logger.SetLevel(logLevel)
	containerMonitorLog.Logger.Formatter = &logrus.TextFormatter{TimestampFormat: time.RFC3339Nano}

	monitorLog = containerMonitorLog
}

func cmRunE(cmd *cobra.Command, args []string) error {
	initLog(cmLogLevel)

	collector, err := newCollector(cmAddress, cmNamespace)
	if err != nil {
		return err
	}
	defer collector.cleanup()

	prometheus.MustRegister(collector)

	http.Handle("/metrics", promhttp.Handler())

	monitorLog.Infof("starting to serve prometheus endpoint at 0.0.0.0:%s/metrics", cmServerPort)

	return http.ListenAndServe(fmt.Sprintf(":%s", cmServerPort), nil)
}

func initContainerMonitor(rootCmd *cobra.Command) {
	containerMonitorCmd.Flags().StringVarP(&cmAddress, "address", "a", "/run/containerd/containerd.sock", "path to the containerd socket")
	containerMonitorCmd.Flags().StringVarP(&cmNamespace, "namespace", "n", "default", "the namespace to get container stats from")
	containerMonitorCmd.Flags().StringVarP(&cmServerPort, "server-port", "p", "8090", "The address the server listens on for HTTP requests.")
	containerMonitorCmd.Flags().StringVar(&cmLogLevel, "log-level", "warn", "Logging level (trace/debug/info/warn/error/fatal/panic).")

	rootCmd.AddCommand(containerMonitorCmd)
}
