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
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/quotamanagementv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
)

type builtInLabelsValues struct {
	// ContainerName is the name of a container within a pod -- e.g. log-saver
	ContainerName string `json:"exported_container,omitempty" yaml:"exported_container,omitempty"`
	// ContainerImage is the container image in the container
	ContainerImage string `json:"container_image,omitempty" yaml:"container_image,omitempty"`
	// ContainerImageID is the container image with digest in the container
	ContainerImageID string `json:"container_image_id,omitempty" yaml:"container_image_id,omitempty"`
	// PodName is the name of a pod.
	PodName string `json:"exported_pod,omitempty" yaml:"exported_pod,omitempty"`
	// NamespaceName is the name of a namespace.
	NamespaceName string `json:"exported_namespace,omitempty" yaml:"exported_namespace,omitempty"`
	// NodeName is the name of a node.
	NodeName string `json:"exported_node" yaml:"exported_node"`

	// NodeUnschedulable is true if spec.unschedulable is true
	NodeUnschedulable string `json:"node_unschedulable,omitempty" yaml:"node_unschedulable,omitempty"`

	// WorkloadName is the name of a workload as defined by the
	// pod OwnerReferences.
	WorkloadName string `json:"workload_name,omitempty" yaml:"workload_name,omitempty"`
	// WorkloadKind is the kind of a workload as defined by the
	// pod OwnerReferences.
	WorkloadKind string `json:"workload_kind,omitempty" yaml:"workload_kind,omitempty"`
	// WorkloadAPIGroup is the api group of a workload as defined by the
	// pod OwnerReferences.
	WorkloadAPIGroup string `json:"workload_api_group,omitempty" yaml:"workload_api_group,omitempty"`
	// WorkloadAPIVersion is the api version of a workload as defined by the
	// pod OwnerReferences.
	WorkloadAPIVersion string `json:"workload_api_version,omitempty" yaml:"workload_api_version,omitempty"`

	// App is the name of a logical app a pod or workload belongs to as
	// defined by the app pod label.  An app may be composed of multiple
	// workloads -- e.g. 2 deployments compose a logical app.
	App string `json:"app,omitempty" yaml:"app,omitempty"`
	// QuotaName is the name of the quota object
	QuotaName string `json:"quota_name,omitempty" yaml:"quota_name,omitempty"`

	// AllocationStartegy is the type of allocation strategy
	AllocationStrategy string `json:"allocation_strategy,omitempty" yaml:"allocation_strategy,omitempty"`

	// PriorityClass is the name of a priorityClass as defined by the
	// spec.priorityClassName field.
	PriorityClass string `json:"priority_class" yaml:"priority_class"`

	// Scheduled is true for pod metrics that are scheduled
	Scheduled string `json:"scheduled,omitempty" yaml:"scheduled,omitempty"`

	// PVCName is the PersistentVolumeClaim name
	PVCName string `json:"exported_pvc,omitempty" yaml:"exported_pvc,omitempty"`

	// PVName is the PersistentVolume name
	PVName string `json:"exported_pv,omitempty" yaml:"exported_pv,omitempty"`

	// StorageClass is the storageClass of a PersistentVolume or PersistentVolumeClaim
	StorageClass string `json:"storage_class,omitempty" yaml:"storage_class,omitempty"`

	// GGroup is the cgroup of utilization metrics
	CGroup string `json:"cgroup,omitempty" yaml:"cgroup,omitempty"`

	// Phase is the phase of a PersistentVolume or PersistentVolumeClaim
	Phase string `json:"phase,omitempty" yaml:"phase,omitempty"`

	// level is the aggregation level for the metrics
	Level collectorcontrollerv1alpha1.AggregationLevel `json:"level,omitempty" yaml:"level,omitempty"`
}

