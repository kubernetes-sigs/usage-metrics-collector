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

package utilization

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	commonlog "sigs.k8s.io/usage-metrics-collector/pkg/log"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
)

var (
	log = commonlog.Log.WithName("collector-utilization")

	responseAgeSeconds *prometheus.GaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metrics_prometheus_collector_server_utilization_response_age_seconds",
		Help: "",
	}, []string{"exported_node", "exported_pod", "reason"})

	requestsTotal *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "metrics_prometheus_collector_server_utilization_requests_total",
		Help: "",
	}, []string{"exported_node", "exported_pod"})

	expiredTotal *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "metrics_prometheus_collector_server_utilization_expired_total",
		Help: "",
	}, []string{"exported_node", "exported_pod"})

	requestErrorsTotal *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "metrics_prometheus_collector_server_utilization_request_errors_total",
		Help: "",
	}, []string{"exported_node", "exported_pod", "reason"})

	summaryErrorsTotal *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "metrics_prometheus_collector_server_utilization_summary_errors_total",
		Help: "",
	}, []string{"exported_node", "exported_pod", "reason"})

	severErrorsTotal *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "metrics_prometheus_collector_server_utilization_sever_errors_total",
		Help: "",
	}, []string{"reason"})

	nonLeaderRequestsTotal *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "metrics_prometheus_collector_server_utilization_non_leader_requests_total",
		Help: "",
	}, []string{"exported_node", "exported_pod", "collector_instance"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(nonLeaderRequestsTotal)
}

type Server struct {
	ResponseMutext sync.RWMutex

	// Responses has the last response for each node keyed by the node name
	Responses map[string]*api.ListMetricsResponse `json:"responses" yaml:"responses"`

	api.UnimplementedMetricsCollectorServer // required
	api.UnimplementedHealthServer           // required

	collectorcontrollerv1alpha1.UtilizationServer

	expireFreq time.Duration
	ttl        time.Duration

	IsLeaderElected atomic.Bool
	IsHealthy       atomic.Bool

	grpcServer *grpc.Server
}

func (c *Server) Collect(ch chan<- prometheus.Metric, metrics map[string]*api.ListMetricsResponse) {
	responseAgeSeconds.Reset()
	for _, v := range metrics {
		responseAgeSeconds.WithLabelValues(v.NodeName, v.PodName, v.Reason).Set(
			time.Since(v.Timestamp.AsTime()).Seconds())
	}
	responseAgeSeconds.Collect(ch)
	requestsTotal.Collect(ch)
	requestErrorsTotal.Collect(ch)
	expiredTotal.Collect(ch)
	summaryErrorsTotal.Collect(ch)
	severErrorsTotal.Collect(ch)
}

func (s *Server) Start(ctx context.Context) error {
	// parse the utilization server configuration values
	var err error
	if s.TTLDuration == "" {
		s.ttl = time.Minute * 5
	} else {
		s.ttl, err = time.ParseDuration(s.TTLDuration)
		if err != nil {
			return err
		}
		if s.ttl < time.Minute {
			return errors.New("ttlDuration must be at least 1 minute")
		}
	}
	if s.ExpireReponsesFrequencyDuration == "" {
		s.expireFreq = time.Minute * 15
	} else {
		s.expireFreq, err = time.ParseDuration(s.ExpireReponsesFrequencyDuration)
		if err != nil {
			return err
		}
		if s.expireFreq < time.Minute*5 {
			return errors.New("expireReponsesFrequencyDuration must be at least 5 minutes")
		}
	}

	if s.ProtoBindPort == 0 {
		return nil
	}

	// Setup the GRPC server
	s.Responses = make(map[string]*api.ListMetricsResponse)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%v", s.ProtoBindPort))
	if err != nil {
		severErrorsTotal.WithLabelValues("failed-listen").Inc()
		log.Error(err, "unable to bind MetricsCollectorServer listen address")
		return err
	}
	s.grpcServer = grpc.NewServer()
	api.RegisterMetricsCollectorServer(s.grpcServer, s)
	api.RegisterHealthServer(s.grpcServer, s)
	log.V(1).Info("listening for utilization metrics", "port", s.ProtoBindPort)

	errs := make(chan error)
	go func() {
		defer s.grpcServer.GracefulStop()
		go func() { errs <- s.grpcServer.Serve(lis) }()
		<-ctx.Done()
	}()

	// setup the JSON service
	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf("0.0.0.0:%v", s.ProtoBindPort),
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		errs <- errors.Wrap(err, "unable to run json server")
		return err
	}
	defer conn.Close()
	gwmux := runtime.NewServeMux()
	err = api.RegisterHealthHandler(context.Background(), gwmux, conn)
	if err != nil {
		return err
	}
	gwServer := &http.Server{Addr: fmt.Sprintf(":%v", s.JSONBindPort), Handler: gwmux}
	rpcServer := grpc.NewServer()
	api.RegisterHealthServer(rpcServer, s)
	go func() {
		log.V(1).Info("serving json", "port", s.JSONBindPort)
		defer rpcServer.GracefulStop()                                      // stop the server when we are done
		go func() { errs <- errors.WithStack(gwServer.ListenAndServe()) }() // run the server
		<-ctx.Done()                                                        // wait until the context is cancelled
	}()

	go s.expireEntries()

	s.IsHealthy.Store(true)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		return nil
	}
}

func (s *Server) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	if !s.IsHealthy.Load() {
		return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING}, fmt.Errorf("not-healthy")
	}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

// Ready returns success if the service should be accepting traffic
func (s *Server) IsLeader(context.Context, *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	if !s.IsLeaderElected.Load() {
		return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING}, fmt.Errorf("not-leader")
	}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

