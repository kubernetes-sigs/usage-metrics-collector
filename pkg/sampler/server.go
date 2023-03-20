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

package sampler

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"sort"
	"sync/atomic"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/samplerserverv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/ctrstats"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
)

// Server runs the metrics-node-sampler server which samples cgroup metrics from
// the filesystem, and servers them over a GRPC API.
type Server struct {
	// MetricsNodeSampler configures the server
	samplerserverv1alpha1.MetricsNodeSampler `json:",inline" yaml:",inline"`

	// FS is the filesystem to use for reading cgroup metrics.  Defaults to the
	// OS filesystem.
	// +optional
	FS fs.FS

	// TimeFunc is the function used for getting the timestamp for when metrics
	// are read.  Defaults to time.Now.
	// +optional
	TimeFunc func(string) time.Time

	SortResults bool

	stop context.CancelFunc

	cache sampleCache
	api.UnimplementedMetricsServer
	api.UnimplementedHealthServer

	pushFrequency            time.Duration
	checkCreatedPodFrequency time.Duration

	PushHealthy  atomic.Bool
	PushErrorMsg atomic.Value
}

// Stop stops the server from sampling metrics
func (s *Server) Stop() {
	if s.stop != nil {
		s.stop()
	}
}

// Start starts the server sampling metrics and serving them
func (s *Server) Start(ctx context.Context, stop context.CancelFunc) error {
	defer stop() // stop the context when we are done

	s.Default()
	if log.Enabled() {
		val, err := json.MarshalIndent(s.MetricsNodeSampler, "", "  ")
		if err != nil {
			return err
		}
		log.Info("starting metrics-node-sampler", "MetricsNodeSampler", val)
	}
	var err error
	s.pushFrequency, err = time.ParseDuration(s.PushFrequency)
	if err != nil {
		return err
	}
	if s.pushFrequency < 10*time.Second {
		return errors.New("pushFrequencyDuration must be at least 10 seconds")
	}

	s.checkCreatedPodFrequency, err = time.ParseDuration(s.CheckCreatedPodFrequency)
	if err != nil {
		return err
	}
	if s.pushFrequency < time.Second {
		return errors.New("pushFrequencyDuration must be at least 1 seconds")
	}

	errs := make(chan error)

	// run the cache
	s.cache.Buffer = s.Buffer
	s.cache.readerConfig = s.Reader
	s.cache.metricsReader.fs = s.FS
	s.cache.metricsReader.readTimeFunc = s.TimeFunc
	s.cache.metricsReader.ctx = ctx

	if s.UseContainerMonitor {
		log.Info("initializing container-monitor metrics")
		s.cache.UseContainerMonitor = s.UseContainerMonitor
		s.cache.ContainerdClient, err = ctrstats.NewContainerdClient(s.ContainerdAddress, s.ContainerdNamespace)
		if err != nil {
			log.Error(err, "unable to create containerd client")
			return err
		}
	}

	go func() {
		errs <- errors.WithStack(s.cache.Start(ctx))
	}()

	pbPort := fmt.Sprintf(":%v", s.PBPort)
	lis, err := net.Listen("tcp", pbPort)
	if err != nil {
		return errors.WithStack(err)
	}

	// run the protocol buffer service
	go func() {
		rpcServer := grpc.NewServer()
		api.RegisterMetricsServer(rpcServer, s)
		api.RegisterHealthServer(rpcServer, s)
		log.V(1).Info("serving proto", "address", lis.Addr())
		defer rpcServer.GracefulStop() // shutdown the server when done
		go func() { errs <- errors.WithStack(rpcServer.Serve(lis)) }()
		<-ctx.Done() // wait for shutdown
	}()

	// setup the json service
	conn, err := grpc.DialContext(
		ctx,
		"0.0.0.0"+pbPort,
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	defer conn.Close()
	gwmux := runtime.NewServeMux()
	err = api.RegisterMetricsHandler(context.Background(), gwmux, conn)
	if err != nil {
		return err
	}
	err = api.RegisterHealthHandler(context.Background(), gwmux, conn)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf(":%v", s.RestPort)
	gwServer := &http.Server{Addr: addr, Handler: gwmux}
	rpcServer := grpc.NewServer()
	api.RegisterMetricsServer(rpcServer, s)
	api.RegisterHealthServer(rpcServer, s)

	// run the REST service
	go func() {
		log.V(1).Info("serving json", "address", addr)
		defer rpcServer.GracefulStop()                                      // stop the server when we are done
		go func() { errs <- errors.WithStack(gwServer.ListenAndServe()) }() // run the server
		<-ctx.Done()                                                        // wait until the context is cancelled
	}()

	if s.PushAddress != "" {
		s.PushErrorMsg.Store("starting push metrics")
		s.PushHealthy.Store(false)
		go func() { errs <- s.PushMetrics(ctx) }()
	} else {
		s.PushHealthy.Store(true)
	}

	// block until we are shutdown or there is an error in a service
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		return nil
	}
}

