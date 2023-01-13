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

package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var (
	options = Options{}
	rootCmd = &cobra.Command{
		Use:   "test-app",
		Run:   options.Run,
		Short: `test-app simulates an application using cpu and memory.`,
	}
)

// RunE this application and return error if any
func (o *Options) Run(_ *cobra.Command, _ []string) {

	for i := 0; i < o.MemoryByteBufferCount; i++ {
		go func() {
			for {
				// allocate memory in a continuos loop
				memory := make([]byte, o.MemoryByteBufferSize)
				time.Sleep(time.Duration(o.MemorySleepSec) * time.Second)
				fmt.Println(len(memory))
				// free the memory
				memory = nil
			}
		}()
	}

	// spin up go routines that spin endlessly
	sleep := time.NewTicker(time.Duration(o.CPUSleepFrequencySec) * time.Second)
	for i := 0; i < o.CPUGoRoutines; i++ {
		go func() {
			for {
				select {
				case <-sleep.C:
					time.Sleep(time.Duration(o.CPUSleepSec) * time.Second)
				default:
				}
			}
		}()
	}

	// block forever
	for {
		time.Sleep(time.Hour)
	}
}

// Options is set by flags
type Options struct {
	MemorySleepSec        float32 `json:"memorySleepSec"`
	MemoryByteBufferSize  int     `json:"memoryByteBufferSize"`
	MemoryByteBufferCount int     `json:"memoryByteBufferCount"`

	CPUSleepSec          float32 `json:"cpuSleepSec"`
	CPUSleepFrequencySec float32 `json:"cpuSleepFrequencySec"`
	CPUGoRoutines        int     `json:"cpuGoRoutines"`
}

func main() {
	// Add the go `flag` package flags -- e.g. `--kubeconfig`
	rootCmd.Flags().AddGoFlagSet(flag.CommandLine)
	rootCmd.Flags().IntVar(&options.CPUGoRoutines, "cpu-go-routines", 1, "number of concurrent go-routines")
	rootCmd.Flags().Float32Var(&options.CPUSleepFrequencySec, "cpu-sleep-sec-frequency", 1.0, "how frequently to send a sleep signal to one of the go routines")
	rootCmd.Flags().Float32Var(&options.CPUSleepSec, "cpu-sleep-sec", 0.1, "how long to sleep for")

	rootCmd.Flags().IntVar(&options.MemoryByteBufferCount, "memory-go-routines", 1, "number of byte buffers to allocated")
	rootCmd.Flags().IntVar(&options.MemoryByteBufferSize, "memory-byte-buffer-size", 1000, "size of each byte buffer")
	rootCmd.Flags().Float32Var(&options.MemorySleepSec, "memory-sleep-sec", 1.0, "how long to sleep for between memory allocations")

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
