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

// Package log contains the shared logger used by the project
package log

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/fsnotify/fsnotify"
	z "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	// Level is the logging level
	Level = z.NewAtomicLevelAt(zapcore.InfoLevel)

	Log = zap.New(zap.WriteTo(os.Stderr), zap.Level(Level)).WithName("usage-metrics-collector")
)

// WatchLevel will update the Log level when the filename changes.
// filename is the path to a file containing an integer log level value.
func WatchLevel(filename string) (*fsnotify.Watcher, chan interface{}, error) {
	watcher, err := fsnotify.NewWatcher()
	stop := make(chan interface{})
	if err != nil {
		return nil, nil, err
	}
	var l int
	if l, err = LoadLevel(filename, 0); err != nil {
		return nil, nil, err
	}

	// watch the directory instead of the file itself
	// because we won't get events for the file when the
	// configmap is updated as it is a symlink.
	if err := watcher.Add(filepath.Dir(filename)); err != nil {
		return nil, nil, err
	}
	go func() {
		for {
			select {
			case _, ok := <-watcher.Events:
				if !ok {
					Log.Info("error reading log level event")
					continue
				}
				l, _ = LoadLevel(filename, l)
			case err, ok := <-watcher.Errors:
				if !ok {
					Log.Info("error reading log level error")
					continue
				}
				Log.Error(err, "error watching level file")
			case <-stop:
				Log.V(1).Info("stopping watch log level")
				return
			}
		}
	}()

	return watcher, stop, nil
}

func LoadLevel(filename string, last int) (int, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		Log.Error(err, "error read level file")
		return 0, err
	}

	level, err := strconv.Atoi(string(b))
	if err != nil {
		Log.Error(err, "error parse level file", "level", string(b))
		return 0, err
	}

	// invert the value.  zap uses -2 to enable log level 2, but
	// expect the file to have '2' to enable log level '2'
	level = level * -1

	if last == level {
		// skip update if it doesn't change the value
		// this is mostly to skip logging the level update
		return level, nil
	}

	// set the new level
	Level.SetLevel(zapcore.Level(level))
	Log.V(1).Info("set log level", "level", level)

	return level, nil
}
