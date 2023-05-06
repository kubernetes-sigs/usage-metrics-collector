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
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/go-logr/logr"
	"github.com/pkg/profile"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/util/retry"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/collector"
	commonlog "sigs.k8s.io/usage-metrics-collector/pkg/log"
	"sigs.k8s.io/usage-metrics-collector/pkg/scheme"
	versioncollector "sigs.k8s.io/usage-metrics-collector/pkg/version"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	logPath, pprofPort                        string
	profileMemory, pprof                      bool
	leaseDuration, renewDeadline, retryPeriod time.Duration
	exitAfterLeaderElectionLoss               bool

	options = Options{Options: ctrl.Options{Scheme: scheme.Scheme}}
	RootCmd = &cobra.Command{
		RunE: RunE,
	}

	log = commonlog.Log.WithName("collector")

	electedMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kube_usage_leader_elected",
		Help: "Whether or not this instance is the leader",
	}, []string{"collector_instance"})
	collectTimeMetric = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kube_usage_collect_cache_time",
		Help: "How long it took to create the last cache response",
	})
)

// Options is set by flags
type Options struct {
	ctrl.Options
	LeaderElection             bool
	LeaderElectionNamespace    string
	LeaderElectionLockName     string
	PodName                    string
	metricsPrometheusCollector string
	externalBindAddress        string
}

// Execute root command
func Execute(v, c, d string) error {
	version = v
	commit = c
	date = d

	fmt.Printf("Build info version: %s, commit: %s, date: %s\n", version, commit, date)
	return RootCmd.Execute()
}

func init() {
	RootCmd.Flags().StringVar(&options.metricsPrometheusCollector, "collector-config-filepath", "", "Path to a MetricsPrometheusCollector json or yaml configuration file.")
	_ = RootCmd.MarkFlagRequired("collector-config-filepath")

	RootCmd.Flags().StringVar(&options.MetricsBindAddress, "internal-http-addr", "127.0.0.1:8099", "Bind address of the webservice exposing the metrics and other endpoints.")
	RootCmd.Flags().MarkHidden("internal-http-addr") // this is scraped internally to create a cacheable response
	RootCmd.Flags().StringVar(&options.externalBindAddress, "http-addr", ":8080", "Bind address to read the cached metrics.")

	// Default this to false so that it doesn't try to do leader election when run locally with `go run`
	RootCmd.Flags().BoolVar(&options.LeaderElection, "leader-election", false, "Enable leader election")

	// This is used for integration testing only
	RootCmd.Flags().StringVar(&options.LeaderElectionNamespace, "leader-election-namespace", os.Getenv("POD_NAMESPACE"), "Set the namespace used for leader election")
	RootCmd.Flags().StringVar(&options.LeaderElectionLockName, "leader-election-lock-name", "metrics-prometheus-collector", "Set the lock name used for leader election")
	RootCmd.Flags().StringVar(&options.PodName, "leader-election-pod-name", os.Getenv("POD_NAME"), "Set the id used for leader election")

	RootCmd.Flags().StringVar(&logPath, "log-level-filepath", "", "path to log level file.  The file must contain a single integer corresponding to the log level (e.g. 2)")

	RootCmd.Flags().BoolVar(&profileMemory, "profile-memory", false, "set to true to enable memory profiling")

	RootCmd.Flags().BoolVar(&pprof, "pprof", false, "set to true to enable pprof profiling")

	RootCmd.Flags().StringVar(&pprofPort, "pprof-port", "6060", "pprof port")

	RootCmd.Flags().DurationVar(&leaseDuration, "lease-duration", 30*time.Second, "controller manager lease duration")
	RootCmd.Flags().DurationVar(&renewDeadline, "renew-deadline", 20*time.Second, "controller manager lease renew deadline")
	RootCmd.Flags().DurationVar(&retryPeriod, "retry-period", 2*time.Second, "controller manager lease renew deadline")

	RootCmd.Flags().BoolVar(&exitAfterLeaderElectionLoss, "exit-after-leader-election-loss", true, "if true, exit the process after leader election loss")

	// Add the go `flag` package flags -- e.g. `--kubeconfig`
	RootCmd.Flags().AddGoFlagSet(flag.CommandLine)

	ctrlmetrics.Registry.MustRegister(electedMetric)
	ctrlmetrics.Registry.MustRegister(collectTimeMetric)
}

