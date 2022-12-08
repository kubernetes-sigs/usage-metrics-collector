//go:build !linux

package sampler

import "runtime"

func (s *sampleCache) getContainerCPUAndMemoryCM() (cpuMetrics, memoryMetrics, error) {
	// Not implemented on Mac
	log.Info("getContainerCPUAndMemoryCM not implemented for platform", "goos", runtime.GOOS, "goarch", runtime.GOARCH)
	return nil, nil, nil
}
