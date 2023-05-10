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
	"strings"
	"sync"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
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

	pushers     map[string]*pusher
	pushersLock sync.Mutex

	CTX context.Context
}

func (s *Server) Stop() {
	if s.stop != nil {
		s.stop()
	}
}

// Start starts the server sampling metrics and serving them
func (s *Server) Start(ctx context.Context, stop context.CancelFunc) error {
	s.CTX = ctx
	s.stop = stop

	s.pushers = make(map[string]*pusher)
	if s.DNSSpec.FailuresBeforeExit == 0 {
		s.DNSSpec.FailuresBeforeExit = 30
	}
	if s.DNSSpec.BackoffSeconds == 0 {
		s.DNSSpec.BackoffSeconds = 60
	}
	if s.DNSSpec.PollSeconds == 0 {
		s.DNSSpec.PollSeconds = 60
	}
	if s.DNSSpec.CollectorServerExpiration == 0 {
		s.DNSSpec.CollectorServerExpiration = time.Minute * 20
	}

	defer stop() // stop the context if we encounter an error

	s.Default()
	if log.Enabled() {
		val, err := json.MarshalIndent(s.MetricsNodeSampler, "", "  ")
		if err != nil {
			return err
		}
		// use println so it renders on multiple lines instead of 1
		fmt.Println(string(val))
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
	if s.checkCreatedPodFrequency < time.Second {
		return errors.New("checkCreatedPodFrequency must be at least 1 seconds")
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

	pbPort := fmt.Sprintf("%s:%v", s.Address, s.PBPort)
	lis, err := net.Listen("tcp", pbPort)
	if err != nil {
		return errors.WithStack(err)
	}
	if s.Address == "" {
		pbPort = "0.0.0.0" + pbPort
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
		pbPort,
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

	// Serve the API under /v1 but also expose pprof endpoints.
	mux := http.NewServeMux()
	mux.Handle("/v1/", gwmux)
	mux.Handle("/", http.DefaultServeMux)

	addr := fmt.Sprintf("%s:%v", s.Address, s.RestPort)
	gwServer := &http.Server{Addr: addr, Handler: mux}
	rpcServer := grpc.NewServer()
	api.RegisterMetricsServer(rpcServer, s)
	api.RegisterHealthServer(rpcServer, s)

	// run the REST service
	go func() {
		log.V(1).Info("serving json", "address", addr)
		defer rpcServer.GracefulStop() // stop the server when we are done
		go func() {
			err := errors.WithStack(gwServer.ListenAndServe())
			if err != nil {
				log.Error(err, "error starting server")
				errs <- err
			}
		}() // run the server
		<-ctx.Done() // wait until the context is cancelled
	}()
	log.V(1).Info("start registering collectors from DNS", "ctx-err", ctx.Err(),
		"ctx", ctx, "ctx-type", fmt.Sprintf("%T", ctx),
		"ctx-address", fmt.Sprintf("%p", ctx))
	go s.RegisterCollectorsFromDNS(ctx)

	// block until we are shutdown or there is an error in a service
	select {
	case err := <-errs:
		log.Error(err, "stopping node-sampler due to error")
		return err
	case <-ctx.Done():
		log.Info("stopping node-sampler due to context done", "err", ctx.Err())
		return nil
	}
}

// RegisterCollectorsFromDNS pushes utilization metrics to collector servers.
// The list of servers is configured through reading the DNS 'A' records.
// This is usually accomplished through a Kubernetes headless service.
func (s *Server) RegisterCollectorsFromDNS(ctx context.Context) {
	// periodically update the list of servers and start pushing metrics
	t := time.NewTicker(time.Duration(s.DNSSpec.PollSeconds) * time.Second)
	log.Info("starting register collectors from DNS")
	for {
		// find the list of servers to push metrics to
		ips, err := net.LookupIP(s.PushHeadlessService)
		if err != nil {
			if strings.Contains(err.Error(), "no such host") {
				log.Info("unable to lookup collector servers from dns records",
					"service", s.PushHeadlessService, "msg", err.Error())
			} else {
				log.Error(err, "unable to lookup collector servers from dns records", "service", s.PushHeadlessService)
			}
		}

		// register the collectors
		if len(ips) > 0 {
			req := &api.RegisterCollectorsRequest{
				Source:     "DNS",
				Collectors: make([]*api.Collector, 0, len(ips)),
			}
			for _, i := range ips {
				req.Collectors = append(req.Collectors, &api.Collector{IpAddress: i.String()})
			}
			log.V(1).Info("registering collectors from DNS", "ips", req)
			_, _ = s.RegisterCollectors(ctx, req)
		}

		select {
		case <-t.C:
			// keep going
		case <-ctx.Done():
			// we are done
			t.Stop()
			return
		}
	}
}

// foundUnsentPods returns true if we have discovered containers that we haven't
// sent metrics for their pods.
func (s *Server) foundUnsentPods(lastResp *api.ListMetricsResponse) bool {
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
	if k := s.cache.metricsReader.knownContainersSet.Load(); k != nil {
		return k.(sets.String).Difference(last).Len() > 0 // we know about pods we haven't sent
	} else {
		// we don't have metrics for any pods
		// this should never happen since we shouldn't have sent metrics if we don't have any
		return false
	}
}

type pusher struct {
	IP           string
	Context      context.Context
	Cancel       context.CancelFunc
	SawCollector time.Time
}

func (p *pusher) connect(s *Server) error {
	defer s.UnRegisterCollectors(p.Context, p.IP)
	return wait.PollImmediateInfiniteWithContext(p.Context, time.Second*30, func(ctx context.Context) (done bool, err error) {
		log.Info("starting metrics pushing", "ip", p.IP)
		// continously try to connect and push metrics
		done, err = p.connectAndPushMetrics(s)
		if done {
			return done, err
		}
		done = time.Since(p.SawCollector) > s.DNSSpec.CollectorServerExpiration
		return done, err
	})
}

func (p *pusher) connectAndPushMetrics(s *Server) (bool, error) {
	a := p.IP
	if strings.Contains(a, ":") {
		a = fmt.Sprintf("[%s]:%v", a, s.PushHeadlessServicePort)
	} else {
		a = fmt.Sprintf("%s:%v", a, s.PushHeadlessServicePort)
	}
	log := log.WithValues("address", a, "node", nodeName)

	// setup a connection to the server to push metrics
	log.Info("connecting to collector")
	conn, err := grpc.Dial(a, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Error(err, "failed collector connection")
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
		log.Error(err, "failed metrics push")
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
		if lastSent == nil {
			// we haven't sent metrics yet
			reason = "initial-sync"
		} else {
			select {
			case <-p.Context.Done():
				log.Info("sending final metrics to collector before shutdown", "err", p.Context.Err())
				resp, err := s.ListMetrics(p.Context, &api.ListMetricsRequest{})
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
				if !s.foundUnsentPods(lastSent) {
					// don't send the metrics if we don't have any new pods
					continue
				}
				reason = "found-new-pods"
			}
		}

		var resp *api.ListMetricsResponse
		for i := 0; i < sendTryCount; i++ {
			resp, err = s.ListMetrics(p.Context, &api.ListMetricsRequest{})
			if err == nil {
				break
			}
			log.Error(errors.WithStack(err), "unable to list metrics")
		}
		if len(resp.Containers) == 0 {
			// we haven't seen any containers, don't send metrics until we do
			continue
		}

		resp.Timestamp = timestamppb.Now()
		resp.Reason = reason
		if err := stream.Send(resp); err != nil {
			log.Error(errors.WithStack(err), "unable to send metrics to collector server")
			return false, err // may be an error with the connection -- re-establish the connection if possible
		}
		log.V(1).Info("sent metrics to collector",
			"containers-len", len(resp.Containers),
			"reason", reason,
			"push-frequency-seconds", s.pushFrequency.Seconds())
		lastSent = resp
	}
}

var (
	// nodeName gets the node name from the downward API
	nodeName = os.Getenv("NODE_NAME")
	// podName gets the node name from the downward API
	podName = os.Getenv("POD_NAME")
)

func (s *Server) UnRegisterCollectors(ctx context.Context, ip string) {
	s.pushersLock.Lock()
	defer s.pushersLock.Unlock()
	delete(s.pushers, ip)
}

// RegisterCollector provides an endpoint for collectors to manually register with the
// samplers.  This is useful in cases where DNS isn't adequate.
// - Collector needs to get utilization data before marking itself as ready and serving metrics
// - DNS is slow to propagate
func (s *Server) RegisterCollectors(ctx context.Context, req *api.RegisterCollectorsRequest) (*api.RegisterCollectorsResponse, error) {
	if req.Source != "DNS" {
		log.Info("registration request from collector", "req", req)
	}
	resp := &api.RegisterCollectorsResponse{}
	s.pushersLock.Lock()
	defer s.pushersLock.Unlock()
	for _, c := range req.Collectors {
		if len(c.IpAddress) == 0 {
			log.Info("got empty IP for registration")
			continue
		}

		if p, ok := s.pushers[c.IpAddress]; ok {
			p.SawCollector = time.Now()
			// already running, do nothing
			continue
		}
		// IMPORTANT: use the server context not the request context or the
		// pusher will shutdown when the request ends
		p := &pusher{IP: c.IpAddress, Context: s.CTX, SawCollector: time.Now()}
		log.Info("starting metrics pusher for new server", "req", req)

		s.pushers[c.IpAddress] = p
		go p.connect(s) // run the metrics pusher
		resp.IpAddresses = append(resp.IpAddresses, c.IpAddress)
	}

	return resp, nil
}

// ListMetrics lists the aggregated metrics for all containers and nodes
func (s *Server) ListMetrics(context.Context, *api.ListMetricsRequest) (*api.ListMetricsResponse, error) {
	var result api.ListMetricsResponse
	samples, _ := s.cache.getAllSamples()
	result.Reason = "pull"

	for k, v := range samples.containers {
		c := &api.ContainerMetrics{
			ContainerID:   k.ContainerID,
			PodUID:        k.PodUID,
			ContainerName: k.ContainerName,
			PodName:       k.PodName,
			NamespaceName: k.NamespaceName,

			CpuCoresNanoSec:            make([]int64, 0, len(v.values)),
			CpuThrottledNanoSec:        make([]int64, 0, len(v.values)),
			CpuPercentPeriodsThrottled: make([]float32, 0, len(v.values)),
			MemoryBytes:                make([]int64, 0, len(v.values)),
			CpuPeriodsSec:              make([]int64, 0, len(v.values)),
			CpuThrottledPeriodsSec:     make([]int64, 0, len(v.values)),

			AvgCPUCoresNanoSec:            int64(v.avg.CPUCoresNanoSec),
			AvgCPUThrottledNanoSec:        int64(v.avg.CPUThrottledUSec),
			AvgCPUPercentPeriodsThrottled: float32(v.avg.CPUPercentPeriodsThrottled),
			AvgMemoryBytes:                int64(v.avg.MemoryBytes),
			OomCount:                      int64(v.avg.MemoryOOM),
			OomKillCount:                  int64(v.avg.MemoryOOMKill),
			AvgCPUPeriodsSec:              int64(v.avg.CPUPeriodsSec),
			AvgCPUThrottledPeriodsSec:     int64(v.avg.CPUThrottledPeriodsSec),
		}
		for i := range v.values {
			if i == 0 {
				// skip the first value to be consistent with how mean is calculated
				continue
			}
			s := v.values[i]
			c.CpuCoresNanoSec = append(c.CpuCoresNanoSec, int64(s.CPUCoresNanoSec))
			c.CpuThrottledNanoSec = append(c.CpuThrottledNanoSec, int64(s.CPUThrottledUSec))
			c.CpuPercentPeriodsThrottled = append(c.CpuPercentPeriodsThrottled, float32(s.CPUPercentPeriodsThrottled))
			c.MemoryBytes = append(c.MemoryBytes, int64(s.MemoryBytes))
			c.CpuPeriodsSec = append(c.CpuPeriodsSec, int64(s.CPUPeriodsSec))
			c.CpuThrottledPeriodsSec = append(c.CpuThrottledPeriodsSec, int64(s.CPUThrottledPeriodsSec))
		}
		if s.SortResults {
			// sort the values so the results are stable
			sort.Sort(Int64Slice(c.CpuCoresNanoSec))
			sort.Sort(Int64Slice(c.CpuThrottledNanoSec))
			sort.Sort(Int64Slice(c.MemoryBytes))
			sort.Sort(Float32Slice(c.CpuPercentPeriodsThrottled))
			sort.Sort(Int64Slice(c.CpuPeriodsSec))
			sort.Sort(Int64Slice(c.CpuThrottledPeriodsSec))
		}
		result.Containers = append(result.Containers, c)
	}

	var node api.NodeMetrics

	for level, values := range samples.node {
		cpuCoresNanoSec := make([]int64, 0, len(values.values)-1)
		memoryBytes := make([]int64, 0, len(values.values)-1)
		for i := range values.values {
			if i == 0 {
				// skip the first value to be consistent with how mean is calculated
				continue
			}
			value := values.values[i]
			cpuCoresNanoSec = append(cpuCoresNanoSec, int64(value.CPUCoresNanoSec))
			memoryBytes = append(memoryBytes, int64(value.MemoryBytes))
		}

		if s.SortResults {
			sort.Sort(Int64Slice(cpuCoresNanoSec))
			sort.Sort(Int64Slice(memoryBytes))
		}

		nodeAggregatedMetrics := api.NodeAggregatedMetrics{
			AggregationLevel:   string(level),
			CpuCoresNanoSec:    cpuCoresNanoSec,
			MemoryBytes:        memoryBytes,
			AvgCPUCoresNanoSec: int64(values.avg.CPUCoresNanoSec),
			AvgMemoryBytes:     int64(values.avg.MemoryBytes),
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
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}
