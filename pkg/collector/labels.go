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

// Package labels is responsible for parsing metric labels from Kubernetes objects
package collector

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/quotamanagementv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
)

// labler parses labels from Kubernetes objects and sets them on Values
type labler struct {
	// BuiltIn contains compiled in metric labels
	BuiltIn builtInLabler
	// Extension contains extension metric labels
	Extension extensionLabler
}

// SetLabelsForContainer parses metric labels from a container and pod
func (l labler) SetLabelsForContainer(
	labels *labelsValues, container *corev1.Container) {
	l.BuiltIn.SetLabelsForContainer(&labels.BuiltIn, container)
	// no extension labels for containers
}

func (l labler) SetLabelsForPod(
	labels *labelsValues, pod *corev1.Pod, w workload,
	node *corev1.Node, namespace *corev1.Namespace) {
	l.BuiltIn.SetLabelsForPod(&labels.BuiltIn, pod, w, node, namespace)
	l.Extension.SetLabelsForPod(&labels.Extension, pod, w, node, namespace)
}

// SetLabelsForNode parses metric labels from a node
func (l labler) SetLabelsForNode(labels *labelsValues, node *corev1.Node) {
	l.BuiltIn.SetLabelsForNode(&labels.BuiltIn, node)
	l.Extension.SetLabelsForNode(&labels.Extension, node)
}

func (l labler) SetLabelsFoCGroup(labels *labelsValues, u *api.NodeAggregatedMetrics) {
	l.BuiltIn.SetLabelsForCGroup(&labels.BuiltIn, u)
}

// SetLabelsForQuota parses metric labels from a namespace
func (l labler) SetLabelsForQuota(labels *labelsValues,
	quota *corev1.ResourceQuota, rqd *quotamanagementv1alpha1.ResourceQuotaDescriptor, namespace *corev1.Namespace) {
	l.BuiltIn.SetLabelsForQuota(&labels.BuiltIn, quota, rqd, namespace)
	l.Extension.SetLabelsForQuota(&labels.Extension, quota, rqd, namespace)
}

// SetLabelsForNamespace parses metric labels from a namespace
func (l labler) SetLabelsForNamespaces(labels *labelsValues, namespace *corev1.Namespace) {
	l.BuiltIn.SetLabelsForNamespace(&labels.BuiltIn, namespace)
	l.Extension.SetLabelsForNamespace(&labels.Extension, namespace)
}

// SetLabelsForPVCQuota set storage class label from resource and quota spec
func (l labler) SetLabelsForPVCQuota(labels *labelsValues,
	quota *corev1.ResourceQuota, resource string) {
	l.BuiltIn.SetLabelsForPVCQuota(&labels.BuiltIn, quota, resource)
}

func (l labler) SetLabelsForPersistentVolume(labels *labelsValues,
	pv *corev1.PersistentVolume, pvc *corev1.PersistentVolumeClaim, node *corev1.Node) {
	l.BuiltIn.SetLabelsForPersistentVolume(&labels.BuiltIn, pv, pvc, node)
	l.Extension.SetLabelsForPersistentVolume(&labels.Extension, pv, pvc, node)
}

func (l labler) SetLabelsForPersistentVolumeClaim(labels *labelsValues,
	pvc *corev1.PersistentVolumeClaim,
	pv *corev1.PersistentVolume, namespace *corev1.Namespace,
	pod *corev1.Pod, w workload, node *corev1.Node) {
	l.BuiltIn.SetLabelsForPersistentVolumeClaim(&labels.BuiltIn, pvc, pv, namespace, pod, w, node)
	l.Extension.SetLabelsForPersistentVolumeClaim(&labels.Extension, pvc, pv, namespace, pod, w, node)
}

func (l labler) SetLabelsForClusterScoped(labels *labelsValues, discoveredLabels map[string]string) {
	l.BuiltIn.SetLabelsForClusterScoped(&labels.BuiltIn, discoveredLabels)
	// TODO: extension labels
}

// labelsValues contains metric label values
type labelsValues struct {
	BuiltIn builtInLabelsValues

	Extension extensionLabelsValues
}

