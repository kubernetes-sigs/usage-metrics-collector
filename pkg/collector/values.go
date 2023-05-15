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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/quotamanagementv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
	"sigs.k8s.io/yaml"
)

// value contains a metric value. Everything in a value should have the exact
// same set of labels.
type value struct {
	ResourceList      corev1.ResourceList                          `json:"resourceList,omitempty" yaml:"resourceList,omitempty"`
	MultiResourceList map[corev1.ResourceName][]resource.Quantity  `json:"multiResourceList,omitempty" yaml:"multiResourceList,omitempty"`
	Level             collectorcontrollerv1alpha1.AggregationLevel `json:"level" yaml:"level"`
	Source            collectorcontrollerv1alpha1.Source           `json:"source" yaml:"source"`
}

// ValueReader reads the requests value as a ResourceList
type ValueReader struct{}

// GetValuesForContainer returns the ResourceLists from a container for:
// - requests_allocated
// - limits_allocated
// - utilization
// - requests_allocated_minus_utilization
// - nr_periods
// - nr_throttled
// - oom_kill
func (r ValueReader) GetValuesForContainer(
	container *corev1.Container, pod *corev1.Pod, usage *api.ContainerMetrics) map[collectorcontrollerv1alpha1.Source]value {
	values := map[collectorcontrollerv1alpha1.Source]value{}

	// get requests and limits values
	if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
		// don't report requests/limits for completed containers
		requests := value{
			ResourceList: container.Resources.Requests,
			Level:        collectorcontrollerv1alpha1.ContainerLevel,
			Source:       collectorcontrollerv1alpha1.ContainerRequestsAllocatedSource,
		}
		limits := value{
			ResourceList: container.Resources.Limits,
			Level:        collectorcontrollerv1alpha1.ContainerLevel,
			Source:       collectorcontrollerv1alpha1.ContainerLimitsAllocatedSource,
		}

		values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedSource] = requests
		values[collectorcontrollerv1alpha1.ContainerLimitsAllocatedSource] = limits
	}

	if usage == nil || len(usage.CpuCoresNanoSec) == 0 || len(usage.MemoryBytes) == 0 {
		return values
	}

	values[collectorcontrollerv1alpha1.AvgContainerUtilizationSource] = value{
		ResourceList: corev1.ResourceList{
			collectorcontrollerv1alpha1.ResourceCPU:    *resource.NewScaledQuantity(usage.AvgCPUCoresNanoSec, resource.Nano),
			collectorcontrollerv1alpha1.ResourceMemory: *resource.NewQuantity(usage.AvgMemoryBytes, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.ContainerLevel,
		Source: collectorcontrollerv1alpha1.AvgContainerUtilizationSource,
	}

	values[collectorcontrollerv1alpha1.ContainerUtilizationSource] = value{
		MultiResourceList: map[corev1.ResourceName][]resource.Quantity{},
		Level:             collectorcontrollerv1alpha1.ContainerLevel,
		Source:            collectorcontrollerv1alpha1.ContainerUtilizationSource,
	}
	values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedMinusUtilizationSource] = value{
		MultiResourceList: map[corev1.ResourceName][]resource.Quantity{},
		Level:             collectorcontrollerv1alpha1.ContainerLevel,
		Source:            collectorcontrollerv1alpha1.ContainerRequestsAllocatedMinusUtilizationSource,
	}

	// get utilization values
	last := len(usage.CpuCoresNanoSec)
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		// for completed pods, assume the final 0 utilization values are because the container had
		// terminated and strip them from the results
		for last > 0 {
			if usage.CpuCoresNanoSec[last-1] != 0 {
				break
			}
			last--
		}
	}

	cpuValues := make([]resource.Quantity, 0, last)
	for i := 0; i < last; i++ {
		cpuValues = append(cpuValues, *resource.NewScaledQuantity(usage.CpuCoresNanoSec[i], resource.Nano))
	}
	if len(cpuValues) > 0 {
		values[collectorcontrollerv1alpha1.ContainerUtilizationSource].MultiResourceList[collectorcontrollerv1alpha1.ResourceCPU] = cpuValues

		// get requests-utilization values
		requestsAllocated := values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedSource].ResourceList[collectorcontrollerv1alpha1.ResourceCPU]
		requestsMinusUtilization := make([]resource.Quantity, len(cpuValues))
		for ii := 0; ii < len(cpuValues); ii++ {
			var requestMinusUtilization resource.Quantity = requestsAllocated.DeepCopy()
			requestMinusUtilization.Sub(cpuValues[ii])
			requestsMinusUtilization[ii] = requestMinusUtilization
		}
		values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedMinusUtilizationSource].MultiResourceList[collectorcontrollerv1alpha1.ResourceCPU] = requestsMinusUtilization
	}

	last = len(usage.MemoryBytes)
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		// for completed pods, assume the final 0 utilization values are because the container had
		// terminated and strip them from the results
		for last > 0 {
			if usage.MemoryBytes[last-1] != 0 {
				break
			}
			last--
		}
	}

	memoryValues := make([]resource.Quantity, 0, last)
	for i := 0; i < last; i++ {
		memoryValues = append(memoryValues, *resource.NewQuantity(usage.MemoryBytes[i], resource.DecimalSI))
	}
	if len(memoryValues) > 0 {
		values[collectorcontrollerv1alpha1.ContainerUtilizationSource].MultiResourceList[collectorcontrollerv1alpha1.ResourceMemory] = memoryValues

		// get requests-utilization values
		requestsAllocated := values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedSource].ResourceList[collectorcontrollerv1alpha1.ResourceMemory]
		requestsMinusUtilization := make([]resource.Quantity, len(memoryValues))
		for ii := 0; ii < len(memoryValues); ii++ {
			var requestMinusUtilization resource.Quantity = requestsAllocated.DeepCopy()
			requestMinusUtilization.Sub(memoryValues[ii])
			requestsMinusUtilization[ii] = requestMinusUtilization
		}
		values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedMinusUtilizationSource].MultiResourceList[collectorcontrollerv1alpha1.ResourceMemory] = requestsMinusUtilization
	}

	if len(usage.CpuPeriodsSec) == 0 || len(usage.CpuThrottledPeriodsSec) == 0 {
		return values
	}

	// get nr_period values
	values[collectorcontrollerv1alpha1.NRPeriodsSource] = createValue(pod, usage.CpuPeriodsSec, collectorcontrollerv1alpha1.NRPeriodsSource, collectorcontrollerv1alpha1.ResourcePeriods)
	values[collectorcontrollerv1alpha1.AvgNRPeriodsSource] = value{
		ResourceList: corev1.ResourceList{
			collectorcontrollerv1alpha1.ResourcePeriods: *resource.NewQuantity(usage.AvgCPUPeriodsSec, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.ContainerLevel,
		Source: collectorcontrollerv1alpha1.AvgNRPeriodsSource,
	}

	// get nr_period values
	values[collectorcontrollerv1alpha1.NRThrottledSource] = createValue(pod, usage.CpuThrottledPeriodsSec, collectorcontrollerv1alpha1.NRThrottledSource, collectorcontrollerv1alpha1.ResourcePeriods)
	values[collectorcontrollerv1alpha1.AvgNRThrottledSource] = value{
		ResourceList: corev1.ResourceList{
			collectorcontrollerv1alpha1.ResourcePeriods: *resource.NewQuantity(usage.AvgCPUThrottledPeriodsSec, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.ContainerLevel,
		Source: collectorcontrollerv1alpha1.AvgNRThrottledSource,
	}

	// get oom / oom kill counter values
	values[collectorcontrollerv1alpha1.OOMKillCountSource] = value{
		ResourceList: corev1.ResourceList{
			collectorcontrollerv1alpha1.ResourceItems: *resource.NewQuantity(usage.OomKillCount, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.ContainerLevel,
		Source: collectorcontrollerv1alpha1.OOMKillCountSource,
	}
	values[collectorcontrollerv1alpha1.OOMCountSource] = value{
		ResourceList: corev1.ResourceList{
			collectorcontrollerv1alpha1.ResourceItems: *resource.NewQuantity(usage.OomCount, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.ContainerLevel,
		Source: collectorcontrollerv1alpha1.OOMCountSource,
	}

	return values
}

// createValue returns value for a container given the source usage
func createValue(pod *corev1.Pod, usage []int64, source collectorcontrollerv1alpha1.Source, resName collectorcontrollerv1alpha1.ResourceName) value {
	val := value{
		MultiResourceList: map[corev1.ResourceName][]resource.Quantity{},
		Level:             collectorcontrollerv1alpha1.ContainerLevel,
		Source:            source,
	}

	last := len(usage)
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		// for completed pods, assume the final 0 utilization values are because the container had
		// terminated and strip them from the results
		for last > 0 {
			if usage[last-1] != 0 {
				break
			}
			last--
		}
	}

	qty := make([]resource.Quantity, 0, last)

	for i := 0; i < last; i++ {
		qty = append(qty, *resource.NewQuantity(usage[i], resource.DecimalSI))
	}
	if len(qty) > 0 {
		val.MultiResourceList[resName] = qty
	}
	return val
}

func (r ValueReader) GetValuesForPod(pod *corev1.Pod) map[collectorcontrollerv1alpha1.Source]value {
	count := value{
		ResourceList: map[corev1.ResourceName]resource.Quantity{
			collectorcontrollerv1alpha1.ResourceItems: *resource.NewQuantity(1, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.PodLevel,
		Source: collectorcontrollerv1alpha1.PodItemsSource,
	}

	return map[collectorcontrollerv1alpha1.Source]value{
		collectorcontrollerv1alpha1.PodItemsSource: count,
	}
}

func (r ValueReader) GetValuesForSchedulerHealth(pod *corev1.Pod, s *collectorcontrollerv1alpha1.SchedulerHealth) map[collectorcontrollerv1alpha1.Source]value {

	if s == nil {
		// scheduler health not configured
		return nil
	}

	// Missing critical information, no metric will be emitted.
	if pod.CreationTimestamp.Time.IsZero() {
		log.V(1).Info("missing creation timestamp in pod", "pod", pod.ObjectMeta)
		return nil
	}

	n := now()
	podAge := n.Sub(pod.CreationTimestamp.Time)

	if podAge > (time.Duration(s.MaxPodAgeMinutes) * time.Minute) {
		// ignore old pods -- they create noise in the metrics
		return nil
	}

	var scheduleDuration time.Duration
	minAge := (time.Duration(s.MinPodAgeMinutes) * time.Minute)

	if pod.Spec.NodeName != "" {
		// pod has been placed on a node -- check for the scheduling condition
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionTrue {
				scheduleDuration = cond.LastTransitionTime.Time.Sub(pod.CreationTimestamp.Time)
				break
			}
		}
	} else if podAge > minAge {
		// For unscheduled pods, use the pod age once it reaches a certain age
		scheduleDuration = n.Sub(pod.CreationTimestamp.Time)
	}

	if scheduleDuration == 0 {
		// unable to determine scheduling time for the pod
		return nil
	}

	return map[collectorcontrollerv1alpha1.Source]value{
		collectorcontrollerv1alpha1.SchedulerPodScheduleWait: {
			Level:  collectorcontrollerv1alpha1.PodLevel,
			Source: collectorcontrollerv1alpha1.SchedulerPodScheduleWait,
			ResourceList: map[corev1.ResourceName]resource.Quantity{
				collectorcontrollerv1alpha1.ResourceTime: *resource.NewMilliQuantity(scheduleDuration.Milliseconds(), resource.DecimalSI),
			},
		},
	}
}

var now = func() time.Time {
	return time.Now()
}

// GetValuesForQuota returns the ResourceLists from a namespace quota
func (r ValueReader) GetValuesForQuota(quota *corev1.ResourceQuota, rqd *quotamanagementv1alpha1.ResourceQuotaDescriptor, enableRqd bool) map[collectorcontrollerv1alpha1.Source]value {
	requestsHard := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaRequestsHardSource,
	}
	limitsHard := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaLimitsHardSource,
	}
	requestsHard.ResourceList, limitsHard.ResourceList = splitRequestsLimitsQuota(quota.Status.Hard)

	pvcHard := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.PVCQuotaRequestsHardSource,
	}
	pvcHard.ResourceList = getRequestsPVCQuota(quota.Status.Hard)

	requestsUsed := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaRequestsUsedSource,
	}
	limitsUsed := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaLimitsUsedSource,
	}
	requestsUsed.ResourceList, limitsUsed.ResourceList = splitRequestsLimitsQuota(quota.Status.Used)

	requestsHardMinusUsed := value{
		Level:        collectorcontrollerv1alpha1.NamespaceLevel,
		Source:       collectorcontrollerv1alpha1.QuotaRequestsHardSource,
		ResourceList: subLists(requestsHard.ResourceList, requestsUsed.ResourceList),
	}

	limitsHardMinusUsed := value{
		Level:        collectorcontrollerv1alpha1.NamespaceLevel,
		Source:       collectorcontrollerv1alpha1.QuotaLimitsHardSource,
		ResourceList: subLists(limitsHard.ResourceList, limitsUsed.ResourceList),
	}

	pvcUsed := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.PVCQuotaRequestsUsedSource,
	}
	pvcUsed.ResourceList = getRequestsPVCQuota(quota.Status.Used)

	items := value{
		ResourceList: map[corev1.ResourceName]resource.Quantity{
			collectorcontrollerv1alpha1.ResourceItems: *resource.NewQuantity(1, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaItemsSource,
	}

	ret := map[collectorcontrollerv1alpha1.Source]value{
		collectorcontrollerv1alpha1.QuotaRequestsHardSource:    requestsHard,
		collectorcontrollerv1alpha1.QuotaLimitsHardSource:      limitsHard,
		collectorcontrollerv1alpha1.QuotaRequestsUsedSource:    requestsUsed,
		collectorcontrollerv1alpha1.QuotaLimitsUsedSource:      limitsUsed,
		collectorcontrollerv1alpha1.QuotaRequestsHardMinusUsed: requestsHardMinusUsed,
		collectorcontrollerv1alpha1.QuotaLimitsHardMinusUsed:   limitsHardMinusUsed,
		collectorcontrollerv1alpha1.QuotaItemsSource:           items,
		collectorcontrollerv1alpha1.PVCQuotaRequestsHardSource: pvcHard,
		collectorcontrollerv1alpha1.PVCQuotaRequestsUsedSource: pvcUsed,
	}

	// RQD-related metrics
	proposedLimitsQuota := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaDescriptorLimitsProposedSource,
	}

	proposedRequestQuota := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaDescriptorRequestsProposedSource,
	}

	maxObservedQuotaRequestsMinusHard := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaDescriptorRequestsMaxObservedMinusHardSource,
	}

	maxObservedQuotaLimitsMinusHard := value{
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaDescriptorLimitsMaxObservedMinusHardSource,
	}

	requestsHardMinusProposed := value{
		Level:        collectorcontrollerv1alpha1.NamespaceLevel,
		Source:       collectorcontrollerv1alpha1.QuotaDescriptorLimitsHardMinusProposedSource,
		ResourceList: subLists(requestsHard.ResourceList, proposedRequestQuota.ResourceList),
	}

	limitsHardMinusProposed := value{
		Level:        collectorcontrollerv1alpha1.NamespaceLevel,
		Source:       collectorcontrollerv1alpha1.QuotaDescriptorRequestsHardMinusProposedSource,
		ResourceList: subLists(limitsHard.ResourceList, proposedLimitsQuota.ResourceList),
	}

	if enableRqd {
		if rqd != nil {
			proposedLimitsQuota.ResourceList =
				getProposedQuota(rqd.Status.ProposedQuota, collectorcontrollerv1alpha1.LimitsResourcePrefix)

			proposedRequestQuota.ResourceList =
				getProposedQuota(rqd.Status.ProposedQuota, collectorcontrollerv1alpha1.RequestsResourcePrefix)

			maxObservedQuota, err := getMaxObservedQuota(rqd)
			if err == nil && maxObservedQuota != nil {
				maxObservedRequestsQuota, maxObservedLimitsQuota := splitRequestsLimitsQuota(maxObservedQuota)
				maxObservedQuotaRequestsMinusHard.ResourceList = subLists(maxObservedRequestsQuota, requestsHard.ResourceList)
				maxObservedQuotaLimitsMinusHard.ResourceList = subLists(maxObservedLimitsQuota, limitsHard.ResourceList)
			}
		}

		requestsHardMinusProposed.ResourceList = subLists(requestsHard.ResourceList, proposedRequestQuota.ResourceList)
		limitsHardMinusProposed.ResourceList = subLists(limitsHard.ResourceList, proposedLimitsQuota.ResourceList)

		ret[collectorcontrollerv1alpha1.QuotaDescriptorLimitsProposedSource] = proposedLimitsQuota
		ret[collectorcontrollerv1alpha1.QuotaDescriptorRequestsProposedSource] = proposedRequestQuota
		ret[collectorcontrollerv1alpha1.QuotaDescriptorRequestsHardMinusProposedSource] = requestsHardMinusProposed
		ret[collectorcontrollerv1alpha1.QuotaDescriptorLimitsHardMinusProposedSource] = limitsHardMinusProposed
		ret[collectorcontrollerv1alpha1.QuotaDescriptorRequestsMaxObservedMinusHardSource] = maxObservedQuotaRequestsMinusHard
		ret[collectorcontrollerv1alpha1.QuotaDescriptorLimitsMaxObservedMinusHardSource] = maxObservedQuotaLimitsMinusHard
	}

	return ret
}