// RunE this application and return error if any
func RunE(_ *cobra.Command, _ []string) error {
	if pprof {
		// Server for pprof
		go func() {
			http.ListenAndServe(fmt.Sprintf(":%s", pprofPort), nil)
		}()
	}

	electedMetric.WithLabelValues(os.Getenv("POD_NAME")).Set(0)
	if profileMemory {
		defer profile.Start(profile.MemProfile).Stop()
	}

	if logPath != "" {
		// dynamically set log level
		watcher, stop, err := commonlog.WatchLevel(logPath)
		if err != nil {
			return err
		}
		defer watcher.Close()
		defer close(stop)
	}

	log.Info("initializing with options",
		"collector-config-filepath", options.metricsPrometheusCollector,
		"internal-http-addr", options.MetricsBindAddress,
		"http-addr", options.externalBindAddress,
		"leader-election", options.LeaderElection,
		"leader-election-namespace", options.LeaderElectionNamespace,
		"log-level-filepath", logPath,
		"profile-memory", profileMemory,
		"leaseDuration", leaseDuration,
		"renewDeadline", renewDeadline,
		"pprof", pprof,
		"pprofPort", pprofPort)

	// get the Kubernetes config
	restConfig, err := ctrl.GetConfig()
	checkError(log, err, "unable to get Kubernetes client config")

	metrics := MetricsServer{}
	err = metrics.ReadCollectorSpec()
	checkError(log, err, "unable to read collector config")

	// register the version before we are the leader
	ctrlmetrics.Registry.MustRegister(
		&versioncollector.Collector{
			Version: version, Commit: commit, Date: date, Name: metrics.Prefix + "_version"})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	metrics.stop = stop
	metrics.ctx = ctx

	options.Options.NewCache = collector.GetNewCacheFunc(metrics.MetricsPrometheusCollector)

	log.Info("creating manager")
	mgr, err := ctrl.NewManager(restConfig, options.Options)
	checkError(log, err, "unable to create manager config")

	// register a simple ping health-check endpoint with the manager
	err = mgr.AddMetricsExtraHandler("/healthz", healthz.CheckHandler{Checker: healthz.Ping})
	checkError(log, err, "unable to configure health-check endpoint")

	metrics.Mgr = mgr
	if err := mgr.Add(&metrics); err != nil {
		checkError(log, err, "unable to add metrics to the manager")
	}

	// Initialize the collector so we aren't cold when we become the leader
	metrics.Col, err = collector.NewCollector(ctx, mgr.GetClient(), &metrics.MetricsPrometheusCollector)
	if err != nil {
		return err
	}

	// If leader election is enabled, then only the leader instance publishes
	// metrics to prometheus.  This ensures that a `sum` operation doesn't
	// sum metrics across the instances.  Leader election is preferred both with
	// multiple instances, and when using a rolling update strategy.
	// Initially set the value to false and change it to true once we are the leader,
	// we can then start publishing.
	// If we are not using leader election set to true.  This is preferred if using
	// a single replica with a replace update strategy.
	metrics.Col.IsLeaderElected.Store(!options.LeaderElection)

	go func() {
		// NOTE: we don't want to start getting node sample traffic until we are the leader
		// so we use a readiness check in the config and don't report ourselves
		// as ready until we are elected leader.
		// This will remove us from the service endpoints for pushing metrics from the
		// node samplers.
		commonlog.Log.Info("starting grpc server")
		if err := metrics.Col.UtilizationServer.Start(ctx); err != nil {
			log.Error(err, "failed to start utilization server")
			panic(err)
		}
	}()

	// start the manager and the reconcilers
	log.Info("starting manager")
	checkError(log, metrics.Mgr.Start(ctx), "")
	return nil
}

// MetricsServer registers the capacity metrics with the controller-manager
// It ensures metrics are only served by the leader by registerig the
// metrics collectors *after* this instance is elected as the leader
// rather than at startup.
type MetricsServer struct {
	Mgr ctrl.Manager
	Col *collector.Collector
	collectorcontrollerv1alpha1.MetricsPrometheusCollector

	// cachedResponseBody contains a cached copy of the http response
	// body to return from the /metrics http endpoint
	cachedResponseBody atomic.Value
	stop               context.CancelFunc
	ctx                context.Context
}

func (ms *MetricsServer) ReadCollectorSpec() error {
	// parse the CollectorSpec
	b, err := os.ReadFile(options.metricsPrometheusCollector)
	if err != nil {
		return err
	}
	spec := bytes.NewBuffer(b)
	if filepath.Ext(options.metricsPrometheusCollector) == ".yaml" {
		d := yaml.NewDecoder(spec)
		d.KnownFields(true)
		err := d.Decode(&ms.MetricsPrometheusCollector)
		if err != nil {
			return err
		}
	} else if filepath.Ext(options.metricsPrometheusCollector) == ".json" {
		d := json.NewDecoder(spec)
		d.DisallowUnknownFields()
		err := d.Decode(&ms.MetricsPrometheusCollector)
		if err != nil {
			return err
		}
	}

	return collectorcontrollerv1alpha1.ValidateCollectorSpecAndApplyDefaults(&ms.MetricsPrometheusCollector)
}