// GetLabelNames returns the set of label names the mask keeps
// nolint: gocyclo
func getBuiltInLabelNames(mask collectorcontrollerv1alpha1.BuiltInLabelsMask) []string {
	var labels []string

	if mask.ContainerName {
		labels = append(labels, collectorcontrollerv1alpha1.ExportedContainerLabel)
	}
	if mask.ContainerImage {
		labels = append(labels, collectorcontrollerv1alpha1.ContainerImageLabel)
	}
	if mask.ContainerImageID {
		labels = append(labels, collectorcontrollerv1alpha1.ContainerImageIDLabel)
	}
	if mask.PodName {
		labels = append(labels, collectorcontrollerv1alpha1.ExportedPodLabel)
	}
	if mask.NamespaceName {
		labels = append(labels, collectorcontrollerv1alpha1.ExportedNamespaceLabel)
	}
	if mask.NodeName {
		labels = append(labels, collectorcontrollerv1alpha1.ExportedNodeLabel)
	}
	if mask.NodeUnschedulable {
		labels = append(labels, collectorcontrollerv1alpha1.NodeUnschedulableLabel)
	}

	if mask.WorkloadName {
		labels = append(labels, collectorcontrollerv1alpha1.WorkloadNameLabel)
	}
	if mask.WorkloadKind {
		labels = append(labels, collectorcontrollerv1alpha1.WorkloadKindLabel)
	}
	if mask.WorkloadAPIGroup {
		labels = append(labels, collectorcontrollerv1alpha1.WorkloadAPIGroupLabel)
	}
	if mask.WorkloadAPIVersion {
		labels = append(labels, collectorcontrollerv1alpha1.WorkloadAPIVersionLabel)
	}

	if mask.App {
		labels = append(labels, collectorcontrollerv1alpha1.AppLabel)
	}

	if mask.PriorityClass {
		labels = append(labels, collectorcontrollerv1alpha1.PriorityClassLabel)
	}
	if mask.Scheduled {
		labels = append(labels, collectorcontrollerv1alpha1.ScheduledLabel)
	}
	if mask.Level {
		labels = append(labels, collectorcontrollerv1alpha1.LevelLabel)
	}

	if mask.QuotaName {
		labels = append(labels, collectorcontrollerv1alpha1.QuotaLabel)
	}

	if mask.AllocationStrategy {
		labels = append(labels, collectorcontrollerv1alpha1.AllocationStrategyLabel)
	}

	// Volume labels
	if mask.PVCName {
		labels = append(labels, collectorcontrollerv1alpha1.PVCNameLabel)
	}
	if mask.PVName {
		labels = append(labels, collectorcontrollerv1alpha1.PVNameLabel)
	}
	if mask.StorageClass {
		labels = append(labels, collectorcontrollerv1alpha1.StorageClassLabel)
	}
	if mask.CGroup {
		labels = append(labels, collectorcontrollerv1alpha1.CGroupLabel)
	}
	if mask.Phase {
		labels = append(labels, collectorcontrollerv1alpha1.PhaseLabel)
	}

	return labels
}

// Mask returns a new BuiltInLabels with the mask applied
// nolint: gocyclo
func builtInMask(mask collectorcontrollerv1alpha1.BuiltInLabelsMask, m builtInLabelsValues) builtInLabelsValues {
	s := m
	if !mask.ContainerName {
		s.ContainerName = ""
	}
	if !mask.ContainerImage {
		s.ContainerImage = ""
	}
	if !mask.ContainerImageID {
		s.ContainerImageID = ""
	}
	if !mask.PodName {
		s.PodName = ""
	}
	if !mask.NamespaceName {
		s.NamespaceName = ""
	}
	if !mask.NodeName {
		s.NodeName = ""
	}
	if !mask.NodeUnschedulable {
		s.NodeUnschedulable = ""
	}

	if !mask.WorkloadName {
		s.WorkloadName = ""
	}
	if !mask.WorkloadKind {
		s.WorkloadKind = ""
	}
	if !mask.WorkloadAPIGroup {
		s.WorkloadAPIGroup = ""
	}
	if !mask.WorkloadAPIVersion {
		s.WorkloadAPIVersion = ""
	}

	if !mask.App {
		s.App = ""
	}

	if !mask.PriorityClass {
		s.PriorityClass = ""
	}
	if !mask.Scheduled {
		s.Scheduled = ""
	}
	if !mask.Level {
		s.Level = ""
	}

	if !mask.QuotaName {
		s.QuotaName = ""
	}

	if !mask.AllocationStrategy {
		s.AllocationStrategy = ""
	}

	// Volume labels
	if !mask.PVCName {
		s.PVCName = ""
	}
	if !mask.PVName {
		s.PVName = ""
	}
	if !mask.StorageClass {
		s.StorageClass = ""
	}
	if !mask.CGroup {
		s.CGroup = ""
	}
	if !mask.Phase {
		s.Phase = ""
	}
	return s
}

