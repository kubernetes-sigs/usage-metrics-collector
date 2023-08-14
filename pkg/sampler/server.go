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

	"github.com/google/cadvisor/client"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"

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

	collectorConnections map[string]collectorConnection
	collectorIPsLock     sync.Mutex

	CTX context.Context
}

// collectorConnection stores state for sending metrics to a collector
type collectorConnection struct {
	stream api.MetricsCollector_PushMetricsClient
	addr   string
	stop   func()
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

	if s.DNSSpec.PollSeconds == 0 {
		s.DNSSpec.PollSeconds = 60
	}

	defer stop() // stop the context if we encounter an error

	s.CTX = ctx
	s.stop = stop
	s.collectorConnections = make(map[string]collectorConnection, 2)
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

	if err := s.startCache(ctx, errs); err != nil {
		return err
	}
	go s.startRegisterCollectorsFromDNS()
	s.startPushToCollectors()

	go s.startGRPC(ctx, errs)

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

// startCache starts the cache for caching utilization samples
func (s *Server) startCache(ctx context.Context, errs chan<- error) error {
	var err error

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

	if s.UseCadvisorMonitor {
		log.Info("initializing cadvisor monitor")
		s.cache.UseCadvisorMonitor = s.UseCadvisorMonitor
		s.cache.CadvisorClient, err = client.NewClient(s.CadvisorEndpoint)
		if err != nil {
			log.Error(err, "unable to create cadvisor client")
			return err
		}
	}

	go func() {
		errs <- errors.WithStack(s.cache.Start(ctx))
	}()
	return nil
}

// startGRPC starts the GRPC server for receiving registration and list requests
func (s *Server) startGRPC(ctx context.Context, errs chan<- error) error {
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

	// block until we are shutdown
	<-ctx.Done()
	log.Info("stopping node-sampler due to context done", "err", ctx.Err())
	return nil
}

// listCollectorIPsFromDNS return the ip addresses for collectors using DNS
func (s *Server) listCollectorIPsFromDNS() []net.IP {
	ips, err := net.LookupIP(s.PushHeadlessService)
	if err != nil {
		if strings.Contains(err.Error(), "no such host") {
			log.Info("unable to lookup collector servers from dns records",
				"service", s.PushHeadlessService, "msg", err.Error())
		} else {
			log.Error(err, "unable to lookup collector servers from dns records", "service", s.PushHeadlessService)
		}
	}
	return ips
}

// ipToAddress handles issues with ipv6 strings and adds the service port
func (s *Server) ipToAddress(ip net.IP) string {
	a := ip.String()
	if strings.Contains(a, ":") {
		// handle ipv6
		a = fmt.Sprintf("[%s]:%v", a, s.PushHeadlessServicePort)
	} else {
		a = fmt.Sprintf("%s:%v", a, s.PushHeadlessServicePort)
	}
	return a
}

// addConnectionsIfMissing creates new streams for any ips not already present
func (s *Server) addConnectionsFromIPs(ips []net.IP) {
	var addrs []string
	for _, i := range ips {
		if len(i) == 0 {
			continue
		}
		addrs = append(addrs, s.ipToAddress(i))
	}
	s.addConnectionsIfMissing(addrs)
}

// addConnectionsFromCollectors registers collectors from a request sent by a collector
func (s *Server) addConnectionsFromCollectors(req *api.RegisterCollectorsRequest) {
	var addrs []string
	for _, i := range req.Collectors {
		if len(i.IpAddress) == 0 {
			continue
		}
		addrs = append(addrs, i.IpAddress)
	}
	s.addConnectionsIfMissing(addrs)
}

// addConnectionsIfMissing adds collector connections for the addresses if
// they are not already established
func (s *Server) addConnectionsIfMissing(addrs []string) {
	s.collectorIPsLock.Lock()
	defer s.collectorIPsLock.Unlock()

	// register any unregistered ips
	for _, addr := range addrs {
		if len(addr) == 0 {
			// edge case
			continue
		}
		if _, ok := s.collectorConnections[addr]; ok {
			// already registered
			continue
		}

		log := log.WithValues("address", addr, "node", nodeName)
		log.Info("initializing new collector connection")

		// setup a connection to the server to push metrics
		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Error(err, "failed to dial collector")
			continue
		}

		// setup a client to the server to push metrics
		client := api.NewMetricsCollectorClient(conn)

		// IMPORTANT: use a different context for each client so we can send the last metrics before
		// shutting down.  If we use the server context, the connection to the collector
		// will be shutdown before we send the final set of metrics.
		sendCtx, stop := context.WithCancel(context.Background())
		stream, err := client.PushMetrics(sendCtx)
		if err != nil {
			stop()
			log.Error(err, "failed to create collector stream")
			continue
		}
		s.collectorConnections[addr] = collectorConnection{stream: stream, stop: stop, addr: addr}
		go func(addr string) {
			// delete the connection if the collector shuts down the connection
			start := time.Now()
			<-sendCtx.Done()
			log.Info("context for collector stream canceled by collector",
				"reason", sendCtx.Err(),
				"second", time.Since(start).Seconds())
			s.removeConnections(addr)
		}(addr)
	}
}