// Start starts the metrics server
// nolint: gocyclo
func (ms *MetricsServer) Start(ctx context.Context) error {
	// TODO: try to do this before becoming the leader.  This isn't trivial
	// because the manager won't let us use the caches prior to starting.
	log.Info("initializing informers")
	if err := ms.Col.InitInformers(ctx); err != nil {
		log.Error(err, "failed to initialize informers")
		panic(err)
	}

	if log.V(1).Enabled() {
		val, err := json.MarshalIndent(ms.MetricsPrometheusCollector, "", "  ")
		if err != nil {
			return err
		}
		log.V(1).Info("starting metrics-prometheus-collector", "MetricsPrometheusCollector", val)
	}

	ctrlmetrics.Registry.MustRegister(ms.Col)

	// start pre-computing metrics eagerly.  we won't publish them until we are the leader.
	go ms.Col.Run(ctx)

	if options.LeaderElection {
		ms.doLeaderElection(ctx)
	} else {
		electedMetric.WithLabelValues(os.Getenv("POD_NAME")).Set(1)
	}

	// this shouldn't be necessary since we shutdown when we aren't the leader, but
	// serves as a sanity check in case we don't shutdown gracefully and quickly

	go ms.continouslyCacheResponse() // continuously read metrics and cache the result
	go ms.serveMetricsFromCache()    // serve "/metrics" http requests from the cache

	<-ctx.Done()
	log.Info("ending sampler server", "reason", ctx.Err())
	return nil
}

// cache the response from making a local http request
type cachedResponse struct {
	data    []byte
	headers http.Header
}

// continouslyCacheResponse continuously hits the collector metrics endpoint and
func (ms *MetricsServer) continouslyCacheResponse() {
	t := time.NewTicker(ms.PreComputeMetrics.Frequency)
	defer t.Stop()
	for {
		// make sure we are getting metrics with utilization
		metricsReady := ms.Col.UtilizationServer.IsReadyResult.Load()
		start := time.Now()
		err := retry.OnError(
			wait.Backoff{
				Duration: time.Second * 6, Factor: 1,
				Steps: 20, Jitter: 0, Cap: time.Minute * 2,
			},
			func(err error) bool { return true },
			func() error {
				resp, err := http.Get("http://" + options.MetricsBindAddress + "/metrics")
				if err != nil {
					log.Error(err, "unable to read cached metrics response")
					return err
				}
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					log.Error(err, "unable to read cached metrics body")
					return err
				}
				resp.Body.Close()
				ms.cachedResponseBody.Store(cachedResponse{
					data:    body,
					headers: resp.Header,
				})
				if metricsReady {
					// don't declare the pod as ready until we have cached metrics
					// that include utilization from a sufficient number of nodes.
					ms.Col.UtilizationServer.IsCached.Store(true)
				}
				return nil
			})
		if err != nil {
			log.Error(err, "unable to cache metrics by reading local /metrics")
			ms.stop()
			os.Exit(1)
		}
		log.Info("finished caching metrics response",
			"seconds", time.Since(start).Seconds())
		collectTimeMetric.Set(time.Since(start).Seconds())
		select {
		case <-t.C:
			// re-populate the cache
			continue
		case <-ms.ctx.Done():
			return
		}
	}
}

func (ms *MetricsServer) serveMetricsFromCache() {
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		resp := ms.cachedResponseBody.Load()
		if resp == nil {
			return
		}
		cr := resp.(cachedResponse)
		_, err := w.Write(cr.data)
		if err != nil {
			log.Error(err, "unable to write metrics response")
			w.WriteHeader(http.StatusInternalServerError)
		}
		for k, v := range cr.headers {
			for i := range v {
				w.Header().Add(k, v[i])
			}
		}
	})
	if err := http.ListenAndServe(options.externalBindAddress, nil); err != nil {
		log.Error(err, "failed to listen on cached metrics address")
		ms.stop() // exit
		os.Exit(1)
	}
}

func (ms *MetricsServer) doLeaderElection(ctx context.Context) {
	ms.Col.IsLeaderElected.Store(false)
	config := ms.Mgr.GetConfig()
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      options.LeaderElectionLockName,
			Namespace: options.LeaderElectionNamespace,
		},
		Client: clientset.NewForConfigOrDie(config).CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: options.PodName,
		},
	}
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Name:            options.LeaderElectionLockName,
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(c context.Context) {
				log.Info("acquired leadership", "id", options.PodName)
				electedMetric.WithLabelValues(os.Getenv("POD_NAME")).Set(1)
				ms.Col.IsLeaderElected.Store(true)
			},
			OnStoppedLeading: func() {
				log.Info("lost leadership", "id", options.PodName)
				electedMetric.WithLabelValues(os.Getenv("POD_NAME")).Set(0)
				ms.Col.IsLeaderElected.Store(false)
			},
			OnNewLeader: func(current_id string) {
				if current_id == options.PodName {
					log.Info("acquired leadership on change", "id", options.PodName, "leader_id", current_id)
					electedMetric.WithLabelValues(os.Getenv("POD_NAME")).Set(1)
					ms.Col.IsLeaderElected.Store(true)
				} else {
					if exitAfterLeaderElectionLoss && ms.Col.IsLeaderElected.Load() {
						// only exit if we gave up leadership
						defer func() {
							log.Info("exiting after leadership election loss")
							os.Exit(0)
						}()
					}
					log.Info("lost leadership on change", "id", options.PodName, "leader_id", current_id)
					electedMetric.WithLabelValues(os.Getenv("POD_NAME")).Set(0)
					ms.Col.IsLeaderElected.Store(false)
				}
			},
		},
	})
}

// checkError exits in in case of errors printing the given output message
func checkError(log logr.Logger, err error, msg string) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stdout, "%v %s", err, msg)
	os.Exit(1)
}
