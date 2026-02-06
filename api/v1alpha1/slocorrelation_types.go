/*
Copyright 2026 Federico Le Pera.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Confidence levels for correlated events
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// CorrelatedEvent represents a Kubernetes change that may have caused the SLO degradation
type CorrelatedEvent struct {
	// kind is the Kubernetes resource kind (e.g., "Deployment", "ConfigMap")
	// +required
	Kind string `json:"kind"`

	// name is the name of the resource that changed
	// +required
	Name string `json:"name"`

	// namespace is the namespace of the resource
	// +required
	Namespace string `json:"namespace"`

	// timestamp is when the change occurred
	// +required
	Timestamp metav1.Time `json:"timestamp"`

	// changeType indicates what kind of change occurred (create, update, delete)
	// +kubebuilder:validation:Enum=create;update;delete
	// +required
	ChangeType string `json:"changeType"`

	// change describes what changed (e.g., "image: v1.4.2 → v1.4.3")
	// +optional
	Change string `json:"change,omitempty"`

	// actor is who or what triggered the change (e.g., "user@example.com", "argocd")
	// +optional
	Actor string `json:"actor,omitempty"`

	// confidence indicates how likely this event caused the SLO degradation
	// +kubebuilder:validation:Enum=high;medium;low
	// +required
	Confidence string `json:"confidence"`
}

// TimeWindow represents the time range analyzed for correlation
type TimeWindow struct {
	// start is the beginning of the analysis window
	// +required
	Start metav1.Time `json:"start"`

	// end is the end of the analysis window
	// +required
	End metav1.Time `json:"end"`
}

// SLOCorrelationSpec defines the desired state of SLOCorrelation
// Note: This is mostly empty as SLOCorrelation is created by the controller
type SLOCorrelationSpec struct {
	// sloRef references the ServiceLevelObjective that triggered this correlation
	// +required
	SLORef SLOReference `json:"sloRef"`
}

// SLOReference identifies the SLO that triggered the correlation
type SLOReference struct {
	// name is the name of the ServiceLevelObjective
	// +required
	Name string `json:"name"`

	// namespace is the namespace of the ServiceLevelObjective
	// +required
	Namespace string `json:"namespace"`
}

// SLOCorrelationStatus defines the observed state of SLOCorrelation
type SLOCorrelationStatus struct {
	// detectedAt is when the burn rate spike was detected
	// +required
	DetectedAt metav1.Time `json:"detectedAt"`

	// burnRateAtDetection is the burn rate value when the spike was detected
	// +required
	BurnRateAtDetection float64 `json:"burnRateAtDetection"`

	// previousBurnRate is the burn rate from the previous reconcile
	// +optional
	PreviousBurnRate float64 `json:"previousBurnRate,omitempty"`

	// severity indicates the severity level that was triggered
	// +kubebuilder:validation:Enum=critical;degraded;warning
	// +required
	Severity string `json:"severity"`

	// window is the time range that was analyzed for correlations
	// +required
	Window TimeWindow `json:"window"`

	// correlatedEvents is the list of changes that may have caused the degradation
	// +optional
	CorrelatedEvents []CorrelatedEvent `json:"correlatedEvents,omitempty"`

	// summary is a human-readable description of the likely cause
	// +optional
	Summary string `json:"summary,omitempty"`

	// eventCount is the total number of events found in the window
	// +optional
	EventCount int `json:"eventCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=slocorr
// +kubebuilder:printcolumn:name="SLO",type=string,JSONPath=`.spec.sloRef.name`
// +kubebuilder:printcolumn:name="Severity",type=string,JSONPath=`.status.severity`
// +kubebuilder:printcolumn:name="Burn Rate",type=number,JSONPath=`.status.burnRateAtDetection`,format=float
// +kubebuilder:printcolumn:name="Events",type=integer,JSONPath=`.status.eventCount`
// +kubebuilder:printcolumn:name="Detected",type=date,JSONPath=`.status.detectedAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SLOCorrelation records a burn rate spike and correlated cluster changes
type SLOCorrelation struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the SLO reference
	// +required
	Spec SLOCorrelationSpec `json:"spec"`

	// status contains the correlation analysis results
	// +optional
	Status SLOCorrelationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SLOCorrelationList contains a list of SLOCorrelation
type SLOCorrelationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SLOCorrelation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SLOCorrelation{}, &SLOCorrelationList{})
}