// expireEntries periodically cleans up expired utilization responses from the cache
func (s *Server) expireEntries() {
	ticker := time.NewTicker(s.expireFreq)
	for range ticker.C {
		m := s.getMetrics(false)
		for k, v := range m {
			if time.Since(v.Timestamp.AsTime()) > s.ttl {
				expiredTotal.WithLabelValues(k, v.PodName).Inc()
				log.V(5).Info("expiring utlization metrics", "node", v.NodeName, "pod", v.PodName)
				s.ClearMetrics(k) // Delete the metric if it is past its time to live
			}
		}
	}
}

// PushMetrics receives metrics pushed to the collector by node-samplers
func (s *Server) PushMetrics(req api.MetricsCollector_PushMetricsServer) error {
	var nodeName, podName string
	for {
		msg, err := req.Recv() // Read metrics message
		if !s.IsLeaderElected.Load() {
			// We shouldn't be getting metrics if we aren't the leader.
			// This can happen if we restart and the endpoints isn't updated to remove us yet.
			log.Error(fmt.Errorf("got node samples when not leader"), "only the leader should get node samples.  Possible that Readiness checks are not setup.")
			nonLeaderRequestsTotal.WithLabelValues(nodeName, podName, os.Getenv("POD_NAME")).Inc()
			s.grpcServer.Stop() // stop the server immediately
			os.Exit(1)          // exit so we restart with the new server
		}

		if err == io.EOF {
			log.V(5).Info("read utilization metrics eof", "node", nodeName, "pod", podName)
			requestErrorsTotal.WithLabelValues(nodeName, podName, "eof").Inc()
			return nil // Connection closed
		}
		if err != nil && strings.Contains(err.Error(), "context canceled") {
			log.V(5).Info("utilization metics context cancelled", "node", nodeName, "pod", podName)
			requestErrorsTotal.WithLabelValues(nodeName, podName, "context-cancelled").Inc()
			return nil // Connection closed
		}
		if err != nil {
			log.Error(err, "error reading utilization metrics message", "node", nodeName, "pod", podName)
			requestErrorsTotal.WithLabelValues(nodeName, podName, "failed-recv").Inc()
			return err
		}
		if msg.NodeName != "" {
			nodeName = msg.NodeName // Save the node name for error logging
		}
		if msg.PodName != "" {
			podName = msg.PodName // Save the pod name for error logging
		}

		if msg.NodeName == "" { // we cannot safely cache these by the node-name if it isn't present
			log.Info("metrics missing nodeName", "node", nodeName, "pod", podName)
			requestErrorsTotal.WithLabelValues(nodeName, podName, "missing-node-name").Inc()
			continue
		}

		log.V(5).Info("read utilization metrics message", "node", nodeName, "pod", podName)
		requestsTotal.WithLabelValues(nodeName, podName).Inc()

		msg.Timestamp = timestamppb.Now() // Record the time we received the values
		s.CacheMetrics(msg)               // Update the cached utilization metrics
	}
}

// GetContainerUsageSummary maps containers to their cached metric values.  metrics are passed as an argument to
// reduce lock contention.
func (c *Server) GetContainerUsageSummary(metrics map[string]*api.ListMetricsResponse) map[sampler.ContainerKey]*api.ContainerMetrics {
	// Transform map of node -> utilization to map of container -> utilization by pulling the containers
	// out of each node response
	var values = map[sampler.ContainerKey]*api.ContainerMetrics{}
	for n, r := range metrics {
		for _, c := range r.Containers {
			c.NodeName = n
			if c.PodUID == "" { // this should never happen
				log.Error(errors.New("pod-id missing from summary"), "response", r)
				summaryErrorsTotal.WithLabelValues(n, r.PodName, "missing-pod-id").Inc()
				continue
			}
			if c.ContainerID == "" { // this may happen for microvms or other edge cases
				log.V(1).Error(errors.New("container-id missing from summary"), "pod", c.PodUID)
				summaryErrorsTotal.WithLabelValues(n, r.PodName, "missing-container-id").Inc()
				continue
			}
			values[sampler.ContainerKey{ContainerID: c.ContainerID, PodUID: c.PodUID}] = c
		}
	}
	log.V(1).Info("return container summary", "container-count", len(values))
	return values
}

const FilterExpiredMetricsEnvName = "COLLECTOR_FILTER_EXPIRED_UTILIZATION_METRICS"

func (s *Server) GetMetrics() map[string]*api.ListMetricsResponse {
	filter := os.Getenv(FilterExpiredMetricsEnvName) // used for testing
	if filter == "false" || filter == "FALSE" {
		return s.getMetrics(false)
	}
	return s.getMetrics(true)
}

// GetMetrics returns a copy of the current cached metrics
func (s *Server) getMetrics(filterExpired bool) map[string]*api.ListMetricsResponse {
	values := make(map[string]*api.ListMetricsResponse, len(s.Responses))
	func() {
		s.ResponseMutext.Lock()
		defer s.ResponseMutext.Unlock()
		for k, v := range s.Responses {
			values[k] = v
		}
	}()

	if filterExpired {
		// Even though we periodically expire values from the map, the ttl isn't guaranteed.
		// Double check for expired values when we return them
		for k, v := range values {
			if time.Since(v.Timestamp.AsTime()) > s.ttl {
				delete(values, k)
			}
		}
	}

	return values
}

// ClearMetrics deletes the metrics with the given key from the cache
func (s *Server) ClearMetrics(key string) {
	s.ResponseMutext.Lock()
	defer s.ResponseMutext.Unlock()
	delete(s.Responses, key)
}

// CacheMetrics caches msg
func (s *Server) CacheMetrics(msg *api.ListMetricsResponse) {
	s.ResponseMutext.Lock()
	defer s.ResponseMutext.Unlock()
	s.Responses[msg.NodeName] = msg
}
