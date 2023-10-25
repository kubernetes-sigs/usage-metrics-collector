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

// +kubebuilder:object:generate=true
package quotamanagementv1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// MaxObservedQuotaAnnotationKey is the key for an alpha field that stores
	// the max observed quotas associated with a ResourceQuotaDescriptor. This annotation's value will be a serialized ResourceList.
	MaxObservedQuotaAnnotationKey = "usagemetricscollector.sigs.k8s.io/maxObservedQuota"
)

// +kubebuilder:object:root=true
// +genclient
// +kubebuilder:resource:scope=Namespace
// +kubebuilder:resource:path=resourcequotadescriptors
// +kubebuilder:resource:shortName=rqd
// +kubebuilder:subresource:status
// +kubebuilder:rbac:groups=quotamanagement.usagemetricscollector.sigs.k8s.io,resources=resourcequotadescriptor,verbs=get;list;update;patch
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".metadata.name",description="RQD name"
// +kubebuilder:printcolumn:name="PolicyState",type="string",JSONPath=".status.policyState",description="The current state of the resource quota with respect to the usage policy."
// +kubebuilder:printcolumn:name="RightsizingEnabled",type="boolean",JSONPath=".status.quotaRightsizingEnabled",description="Indicates whether rightsizing is enabled for the quota"
// +kubebuilder:printcolumn:name="ScheduledRightsizeDate",type="string",format="date-time",JSONPath=".status.scheduledRightsizeDate",description="Date that a rightsize event will occur"
// +kubebuilder:printcolumn:name="LastRightsizeTime",type="string",format="date-time",JSONPath=".status.lastRightsizeTime",description="Timestamp of last rightsize"
// +kubebuilder:printcolumn:name="LastRevertTime",type="string",format="date-time",JSONPath=".status.lastRevertTime",description="Timestamp of last revert"
// +kubebuilder:printcolumn:name="DashboardLink",type="string",JSONPath=".status.dashboardLink",description="Link to capacity dashboard"
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceQuotaDescriptor is used to describe what quota given out to a namespace
// is intended to be used for. This is created per priority class per namespace.
type ResourceQuotaDescriptor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec ResourceQuotaDescriptorSpec `json:"spec"`

	Status ResourceQuotaDescriptorStatus `json:"status,omitempty"`
}

// ResourceQuotaDescriptorStatus describes the current state of the
// ResourceQuotaDescriptor.
type ResourceQuotaDescriptorStatus struct {
	// Conditions is a list of conditions relating to the ResourceQuotaDescriptor.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=10
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservationWindowStart is the start time of the observation window used to populate
	// the status values.
	// +optional
	ObservationWindowStart *metav1.Time `json:"observationWindowStart,omitempty"`
	// ObservationWindowEnd is the end time of the observation window used to populate
	// the status values.
	// +optional
	ObservationWindowEnd *metav1.Time `json:"observationWindowEnd,omitempty"`
	// ObservationWindowLength is the duration of the obseration window used to populate
	// the status values.
	// +optional
	ObservationWindowLength *metav1.Duration `json:"observationWindowLength,omitempty"`

	// QuotaRightsizingEnabled indicates whether the indicated ResourceQuota
	// will be subject to right-sizing when it is OutOfPolicy as indicated by
	// the status.policyState field.
	// +optional
	QuotaRightsizingEnabled *bool `json:"quotaRightsizingEnabled,omitempty"`

	// PolicyState defines the state of the resource quota with respect to the usage policy.
	// +optional
	PolicyState PolicyState `json:"policyState,omitempty"`

	// GracePeriodLength is the length of time a Quota may be OutOfPolicy before
	// it is rightsized according to the policy.
	// +optional
	GracePeriodLength *metav1.Duration `json:"gracePeriodLength,omitempty"`

	// ScheduledRightsizeDate is the time that the quota will be rightsized to the proposed
	// value.
	// Not set if no rightsize is scheduled.
	// +optional
	ScheduledRightsizeDate *metav1.Time `json:"scheduledRightsizeDate,omitempty"`

	// ActualQuota contains the most recent quota values observed during the window.
	// +optional
	ActualQuota corev1.ResourceList `json:"actualQuota,omitempty"`

	// ProposedQuota is the quota values proposed for rightsizing.  If
	// rightsizing is enabled the quota values will be changed to the proposed
	// values, after the status.gracePeriodLength has elapsed, on the
	// status.scheduledRightsizeDate. ProposedQuota is driven off the observed
	// quota used and the target policy defined in the spec.
	// +optional
	ProposedQuota corev1.ResourceList `json:"proposedQuota,omitempty"`

	// PreRightsizeQuota is the *max* observed quota values prior to any rightsizing event.
	// It may be used to authorize increasing quota values back to their
	// pre-rightsized values.
	// +optional
	PreRightsizeQuota corev1.ResourceList `json:"preRightsizeQuota,omitempty"`

	// MaxUsedQuota is the max quota used during the observation window.
	// +optional
	MaxUsedQuota corev1.ResourceList `json:"maxUsedQuota,omitempty"`

	// P95UsedQuota is the p95 quota used during the observation window
	// +optional
	P95UsedQuota corev1.ResourceList `json:"p95UsedQuota,omitempty"`

	// AvgUsedQuota is the average quota used during the observation window
	// +optional
	AvgUsedQuota corev1.ResourceList `json:"avgUsedQuota,omitempty"`

	// DashboardLink is an HTTPS link to a dashboard where users can see
	// detailed information about their quota use.
	// +kubebuilder:validation:MaxLength=512
	// +optional
	DashboardLink string `json:"dashboardLink,omitempty"`

	// LastRightsizeTime is the last time the quota's values were set to the
	// proposed quota value. Not set if no rightsize has been performed.
	// +optional
	LastRightsizeTime *metav1.Time `json:"lastRightsizeTime,omitempty"`

	// RevertToPreRightsizeQuota indicates that the quota should be reverted to
	// the values in the status.preRightSizeQuota field. This field will be
	// cleared after the old values are restored.
	// +optional
	RevertToPreRightsizeQuota bool `json:"revertToPreRightsizeQuota,omitempty"`

	// LastRevertTime is the last time the quota's values were reverted to the
	// pre-rightsize values. Not set if no revert has been performed.
	// +optional
	LastRevertTime *metav1.Time `json:"lastRevertTime,omitempty"`

	// PeriodicRightsize if set to true will periodically attempt to righsize quota that has
	// is out of policy and whose scheduledRightsizeDate is in the past.  Does not require
	// explicitly scheduling a rightsize event.
	PeriodicRightsize bool `json:"periodicRightsize,omitempty"`

	// ObservationWindowDays is used to determine how many days of usage data to look at when identifying
	// the high water mark.  e.g. a value of "30" will look at the last 30 days of usage data to find the
	// highest usage point.  Organizations can use this to set more aggressive rightsizing.
	// +kubebuilder:validation:Enum=30;60;90
	// +kubebuilder:default=90
	ObservationWindowDays int `json:"observationWindowDays,omitempty"`
}

