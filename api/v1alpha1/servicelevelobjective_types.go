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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
type BurnRateAlert struct {
	Name           string  `json:"name"`
	ConsumePercent float64 `json:"consumePercent"`
	ConsumeWindow  string  `json:"consumeWindow"`
	LongWindow     string  `json:"longWindow"`
	ShortWindow    string  `json:"shortWindow"`
	Severity       string  `json:"severity"`
}
type BudgetAlert struct {
	Name     string  `json:"name"`
	Percent  float64 `json:"percent"`
	Severity string  `json:"severity"`
}
type Alerting struct {
	Enabled        bool            `json:"enabled"`
	BudgetAlerts   []BudgetAlert   `json:"budgetAlerts,omitempty"`
	BurnRateAlerts []BurnRateAlert `json:"burnRateAlerts,omitempty"`
}
type Query struct {
	Success string `json:"success"`
	Total   string `json:"total"`
}
type SLI struct {
	// PromQL query that returns the SLI value
	Query Query `json:"query"`
}
type Objective struct {
	// name is the unique name of the objective within the Service Level Objective.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +required
	Name string `json:"name"`
	// targetPercentage is the target percentage for the objective (e.g., 99.9).
	// Target as percentage (e.g. 99.9, 95.5)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=200
	Target float64 `json:"target"`
	// window is the time window over which the objective is measured (e.g., "30d" for 30 days).
	// +kubebuilder:validation:Pattern=`^(\d+d|\d+h|\d+m|\d+s)$`
	// +required
	Window string `json:"window"`
	Sli    SLI    `json:"sli"`

	Alerting    Alerting    `json:"alerting,omitempty"`
	BudgetAlert BudgetAlert `json:"budgetAlert,omitempty"`
}

// ServiceLevelObjectiveSpec defines the desired state of ServiceLevelObjective
type ServiceLevelObjectiveSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// displayName is the human-readable name for the Service Level Objective.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +required
	DisplayName string `json:"displayName"`

	// objectives is a list of individual objectives that make up the Service Level Objective.
	// +kubebuilder:validation:MinItems=1
	// +required
	Objectives []Objective `json:"objectives"`
}

// ErrorBudgetStatus represents error budget consumption
type ErrorBudgetStatus struct {
	// Total error budget for the window (e.g., "43.2m" for 43.2 minutes)
	Total string `json:"total"`

	// Consumed error budget so far (e.g., "10.5m")
	Consumed string `json:"consumed"`

	// Remaining error budget (e.g., "32.7m")
	Remaining string `json:"remaining"`

	// PercentRemaining is the percentage of budget left (e.g., 75.69)
	PercentRemaining float64 `json:"percentRemaining"`
}

type BurnRateStatus struct {
	// LongBurnRate represents the long-term burn rate
	LongBurnRate float64 `json:"longBurnRate"`

	// ShortBurnRate represents the short-term burn rate
	ShortBurnRate float64 `json:"shortBurnRate"`

	// BurnRateThreshold is the threshold for alerting
	BurnRateThreshold float64 `json:"burnRateThreshold"`

	// Status indicates if burn rate is within acceptable limits
	Status string `json:"status"`
}
type ObjectiveStatus struct {
	// Name of the objective (matches Objective.Name)
	Name string `json:"name"`

	// Target percentage (copied from spec for convenience)
	Target float64 `json:"target"`

	// Actual current percentage (e.g., 99.87)
	Actual float64 `json:"actual"`

	// Status indicates if objective is being met
	// +kubebuilder:validation:Enum=met;at-risk;violated;unknown
	Status string `json:"status"`

	// ErrorBudget details
	ErrorBudget ErrorBudgetStatus `json:"errorBudget"`

	BurnRate BurnRateStatus `json:"burnRate,omitempty"`
	// LastQueried is when we last queried Prometheus
	// +optional
	LastQueried metav1.Time `json:"lastQueried,omitempty"`
}

// ServiceLevelObjectiveStatus defines the observed state of ServiceLevelObjective.
type ServiceLevelObjectiveStatus struct {
	// objectives represent the current status of each objective defined in the spec.
	// +optional
	Objectives []ObjectiveStatus `json:"objectives,omitempty"`

	// lastUpdateTime indicates the last time the status was updated.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// conditions represent the current state of the ServiceLevelObjective resource.
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

// ServiceLevelObjective is the Schema for the servicelevelobjectives API
type ServiceLevelObjective struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of ServiceLevelObjective
	// +required
	Spec ServiceLevelObjectiveSpec `json:"spec"`

	// status defines the observed state of ServiceLevelObjective
	// +optional
	Status ServiceLevelObjectiveStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ServiceLevelObjectiveList contains a list of ServiceLevelObjective
type ServiceLevelObjectiveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceLevelObjective `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceLevelObjective{}, &ServiceLevelObjectiveList{})
}
