// Package scheme provides a shared scheme for Kubernetes types
package scheme

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	quotamangementv1alpha1 "sigs.k8s.io/usage-metrics-collector/pkg/api/quotamanagementv1alpha1"
)

// Scheme is the scheme used by the collectors.
var Scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = metricsv1beta1.AddToScheme(Scheme)
	_ = quotamangementv1alpha1.AddToScheme(Scheme)
}