// splitRequestsLimitsQuota normalizes the input ResourceList keys for requests and limits
// by dropping the `requests.` and `limits.` prefixes and partitioning it into 2 ResourceLists
// Returns the ResourceLists for `requests` and `limits` respectively.
func splitRequestsLimitsQuota(input corev1.ResourceList) (corev1.ResourceList, corev1.ResourceList) {
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}
	for k, v := range input {
		if strings.HasPrefix(string(k), "requests.") {
			requests[corev1.ResourceName(strings.TrimPrefix(string(k), "requests."))] = v
		} else if strings.HasPrefix(string(k), "limits.") {
			limits[corev1.ResourceName(strings.TrimPrefix(string(k), "limits."))] = v
		}
	}
	return requests, limits
}

// subLists subtracks quantities in `b` from quantities that exist under the
// same key in `a` and returns a ResourceList with the difference. A key that
// does not appear in both lists will not be present in the result.
func subLists(a, b corev1.ResourceList) corev1.ResourceList {
	// IMPORTANT: must deep copy the lists since they are maps, failure to do so
	// will mutate the original maps, which are used for other things
	// TODO: write a tests the ensures these are deep copied
	a = a.DeepCopy()
	b = b.DeepCopy()
	result := corev1.ResourceList{}
	for k, vA := range a {
		vB, ok := b[k]
		if ok {
			vA.Sub(vB)
			result[k] = vA
		}
	}

	return result
}

