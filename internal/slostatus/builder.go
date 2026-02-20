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

// Package slostatus provides shared helpers for building ObjectiveStatus values
// used by both the ServiceLevelObjective and SLOComposition controllers.
package slostatus

import (
	"math"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	"github.com/federicolepera/slok/internal/burnrate"
	"github.com/federicolepera/slok/internal/errorbudget"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildUnknownStatus returns an ObjectiveStatus with all fields set to sentinel
// "unknown" values. Use this when a Prometheus query fails or budget calculation
// cannot be performed.
func BuildUnknownStatus(name string, target float64) observabilityv1alpha1.ObjectiveStatus {
	return observabilityv1alpha1.ObjectiveStatus{
		Name:   name,
		Target: target,
		Actual: 0,
		Status: observabilityv1alpha1.ObjectiveConditionUnknown,
		ErrorBudget: observabilityv1alpha1.ErrorBudgetStatus{
			Total:            "unknown",
			Consumed:         "unknown",
			Remaining:        "unknown",
			PercentRemaining: 0,
		},
		LastQueried: metav1.Now(),
	}
}

// BuildBurnRateStatuses converts internal BurnRate values to the API status type.
func BuildBurnRateStatuses(burnRates []burnrate.BurnRate) []observabilityv1alpha1.BurnRateStatus {
	statuses := make([]observabilityv1alpha1.BurnRateStatus, 0, len(burnRates))
	for _, br := range burnRates {
		statuses = append(statuses, observabilityv1alpha1.BurnRateStatus{
			LongBurnRate:  br.LongBurnRate,
			ShortBurnRate: br.ShortBurnRate,
			LongWindow:    br.LongWindow,
			ShortWindow:   br.ShortWindow,
		})
	}
	return statuses
}

// BuildSuccessStatus builds an ObjectiveStatus from successfully computed metrics.
func BuildSuccessStatus(name string, target, sliValue float64, budget *errorbudget.Budget, conditionStatus string, burnRateStatuses []observabilityv1alpha1.BurnRateStatus) observabilityv1alpha1.ObjectiveStatus {
	return observabilityv1alpha1.ObjectiveStatus{
		Name:   name,
		Target: target,
		Actual: math.Round(sliValue*100) / 100,
		Status: conditionStatus,
		ErrorBudget: observabilityv1alpha1.ErrorBudgetStatus{
			Total:            budget.Total,
			Consumed:         budget.Consumed,
			Remaining:        budget.Remaining,
			PercentRemaining: budget.PercentRemaining,
		},
		BurnRate:    burnRateStatuses,
		LastQueried: metav1.Now(),
	}
}
