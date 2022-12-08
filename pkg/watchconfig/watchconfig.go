// Package watchconfig watches for config file changes
package watchconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	commonlog "sigs.k8s.io/usage-metrics-collector/pkg/log"

	"github.com/fsnotify/fsnotify"
)

var (
	log = commonlog.Log.WithName("watch-config")
)

type ConfigFile struct {
	lastRead       []byte
	modTime        time.Time
	ConfigFilename string
}

// WatchConfig will watch the config file and call onChange if the file contents change
func (cf ConfigFile) WatchConfig(onChange func(*fsnotify.Watcher, chan interface{})) error {
	watcher, err := fsnotify.NewWatcher()
	stop := make(chan interface{})
	if err != nil {
		return err
	}

	// read the initial config
	if err := cf.read(); err != nil {
		return err
	}

	// watch the directory instead of the file itself
	// because we won't get events for the file when the
	// configmap is updated as it is a symlink.
	if err := watcher.Add(filepath.Dir(cf.ConfigFilename)); err != nil {
		return err
	}
	go func() {
		for {
			select {
			case _, ok := <-watcher.Events:
				if !ok {
					log.Info("error reading config event", "filename", cf.ConfigFilename)
					continue
				}
				changed, err := cf.changed()
				if err != nil {
					log.Error(err, "error watching config file", "filename", cf.ConfigFilename)
					continue
				}
				if changed {
					// configFile has changed
					onChange(watcher, stop)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					log.Info("error reading config event", "filename", cf.ConfigFilename)
					continue
				}
				log.Error(err, "error watching config file", "filename", cf.ConfigFilename)
			case <-stop:
				log.V(1).Info("stopping watch config file", "filename", cf.ConfigFilename)
				return
			}
		}
	}()

	return nil
}

// read caches the config file mod time and contents to check for changes in the future
func (cf *ConfigFile) read() error {
	f, err := os.Stat(cf.ConfigFilename)
	if err != nil {
		log.Error(err, "error read config file", "filename", cf.ConfigFilename)
		return err
	}
	cf.modTime = f.ModTime()

	b, err := os.ReadFile(cf.ConfigFilename)
	if err != nil {
		log.Error(err, "error stat config file", "filename", cf.ConfigFilename)
		return err
	}
	cf.lastRead = b
	return nil
}

// changed returns true if the file has changed since the last time read or changed was called
func (cf *ConfigFile) changed() (bool, error) {
	f, err := os.Stat(cf.ConfigFilename)
	if err != nil {
		log.Error(err, "error state config file", "filename", cf.ConfigFilename)
		return false, err
	}

	// file hasn't changed since we last read it
	if !f.ModTime().After(cf.modTime) {
		return false, nil
	}

	b, err := os.ReadFile(cf.ConfigFilename)
	if err != nil {
		log.Error(err, "error stat config file", "filename", cf.ConfigFilename)
		return false, err
	}

	// check if the config file has changed
	if bytes.Equal(cf.lastRead, b) {
		return false, nil
	}

	log.Info("config file changed", "filename", cf.ConfigFilename)

	cf.lastRead = b
	return true, nil
}