// initInternalLabelsMask creates the internal ExtensionLabelsMask corresponding to the external LabelsMask
// Create the new ExtensionLabelsMask and set its generic fields to the corresponding named values
func (c *Collector) initInternalLabelsMask(mask *collectorcontrollerv1alpha1.LabelsMask, e collectorcontrollerv1alpha1.Extensions) {
	// create the new internal extension and add it to the collectors map
	if c.extensionLabelMaskById == nil {
		c.extensionLabelMaskById = make(map[collectorcontrollerv1alpha1.LabelsMaskId]*extensionLabelsMask)
	}
	id := c.nextId
	c.nextId++
	mask.ID = collectorcontrollerv1alpha1.LabelsMaskId(id)
	extension := &extensionLabelsMask{}

	extension.keys.Label00 = c.labelNamesByIds[0]
	if mask.Extensions[c.labelNamesByIds[0]] {
		extension.Label00 = true
	}
	extension.keys.Label01 = c.labelNamesByIds[1]
	if mask.Extensions[c.labelNamesByIds[1]] {
		extension.Label01 = true
	}
	extension.keys.Label02 = c.labelNamesByIds[2]
	if mask.Extensions[c.labelNamesByIds[2]] {
		extension.Label02 = true
	}
	extension.keys.Label03 = c.labelNamesByIds[3]
	if mask.Extensions[c.labelNamesByIds[3]] {
		extension.Label03 = true
	}
	extension.keys.Label04 = c.labelNamesByIds[4]
	if mask.Extensions[c.labelNamesByIds[4]] {
		extension.Label04 = true
	}

	extension.keys.Label05 = c.labelNamesByIds[5]
	if mask.Extensions[c.labelNamesByIds[5]] {
		extension.Label05 = true
	}
	extension.keys.Label06 = c.labelNamesByIds[6]
	if mask.Extensions[c.labelNamesByIds[6]] {
		extension.Label06 = true
	}
	extension.keys.Label07 = c.labelNamesByIds[7]
	if mask.Extensions[c.labelNamesByIds[7]] {
		extension.Label07 = true
	}
	extension.keys.Label08 = c.labelNamesByIds[8]
	if mask.Extensions[c.labelNamesByIds[8]] {
		extension.Label08 = true
	}
	extension.keys.Label09 = c.labelNamesByIds[9]
	if mask.Extensions[c.labelNamesByIds[9]] {
		extension.Label09 = true
	}

	extension.keys.Label10 = c.labelNamesByIds[10]
	if mask.Extensions[c.labelNamesByIds[10]] {
		extension.Label10 = true
	}
	extension.keys.Label11 = c.labelNamesByIds[11]
	if mask.Extensions[c.labelNamesByIds[11]] {
		extension.Label11 = true
	}
	extension.keys.Label12 = c.labelNamesByIds[12]
	if mask.Extensions[c.labelNamesByIds[12]] {
		extension.Label12 = true
	}
	extension.keys.Label13 = c.labelNamesByIds[13]
	if mask.Extensions[c.labelNamesByIds[13]] {
		extension.Label13 = true
	}
	extension.keys.Label14 = c.labelNamesByIds[14]
	if mask.Extensions[c.labelNamesByIds[14]] {
		extension.Label14 = true
	}

	extension.keys.Label15 = c.labelNamesByIds[15]
	if mask.Extensions[c.labelNamesByIds[15]] {
		extension.Label15 = true
	}
	extension.keys.Label16 = c.labelNamesByIds[16]
	if mask.Extensions[c.labelNamesByIds[16]] {
		extension.Label16 = true
	}
	extension.keys.Label17 = c.labelNamesByIds[17]
	if mask.Extensions[c.labelNamesByIds[17]] {
		extension.Label17 = true
	}
	extension.keys.Label18 = c.labelNamesByIds[18]
	if mask.Extensions[c.labelNamesByIds[18]] {
		extension.Label18 = true
	}
	extension.keys.Label19 = c.labelNamesByIds[19]
	if mask.Extensions[c.labelNamesByIds[19]] {
		extension.Label19 = true
	}

	c.extensionLabelMaskById[mask.ID] = extension
}

// mask masks labels by clearing them
func (c *Collector) mask(mask collectorcontrollerv1alpha1.LabelsMask, m labelsValues) labelsValues {
	m.BuiltIn = builtInMask(mask.BuiltIn, m.BuiltIn)

	// get the internal version that maps label names to fields
	internalMask := c.extensionLabelMaskById[mask.ID]
	m.Extension = internalMask.Mask(m.Extension)
	return m
}

// LabelOverrides may be appended to in order to explicitly override values
// To introduce a new label and override it, use an extension to create the label
// and then override it by appending to this list.
var LabelOverrides []LabelOverrider

type LabelOverrider interface {
	// OverrideLabels takes as an arg a map of all labels for a sample and returns
	// a map of labels to change the values of.
	OverrideLabels(map[string]string) map[string]string
}

// LabelOverriderFn simplifies writing override types
type LabelOverriderFn func(map[string]string) map[string]string

func (l LabelOverriderFn) OverrideLabels(m map[string]string) map[string]string {
	return l(m)
}

func overrideValues(values, names []string) {
	for _, o := range LabelOverrides {
		l := map[string]string{}
		for i := range names {
			l[names[i]] = values[i]
		}
		l = o.OverrideLabels(l)
		for i := range names {
			if v, ok := l[names[i]]; ok {
				values[i] = v // replace our value with the override value
			}
		}
	}
}

// getLabelValues returns the set of label values for this mask
func (c *Collector) getLabelValues(mask collectorcontrollerv1alpha1.LabelsMask, m labelsValues, names []string) []string {
	if mask.BuiltIn.Level {
		// set the level from the mask level name
		m.BuiltIn.Level = mask.Level
	} else {
		m.BuiltIn.Level = ""
	}
	b := getBuiltInLabelValues(mask.BuiltIn, m.BuiltIn)

	// get the internal version that maps label names to fields
	internalMask := c.extensionLabelMaskById[mask.ID]
	extValues := internalMask.GetLabelValues(m.Extension) // get the values

	values := append(b, extValues...)

	// extension point for overriding values
	overrideValues(values, names)

	return values
}

// getLabelNames returns the set of label names for this mask
func (c *Collector) getLabelNames(mask collectorcontrollerv1alpha1.LabelsMask) []string {
	b := getBuiltInLabelNames(mask.BuiltIn)

	// get the internal version that maps label names to fields
	internalMask := c.extensionLabelMaskById[mask.ID]
	e := internalMask.GetLabelNames()

	return append(b, e...)
}

const (
	Undefined = "undefined"
)