// GetLabelValues returns the label values with the mask applied.
// nolint: gocyclo
func getBuiltInLabelValues(mask collectorcontrollerv1alpha1.BuiltInLabelsMask, m builtInLabelsValues) []string {
	var labels []string
	if mask.ContainerName {
		labels = append(labels, m.ContainerName)
	}
	if mask.ContainerImage {
		labels = append(labels, m.ContainerImage)
	}
	if mask.ContainerImageID {
		labels = append(labels, m.ContainerImageID)
	}
	if mask.PodName {
		labels = append(labels, m.PodName)
	}
	if mask.NamespaceName {
		labels = append(labels, m.NamespaceName)
	}
	if mask.NodeName {
		labels = append(labels, m.NodeName)
	}
	if mask.NodeUnschedulable {
		labels = append(labels, m.NodeUnschedulable)
	}

	if mask.WorkloadName {
		labels = append(labels, m.WorkloadName)
	}
	if mask.WorkloadKind {
		labels = append(labels, m.WorkloadKind)
	}
	if mask.WorkloadAPIGroup {
		labels = append(labels, m.WorkloadAPIGroup)
	}
	if mask.WorkloadAPIVersion {
		labels = append(labels, m.WorkloadAPIVersion)
	}

	if mask.App {
		labels = append(labels, m.App)
	}

	if mask.PriorityClass {
		labels = append(labels, m.PriorityClass)
	}
	if mask.Scheduled {
		labels = append(labels, m.Scheduled)
	}
	if mask.Level {
		labels = append(labels, m.Level.String())
	}

	if mask.QuotaName {
		labels = append(labels, m.QuotaName)
	}

	if mask.AllocationStrategy {
		labels = append(labels, m.AllocationStrategy)
	}

	// Volume labels
	if mask.PVCName {
		labels = append(labels, m.PVCName)
	}
	if mask.PVName {
		labels = append(labels, m.PVName)
	}
	if mask.StorageClass {
		labels = append(labels, m.StorageClass)
	}
	if mask.CGroup {
		labels = append(labels, m.CGroup)
	}
	if mask.Phase {
		labels = append(labels, m.Phase)
	}
	return labels
}

