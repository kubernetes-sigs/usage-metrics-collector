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

package collector

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
)

var (
	_               cache.NewCacheFunc = GetNewCacheFunc(collectorcontrollerv1alpha1.MetricsPrometheusCollector{})
	KeepAnnotations                    = []string{
		"workload-name", "workload-kind", "workload-api-group", "workload-api-version",
		"kubernetes.io/hostname", corev1.MirrorPodAnnotationKey,
	}
)

// GetNewCacheFunc returns a function which sets informer cache options to reduce memory consumption.
// * Delete labels and annotations that aren't used by the collector
// * Don't deep copy objects when reading from the cache
func GetNewCacheFunc(c collectorcontrollerv1alpha1.MetricsPrometheusCollector) cache.NewCacheFunc {
	options := cache.Options{}
	if c.CacheOptions.UnsafeDisableDeepCopy {
		// This is only safe if we don't modify the objects read from the cache
		// We should never modify objects read from the cache in the collector.
		// This reduces memory consumption by using a shallow copy of cached objects.
		options.UnsafeDisableDeepCopyByObject = map[client.Object]bool{cache.ObjectAll{}: c.CacheOptions.UnsafeDisableDeepCopy}
	}

	if len(c.CacheOptions.DropAnnotations) > 0 {
		// Because storing objects in the cache, remove potentially large data that we don't use
		// Examples: the last-applied annotation, ann
		options.DefaultTransform = func(i interface{}) (interface{}, error) {
			m, ok := i.(metav1.Object)
			if !ok {
				return i, nil
			}

			// Annotations can be potentially very large, and we don't need to store them
			// if we aren't using them in the collector
			an := m.GetAnnotations()
			for _, a := range c.CacheOptions.DropAnnotations {
				delete(an, a)
			}
			m.SetAnnotations(an)
			return i, nil
		}
	}
	return cache.BuilderWithOptions(options)
}