// removeConnections stops existing connections
func (s *Server) removeConnections(ips ...string) {
	s.collectorIPsLock.Lock()
	defer s.collectorIPsLock.Unlock()
	for _, i := range ips {
		s.collectorConnections[i].stop() // cancel the connection
		delete(s.collectorConnections, i)
	}
}

// shutdownAllConnections will shutdown all connections to collectors
func (s *Server) shutdownAllConnections() {
	s.collectorIPsLock.Lock()
	defer s.collectorIPsLock.Unlock()
	for _, i := range s.collectorConnections {
		i.stop() // cancel the connection
	}
	s.collectorConnections = make(map[string]collectorConnection, 2)
}

func (s *Server) getConnections() ([]collectorConnection, []string) {
	collectors := make([]collectorConnection, 0, 2)
	addrs := make([]string, 0, 2)
	func() {
		s.collectorIPsLock.Lock()
		defer s.collectorIPsLock.Unlock()
		for _, c := range s.collectorConnections {
			collectors = append(collectors, c)
			addrs = append(addrs, c.addr)
		}
	}()
	return collectors, addrs
}

// startRegisterCollectorsFromDNS pushes utilization metrics to collector servers.
// The list of servers is configured through reading the DNS 'A' records.
// This is usually accomplished through a Kubernetes headless service.
func (s *Server) startRegisterCollectorsFromDNS() {
	t := time.NewTicker(time.Duration(s.DNSSpec.PollSeconds) * time.Second)
	log.Info("starting register collectors from DNS")
	for {
		s.addConnectionsFromIPs(s.listCollectorIPsFromDNS())
		select {
		case <-t.C:
			// keep going
		case <-s.CTX.Done():
			// we are done
			t.Stop()
			return
		}
	}
}

const backoffSeconds = 1

// sendMetrics sends the resp to each of the collectors and returns the slice of connections it
// failed to send a response to
// sendMetrics will retry sending up to s.SendPushMetricsRetryCount times
func (s *Server) sendMetrics(resp *api.ListMetricsResponse, collectors []collectorConnection) []string {
	var err error
	var badConnections []string

	// Send metrics to each collector
	for _, c := range collectors {
		start := time.Now()

		// clone the response because we send a different one to each collector
		retries := 0
		for ; retries <= s.SendPushMetricsRetryCount; retries++ {
			// clone the response in case the proto has any state
			r := proto.Clone(resp).(*api.ListMetricsResponse)

			// send the message
			if err = c.stream.Send(r); err == nil {
				// success
				log.Info("sent metrics to collector",
					"retries", retries,
					"seconds", time.Since(start),
					"addr", c.addr)
				break
			}
			time.Sleep(backoffSeconds)
		}
		if err != nil {
			log.Error(errors.WithStack(err), "failed retry metrics send", "addr", c.addr, "retries", retries)
			badConnections = append(badConnections, c.addr) // clean these up
			continue
		}
		log.Info("sent metrics to collector", "seconds", time.Since(start), "addr", c.addr, "containers-length", len(resp.Containers))
	}
	return badConnections
}