// getOverrideBuiltInLabelValues returns the label values with the mask and overrides applied.
// nolint: gocyclo
func getOverrideBuiltInLabelValues(m builtInLabelsValues, overrides map[string]string) builtInLabelsValues {
	f := func(labelName, labelValue string) string {
		if val, ok := overrides[labelName]; ok {
			return val
		}
		return labelValue
	}

	s := m
	s.ContainerName = f(collectorcontrollerv1alpha1.ExportedContainerLabel, m.ContainerName)
	s.ContainerImage = f(collectorcontrollerv1alpha1.ContainerImageLabel, m.ContainerImage)
	s.ContainerImageID = f(collectorcontrollerv1alpha1.ContainerImageIDLabel, m.ContainerImageID)
	s.PodName = f(collectorcontrollerv1alpha1.ExportedPodLabel, m.PodName)
	s.NamespaceName = f(collectorcontrollerv1alpha1.ExportedNamespaceLabel, m.NamespaceName)
	s.NodeName = f(collectorcontrollerv1alpha1.ExportedNodeLabel, m.NodeName)
	s.NodeUnschedulable = f(collectorcontrollerv1alpha1.NodeUnschedulableLabel, m.NodeUnschedulable)
	s.WorkloadName = f(collectorcontrollerv1alpha1.WorkloadNameLabel, m.WorkloadName)
	s.WorkloadKind = f(collectorcontrollerv1alpha1.WorkloadKindLabel, m.WorkloadKind)
	s.WorkloadAPIGroup = f(collectorcontrollerv1alpha1.WorkloadAPIGroupLabel, m.WorkloadAPIGroup)
	s.WorkloadAPIGroup = f(collectorcontrollerv1alpha1.WorkloadAPIGroupLabel, m.WorkloadAPIGroup)
	s.WorkloadAPIVersion = f(collectorcontrollerv1alpha1.WorkloadAPIVersionLabel, m.WorkloadAPIVersion)
	s.App = f(collectorcontrollerv1alpha1.AppLabel, m.App)
	s.PriorityClass = f(collectorcontrollerv1alpha1.PriorityClassLabel, m.PriorityClass)
	s.Scheduled = f(collectorcontrollerv1alpha1.ScheduledLabel, m.Scheduled)
	s.Level = collectorcontrollerv1alpha1.AggregationLevel(f(collectorcontrollerv1alpha1.LevelLabel, m.Level.String()))
	s.QuotaName = f(collectorcontrollerv1alpha1.QuotaLabel, m.QuotaName)
	s.AllocationStrategy = f(collectorcontrollerv1alpha1.AllocationStrategyLabel, m.AllocationStrategy)
	s.PVCName = f(collectorcontrollerv1alpha1.PVCNameLabel, m.PVCName)
	s.PVName = f(collectorcontrollerv1alpha1.PVNameLabel, m.PVName)
	s.StorageClass = f(collectorcontrollerv1alpha1.StorageClassLabel, m.StorageClass)
	s.CGroup = f(collectorcontrollerv1alpha1.CGroupLabel, m.CGroup)
	s.Phase = f(collectorcontrollerv1alpha1.PhaseLabel, m.Phase)

	return s
}

type builtInLabler struct {
	UseQuotaNameForPriorityClass bool
}

func (l builtInLabler) SetLabelsForContainer(labels *builtInLabelsValues, c *corev1.Container) {
	if c == nil {
		return
	}
	labels.ContainerName = c.Name
	labels.ContainerImage = c.Image
}

func (l builtInLabler) SetLabelsForPod(
	labels *builtInLabelsValues, p *corev1.Pod, w workload,
	node *corev1.Node, namespace *corev1.Namespace) {
	if p == nil {
		labels.PodName = Undefined
		return
	}

	labels.PodName = p.Name
	labels.NamespaceName = p.Namespace
	labels.NodeName = p.Spec.NodeName
	if node != nil {
		labels.NodeUnschedulable = fmt.Sprintf("%v", node.Spec.Unschedulable)
	}

	labels.WorkloadAPIGroup = w.APIGroup
	labels.WorkloadAPIVersion = w.APIVersion
	labels.WorkloadKind = w.Kind
	labels.WorkloadName = w.Name
	labels.App = p.Labels["app"]

	labels.PriorityClass = p.Spec.PriorityClassName
	labels.Scheduled = fmt.Sprintf("%v", p.Spec.NodeName != "")
}

func (l builtInLabler) SetLabelsForNode(
	labels *builtInLabelsValues, node *corev1.Node) {
	if node == nil {
		labels.NodeName = Undefined
		return
	}
	labels.NodeName = node.Name
	labels.NodeUnschedulable = fmt.Sprintf("%v", node.Spec.Unschedulable)
}

func (l builtInLabler) SetLabelsForCGroup(
	labels *builtInLabelsValues, usage *api.NodeAggregatedMetrics) {
	labels.CGroup = filepath.Base(usage.GetAggregationLevel())
}

