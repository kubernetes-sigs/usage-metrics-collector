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
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	commonlog "sigs.k8s.io/usage-metrics-collector/pkg/log"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler"
	"sigs.k8s.io/usage-metrics-collector/pkg/watchconfig"
)

var (
	logPath            string
	terminationSeconds int

	log = commonlog.Log.WithName("metrics-node-sampler")
)

type Server struct {
	sampler.Server
	configFilepath string
}

func (s *Server) RunE(cmd *cobra.Command, args []string) error {
	if logPath != "" {
		// dynamically set log level
		watcher, stop, err := commonlog.WatchLevel(logPath)
		if err != nil {
			return err
		}
		defer watcher.Close()
		defer close(stop)
	}

	// parse the config from a file
	b, err := os.ReadFile(s.configFilepath)
	if err != nil {
		log.Error(err, "unable to read sampler-config-filepath")
		return err
	}
	err = yaml.UnmarshalStrict(b, &s.Server)
	if err != nil {
		log.Error(err, "unable to unmarshal sampler-config-filepath")
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	go func() {
		// force process to exit so we don't get stuck in a terminating state
		// its much better to get killed and recreated than to hang
		<-ctx.Done()
		log.Info("shutting down metrics-node-sampler", "err", ctx.Err())
		time.Sleep(time.Duration(terminationSeconds) * time.Second)
		log.Info("terminating metrics-node-sampler")
		os.Exit(0)
	}()

	if s.Server.ExitOnConfigChange {
		err = watchconfig.ConfigFile{
			ConfigFilename: s.configFilepath,
		}.WatchConfig(func(w *fsnotify.Watcher, s chan interface{}) {
			log.Info("stopping metrics-node-sampler to read new config file")
			stop()
			w.Close()
			close(s)
		})
		if err != nil {
			log.Error(err, "unable to watch config")
		}
	}

	return s.Start(ctx, stop)
}

var (
	S       = &Server{}
	RootCmd = &cobra.Command{
		Use:  "metrics-node-sampler",
		RunE: S.RunE,
	}
)

func init() {
	RootCmd.Flags().StringVar(&S.configFilepath, "sampler-config-filepath", "", "filepath to the metrics-node-sampler config")
	_ = RootCmd.MarkFlagRequired("sampler-config-filepath")
	RootCmd.Flags().StringVar(&logPath, "log-level-filepath", "", "path to log level file.  The file must contain a single integer corresponding to the log level (e.g. 2")
	RootCmd.Flags().IntVar(&terminationSeconds, "termination-seconds", 10, "time to wait for shutdown before os.Exit is called")

	RootCmd.Flags().AddGoFlagSet(flag.CommandLine)

	initContainerMonitor(RootCmd)
}

func Execute(version, commit, date string) {
	log.Info("Build info", "version", version, "commit", commit, "date", date)

	if err := RootCmd.Execute(); err != nil {
		log.Error(err, "failed to run metrics-node-sampler")
		os.Exit(1)
	}
}
