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

type Route struct {
	Name   string   `json:"name"`
	Weight float64  `json:"weight"`
	Chain  []string `json:"chain"`
}

// +kubebuilder:validation:XValidation:rule="self.routes.map(r, r.weight).sum() >= 0.999 && self.routes.map(r, r.weight).sum() <= 1.001",message="route weights must sum to 1.0"
type CompositionParams struct {
	Routes []Route `json:"routes,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.type == 'WEIGHTED_ROUTES' ? (has(self.params) && self.params.routes.size() > 0) : !has(self.params)",message="params with at least one route is required when type is WEIGHTED_ROUTES, and must not be set otherwise"
type Composition struct {
	// +required
	// +kubebuilder:validation:Enum=AND_MIN;WEIGHTED_ROUTES
	Type string `json:"type"`
	// +optional
	Params *CompositionParams `json:"params,omitempty"`
}
type SLOObjective struct {
	// name is the name of the objective (e.g., "availability", "latency").
	// +required
	Name string `json:"name"`

	// * optional namespace for the objective, if not specified it will be assumed to be in the same namespace as the SLOComposition.
	Namespace string `json:"namespace,omitempty"`
}
type SLORef struct {
	// name is the name of the SLO resource being referenced.
	// +required
	Name string `json:"name"`

	Ref SLOObjective `json:"ref"`
}

// SLOCompositionSpec defines the desired state of SLOComposition
type SLOCompositionSpec struct {
	// target is the target percentage for the objective (e.g., 99.9).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=200
	// +required
	Target float64 `json:"target"`

	// window is the time window over which the objective is measured (e.g., "30d" for 30 days).
	// Only days are supported (e.g., "7d", "30d").
	// +kubebuilder:validation:Pattern=`^\d+d$`
	// +required
	Window string `json:"window"`

	// description is an optional human-readable description of the SLO.
	// +optional
	Description string `json:"description,omitempty,omitzero"`

	// +required
	Objectives []SLORef `json:"objectives"`

	// +required
	Composition Composition `json:"composition"`

	// alerting configures alerting rules (budget and burn rate) for this composition.
	// +optional
	Alerting *Alerting `json:"alerting,omitempty"`
}

// SLOCompositionStatus defines the observed state of SLOComposition.
type SLOCompositionStatus struct {
	// objectiveComposition represents the current status of the composed objective.
	// +optional
	ObjectiveComposition ObjectiveStatus `json:"objectiveComposition,omitempty"`

	// lastUpdateTime indicates the last time the status was updated.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the SLOComposition resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sloc
// +kubebuilder:printcolumn:name="Target",type=number,JSONPath=`.spec.target`,format=float
// +kubebuilder:printcolumn:name="Window",type=string,JSONPath=`.spec.window`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.objectiveComposition.status`
// +kubebuilder:printcolumn:name="Actual",type=number,JSONPath=`.status.objectiveComposition.actual`,format=float
// +kubebuilder:printcolumn:name="Budget %",type=number,JSONPath=`.status.objectiveComposition.errorBudget.percentRemaining`,format=float
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SLOComposition is the Schema for the slocompositions API
type SLOComposition struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of SLOComposition
	// +required
	Spec SLOCompositionSpec `json:"spec"`

	// status defines the observed state of SLOComposition
	// +optional
	Status SLOCompositionStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// SLOCompositionList contains a list of SLOComposition
type SLOCompositionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SLOComposition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SLOComposition{}, &SLOCompositionList{})
}
