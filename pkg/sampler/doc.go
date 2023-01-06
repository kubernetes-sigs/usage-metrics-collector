// Package sampler implements metrics-node-sampler which samples local utilization
// metrics from the node it is running on and serves aggregated values from a
// grpc API.
//
// Samples are stored in memory over a sliding window.  Responses contain
// values aggregated from the samples currently in memory.
package sampler
