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

// Labeler parses labels from Kubernetes objects and sets them on Values
type Labeler struct {
	// BuiltIn contains compiled in metric labels
	BuiltIn builtInLabler
	// Extension contains extension metric labels
	Extension extensionLabler
}

// SetLabelsForContainer parses metric labels from a container and pod
func (l Labeler) SetLabelsForContainer(
	labels *LabelsValues, container *corev1.Container) {
	l.BuiltIn.SetLabelsForContainer(&labels.BuiltIn, container)
	// no extension labels for containers
}

func (l Labeler) SetLabelsForPod(
	labels *LabelsValues, pod *corev1.Pod, w workload,
	node *corev1.Node, namespace *corev1.Namespace) {
	l.BuiltIn.SetLabelsForPod(&labels.BuiltIn, pod, w, node, namespace)
	l.Extension.SetLabelsForPod(&labels.Extension, pod, w, node, namespace)
}

// SetLabelsForNode parses metric labels from a node
func (l Labeler) SetLabelsForNode(labels *LabelsValues, node *corev1.Node) {
	l.BuiltIn.SetLabelsForNode(&labels.BuiltIn, node)
	l.Extension.SetLabelsForNode(&labels.Extension, node)
}

func (l Labeler) SetLabelsFoCGroup(labels *LabelsValues, u *api.NodeAggregatedMetrics) {
	l.BuiltIn.SetLabelsForCGroup(&labels.BuiltIn, u)
}

// SetLabelsForQuota parses metric labels from a namespace
func (l Labeler) SetLabelsForQuota(labels *LabelsValues,
	quota *corev1.ResourceQuota, rqd *quotamanagementv1alpha1.ResourceQuotaDescriptor, namespace *corev1.Namespace) {
	l.BuiltIn.SetLabelsForQuota(&labels.BuiltIn, quota, rqd, namespace)
	l.Extension.SetLabelsForQuota(&labels.Extension, quota, rqd, namespace)
}

// SetLabelsForNamespace parses metric labels from a namespace
func (l Labeler) SetLabelsForNamespaces(labels *LabelsValues, namespace *corev1.Namespace) {
	l.BuiltIn.SetLabelsForNamespace(&labels.BuiltIn, namespace)
	l.Extension.SetLabelsForNamespace(&labels.Extension, namespace)
}

// SetLabelsForPVCQuota set storage class label from resource and quota spec
func (l Labeler) SetLabelsForPVCQuota(labels *LabelsValues,
	quota *corev1.ResourceQuota, resource string) {
	l.BuiltIn.SetLabelsForPVCQuota(&labels.BuiltIn, quota, resource)
}

func (l Labeler) SetLabelsForPersistentVolume(labels *LabelsValues,
	pv *corev1.PersistentVolume, pvc *corev1.PersistentVolumeClaim, node *corev1.Node) {
	l.BuiltIn.SetLabelsForPersistentVolume(&labels.BuiltIn, pv, pvc, node)
	l.Extension.SetLabelsForPersistentVolume(&labels.Extension, pv, pvc, node)
}

func (l Labeler) SetLabelsForPersistentVolumeClaim(labels *LabelsValues,
	pvc *corev1.PersistentVolumeClaim,
	pv *corev1.PersistentVolume, namespace *corev1.Namespace,
	pod *corev1.Pod, w workload, node *corev1.Node) {
	l.BuiltIn.SetLabelsForPersistentVolumeClaim(&labels.BuiltIn, pvc, pv, namespace, pod, w, node)
	l.Extension.SetLabelsForPersistentVolumeClaim(&labels.Extension, pvc, pv, namespace, pod, w, node)
}

func (l Labeler) SetLabelsForClusterScoped(labels *LabelsValues, discoveredLabels map[string]string) {
	l.BuiltIn.SetLabelsForClusterScoped(&labels.BuiltIn, discoveredLabels)
	// TODO: extension labels
}