// getProposedQuota reads proposed quota from rqd status
func getProposedQuota(proposed corev1.ResourceList, resourcePrefix string) corev1.ResourceList {
	rlQuota := make(map[corev1.ResourceName]resource.Quantity)

	for _, resourceName := range sets.List(collectorcontrollerv1alpha1.ResourceNames) {
		name := corev1.ResourceName(resourcePrefix + "." + resourceName.String())
		if _, ok := proposed[name]; ok {
			rlQuota[resourceName] = proposed[name]
		}
	}

	return rlQuota
}

// getRequestsPVCQuota gets the requests storage and persistentvolumeclaims
// for PVC storage classes see https://kubernetes.io/docs/concepts/policy/resource-quotas
func getRequestsPVCQuota(input corev1.ResourceList) corev1.ResourceList {
	requests := corev1.ResourceList{}
	for k, v := range input {
		if strings.Contains(k.String(), "storageclass.storage.k8s.io") {
			requests[k] = v
		}
	}
	return requests
}

// getNodeRequestsLimits aggregates the requests and limits for all pods running on node
func getNodeRequestsLimits(pods []*corev1.Pod) (corev1.ResourceList, corev1.ResourceList) {
	var cpuRequests, memoryRequests resource.Quantity
	var cpuLimits, memoryLimits resource.Quantity

	for _, pod := range pods {
		// skip pods in completed state
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		for _, container := range pod.Spec.Containers {
			cpuRequests.Add(*container.Resources.Requests.Cpu())
			memoryRequests.Add(*container.Resources.Requests.Memory())
			cpuLimits.Add(*container.Resources.Limits.Cpu())
			memoryLimits.Add(*container.Resources.Limits.Memory())
		}

	}

	return corev1.ResourceList{
			collectorcontrollerv1alpha1.ResourceCPU:    cpuRequests,
			collectorcontrollerv1alpha1.ResourceMemory: memoryRequests,
		},
		corev1.ResourceList{
			collectorcontrollerv1alpha1.ResourceCPU:    cpuLimits,
			collectorcontrollerv1alpha1.ResourceMemory: memoryLimits,
		}
}