const (
	// ResourceQuotaDescriptorOutOfPolicy is set to true if the namespace ResourceQuota usage does not
	// adhere to the policy stated in the ResourceQuotaDescriptor.
	ResourceQuotaDescriptorOutOfPolicy string = "OutOfPolicy"

	// ResourceQuotaDescriptorRightsizeScheduled is set to true if the Quota is OutOfPolicy and the
	// a rightsize is scheduled.
	ResourceQuotaDescriptorRightsizeScheduled string = "RightsizeScheduled"

	// ResourceQuotaDescriptorRequiresRightsize is set to true if the Quota had its values changed
	// as part of a rightsize event.
	ResourceQuotaDescriptorRightsized string = "Rightsized"
)

// ResourceQuotaDescriptorSpec describes how a portion of ResourceQuota is used.
type ResourceQuotaDescriptorSpec struct {
	// ResourceQuotaRef is an object reference associating this
	// ResourceQuotaDescriptor with a ResourceQuota.
	// +required
	ResourceQuotaRef corev1.LocalObjectReference `json:"resourceQuotaRef,omitempty"`

	// Issue is a URL reference to more context (ex: github issue)
	// +optional
	Issue string `json:"issue,omitempty"`

	// AllocationStrategy describes how the resources are to be used. Possible strategies are
	// Delayed, DisasterRecovery, Periodic, Constant
	// +required
	AllocationStrategy AllocationStrategy `json:"allocationStrategy"`

	// TargetAllocations defines the policy applied to quota usage.
	// +optional
	TargetAllocationsPolicy *TargetAllocationsPolicy `json:"targetAllocationsPolicy,omitempty"`
}

// AllocationPercent is the target percentage for max quota allocated over a window.
// ex: 90 for a policy requiring that at least 90% of the available quota was allocated over
// an observation window.
// +kubebuilder:validation:Maximum=100
// +kubebuilder:validation:Minimum=0
type AllocationPercent int

// TargetAllocationsPolicy defines the policy applied to quota usage.
type TargetAllocationsPolicy struct {
	// LimitsTargetPercent describes the maximum amount of compute resources allowed.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	LimitsTargetPercent map[corev1.ResourceName]AllocationPercent `json:"limitsTargetPercent,omitempty"`
	// RequestsTargetPercent describes the minimum amount of compute resources required.
	// If RequestsTargetPercent is omitted for a container, it defaults to Limits if that is explicitly specified,
	// otherwise to an implementation-defined value.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	RequestsTargetPercent map[corev1.ResourceName]AllocationPercent `json:"requestsTargetPercent,omitempty"`
}