// LabelsValues contains metric label values
type LabelsValues struct {
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

	extension.keys.Label20 = c.labelNamesByIds[20]
	if mask.Extensions[c.labelNamesByIds[20]] {
		extension.Label20 = true
	}
	extension.keys.Label21 = c.labelNamesByIds[21]
	if mask.Extensions[c.labelNamesByIds[21]] {
		extension.Label21 = true
	}
	extension.keys.Label22 = c.labelNamesByIds[22]
	if mask.Extensions[c.labelNamesByIds[22]] {
		extension.Label22 = true
	}
	extension.keys.Label23 = c.labelNamesByIds[23]
	if mask.Extensions[c.labelNamesByIds[23]] {
		extension.Label23 = true
	}
	extension.keys.Label24 = c.labelNamesByIds[24]
	if mask.Extensions[c.labelNamesByIds[24]] {
		extension.Label24 = true
	}
	extension.keys.Label25 = c.labelNamesByIds[25]
	if mask.Extensions[c.labelNamesByIds[25]] {
		extension.Label25 = true
	}
	extension.keys.Label26 = c.labelNamesByIds[26]
	if mask.Extensions[c.labelNamesByIds[26]] {
		extension.Label26 = true
	}
	extension.keys.Label27 = c.labelNamesByIds[27]
	if mask.Extensions[c.labelNamesByIds[27]] {
		extension.Label27 = true
	}
	extension.keys.Label28 = c.labelNamesByIds[28]
	if mask.Extensions[c.labelNamesByIds[28]] {
		extension.Label28 = true
	}
	extension.keys.Label29 = c.labelNamesByIds[29]
	if mask.Extensions[c.labelNamesByIds[29]] {
		extension.Label29 = true
	}
	extension.keys.Label30 = c.labelNamesByIds[30]
	if mask.Extensions[c.labelNamesByIds[30]] {
		extension.Label30 = true
	}
	extension.keys.Label31 = c.labelNamesByIds[31]
	if mask.Extensions[c.labelNamesByIds[31]] {
		extension.Label31 = true
	}
	extension.keys.Label32 = c.labelNamesByIds[32]
	if mask.Extensions[c.labelNamesByIds[32]] {
		extension.Label32 = true
	}
	extension.keys.Label33 = c.labelNamesByIds[33]
	if mask.Extensions[c.labelNamesByIds[33]] {
		extension.Label33 = true
	}
	extension.keys.Label34 = c.labelNamesByIds[34]
	if mask.Extensions[c.labelNamesByIds[34]] {
		extension.Label34 = true
	}
	extension.keys.Label35 = c.labelNamesByIds[35]
	if mask.Extensions[c.labelNamesByIds[35]] {
		extension.Label35 = true
	}
	extension.keys.Label36 = c.labelNamesByIds[36]
	if mask.Extensions[c.labelNamesByIds[36]] {
		extension.Label36 = true
	}
	extension.keys.Label37 = c.labelNamesByIds[37]
	if mask.Extensions[c.labelNamesByIds[37]] {
		extension.Label37 = true
	}
	extension.keys.Label38 = c.labelNamesByIds[38]
	if mask.Extensions[c.labelNamesByIds[38]] {
		extension.Label38 = true
	}
	extension.keys.Label39 = c.labelNamesByIds[39]
	if mask.Extensions[c.labelNamesByIds[39]] {
		extension.Label39 = true
	}
	extension.keys.Label40 = c.labelNamesByIds[40]
	if mask.Extensions[c.labelNamesByIds[40]] {
		extension.Label40 = true
	}
	extension.keys.Label41 = c.labelNamesByIds[41]
	if mask.Extensions[c.labelNamesByIds[41]] {
		extension.Label41 = true
	}
	extension.keys.Label42 = c.labelNamesByIds[42]
	if mask.Extensions[c.labelNamesByIds[42]] {
		extension.Label42 = true
	}
	extension.keys.Label43 = c.labelNamesByIds[43]
	if mask.Extensions[c.labelNamesByIds[43]] {
		extension.Label43 = true
	}
	extension.keys.Label44 = c.labelNamesByIds[44]
	if mask.Extensions[c.labelNamesByIds[44]] {
		extension.Label44 = true
	}
	extension.keys.Label45 = c.labelNamesByIds[45]
	if mask.Extensions[c.labelNamesByIds[45]] {
		extension.Label45 = true
	}
	extension.keys.Label46 = c.labelNamesByIds[46]
	if mask.Extensions[c.labelNamesByIds[46]] {
		extension.Label46 = true
	}
	extension.keys.Label47 = c.labelNamesByIds[47]
	if mask.Extensions[c.labelNamesByIds[47]] {
		extension.Label47 = true
	}
	extension.keys.Label48 = c.labelNamesByIds[48]
	if mask.Extensions[c.labelNamesByIds[48]] {
		extension.Label48 = true
	}
	extension.keys.Label49 = c.labelNamesByIds[49]
	if mask.Extensions[c.labelNamesByIds[49]] {
		extension.Label49 = true
	}
	extension.keys.Label50 = c.labelNamesByIds[50]
	if mask.Extensions[c.labelNamesByIds[50]] {
		extension.Label50 = true
	}
	extension.keys.Label51 = c.labelNamesByIds[51]
	if mask.Extensions[c.labelNamesByIds[51]] {
		extension.Label51 = true
	}
	extension.keys.Label52 = c.labelNamesByIds[52]
	if mask.Extensions[c.labelNamesByIds[52]] {
		extension.Label52 = true
	}
	extension.keys.Label53 = c.labelNamesByIds[53]
	if mask.Extensions[c.labelNamesByIds[53]] {
		extension.Label53 = true
	}
	extension.keys.Label54 = c.labelNamesByIds[54]
	if mask.Extensions[c.labelNamesByIds[54]] {
		extension.Label54 = true
	}
	extension.keys.Label55 = c.labelNamesByIds[55]
	if mask.Extensions[c.labelNamesByIds[55]] {
		extension.Label55 = true
	}
	extension.keys.Label56 = c.labelNamesByIds[56]
	if mask.Extensions[c.labelNamesByIds[56]] {
		extension.Label56 = true
	}
	extension.keys.Label57 = c.labelNamesByIds[57]
	if mask.Extensions[c.labelNamesByIds[57]] {
		extension.Label57 = true
	}
	extension.keys.Label58 = c.labelNamesByIds[58]
	if mask.Extensions[c.labelNamesByIds[58]] {
		extension.Label58 = true
	}
	extension.keys.Label59 = c.labelNamesByIds[59]
	if mask.Extensions[c.labelNamesByIds[59]] {
		extension.Label59 = true
	}
	extension.keys.Label60 = c.labelNamesByIds[60]
	if mask.Extensions[c.labelNamesByIds[60]] {
		extension.Label60 = true
	}
	extension.keys.Label61 = c.labelNamesByIds[61]
	if mask.Extensions[c.labelNamesByIds[61]] {
		extension.Label61 = true
	}
	extension.keys.Label62 = c.labelNamesByIds[62]
	if mask.Extensions[c.labelNamesByIds[62]] {
		extension.Label62 = true
	}
	extension.keys.Label63 = c.labelNamesByIds[63]
	if mask.Extensions[c.labelNamesByIds[63]] {
		extension.Label63 = true
	}
	extension.keys.Label64 = c.labelNamesByIds[64]
	if mask.Extensions[c.labelNamesByIds[64]] {
		extension.Label64 = true
	}
	extension.keys.Label65 = c.labelNamesByIds[65]
	if mask.Extensions[c.labelNamesByIds[65]] {
		extension.Label65 = true
	}
	extension.keys.Label66 = c.labelNamesByIds[66]
	if mask.Extensions[c.labelNamesByIds[66]] {
		extension.Label66 = true
	}
	extension.keys.Label67 = c.labelNamesByIds[67]
	if mask.Extensions[c.labelNamesByIds[67]] {
		extension.Label67 = true
	}
	extension.keys.Label68 = c.labelNamesByIds[68]
	if mask.Extensions[c.labelNamesByIds[68]] {
		extension.Label68 = true
	}
	extension.keys.Label69 = c.labelNamesByIds[69]
	if mask.Extensions[c.labelNamesByIds[69]] {
		extension.Label69 = true
	}
	extension.keys.Label70 = c.labelNamesByIds[70]
	if mask.Extensions[c.labelNamesByIds[70]] {
		extension.Label70 = true
	}
	extension.keys.Label71 = c.labelNamesByIds[71]
	if mask.Extensions[c.labelNamesByIds[71]] {
		extension.Label71 = true
	}
	extension.keys.Label72 = c.labelNamesByIds[72]
	if mask.Extensions[c.labelNamesByIds[72]] {
		extension.Label72 = true
	}
	extension.keys.Label73 = c.labelNamesByIds[73]
	if mask.Extensions[c.labelNamesByIds[73]] {
		extension.Label73 = true
	}
	extension.keys.Label74 = c.labelNamesByIds[74]
	if mask.Extensions[c.labelNamesByIds[74]] {
		extension.Label74 = true
	}
	extension.keys.Label75 = c.labelNamesByIds[75]
	if mask.Extensions[c.labelNamesByIds[75]] {
		extension.Label75 = true
	}
	extension.keys.Label76 = c.labelNamesByIds[76]
	if mask.Extensions[c.labelNamesByIds[76]] {
		extension.Label76 = true
	}
	extension.keys.Label77 = c.labelNamesByIds[77]
	if mask.Extensions[c.labelNamesByIds[77]] {
		extension.Label77 = true
	}
	extension.keys.Label78 = c.labelNamesByIds[78]
	if mask.Extensions[c.labelNamesByIds[78]] {
		extension.Label78 = true
	}
	extension.keys.Label79 = c.labelNamesByIds[79]
	if mask.Extensions[c.labelNamesByIds[79]] {
		extension.Label79 = true
	}
	extension.keys.Label80 = c.labelNamesByIds[80]
	if mask.Extensions[c.labelNamesByIds[80]] {
		extension.Label80 = true
	}
	extension.keys.Label81 = c.labelNamesByIds[81]
	if mask.Extensions[c.labelNamesByIds[81]] {
		extension.Label81 = true
	}
	extension.keys.Label82 = c.labelNamesByIds[82]
	if mask.Extensions[c.labelNamesByIds[82]] {
		extension.Label82 = true
	}
	extension.keys.Label83 = c.labelNamesByIds[83]
	if mask.Extensions[c.labelNamesByIds[83]] {
		extension.Label83 = true
	}
	extension.keys.Label84 = c.labelNamesByIds[84]
	if mask.Extensions[c.labelNamesByIds[84]] {
		extension.Label84 = true
	}
	extension.keys.Label85 = c.labelNamesByIds[85]
	if mask.Extensions[c.labelNamesByIds[85]] {
		extension.Label85 = true
	}
	extension.keys.Label86 = c.labelNamesByIds[86]
	if mask.Extensions[c.labelNamesByIds[86]] {
		extension.Label86 = true
	}
	extension.keys.Label87 = c.labelNamesByIds[87]
	if mask.Extensions[c.labelNamesByIds[87]] {
		extension.Label87 = true
	}
	extension.keys.Label88 = c.labelNamesByIds[88]
	if mask.Extensions[c.labelNamesByIds[88]] {
		extension.Label88 = true
	}
	extension.keys.Label89 = c.labelNamesByIds[89]
	if mask.Extensions[c.labelNamesByIds[89]] {
		extension.Label89 = true
	}
	extension.keys.Label90 = c.labelNamesByIds[90]
	if mask.Extensions[c.labelNamesByIds[90]] {
		extension.Label90 = true
	}
	extension.keys.Label91 = c.labelNamesByIds[91]
	if mask.Extensions[c.labelNamesByIds[91]] {
		extension.Label91 = true
	}
	extension.keys.Label92 = c.labelNamesByIds[92]
	if mask.Extensions[c.labelNamesByIds[92]] {
		extension.Label92 = true
	}
	extension.keys.Label93 = c.labelNamesByIds[93]
	if mask.Extensions[c.labelNamesByIds[93]] {
		extension.Label93 = true
	}
	extension.keys.Label94 = c.labelNamesByIds[94]
	if mask.Extensions[c.labelNamesByIds[94]] {
		extension.Label94 = true
	}
	extension.keys.Label95 = c.labelNamesByIds[95]
	if mask.Extensions[c.labelNamesByIds[95]] {
		extension.Label95 = true
	}
	extension.keys.Label96 = c.labelNamesByIds[96]
	if mask.Extensions[c.labelNamesByIds[96]] {
		extension.Label96 = true
	}
	extension.keys.Label97 = c.labelNamesByIds[97]
	if mask.Extensions[c.labelNamesByIds[97]] {
		extension.Label97 = true
	}
	extension.keys.Label98 = c.labelNamesByIds[98]
	if mask.Extensions[c.labelNamesByIds[98]] {
		extension.Label98 = true
	}
	extension.keys.Label99 = c.labelNamesByIds[99]
	if mask.Extensions[c.labelNamesByIds[99]] {
		extension.Label99 = true
	}

	c.extensionLabelMaskById[mask.ID] = extension
}

// mask masks labels by clearing them
func (c *Collector) mask(mask collectorcontrollerv1alpha1.LabelsMask, m LabelsValues) LabelsValues {
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
func (c *Collector) getLabelValues(mask collectorcontrollerv1alpha1.LabelsMask, m LabelsValues, names []string) []string {
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