// PushMetrics pushes utilization metrics to a server
func (s *Server) PushMetrics(ctx context.Context) error {
	var connectErrorMessage string
	log := log.WithValues("address", s.PushAddress)
	return wait.PollImmediateInfiniteWithContext(ctx, time.Second*30, func(ctx context.Context) (done bool, err error) {
		// continously try to connect and push metrics
		retry, err := s.connectAndPushMetrics(ctx)
		if err != nil {
			s.PushErrorMsg.Store(err.Error())
			s.PushHealthy.Store(false)
			if err.Error() != connectErrorMessage { // don't spam the log file since we retry
				log.Error(err,
					"unable to establish grpc connection to prometheus-collector instance")
			}
		}
		return retry, nil // never return an error, it stops polling if we do
	})
}

// foundUnsentPods returns true if we have discovered containers that we haven't
// sent metrics for their pods.
func (s *Server) foundUnsentPods(ctx context.Context, lastResp *api.ListMetricsResponse) bool {
	if lastResp == nil {
		// we've never sent pods
		return true
	}

	// get the pods we've already sent
	last := sets.NewString()
	for _, c := range lastResp.Containers {
		last.Insert(c.PodUID)
	}

	// get the pods we have metrics for
	known := s.cache.metricsReader.knownContainersSet.Load().(sets.String)

	return known.Difference(last).Len() > 0 // we know about pods we haven't sent
}

func (s *Server) connectAndPushMetrics(ctx context.Context) (bool, error) {
	log := log.WithValues("address", s.PushAddress)

	// setup a connection to the server to push metrics
	conn, err := grpc.Dial(s.PushAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return false, err // re-establish the connection
	}

	// setup a client to the server to push metrics
	client := api.NewMetricsCollectorClient(conn)

	// use a different context for the client so we can send the last metrics before
	// shutting down.  this won't work with the old context since it is cancelled
	sendCtx, stop := context.WithCancel(context.Background())
	defer stop() // cancel the send context
	stream, err := client.PushMetrics(sendCtx)
	if err != nil {
		return false, err // re-establish the connection
	}
	defer func() {
		log.Info("closed stream", "err", stream.CloseSend())
	}() // send EOF

	log.Info("established grpc connection to prometheus-collector instance")

	// continuously push metrics to the server
	ticker := time.NewTicker(s.pushFrequency)
	createdPodTicker := time.NewTicker(s.checkCreatedPodFrequency)
	defer ticker.Stop()
	defer createdPodTicker.Stop()
	var lastSent *api.ListMetricsResponse
	sendTryCount := s.SendPushMetricsRetryCount + 1 // try to send at least once
	reason := "unknown"
	for {
		select {
		case <-ctx.Done():
			log.Info("sending final metrics to collector before shutdown", "node", nodeName)
			resp, err := s.ListMetrics(ctx, &api.ListMetricsRequest{})
			resp.Reason = "sampler-shutdown"
			if err != nil {
				log.Error(errors.WithStack(err), "unable to list metrics")
				// don't re-enter loop -- we need to shutdown
			} else if err := stream.Send(resp); err != nil {
				log.Error(errors.WithStack(err), "unable to send metrics")
				// don't re-enter loop -- we need to shutdown
			}
			log.Info("closing push metrics stream")
			return true, nil // we are done
		case <-ticker.C:
			// block until it is time to send metrics
			reason = "periodic-sync"
		case <-createdPodTicker.C: // check for new pods that we haven't seen
			// check if there are any pods we haven't sent metrics for
			if !s.foundUnsentPods(ctx, lastSent) {
				// don't send the metrics if we don't have any new pods
				continue
			}
			reason = "found-new-pods"
		}
		if lastSent == nil {
			// we haven't sent metrics yet
			reason = "initial-sync"
		}
		var resp *api.ListMetricsResponse
		for i := 0; i < sendTryCount; i++ {
			resp, err = s.ListMetrics(ctx, &api.ListMetricsRequest{})
			if err == nil {
				break
			}
			log.Error(errors.WithStack(err), "unable to list metrics locally")
			s.PushErrorMsg.Store(err.Error())
			s.PushHealthy.Store(false) // we were not able to push metrics
		}
		resp.Timestamp = timestamppb.Now()
		resp.Reason = reason
		if err := stream.Send(resp); err != nil {
			log.Error(errors.WithStack(err), "unable to send metrics to collector server")
			return false, err // may be an error with the connection -- re-establish the connection
		}
		s.PushHealthy.Store(true) // we are able to push metrics
		log.V(1).Info("sent metrics to collector", "node", nodeName)
		lastSent = resp
	}
}

