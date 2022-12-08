package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/pkg/profile"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/collector"
	commonlog "sigs.k8s.io/usage-metrics-collector/pkg/log"
	"sigs.k8s.io/usage-metrics-collector/pkg/scheme"
	versioncollector "sigs.k8s.io/usage-metrics-collector/pkg/version"
	"sigs.k8s.io/usage-metrics-collector/pkg/watchconfig"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	logPath                     string
	profileMemory               bool
	exitAfterLeaderElectionLoss time.Duration

	options = Options{Options: ctrl.Options{
		Scheme:           scheme.Scheme,
		LeaderElectionID: "capacity-metrics-prometheus-collector-lock",
	}}
	RootCmd = &cobra.Command{
		RunE: RunE,
	}

	log = commonlog.Log.WithName("collector")

	electedMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "leader_elected",
		Help: "Whether or not this instance is the leader",
	}, []string{"collector_instance"})
)

// Options is set by flags
type Options struct {
	ctrl.Options
	metricsPrometheusCollector string
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

	RootCmd.Flags().StringVar(&options.MetricsBindAddress, "http-addr", ":8080", "Bind address of the webservice exposing the metrics and other endpoints.")

	// Default this to false so that it doesn't try to do leader election when run locally with `go run`
	RootCmd.Flags().BoolVar(&options.LeaderElection, "leader-election", false, "Enable leader election")

	// This is used for integration testing only
	RootCmd.Flags().StringVar(&options.LeaderElectionNamespace, "leader-election-namespace", "", "Set the namespace used for leader election -- for testing only")
	_ = RootCmd.Flags().MarkHidden("leader-election-namespace")

	RootCmd.Flags().StringVar(&logPath, "log-level-filepath", "", "path to log level file.  The file must contain a single integer corresponding to the log level (e.g. 2)")

	RootCmd.Flags().BoolVar(&profileMemory, "profile-memory", false, "set to true to enable memory profiling")

	RootCmd.Flags().DurationVar(&exitAfterLeaderElectionLoss, "exit-after-leader-election-loss", time.Second*15, "if set to a non-zero durtion, exit after leader election loss + duration")

	// Add the go `flag` package flags -- e.g. `--kubeconfig`
	RootCmd.Flags().AddGoFlagSet(flag.CommandLine)

	ctrlmetrics.Registry.MustRegister(electedMetric)
}

// RunE this application and return error if any
func RunE(_ *cobra.Command, _ []string) error {
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
		"http-addr", options.MetricsBindAddress,
		"leader-election", options.LeaderElection,
		"leader-election-namespace", options.LeaderElectionNamespace,
		"log-level-filepath", logPath,
		"profile-memory", profileMemory)

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

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	ctx, _ = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)

	if metrics.MetricsPrometheusCollector.ExitOnConfigChange {
		err := watchconfig.ConfigFile{
			ConfigFilename: options.metricsPrometheusCollector,
		}.WatchConfig(func(w *fsnotify.Watcher, s chan interface{}) {
			log.Info("stopping metrics-prometheus-collector to read new config file")
			stop()
			w.Close()
			close(s)
		})
		if err != nil {
			log.Error(err, "unable to watch config")
		}
	}

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
	// Note: mgr.Start will return an error if leadership is lost and leader-election is used.
	log.Info("starting manager")
	checkError(log, mgr.Start(ctx), "")
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
	return ms.validateCollectorSpec()
}

func (ms *MetricsServer) validateCollectorSpec() error {
	// validate extension labels
	totalExtensionLabels := len(ms.Extensions.Pods)
	totalExtensionLabels += len(ms.Extensions.Namespaces)
	totalExtensionLabels += len(ms.Extensions.Nodes)
	totalExtensionLabels += len(ms.Extensions.Quota)
	totalExtensionLabels += len(ms.Extensions.PVCs)
	totalExtensionLabels += len(ms.Extensions.PVs)
	totalExtensionLabels += len(ms.Extensions.NodeTaints)

	if s, m := totalExtensionLabels, collector.MaxExtensionLabels; s > m {
		return fmt.Errorf("collector config specifies %v extension labels which exceed the max (%v)", s, m)
	}

	return nil
}

// Start starts the metrics server
// nolint: gocyclo
func (ms *MetricsServer) Start(ctx context.Context) error {
	mgr := ms.Mgr
	// Important -- don't register metrics until we are the leader
	// otherwise we will be duplicating values by sending them from multiple instances.
	// This is especially bad since we won't have the utilization data.

	// Note: This shouldn't be necessary -- this function isn't called until we are elected as the leader
	// but it is here as a defensive check.
	<-mgr.Elected()
	log.Info("elected as leader -- serving capacity metrics")

	// mark us as ready to start receiving node samples -- this requires configuring a readiness check
	// in the pod yaml
	ms.Col.UtilizationServer.IsLeaderElected.Store(true)
	// this shouldn't be necessary since we shutdown when we aren't the leader, but
	// serves as a sanity check in case we don't shutdown gracefully and quickly
	defer func() {
		ms.Col.UtilizationServer.IsLeaderElected.Store(false)
		// Ensure that we exit so we stop getting utilization requests
		time.Sleep(exitAfterLeaderElectionLoss)
		log.Info("exiting after leader election loss")
		os.Exit(0)
	}()

	// update the metric showing we are the leader
	electedMetric.WithLabelValues(os.Getenv("POD_NAME")).Set(1)

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

	// don't pre-compute and cache the metrics until we are the leader otherwise they may be ancient
	// when we are elected
	go ms.Col.Run(ctx)

	ctrlmetrics.Registry.MustRegister(ms.Col)

	<-ctx.Done()
	return ctx.Err()
}

// checkError exits in in case of errors printing the given output message
func checkError(log logr.Logger, err error, msg string) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stdout, "%v %s", err, msg)
	os.Exit(1)
}