func (l builtInLabler) SetLabelsForQuota(
	labels *builtInLabelsValues, quota *corev1.ResourceQuota, rqd *quotamanagementv1alpha1.ResourceQuotaDescriptor, namespace *corev1.Namespace) {
	if quota == nil {
		labels.QuotaName = Undefined
		return
	}

	if rqd == nil {
		labels.AllocationStrategy = Undefined
	} else {
		labels.AllocationStrategy = string(rqd.Spec.AllocationStrategy.AllocationStrategyType)
	}

	labels.NamespaceName = quota.Namespace
	labels.QuotaName = quota.Name

	if l.UseQuotaNameForPriorityClass {
		labels.PriorityClass = quota.Name
		return
	}

	if quota.Spec.ScopeSelector != nil {
		// set from scope selector -- note this is error prone since there
		// can be multiple MatchExpressions with different PriorityClasses
		for _, s := range quota.Spec.ScopeSelector.MatchExpressions {
			// check if this scope specifies exactly 1 priority classes
			if s.ScopeName == corev1.ResourceQuotaScopePriorityClass &&
				len(s.Values) == 1 &&
				s.Operator == corev1.ScopeSelectorOpIn {
				// conflicting priority classes -- mark as undefined and break out of loop
				if labels.PriorityClass != s.Values[0] && labels.PriorityClass != "" {
					labels.PriorityClass = "undefined"
					return
				}
				// set the priority class
				labels.PriorityClass = s.Values[0]
			}
		}
	}
}

func (l builtInLabler) SetLabelsForNamespace(labels *builtInLabelsValues, namespace *corev1.Namespace) {
	if namespace == nil {
		labels.NamespaceName = Undefined
		return
	}
	labels.NamespaceName = namespace.Name
}

func (l builtInLabler) SetLabelsForPVCQuota(labels *builtInLabelsValues, quota *corev1.ResourceQuota, resource collectorcontrollerv1alpha1.ResourceName) {
	// set pvc storage class label
	for rsName := range quota.Spec.Hard {
		if resource == rsName {
			s := strings.Split(rsName.String(), ".")
			labels.StorageClass = s[0]
		}
	}

}

func (l builtInLabler) SetLabelsForPersistentVolume(labels *builtInLabelsValues,
	pv *corev1.PersistentVolume, pvc *corev1.PersistentVolumeClaim, node *corev1.Node) {
	if pv == nil {
		labels.PVName = Undefined
		return
	}
	if node == nil {
		// if we couldn't find the node, but it has a hostname, expcitly set it
		labels.NodeName = pv.Annotations["kubernetes.io/hostname"]
	} else {
		l.SetLabelsForNode(labels, node)
	}

	l.setLabelsForPersistentVolumeClaim(labels, pvc)
	l.setLabelsForPersistentVolume(labels, pv)
}

func (l builtInLabler) SetLabelsForPersistentVolumeClaim(labels *builtInLabelsValues,
	pvc *corev1.PersistentVolumeClaim, pv *corev1.PersistentVolume, namespace *corev1.Namespace,
	pod *corev1.Pod, w workload, node *corev1.Node) {
	if pvc == nil {
		labels.PVCName = Undefined
		return
	}
	l.SetLabelsForPod(labels, pod, w, node, namespace)
	l.setLabelsForPersistentVolume(labels, pv)
	l.setLabelsForPersistentVolumeClaim(labels, pvc)
}

// setLabelsForPersistentVolume sets the labels read from the PV
func (l builtInLabler) setLabelsForPersistentVolume(labels *builtInLabelsValues, pv *corev1.PersistentVolume) {
	if pv == nil {
		labels.PVName = Undefined
		return
	}

	labels.PVName = pv.Name
	labels.StorageClass = pv.Spec.StorageClassName
	labels.Phase = string(pv.Status.Phase)
}

// setLabelsForPersistentVolumeClaim sets the labels read from the PVC
func (l builtInLabler) setLabelsForPersistentVolumeClaim(labels *builtInLabelsValues, pvc *corev1.PersistentVolumeClaim) {
	if pvc == nil {
		labels.PVCName = Undefined
		return
	}

	labels.PVCName = pvc.Name
	labels.NamespaceName = pvc.Namespace
	labels.Phase = string(pvc.Status.Phase)
}

func (l builtInLabler) SetLabelsForClusterScoped(labels *builtInLabelsValues, discoveredLabels map[string]string) {
	labels.PriorityClass = discoveredLabels["priority_class"]
}
