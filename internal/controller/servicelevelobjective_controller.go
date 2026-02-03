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

package controller

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/federicolepera/slok/internal/burnrate"
	sloklog "github.com/federicolepera/slok/internal/log"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	"github.com/federicolepera/slok/internal/errorbudget"
	"github.com/federicolepera/slok/internal/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceLevelObjectiveReconciler reconciles a ServiceLevelObjective object
type ServiceLevelObjectiveReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	PrometheusClient prometheus.PrometheusClient
	PrometheusURL    string
}
type burnRatePreset struct {
	ShortWindow string
	LongWindow  string
	BurnRate    float64
}

var defaultBurnRatePresets = []burnRatePreset{
	{ShortWindow: "5m", LongWindow: "1h", BurnRate: 14},
	{ShortWindow: "1h", LongWindow: "6h", BurnRate: 6},
	{ShortWindow: "6h", LongWindow: "3d", BurnRate: 1},
	{ShortWindow: "7d", LongWindow: "30d", BurnRate: 0.5},
}

// +kubebuilder:rbac:groups=observability.slok.io,resources=servicelevelobjectives,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.slok.io,resources=servicelevelobjectives/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.slok.io,resources=servicelevelobjectives/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServiceLevelObjective object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *ServiceLevelObjectiveReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := sloklog.New(logf.FromContext(ctx)).WithValues("slo", req.NamespacedName)

	var slo observabilityv1alpha1.ServiceLevelObjective
	if err := r.Get(ctx, req.NamespacedName, &slo); err != nil {
		logger.Error(err, "unable to fetch ServiceLevelObjective")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize Prometheus client if not already done
	if r.PrometheusClient == nil {
		if r.PrometheusURL == "" {
			promURL := "http://localhost:9090" // Default Prometheus URL
			r.PrometheusURL = promURL
		}
		if promClient, err := prometheus.NewClient(r.PrometheusURL); err != nil {
			logger.Error(err, "unable to create Prometheus client", "prometheus_url", r.PrometheusURL)
			return ctrl.Result{}, err
		} else {
			r.PrometheusClient = promClient
			logger.Info("prometheus client initialized", "prometheus_url", r.PrometheusURL)
		}
	}

	// Check Prometheus connection
	if err := r.PrometheusClient.CheckConnection(ctx); err != nil {
		logger.Error(err, "unable to connect to Prometheus", "prometheus_url", r.PrometheusURL)
		return ctrl.Result{}, err
	}

	objectiveStatuses := make([]observabilityv1alpha1.ObjectiveStatus, 0)

	logger.Info("init reconcile ServiceLevelObjective", "name", slo.Name)

	for _, obj := range slo.Spec.Objectives {
		prometheusRule, err := prometheus.CreatePrometheusRule(slo.Name, slo.Namespace, obj)
		if err != nil {
			logger.Error(err, "unable to create Prometheus rule", "objective_name", obj.Name)
		} else {
			if err := r.Create(ctx, &prometheusRule); err != nil {
				logger.Error(err, "unable to create Prometheus rule in cluster", "prometheus_rule", prometheusRule.Name)
			}
		}
		logger.Info("Objective", "name", obj.Name, "target", obj.Target, "window", obj.Window, "sli_query", obj.Sli.Query)

		// Replace $window placeholder with the actual objective window value
		successQuery := strings.ReplaceAll(obj.Sli.Query.Success, "$window", obj.Window)
		totalQuery := strings.ReplaceAll(obj.Sli.Query.Total, "$window", obj.Window)
		sliSuccessValue, err := r.PrometheusClient.QuerySLI(ctx, successQuery)
		if err != nil {
			logger.Error(err, "unable to query SLI", "sli_query", obj.Sli.Query)
			objectiveStatuses = append(objectiveStatuses, observabilityv1alpha1.ObjectiveStatus{
				Name:   obj.Name,
				Target: obj.Target,
				Status: observabilityv1alpha1.ObjectiveConditionUnknown,
				ErrorBudget: observabilityv1alpha1.ErrorBudgetStatus{
					Total:            "unknown",
					Consumed:         "unknown",
					Remaining:        "unknown",
					PercentRemaining: 0,
				},
				LastQueried: metav1.Now(),
			})
			continue
		}
		sliTotalValue, err := r.PrometheusClient.QuerySLI(ctx, totalQuery)
		if err != nil {
			logger.Error(err, "unable to query SLI total", "sli_query", obj.Sli.Query)
			objectiveStatuses = append(objectiveStatuses, observabilityv1alpha1.ObjectiveStatus{
				Name:   obj.Name,
				Target: obj.Target,
				Status: observabilityv1alpha1.ObjectiveConditionUnknown,
				ErrorBudget: observabilityv1alpha1.ErrorBudgetStatus{
					Total:            "unknown",
					Consumed:         "unknown",
					Remaining:        "unknown",
					PercentRemaining: 0,
				},
				LastQueried: metav1.Now(),
			})
			continue
		}

		budget, sliValue, err := errorbudget.Calculate(obj, sliSuccessValue, sliTotalValue)
		if err != nil {
			logger.Error(err, "unable to calculate error budget", "objective_name", obj.Name)
			objectiveStatuses = append(objectiveStatuses, observabilityv1alpha1.ObjectiveStatus{
				Name:   obj.Name,
				Target: obj.Target,
				Actual: sliValue,
				Status: observabilityv1alpha1.ObjectiveConditionUnknown,
				ErrorBudget: observabilityv1alpha1.ErrorBudgetStatus{
					Total:            "unknown",
					Consumed:         "unknown",
					Remaining:        "unknown",
					PercentRemaining: 0,
				},
				LastQueried: metav1.Now(),
			})
			continue
		}
		prometheus.ObjectiveActual.WithLabelValues(
			slo.Namespace,
			slo.Name,
			obj.Name,
			fmt.Sprintf("%s/%s", slo.Name, obj.Name),
		).Set(sliValue)
		prometheus.ObjectiveTarget.WithLabelValues(
			slo.Namespace,
			slo.Name,
			obj.Name,
			fmt.Sprintf("%s/%s", slo.Name, obj.Name),
		).Set(obj.Target)
		prometheus.ObjectivePercentRemaining.WithLabelValues(
			slo.Namespace,
			slo.Name,
			obj.Name,
			fmt.Sprintf("%s/%s", slo.Name, obj.Name),
		).Set(budget.PercentRemaining)
		status := errorbudget.DetermineStatus(obj.Target, sliValue, budget.PercentRemaining)
		prometheus.ObjectiveStatus.DeleteLabelValues(
			slo.Namespace,
			slo.Name,
			obj.Name,
			status,
			fmt.Sprintf("%s/%s", slo.Name, obj.Name),
		)
		prometheus.ObjectiveStatus.WithLabelValues(
			slo.Namespace,
			slo.Name,
			obj.Name,
			status,
			fmt.Sprintf("%s/%s", slo.Name, obj.Name),
		).Set(1)
		var burnRates []burnrate.BurnRate
		for _, preset := range defaultBurnRatePresets {
			successLong := strings.ReplaceAll(obj.Sli.Query.Success, "$window", preset.LongWindow)
			totalLong := strings.ReplaceAll(obj.Sli.Query.Total, "$window", preset.LongWindow)
			successShort := strings.ReplaceAll(obj.Sli.Query.Success, "$window", preset.ShortWindow)
			totalShort := strings.ReplaceAll(obj.Sli.Query.Total, "$window", preset.ShortWindow)
			shortQuery := fmt.Sprintf("%s / %s", successShort, totalShort)
			sliShortWindow, err := r.PrometheusClient.QuerySLI(ctx, shortQuery)
			if err != nil {
				logger.Error(err, "unable to query SLI for short burn rate", "sli_query", shortQuery)
				continue
			}
			longQuery := fmt.Sprintf("%s / %s", successLong, totalLong)
			sliLongWindow, err := r.PrometheusClient.QuerySLI(ctx, longQuery)
			if err != nil {
				logger.Error(err, "unable to query SLI for long burn rate", "sli_query", longQuery)
				continue
			}
			burnRate, err := burnrate.Calculate(obj, sliShortWindow, sliLongWindow)
			if err != nil {
				logger.Error(err, "unable to calculate burn rate", "objective_name", obj.Name)
				continue
			}

			burnRate.ShortWindow = preset.ShortWindow
			burnRate.LongWindow = preset.LongWindow
			burnRates = append(burnRates, burnRate)
		}
		var burnRateStatuses []observabilityv1alpha1.BurnRateStatus
		for _, br := range burnRates {
			logger.Info("Burn rate for objective", "objective_name", obj.Name, "short_burn_rate", br.ShortBurnRate, "long_burn_rate", br.LongBurnRate)
			burnRateStatuses = append(burnRateStatuses, observabilityv1alpha1.BurnRateStatus{
				LongBurnRate:  br.LongBurnRate,
				ShortBurnRate: br.ShortBurnRate,
				LongWindow:    br.LongWindow,
				ShortWindow:   br.ShortWindow,
			})
		}

		objectiveStatuses = append(objectiveStatuses, observabilityv1alpha1.ObjectiveStatus{
			Name:   obj.Name,
			Target: obj.Target,
			Actual: math.Round(sliValue*100) / 100,
			Status: status,
			ErrorBudget: observabilityv1alpha1.ErrorBudgetStatus{
				Total:            budget.Total,
				Consumed:         budget.Consumed,
				Remaining:        budget.Remaining,
				PercentRemaining: budget.PercentRemaining,
			},
			BurnRate:    burnRateStatuses,
			LastQueried: metav1.Now(),
		})
	}
	slo.Status.Objectives = objectiveStatuses
	slo.Status.LastUpdateTime = metav1.Now()

	meta.SetStatusCondition(&slo.Status.Conditions, metav1.Condition{
		Type:   "Available",
		Status: metav1.ConditionTrue,
		Reason: "Reconciled",
	})
	if err := r.Status().Update(ctx, &slo); err != nil {
		logger.Error(err, "Failed to update SLO status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled SLO")

	// Requeue after 1 minute
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceLevelObjectiveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&observabilityv1alpha1.ServiceLevelObjective{}).
		Named("servicelevelobjective").
		Complete(r)
}
