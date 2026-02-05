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

type BudgetAlert struct {
	// name is the unique identifier for this budget alert.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`

	// percent is the error budget remaining threshold below which the alert fires (e.g., 10.0 means < 10%).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +required
	Percent float64 `json:"percent"`

	// severity is the alert severity level (e.g., "critical", "warning").
	// +kubebuilder:validation:Enum=critical;warning;info
	// +required
	Severity string `json:"severity"`
}
type BurnRateAlert struct {
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`

	// consumePercent is the percentage of error budget that would be consumed
	// within the consumeWindow at the current burn rate to trigger the alert (e.g., 2.0 means 2%).
	// +kubebuilder:validation:Minimum=0
	// +required
	ConsumePercent float64 `json:"consumePercent"`

	// consumeWindow is the time window used to compute the burn rate threshold (e.g., "1h", "6h").
	// +kubebuilder:validation:Pattern=`^(\d+d|\d+h|\d+m|\d+s)$`
	// +required
	ConsumeWindow string `json:"consumeWindow"`

	// longWindow is the long observation window for the burn rate query (e.g., "1h").
	// +kubebuilder:validation:Pattern=`^(\d+d|\d+h|\d+m|\d+s)$`
	// +required
	LongWindow string `json:"longWindow"`

	// shortWindow is the short observation window for the burn rate query (e.g., "5m").
	// +kubebuilder:validation:Pattern=`^(\d+d|\d+h|\d+m|\d+s)$`
	// +required
	ShortWindow string `json:"shortWindow"`

	// severity is the alert severity level (e.g., "critical", "warning").
	// +kubebuilder:validation:Enum=critical;warning;info
	// +required
	Severity string `json:"severity"`
}

// BurnRateAlert defines a burn rate alerting rule.
// It determines how fast the error budget is being consumed
// and triggers alerts when the burn rate exceeds the threshold.
type BurnRates struct {
	Enabled bool `json:"enabled,omitempty"`
	// name is the unique identifier for this burn rate alert.
	// +optional
	Alerts []BurnRateAlert `json:"alerts,omitempty"`
}

// BudgetAlert defines an error budget threshold alerting rule.
// It triggers an alert when the remaining error budget drops below the specified percentage.
type BudgetErrors struct {
	Enabled bool `json:"enabled,omitempty"`

	// alerts is a list of budget alerting rules.
	// Each alert specifies a threshold percentage and severity level.
	// +optional
	Alerts []BudgetAlert `json:"alerts,omitempty"`
}

// Alerting configures the alerting behaviour for an objective.
// When enabled, PrometheusRule resources are created for budget and/or burn rate alerts.
type Alerting struct {
	// budgetAlerts is a list of error budget threshold alerts.
	// +optional
	BudgetErrorAlerts BudgetErrors `json:"budgetErrorAlerts,omitempty"`

	// burnRateAlerts is a list of burn rate alerting rules.
	// +optional
	BurnRateAlerts BurnRates `json:"burnRateAlerts,omitempty"`
}

// Query holds the PromQL metric selectors used to compute the SLI error ratio.
// Required when not using a template.
type Query struct {
	// totalQuery is the Prometheus metric selector for total events.
	// Example: http_requests_total{service="api"}
	// +optional
	TotalQuery string `json:"totalQuery,omitempty"`

	// errorQuery is the Prometheus metric selector for error events.
	// Example: http_requests_total{service="api",status=~"5.."}
	// +optional
	ErrorQuery string `json:"errorQuery,omitempty"`
}

type TemplateStruct struct {
	// name is the name of the template to use for this SLI.
	// Available templates: http-availability, http-latency, kubernetes-apiserver
	// +kubebuilder:validation:Enum=http-availability;http-latency;kubernetes-apiserver
	// +optional
	Name string `json:"name,omitempty"`

	// labels are the Prometheus label selectors to filter metrics.
	// Example: {"service": "payment-api", "namespace": "production"}
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// params are template-specific parameters.
	// For http-latency: threshold (e.g., "0.5" for 500ms)
	// +optional
	Params map[string]string `json:"params,omitempty"`
}

// SLI (Service Level Indicator) defines how the objective is measured.
// Use either a template (recommended) or manual queries.
type SLI struct {
	// query contains the manual Prometheus metric selectors.
	// Required if template is not specified.
	// +optional
	Query Query `json:"query,omitempty"`

	// template specifies a predefined SLI template.
	// When set, query fields are ignored.
	// +optional
	Template TemplateStruct `json:"template,omitempty"`
}

// Possible values for ObjectiveStatus.Status.
const (
	ObjectiveConditionMet      = "met"
	ObjectiveConditionWarning  = "warning"
	ObjectiveConditionDegraded = "degraded"
	ObjectiveConditionCritical = "critical"
	ObjectiveConditionViolated = "violated"
	ObjectiveConditionUnknown  = "unknown"
)

// Objective represents a single measurable target within a ServiceLevelObjective.
type Objective struct {
	// name is the unique name of the objective within the Service Level Objective.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +required
	Name string `json:"name"`

	// target is the target percentage for the objective (e.g., 99.9).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=200
	// +required
	Target float64 `json:"target"`

	// window is the time window over which the objective is measured (e.g., "30d" for 30 days).
	// +optional
	Window string `json:"window"`

	// sli defines the Service Level Indicator used to measure this objective.
	// +required
	Sli SLI `json:"sli"`

	// alerting configures alerting rules (budget and burn rate) for this objective.
	// +optional
	Alerting Alerting `json:"alerting,omitempty"`
}

// ServiceLevelObjectiveSpec defines the desired state of a ServiceLevelObjective.
type ServiceLevelObjectiveSpec struct {
	// displayName is the human-readable name for the Service Level Objective.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +required
	DisplayName string `json:"displayName"`

	// objectives defines the objective for this Service Level Objective.
	// +required
	Objective Objective `json:"objective"`
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

// BurnRateStatus represents the observed burn rate for an objective.
type BurnRateStatus struct {
	// shortWindow is the duration of the short observation window (e.g., "5m").
	ShortWindow string `json:"shortWindow"`

	// shortBurnRate is the burn rate computed over the short observation window.
	ShortBurnRate float64 `json:"shortBurnRate"`

	// longWindow is the duration of the long observation window (e.g., "1h").
	LongWindow string `json:"longWindow"`

	// longBurnRate is the burn rate computed over the long observation window.
	LongBurnRate float64 `json:"longBurnRate"`
}

// ObjectiveStatus represents the observed state of a single objective.
type ObjectiveStatus struct {
	// name of the objective (matches Objective.Name).
	Name string `json:"name"`

	// target percentage (copied from spec for convenience).
	Target float64 `json:"target"`

	// actual is the current SLI percentage (e.g., 99.87).
	Actual float64 `json:"actual"`

	// status indicates whether the objective is being met.
	// +kubebuilder:validation:Enum=met;warning;degraded;critical;violated;unknown
	Status string `json:"status"`

	// errorBudget contains details about error budget consumption.
	ErrorBudget ErrorBudgetStatus `json:"errorBudget"`

	// burnRate contains the observed burn rate metrics.
	// +optional
	BurnRate []BurnRateStatus `json:"burnRate,omitempty"`

	// lastQueried is the timestamp of the last Prometheus query for this objective.
	// +optional
	LastQueried metav1.Time `json:"lastQueried,omitempty"`
}

// ServiceLevelObjectiveStatus defines the observed state of ServiceLevelObjective.
type ServiceLevelObjectiveStatus struct {
	// objectives represent the current status of each objective defined in the spec.
	// +optional
	Objective ObjectiveStatus `json:"objective,omitempty"`

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
// +kubebuilder:resource:shortName=slo
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.objective.status`
// +kubebuilder:printcolumn:name="Actual",type=number,JSONPath=`.status.objective.actual`,format=float
// +kubebuilder:printcolumn:name="Target",type=number,JSONPath=`.status.objective.target`,format=float
// +kubebuilder:printcolumn:name="Budget %",type=number,JSONPath=`.status.objective.errorBudget.percentRemaining`,format=float
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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