// getAllocatableMinusRequests calculates the delta between node allocatable and requests
func getAllocatableMinusRequests(allocatable, requests corev1.ResourceList) corev1.ResourceList {
	cpu := allocatable.Cpu().DeepCopy()
	memory := allocatable.Memory().DeepCopy()
	cpu.Sub(*requests.Cpu())
	memory.Sub(*requests.Memory())

	return corev1.ResourceList{
		collectorcontrollerv1alpha1.ResourceCPU:    cpu,
		collectorcontrollerv1alpha1.ResourceMemory: memory,
	}
}

func getMaxObservedQuota(rqd *quotamanagementv1alpha1.ResourceQuotaDescriptor) (corev1.ResourceList, error) {
	if rqd.ObjectMeta.Annotations == nil {
		return nil, nil
	}

	serializedMaxQ, ok := rqd.ObjectMeta.Annotations[quotamanagementv1alpha1.MaxObservedQuotaAnnotationKey]
	if !ok {
		return nil, nil
	}

	maxObservedQuota := corev1.ResourceList{}

	err := yaml.UnmarshalStrict([]byte(serializedMaxQ), &maxObservedQuota)
	if err != nil {
		return nil, err
	}

	return maxObservedQuota, nil
}

// GetValuesForNode returns the metric values for a Node.  pods is the Pods scheduled to this Node.
func (r ValueReader) GetValuesForNode(node *corev1.Node, pods []*corev1.Pod) map[collectorcontrollerv1alpha1.Source]value {
	allocatable := value{
		ResourceList: node.Status.Allocatable,
		Level:        collectorcontrollerv1alpha1.NodeLevel,
		Source:       collectorcontrollerv1alpha1.NodeAllocatableSource,
	}
	capacity := value{
		ResourceList: node.Status.Capacity,
		Level:        collectorcontrollerv1alpha1.NamespaceLevel,
		Source:       collectorcontrollerv1alpha1.NodeCapacitySource,
	}
	count := value{
		ResourceList: map[corev1.ResourceName]resource.Quantity{
			collectorcontrollerv1alpha1.ResourceItems: *resource.NewQuantity(1, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.NodeLevel,
		Source: collectorcontrollerv1alpha1.NodeItemsSource,
	}

	requests := value{
		Level:  collectorcontrollerv1alpha1.NodeLevel,
		Source: collectorcontrollerv1alpha1.NodeRequestsSource,
	}
	limits := value{
		Level:  collectorcontrollerv1alpha1.NodeLevel,
		Source: collectorcontrollerv1alpha1.NodeLimitsSource,
	}
	requests.ResourceList, limits.ResourceList = getNodeRequestsLimits(pods)

	allocatableMinusRequests := value{
		Level:  collectorcontrollerv1alpha1.NodeLevel,
		Source: collectorcontrollerv1alpha1.NodeAllocatableMinusRequests,
	}

	allocatableMinusRequests.ResourceList = getAllocatableMinusRequests(allocatable.ResourceList, requests.ResourceList)

	return map[collectorcontrollerv1alpha1.Source]value{
		collectorcontrollerv1alpha1.NodeAllocatableSource:        allocatable,
		collectorcontrollerv1alpha1.NodeCapacitySource:           capacity,
		collectorcontrollerv1alpha1.NodeItemsSource:              count,
		collectorcontrollerv1alpha1.NodeRequestsSource:           requests,
		collectorcontrollerv1alpha1.NodeLimitsSource:             limits,
		collectorcontrollerv1alpha1.NodeAllocatableMinusRequests: allocatableMinusRequests}
}

func (r ValueReader) GetValuesForPVC(pvc *corev1.PersistentVolumeClaim) map[collectorcontrollerv1alpha1.Source]value {
	requests := value{
		ResourceList: pvc.Spec.Resources.Requests,
		Level:        collectorcontrollerv1alpha1.PVCLevel,
		Source:       collectorcontrollerv1alpha1.PVCRequestsSource,
	}
	limits := value{
		ResourceList: pvc.Spec.Resources.Limits,
		Level:        collectorcontrollerv1alpha1.PVCLevel,
		Source:       collectorcontrollerv1alpha1.PVCLimitsSource,
	}
	capacity := value{
		ResourceList: pvc.Status.Capacity,
		Level:        collectorcontrollerv1alpha1.PVCLevel,
		Source:       collectorcontrollerv1alpha1.PVCCapacitySource,
	}
	count := value{
		ResourceList: map[corev1.ResourceName]resource.Quantity{
			collectorcontrollerv1alpha1.ResourceItems: *resource.NewQuantity(1, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.PVCLevel,
		Source: collectorcontrollerv1alpha1.PVCItemsSource,
	}

	return map[collectorcontrollerv1alpha1.Source]value{
		collectorcontrollerv1alpha1.PVCRequestsSource: requests,
		collectorcontrollerv1alpha1.PVCLimitsSource:   limits,
		collectorcontrollerv1alpha1.PVCCapacitySource: capacity,
		collectorcontrollerv1alpha1.PVCItemsSource:    count,
	}
}

func (r ValueReader) GetValuesForPV(pv *corev1.PersistentVolume) map[collectorcontrollerv1alpha1.Source]value {
	capacity := value{
		ResourceList: pv.Spec.Capacity,
		Level:        collectorcontrollerv1alpha1.PVLevel,
		Source:       collectorcontrollerv1alpha1.PVCCapacitySource,
	}
	count := value{
		ResourceList: map[corev1.ResourceName]resource.Quantity{
			collectorcontrollerv1alpha1.ResourceItems: *resource.NewQuantity(1, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.PVLevel,
		Source: collectorcontrollerv1alpha1.PVItemsSource,
	}

	return map[collectorcontrollerv1alpha1.Source]value{
		collectorcontrollerv1alpha1.PVCapacitySource: capacity,
		collectorcontrollerv1alpha1.PVItemsSource:    count}
}
