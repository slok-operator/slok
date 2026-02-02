/*
Copyright 2026.

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

package prometheus

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Metric labels used across all metrics
const (
	LabelNamespace             = "namespace"
	LabelServiceLevelObjective = "service_level_objective"
	LabelObjectiveName         = "objective_name"
	LabelObjectiveId           = "objective_id"
	LabelStatus                = "status"
)

var (
	// ========== SLO METRICS ==========

	// ObjectiveStatus indicates whether the SLO objective is met (1) or not (0)
	ObjectiveStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "slok_slo_objective_status",
			Help: "Status of the SLO objective (1 = met, 0 = not met)",
		},
		[]string{LabelNamespace, LabelServiceLevelObjective, LabelObjectiveName, LabelStatus, LabelObjectiveId},
	)

	ObjectivePercentRemaining = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "slok_slo_objective_percent_remaining",
			Help: "Percentage of error budget remaining for the SLOss objective",
		},
		[]string{LabelNamespace, LabelServiceLevelObjective, LabelObjectiveName, LabelObjectiveId},
	)

	ObjectiveTarget = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "slok_slo_objective_target",
			Help: "Target value for the SLO objective",
		},
		[]string{LabelNamespace, LabelServiceLevelObjective, LabelObjectiveName, LabelObjectiveId},
	)

	ObjectiveActual = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "slok_slo_objective_actual",
			Help: "Actual value for the SLO objective",
		},
		[]string{LabelNamespace, LabelServiceLevelObjective, LabelObjectiveName, LabelObjectiveId},
	)
)

func init() {
	// Register all metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
		// Slo metrics
		ObjectiveStatus,
		ObjectivePercentRemaining,
		ObjectiveTarget,
		ObjectiveActual,
	)
}
