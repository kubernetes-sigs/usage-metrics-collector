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
	"context"
	"strings"

	"gopkg.in/inf.v0"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// MemoryRound defines infinite decimal precision for memory
	MemoryRound = 0
	// CPUUSecRound defines infinite decimal precision for cpu usage seconds
	CPUUSecRound = 6
	// CPUSecRound defines infinite decimal precision for cpu seconds
	CPUSecRound = 1
	// AveragePrecision defines infinite decimal precision for averages
	AveragePrecision = 6
)

// getWorkloadForPod returns the workload the pod belongs to by walking owners references
// Returns the name, workload kind, workload api group, workload api version
func getWorkloadForPod(c client.Client, pod *corev1.Pod) workload {
	if name, ok := pod.Annotations["workload-name"]; ok {
		// override the OwnerReference resolution if these annotations are set
		// on the pod
		return workload{
			Name:       name,
			Kind:       pod.Annotations["workload-kind"],
			APIGroup:   pod.Annotations["workload-api-group"],
			APIVersion: pod.Annotations["workload-api-version"],
		}
	}
	for _, o := range pod.OwnerReferences {
		if o.Controller == nil || !*o.Controller {
			continue
		}

		// if the pod has a controller OwnerReference, use that to find the workload
		switch o.Kind {
		case "ReplicaSet":
			// special case resolving replicasets to deployments -- consider doing this for other workload types
			return getWorkloadFor(c, o, pod.ObjectMeta, &appsv1.ReplicaSet{})
		case "Job":
			// special case resolving replicasets to deployments -- consider doing this for other workload types
			return getWorkloadFor(c, o, pod.ObjectMeta, &batchv1.Job{})
		default:
			// all non-replicaset cases -- use the owner directly
			group, version := getAPIGroupVersion(o.APIVersion)
			return workload{
				Name:       strings.ToLower(o.Name),
				Kind:       strings.ToLower(o.Kind),
				APIGroup:   group,
				APIVersion: version,
			}
		}
	}
	return workload{
		Name:       pod.Name,
		Kind:       "pod",
		APIGroup:   "core",
		APIVersion: "v1",
	}
}

// GetAPIGroupVersion formats the k8s apiVersion to be compatible with Prometheus characters
// and returns k8s API group and version
func getAPIGroupVersion(apiVersion string) (string, string) {
	api := strings.Split(apiVersion, "/")
	if len(api) < 2 {
		return "core", api[0]
	}
	group := api[0]
	version := api[1]

	// matches any / or . character
	formattedGroup := strings.ToLower(strings.ReplaceAll(group, ".", "_"))
	formattedVersion := strings.ToLower(strings.ReplaceAll(version, ".", "_"))
	return formattedGroup, formattedVersion
}

// getWorkloadFor finds the workload for an OwnerReference pointing to a workload
// o is the OwnerReference pointing at an owner
// obj is the object owned by a workload (e.g. Pod)
func getWorkloadFor(c client.Client, o metav1.OwnerReference, obj metav1.ObjectMeta, rs client.Object) workload {
	err := c.Get(context.Background(),
		types.NamespacedName{Name: o.Name, Namespace: obj.Namespace}, rs)
	if name, ok := rs.GetAnnotations()["workload-name"]; ok {
		// override the OwnerReference resolution if these annotations are set
		// on the pod
		return workload{
			Name:       name,
			Kind:       rs.GetAnnotations()["workload-kind"],
			APIGroup:   rs.GetAnnotations()["workload-api-group"],
			APIVersion: rs.GetAnnotations()["workload-api-version"],
		}
	}

	group, version := getAPIGroupVersion(o.APIVersion)
	if err != nil {
		return workload{
			Name:       strings.ToLower(o.Name),
			Kind:       strings.ToLower(o.Kind),
			APIGroup:   group,
			APIVersion: version,
		}
	}

	for _, o := range rs.GetOwnerReferences() {
		if o.Controller == nil || !*o.Controller {
			continue
		}
		return workload{
			Name:       strings.ToLower(o.Name),
			Kind:       strings.ToLower(o.Kind),
			APIGroup:   group,
			APIVersion: version,
		}
	}
	return workload{
		Name:       strings.ToLower(o.Name),
		Kind:       strings.ToLower(o.Kind),
		APIGroup:   group,
		APIVersion: version,
	}
}

// divide performs an infinite decimal division
func divide(x, y *inf.Dec, precision int) *inf.Dec {
	return new(inf.Dec).QuoRound(
		x,
		y,
		inf.Scale(precision),
		inf.RoundHalfUp,
	)
}

// divideQtyQty performs an infinite decimal division between two resource.Quantity
// nolint: deadcode,unused
func divideQtyQty(x, y resource.Quantity, precision int) resource.Quantity {
	res := divide(x.AsDec(), y.AsDec(), precision)
	return *resource.NewDecimalQuantity(*res, resource.DecimalSI)
}

// divideQtyInt64 performs an infinite decimal division between resource.Quantity and Int64
func divideQtyInt64(x resource.Quantity, y int64, precision int) resource.Quantity {
	res := divide(x.AsDec(), inf.NewDec(y, 0), precision)
	return *resource.NewDecimalQuantity(*res, resource.DecimalSI)
}

// divideInt64Qty performs an infinite decimal division between Int64 and resource.Quantity
// nolint: deadcode,unused
func divideInt64Qty(x int64, y resource.Quantity, precision int) resource.Quantity {
	res := divide(inf.NewDec(x, 0), y.AsDec(), precision)
	return *resource.NewDecimalQuantity(*res, resource.DecimalSI)
}

// multipleQtyInt64 performs infinite decimal multiplication between resource.Quantity and Int64
// nolint: deadcode,unused
func multipleQtyInt64(x resource.Quantity, y int64) resource.Quantity {
	res := new(inf.Dec).Mul(x.AsDec(), inf.NewDec(y, 0))
	return *resource.NewDecimalQuantity(*res, resource.DecimalSI)
}
