//go:build linux && amd64

package main

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
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"sigs.k8s.io/usage-metrics-collector/pkg/ctrstats"
)

var (
	address    string
	namespace  string
	serverPort string
	logLevel   string

	rootCmd = &cobra.Command{
		Use:   "container-monitor",
		Short: "tool for exporting container stats in prometheus format from containers using task service API",
		RunE:  RunE,
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
	monitorLog.WithFields(log.Fields{
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
				monitorLog.WithFields(log.Fields{
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
	containerMonitorLog := log.WithFields(log.Fields{
		"name": "container-monitor",
		"pid":  os.Getpid(),
	})

	// set log level, default to warn
	logLevel, err := log.ParseLevel(level)
	if err != nil {
		logLevel = log.WarnLevel
	}

	containerMonitorLog.Logger.SetLevel(logLevel)
	containerMonitorLog.Logger.Formatter = &log.TextFormatter{TimestampFormat: time.RFC3339Nano}

	monitorLog = containerMonitorLog
}

func RunE(cmd *cobra.Command, args []string) error {
	initLog(logLevel)

	collector, err := newCollector(address, namespace)
	if err != nil {
		return err
	}
	defer collector.cleanup()

	prometheus.MustRegister(collector)

	http.Handle("/metrics", promhttp.Handler())

	monitorLog.Infof("starting to serve prometheus endpoint at 0.0.0.0:%s/metrics", serverPort)

	return http.ListenAndServe(fmt.Sprintf(":%s", serverPort), nil)
}

func main() {
	rootCmd.Flags().StringVarP(&address, "address", "a", "/run/containerd/containerd.sock", "path to the containerd socket")
	rootCmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "the namespace to get container stats from")
	rootCmd.Flags().StringVarP(&serverPort, "server-port", "p", "8090", "The address the server listens on for HTTP requests.")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "warn", "Logging level (trace/debug/info/warn/error/fatal/panic).")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