var (
	// nodeName gets the node name from the downward API
	nodeName = os.Getenv("NODE_NAME")
	// podName gets the node name from the downward API
	podName = os.Getenv("POD_NAME")
)

// ListMetrics lists the aggregated metrics for all containers and nodes
func (s *Server) ListMetrics(context.Context, *api.ListMetricsRequest) (*api.ListMetricsResponse, error) {
	var result api.ListMetricsResponse
	samples, _ := s.cache.getAllSamples()
	result.Reason = "pull"

	for k, v := range samples.containers {
		c := &api.ContainerMetrics{
			ContainerID: k.ContainerID,
			PodUID:      k.PodUID,
		}
		for _, s := range v {
			c.CpuCoresNanoSec = append(c.CpuCoresNanoSec, int64(s.CPUCoresNanoSec))
			c.CpuThrottledNanoSec = append(c.CpuThrottledNanoSec, int64(s.CPUThrottledUSec))
			c.CpuPercentPeriodsThrottled = append(c.CpuPercentPeriodsThrottled, float32(s.CPUPercentPeriodsThrottled))
			c.MemoryBytes = append(c.MemoryBytes, int64(s.MemoryBytes))
			c.OomCount = append(c.OomCount, int64(s.CumulativeMemoryOOM))
			c.OomKillCount = append(c.OomKillCount, int64(s.CumulativeMemoryOOMKill))
			c.CpuPeriodsSec = append(c.CpuPeriodsSec, int64(s.CPUPeriodsSec))
			c.CpuThrottledPeriodsSec = append(c.CpuThrottledPeriodsSec, int64(s.CPUThrottledPeriodsSec))
		}
		if s.SortResults {
			// sort the values so the results are stable
			sort.Sort(Int64Slice(c.CpuCoresNanoSec))
			sort.Sort(Int64Slice(c.CpuThrottledNanoSec))
			sort.Sort(Int64Slice(c.MemoryBytes))
			sort.Sort(Int64Slice(c.OomCount))
			sort.Sort(Int64Slice(c.OomKillCount))
			sort.Sort(Float32Slice(c.CpuPercentPeriodsThrottled))
			sort.Sort(Int64Slice(c.CpuPeriodsSec))
			sort.Sort(Int64Slice(c.CpuThrottledPeriodsSec))
		}
		result.Containers = append(result.Containers, c)
	}

	var node api.NodeMetrics

	for level, values := range samples.node {
		var CpuCoresNanoSec, MemoryBytes []int64
		for _, value := range values {
			CpuCoresNanoSec = append(CpuCoresNanoSec, int64(value.CPUCoresNanoSec))
			MemoryBytes = append(MemoryBytes, int64(value.MemoryBytes))
		}

		if s.SortResults {
			sort.Sort(Int64Slice(CpuCoresNanoSec))
			sort.Sort(Int64Slice(MemoryBytes))
		}

		nodeAggregatedMetrics := api.NodeAggregatedMetrics{
			AggregationLevel: string(level),
			CpuCoresNanoSec:  CpuCoresNanoSec,
			MemoryBytes:      MemoryBytes,
		}

		node.AggregatedMetrics = append(node.AggregatedMetrics, &nodeAggregatedMetrics)
	}
	if s.SortResults {
		sort.Sort(NodeAggregatedMetricsSlice(node.AggregatedMetrics))
	}
	result.Node = &node

	result.NodeName = nodeName
	result.PodName = podName

	if log.V(5).Enabled() {
		// condition logging to save the String() call
		log.V(5).Info("list-metrics", "response", result.String())
	}

	return &result, nil
}

type Int64Slice []int64
type Float32Slice []float32
type NodeAggregatedMetricsSlice []*api.NodeAggregatedMetrics

func (x Int64Slice) Len() int           { return len(x) }
func (x Int64Slice) Less(i, j int) bool { return x[i] < x[j] }
func (x Int64Slice) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

func (x Float32Slice) Len() int           { return len(x) }
func (x Float32Slice) Less(i, j int) bool { return x[i] < x[j] }
func (x Float32Slice) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

func (x NodeAggregatedMetricsSlice) Len() int { return len(x) }
func (x NodeAggregatedMetricsSlice) Less(i, j int) bool {
	return x[i].AggregationLevel < x[j].AggregationLevel
}
func (x NodeAggregatedMetricsSlice) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

func (s *Server) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	if !s.PushHealthy.Load() {
		var msg string
		if v := s.PushErrorMsg.Load(); v != nil {
			msg = v.(string)
		}
		return nil, status.Error(codes.Internal, msg)
	}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}