// AllocationStrategy describes how a set of resource quota is to be used.
// Exactly one item is expected to be specified.
type AllocationStrategy struct {
	// AllocationStrategyType is the union discriminator for which strategy is used.
	// +kubebuilder:default=Unspecified
	// +required
	AllocationStrategyType AllocationStrategyType `json:"allocationStrategyType"`

	// Delayed indicates that this capacity is intended to be consumed at a later date.
	// +optional
	Delayed *AllocationDelayedStrategy `json:"delayed,omitempty"`

	// DisasterRecovery indicates that this capacity is used for disaster recovery planning,
	// and may not be allocated (or may consume significantly less CPU if it is allocated)
	// until needed, either during a disaster recovery event or a test of disaster recovery
	// resiliency.
	// +optional
	DisasterRecovery *AllocationDisasterRecoveryStrategy `json:"disasterRecovery,omitempty"`

	// Periodic indicates interval at which you want to allocate resources
	// +optional
	Periodic *AllocationPeriodicStrategy `json:"periodic,omitempty"`

	// Constant indicates that resources are in constant or near constant use
	// eg. allocated for a significant portion of the day+
	// +optional
	Constant *AllocationConstantStrategy `json:"constant,omitempty"`

	// Reserved indicates that quota has been set aside for the user,
	// so it can't be reassigned. Subsequently, this quota will be scaled down
	// and assigned to namespaces used by the user.
	// +optional
	Reserved *AllocationReservedStrategy `json:"reserved,omitempty"`

	// Decommissioned indicates that the quota is no longer in use or
	// needed by the user and should be clawed back and deleted.
	// +optional
	Decommissioned *AllocationDecommissionedStrategy `json:"decommissioned,omitempty"`

	// Unspecified indicates that the namespace owner did not set their RQD.
	// +optional
	Unspecified *AllocationUnspecifiedStrategy `json:"unspecified,omitempty"`
}

// AllocationStrategyType is the union discriminator for which strategy is used.
// +kubebuilder:validation:Enum=OutOfPolicy;InPolicy
type PolicyState string

const (
	OutOfPolicy PolicyState = "OutOfPolicy"
	InPolicy    PolicyState = "InPolicy"
)

// AllocationStrategyType is the union discriminator for which strategy is used.
// +kubebuilder:validation:Enum=Delayed;DisasterRecovery;Periodic;Constant;Reserved;Unspecified;Decommissioned
type AllocationStrategyType string

const (
	Delayed          AllocationStrategyType = "Delayed"
	DisasterRecovery AllocationStrategyType = "DisasterRecovery"
	Periodic         AllocationStrategyType = "Periodic"
	Constant         AllocationStrategyType = "Constant"
	Reserved         AllocationStrategyType = "Reserved"
	Unspecified      AllocationStrategyType = "Unspecified"
	Decommissioned   AllocationStrategyType = "Decommissioned"
)

type AllocationPeriodicStrategy struct {
	// Interval defines how frequently the quota is expected to meet its policy.
	// e.g. for a namespace that runs a job every 90 days, and is unallocated otherwise the duration would be 90 days.
	// +kubebuilder:validation:Pattern:=(\d+)(s|m|h|d|w)
	// +required
	Interval string `json:"interval"`
}

type AllocationDelayedStrategy struct {
	// ExpectedUsageDate is a timestamp of when the quota is expected to be allocated to pods.
	// This can be used to indicate an upcoming launch event, whereby you may not
	// be fully utilising your allocated resources until a known "go-live" date.
	// +required
	ExpectedUsageDate metav1.Time `json:"expectedUsageDate"`
}

// AllocationDisasterRecoveryStrategy indicates that this capacity is used for disaster recovery
// planning, and may not be allocated (or may consume significantly less CPU if it is allocated)
// until needed, either during a disaster recovery event or a test of disaster recovery resiliency.
type AllocationDisasterRecoveryStrategy struct {
	// AlwaysAllocated indicates whether the quota is expected to be 'allocated' (e.g. consumed
	// by pods) all the time, for example in an 'active-active' or an `active-passive` configuration.
	// If false, it's expected that the quota will not be allocated until a disaster recovery
	// or test event actually occurs.
	// +required
	AlwaysAllocated bool `json:"alwaysAllocated"`
}

type AllocationConstantStrategy struct {
	ConstantAllocated bool `json:"constantAllocated"`
}

// AllocationReservedStrategy indicates that quota has been set aside for the user,
// so it can't be reassigned. Subsequently, this quota will be scaled down
// and assigned to namespaces used by the user.
type AllocationReservedStrategy struct {
	// ReservedDate is the timestamp of when the quota was reserved
	// +required
	ReservedDate metav1.Time `json:"reservedDate"`
	// Reason is the justification for this reservation
	// e.g. For Product X Migration
	// +kubebuilder:validation:MaxLength=256
	// +required
	Reason string `json:"reason"`
}

// AllocationDecommissionedStrategy indicates that the quota is no longer in use or
// needed by the user and should be clawed back and deleted.
type AllocationDecommissionedStrategy struct {
	// DecommissionDate is the timestamp of when the quota was decommissioned
	// +required
	DecommissionDate metav1.Time `json:"decommissionDate"`
}

// AllocationDecommissionedStrategy indicates that the namespace owner has not set their RQD
type AllocationUnspecifiedStrategy struct {
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceQuotaDescriptorList contains a list of ResourceQuotaDescriptorLists, used for
// deserializing LIST requests.
type ResourceQuotaDescriptorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ResourceQuotaDescriptor `json:"items,omitempty"`
}