// pushMetricsToAllCollectors pushes the metrics to all of the collectors with
// connections.
// pushMetricsToAllCollectors returns the response if it was sent to any collectors,
// or nil if the response was not ready to send yet.
func (s *Server) pushMetricsToAllCollectors(reason string) *api.ListMetricsResponse {
	start := time.Now()
	// Make a shallow copy of the collector connections instead of locking it the whole time.
	collectors, addrs := s.getConnections()

	log := log.WithValues("reason", reason, "addresses", addrs)

	// Get the latest copy of the metrics to send to the collectors.
	// Ignore the error, ListMetrics never returns one
	resp, _ := s.ListMetrics(context.Background(), &api.ListMetricsRequest{})
	resp.Timestamp = timestamppb.Now()
	resp.Reason = reason
	if len(resp.Containers) == 0 {
		// no data yet, send nothing
		log.Info("skipping send until we have container data")
		return nil
	}

	// Send the metrics to each collector
	badConnections := s.sendMetrics(resp, collectors)

	// Failed connections should be re-established by DNS or the collector
	// self-registering if the collector is healthy.
	log.Info("removing collector connections due to failed send",
		"addresses", badConnections,
		"maxRetries", s.SendPushMetricsRetryCount)
	s.removeConnections(badConnections...)

	log.Info("finished push metrics to collectors",
		"reason", reason,
		"seconds", time.Since(start).Seconds(),
		"failed-push", badConnections,
		"containers-length", len(resp.Containers),
		"push-frequency-seconds", s.pushFrequency.Seconds())
	return resp
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

// startPushToCollectors continuously pushes metrics from the cache to the already registered collectors
func (s *Server) startPushToCollectors() {
	go func() {
		log.Info("starting push metrics loop")

		ticker := time.NewTicker(s.pushFrequency)
		createdPodTicker := time.NewTicker(s.checkCreatedPodFrequency)
		defer func() {
			ticker.Stop()
			createdPodTicker.Stop()
			s.shutdownAllConnections()
		}() // send EOF to all collectors

		var lastSent = s.pushMetricsToAllCollectors("initial-sync")

		// continuously push results until shutdown
		for {
			select {
			case <-s.CTX.Done(): //shutdown and send metrics
				log.Info("sending final metrics to collector before shutdown", "err", s.CTX.Err())
				_ = s.pushMetricsToAllCollectors("sampler-shutdown")
				return
			case <-ticker.C:
				lastSent = s.pushMetricsToAllCollectors("periodic-sync")
			case <-createdPodTicker.C:
				// check if there are pods we haven't sent metrics for in the last request.
				if s.foundUnsentPods(lastSent) {
					lastSent = s.pushMetricsToAllCollectors("found-new-pods")
				}
			}
		}
	}()
}

var (
	// nodeName gets the node name from the downward API
	nodeName = os.Getenv("NODE_NAME")
	// podName gets the node name from the downward API
	podName = os.Getenv("POD_NAME")
)

// RegisterCollector provides an endpoint for collectors to manually register with the
// samplers.  This is useful in cases where DNS isn't adequate.
// - Collector needs to get utilization data before marking itself as ready and serving metrics
// - DNS is slow to propagate
func (s *Server) RegisterCollectors(ctx context.Context, req *api.RegisterCollectorsRequest) (*api.RegisterCollectorsResponse, error) {
	s.addConnectionsFromCollectors(req)
	return &api.RegisterCollectorsResponse{}, nil
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
		//Network summaries, if there is any.
		if v.avg.CAdvisorNetworkStats != nil {
			c.AvgNetworkRxBytes = int64(v.avg.CAdvisorNetworkStats.RxBytes)
			c.AvgNetworkRxPackets = int64(v.avg.CAdvisorNetworkStats.RxPackets)
			c.AvgNetworkRxErrors = int64(v.avg.CAdvisorNetworkStats.RxErrors)
			c.AvgNetworkRxDropped = int64(v.avg.CAdvisorNetworkStats.RxDropped)
			c.AvgNetworkTxBytes = int64(v.avg.CAdvisorNetworkStats.TxBytes)
			c.AvgNetworkTxPackets = int64(v.avg.CAdvisorNetworkStats.TxPackets)
			c.AvgNetworkTxErrors = int64(v.avg.CAdvisorNetworkStats.TxErrors)
			c.AvgNetworkTxDropped = int64(v.avg.CAdvisorNetworkStats.TxDropped)
		}

		for i := range v.values {
			if i == 0 && pointer.BoolDeref(s.Reader.DropFirstValue, false) {
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
			if i == 0 && pointer.BoolDeref(s.Reader.DropFirstValue, false) {
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
