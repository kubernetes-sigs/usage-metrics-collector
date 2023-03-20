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
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/quotamanagementv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
	"sigs.k8s.io/yaml"
)

// value contains a metric value. Everything in a value should have the exact
// same set of labels.
type value struct {
	ResourceList      corev1.ResourceList                         `json:"resourceList,omitempty" yaml:"resourceList,omitempty"`
	MultiResourceList map[corev1.ResourceName][]resource.Quantity `json:"multiResourceList,omitempty" yaml:"multiResourceList,omitempty"`
	Level             string                                      `json:"level" yaml:"level"`
	Source            string                                      `json:"source" yaml:"source"`
}

// RequestsValueReader reads the requests value as a ResourceList
type valueReader struct{}

// GetValuesForContainer returns the ResourceLists from a container for:
// - requests_allocated
// - limits_allocated
// - utilization
// - requests_allocated_minus_utilization
// - nr_periods
// - nr_throttled
// - oom_kill
func (r valueReader) GetValuesForContainer(
	container *corev1.Container, pod *corev1.Pod, usage *api.ContainerMetrics) map[string]value {
	values := map[string]value{}

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
	var cpuValues []resource.Quantity
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
	for i := 0; i < last; i++ {
		cpuValues = append(cpuValues, *resource.NewScaledQuantity(usage.CpuCoresNanoSec[i], resource.Nano))
	}
	if len(cpuValues) > 0 {
		values[collectorcontrollerv1alpha1.ContainerUtilizationSource].MultiResourceList["cpu"] = cpuValues

		// get requests-utilization values
		requestsAllocated := values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedSource].ResourceList["cpu"]
		requestsMinusUtilization := make([]resource.Quantity, len(cpuValues))
		for ii := 0; ii < len(cpuValues); ii++ {
			var requestMinusUtilization resource.Quantity = requestsAllocated.DeepCopy()
			requestMinusUtilization.Sub(cpuValues[ii])
			requestsMinusUtilization[ii] = requestMinusUtilization
		}
		values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedMinusUtilizationSource].MultiResourceList["cpu"] = requestsMinusUtilization
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
	var memoryValues []resource.Quantity
	for i := 0; i < last; i++ {
		memoryValues = append(memoryValues, *resource.NewQuantity(usage.MemoryBytes[i], resource.DecimalSI))
	}
	if len(memoryValues) > 0 {
		values[collectorcontrollerv1alpha1.ContainerUtilizationSource].MultiResourceList["memory"] = memoryValues

		// get requests-utilization values
		requestsAllocated := values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedSource].ResourceList["memory"]
		requestsMinusUtilization := make([]resource.Quantity, len(memoryValues))
		for ii := 0; ii < len(memoryValues); ii++ {
			var requestMinusUtilization resource.Quantity = requestsAllocated.DeepCopy()
			requestMinusUtilization.Sub(memoryValues[ii])
			requestsMinusUtilization[ii] = requestMinusUtilization
		}
		values[collectorcontrollerv1alpha1.ContainerRequestsAllocatedMinusUtilizationSource].MultiResourceList["memory"] = requestsMinusUtilization
	}

	if len(usage.CpuPeriodsSec) == 0 || len(usage.CpuThrottledPeriodsSec) == 0 || len(usage.OomKillCount) == 0 {
		return values
	}

	// get nr_period values
	values[collectorcontrollerv1alpha1.NRPeriodsSource] = createValue(pod, usage.CpuPeriodsSec, collectorcontrollerv1alpha1.NRPeriodsSource, "periods")
	// get nr_period values
	values[collectorcontrollerv1alpha1.NRThrottledSource] = createValue(pod, usage.CpuThrottledPeriodsSec, collectorcontrollerv1alpha1.NRThrottledSource, "periods")
	// get oom kill counter values
	values[collectorcontrollerv1alpha1.OOMKillCountSource] = createValue(pod, usage.OomKillCount, collectorcontrollerv1alpha1.OOMKillCountSource, "items")

	return values
}

// createValue returns value for a container given the source usage
func createValue(pod *corev1.Pod, usage []int64, source, resName string) value {
	val := value{
		MultiResourceList: map[corev1.ResourceName][]resource.Quantity{},
		Level:             collectorcontrollerv1alpha1.ContainerLevel,
		Source:            source,
	}

	var qty []resource.Quantity
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
	for i := 0; i < last; i++ {
		qty = append(qty, *resource.NewQuantity(usage[i], resource.DecimalSI))
	}
	if len(qty) > 0 {
		val.MultiResourceList[corev1.ResourceName(resName)] = qty
	}
	return val
}

func (r valueReader) GetValuesForPod(pod *corev1.Pod) map[string]value {
	count := value{
		ResourceList: map[corev1.ResourceName]resource.Quantity{
			collectorcontrollerv1alpha1.ItemsResource: *resource.NewQuantity(1, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.PodLevel,
		Source: collectorcontrollerv1alpha1.PodItemsSource,
	}

	if pod.Spec.NodeName != "" {
		for _, c := range pod.Status.Conditions {
			if c.Type != corev1.PodScheduled || c.Status != corev1.ConditionTrue {
				continue
			}
			scheduleTime := c.LastTransitionTime.Time.Sub(pod.CreationTimestamp.Time)
			count.ResourceList[collectorcontrollerv1alpha1.ScheduleResource] = *resource.NewQuantity(int64(scheduleTime.Seconds()), resource.DecimalSI)
			break
		}
	} else if !pod.CreationTimestamp.IsZero() {
		waitTime := now().Sub(pod.CreationTimestamp.Time)
		count.ResourceList[collectorcontrollerv1alpha1.ScheduleWaitResource] = *resource.NewQuantity(int64(waitTime.Seconds()), resource.DecimalSI)
	}

	return map[string]value{
		collectorcontrollerv1alpha1.PodItemsSource: count,
	}
}

var now = func() time.Time {
	return time.Now()
}

// GetValuesForQuota returns the ResourceLists from a namespace quota
func (r valueReader) GetValuesForQuota(quota *corev1.ResourceQuota, rqd *quotamanagementv1alpha1.ResourceQuotaDescriptor, enableRqd bool) map[string]value {
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
			collectorcontrollerv1alpha1.ItemsResource: *resource.NewQuantity(1, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.NamespaceLevel,
		Source: collectorcontrollerv1alpha1.QuotaItemsSource,
	}

	ret := map[string]value{
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

	for _, resourceType := range collectorcontrollerv1alpha1.ResourceTypes {
		name := corev1.ResourceName(resourcePrefix + "." + resourceType)
		if _, ok := proposed[name]; ok {
			rlQuota[corev1.ResourceName(resourceType)] = proposed[name]
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
			"cpu":    cpuRequests,
			"memory": memoryRequests,
		},
		corev1.ResourceList{
			"cpu":    cpuLimits,
			"memory": memoryLimits,
		}
}

// getAllocatableMinusRequests calculates the delta between node allocatable and requests
func getAllocatableMinusRequests(allocatable, requests corev1.ResourceList) corev1.ResourceList {
	cpu := allocatable.Cpu().DeepCopy()
	memory := allocatable.Memory().DeepCopy()
	cpu.Sub(*requests.Cpu())
	memory.Sub(*requests.Memory())

	return corev1.ResourceList{
		"cpu":    cpu,
		"memory": memory,
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
func (r valueReader) GetValuesForNode(node *corev1.Node, pods []*corev1.Pod) map[string]value {
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
			collectorcontrollerv1alpha1.ItemsResource: *resource.NewQuantity(1, resource.DecimalSI),
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

	return map[string]value{
		collectorcontrollerv1alpha1.NodeAllocatableSource:        allocatable,
		collectorcontrollerv1alpha1.NodeCapacitySource:           capacity,
		collectorcontrollerv1alpha1.NodeItemsSource:              count,
		collectorcontrollerv1alpha1.NodeRequestsSource:           requests,
		collectorcontrollerv1alpha1.NodeLimitsSource:             limits,
		collectorcontrollerv1alpha1.NodeAllocatableMinusRequests: allocatableMinusRequests}
}

func (r valueReader) GetValuesForPVC(pvc *corev1.PersistentVolumeClaim) map[string]value {
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
			collectorcontrollerv1alpha1.ItemsResource: *resource.NewQuantity(1, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.PVCLevel,
		Source: collectorcontrollerv1alpha1.PVCItemsSource,
	}

	return map[string]value{
		collectorcontrollerv1alpha1.PVCRequestsSource: requests,
		collectorcontrollerv1alpha1.PVCLimitsSource:   limits,
		collectorcontrollerv1alpha1.PVCCapacitySource: capacity,
		collectorcontrollerv1alpha1.PVCItemsSource:    count,
	}
}

func (r valueReader) GetValuesForPV(pv *corev1.PersistentVolume) map[string]value {
	capacity := value{
		ResourceList: pv.Spec.Capacity,
		Level:        collectorcontrollerv1alpha1.PVLevel,
		Source:       collectorcontrollerv1alpha1.PVCCapacitySource,
	}
	count := value{
		ResourceList: map[corev1.ResourceName]resource.Quantity{
			collectorcontrollerv1alpha1.ItemsResource: *resource.NewQuantity(1, resource.DecimalSI),
		},
		Level:  collectorcontrollerv1alpha1.PVLevel,
		Source: collectorcontrollerv1alpha1.PVItemsSource,
	}

	return map[string]value{
		collectorcontrollerv1alpha1.PVCapacitySource: capacity,
		collectorcontrollerv1alpha1.PVItemsSource:    count}
}
