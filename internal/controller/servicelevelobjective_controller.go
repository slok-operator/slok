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
	"time"

	"github.com/federicolepera/slok/internal/burnrate"
	"github.com/federicolepera/slok/internal/correlation"
	"github.com/federicolepera/slok/internal/errorbudget"
	sloklog "github.com/federicolepera/slok/internal/log"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	"github.com/federicolepera/slok/internal/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceLevelObjectiveReconciler reconciles a ServiceLevelObjective object
type ServiceLevelObjectiveReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	PrometheusClient prometheus.PrometheusClient
	PrometheusURL    string
	// Correlation support
	ChangeCollector   *correlation.ChangeCollector
	CorrelationEngine *correlation.CorrelationEngine
	AnomalyDetector   *correlation.AnomalyDetector
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
// +kubebuilder:rbac:groups=observability.slok.io,resources=slocorrelations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.slok.io,resources=slocorrelations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;events,verbs=get;list;watch

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

	logger.Info("init reconcile ServiceLevelObjective", "name", slo.Name)

	obj := slo.Spec.Objective
	var objectiveStatus observabilityv1alpha1.ObjectiveStatus

	desiredRule, err := prometheus.CreatePrometheusRule(slo.Name, slo.Namespace, obj)
	if err != nil {
		logger.Error(err, "unable to create Prometheus rule", "objective_name", obj.Name)
	} else {
		existingRule := &monitoringv1.PrometheusRule{}
		existingRule.Name = desiredRule.Name
		existingRule.Namespace = desiredRule.Namespace

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, existingRule, func() error {
			existingRule.Labels = desiredRule.Labels
			existingRule.Spec = desiredRule.Spec
			return controllerutil.SetControllerReference(&slo, existingRule, r.Scheme)
		})
		if err != nil {
			logger.Error(err, "unable to create or update Prometheus rule", "prometheus_rule", desiredRule.Name)
		}
	}
	logger.Info("Objective", "name", obj.Name, "target", obj.Target, "window", obj.Window, "sli_query", obj.Sli.Query)

	sliErrorRate5mQuery := fmt.Sprintf("slok:sli_error_rate:5m{objective_name=\"%s\",slo_name=\"%s\",slo_namespace=\"%s\"}", obj.Name, slo.Name, slo.Namespace)
	sliErrorRate5m, err := r.PrometheusClient.QuerySLI(ctx, sliErrorRate5mQuery)
	if err != nil {
		logger.Error(err, "unable to query SLI error rate for 5m window", "sli_query", sliErrorRate5mQuery)
		objectiveStatus = observabilityv1alpha1.ObjectiveStatus{
			Name:   obj.Name,
			Target: obj.Target,
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
		slo.Status.Objective = objectiveStatus
		slo.Status.LastUpdateTime = metav1.Now()
		meta.SetStatusCondition(&slo.Status.Conditions, metav1.Condition{
			Type:   "Available",
			Status: metav1.ConditionFalse,
			Reason: "QueryFailed",
		})
		if err := r.Status().Update(ctx, &slo); err != nil {
			logger.Error(err, "Failed to update SLO status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}
	logger.Info("SLI error rate for 5m window", "objective_name", obj.Name, "sli_error_rate_5m", sliErrorRate5m)

	sliBurnRateWindowedQuery := fmt.Sprintf("slok:burn_rate:%s{objective_name=\"%s\",slo_name=\"%s\",slo_namespace=\"%s\"}", obj.Window, obj.Name, slo.Name, slo.Namespace)
	sliBurnRateWindowed, err := r.PrometheusClient.QuerySLI(ctx, sliBurnRateWindowedQuery)
	if err != nil {
		logger.Error(err, "unable to query SLI burn rate windowed", "sli_query", sliBurnRateWindowedQuery)
		objectiveStatus = observabilityv1alpha1.ObjectiveStatus{
			Name:   obj.Name,
			Target: obj.Target,
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
		slo.Status.Objective = objectiveStatus
		slo.Status.LastUpdateTime = metav1.Now()
		meta.SetStatusCondition(&slo.Status.Conditions, metav1.Condition{
			Type:   "Available",
			Status: metav1.ConditionFalse,
			Reason: "QueryFailed",
		})
		if err := r.Status().Update(ctx, &slo); err != nil {
			logger.Error(err, "Failed to update SLO status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	budget, sliValue, err := errorbudget.Calculate(obj, sliBurnRateWindowed, sliErrorRate5m)
	if err != nil {
		logger.Error(err, "unable to calculate error budget", "objective_name", obj.Name)
		objectiveStatus = observabilityv1alpha1.ObjectiveStatus{
			Name:   obj.Name,
			Target: obj.Target,
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
		slo.Status.Objective = objectiveStatus
		slo.Status.LastUpdateTime = metav1.Now()
		meta.SetStatusCondition(&slo.Status.Conditions, metav1.Condition{
			Type:   "Available",
			Status: metav1.ConditionFalse,
			Reason: "CalculationFailed",
		})
		if err := r.Status().Update(ctx, &slo); err != nil {
			logger.Error(err, "Failed to update SLO status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	var burnRates []burnrate.BurnRate
	for _, preset := range defaultBurnRatePresets {
		sliBurnRateShortQuery := fmt.Sprintf("slok:burn_rate:%s{objective_name=\"%s\",slo_name=\"%s\",slo_namespace=\"%s\"}", preset.ShortWindow, obj.Name, slo.Name, slo.Namespace)
		sliBurnRateShortLongQuery := fmt.Sprintf("slok:burn_rate:%s{objective_name=\"%s\",slo_name=\"%s\",slo_namespace=\"%s\"}", preset.LongWindow, obj.Name, slo.Name, slo.Namespace)
		sliBurnRateShort, err := r.PrometheusClient.QuerySLI(ctx, sliBurnRateShortQuery)
		if err != nil {
			logger.Error(err, "unable to query SLI for short burn rate", "sli_query", sliBurnRateShortQuery)
			continue
		}
		sliBurnRateLong, err := r.PrometheusClient.QuerySLI(ctx, sliBurnRateShortLongQuery)
		if err != nil {
			logger.Error(err, "unable to query SLI for long burn rate", "sli_query", sliBurnRateShortLongQuery)
			continue
		}
		burnRates = append(burnRates, burnrate.BurnRate{
			ShortBurnRate: sliBurnRateShort,
			LongBurnRate:  sliBurnRateLong,
			ShortWindow:   preset.ShortWindow,
			LongWindow:    preset.LongWindow,
		})
	}

	var burnRateStatuses []observabilityv1alpha1.BurnRateStatus
	status := errorbudget.DetermineStatus(obj.Target, sliValue, budget.PercentRemaining, burnRates)
	for _, br := range burnRates {
		logger.Info("Burn rate for objective", "objective_name", obj.Name, "short_window", br.ShortWindow, "long_window", br.LongWindow, "short_burn_rate", br.ShortBurnRate, "long_burn_rate", br.LongBurnRate)
		burnRateStatuses = append(burnRateStatuses, observabilityv1alpha1.BurnRateStatus{
			LongBurnRate:  br.LongBurnRate,
			ShortBurnRate: br.ShortBurnRate,
			LongWindow:    br.LongWindow,
			ShortWindow:   br.ShortWindow,
		})
	}

	// Check for burn rate spike and create correlation if needed
	if r.AnomalyDetector != nil && r.CorrelationEngine != nil && len(burnRates) > 0 {
		// Use 5m burn rate for spike detection (most responsive)
		currentBurnRate := burnRates[0].ShortBurnRate
		spikeResult := r.AnomalyDetector.DetectSpike(slo.Namespace, slo.Name, currentBurnRate)

		if spikeResult.Detected {
			logger.Info("Burn rate spike detected, creating correlation",
				"severity", spikeResult.Severity,
				"current_burn_rate", spikeResult.CurrentBurnRate,
				"previous_burn_rate", spikeResult.PreviousBurnRate,
			)

			// Create correlation analysis
			corr := r.CorrelationEngine.Analyze(
				slo.Name,
				slo.Namespace,
				time.Now(),
				spikeResult.CurrentBurnRate,
				spikeResult.PreviousBurnRate,
				spikeResult.Severity,
				slo.Spec.WorkloadSelector,
			)

			// Set owner reference
			if err := controllerutil.SetControllerReference(&slo, corr, r.Scheme); err != nil {
				logger.Error(err, "Failed to set owner reference on SLOCorrelation")
			} else {
				// Save status before Create (Create overwrites corr with API response which has empty status)
				savedStatus := corr.Status

				// Create the correlation resource (spec only)
				if err := r.Create(ctx, corr); err != nil {
					if !errors.IsAlreadyExists(err) {
						logger.Error(err, "Failed to create SLOCorrelation", "name", corr.Name)
					}
				} else {
					// Restore status and update the status subresource
					corr.Status = savedStatus
					if err := r.Status().Update(ctx, corr); err != nil {
						logger.Error(err, "Failed to update SLOCorrelation status", "name", corr.Name)
					} else {
						logger.Info("Created SLOCorrelation", "name", corr.Name, "events", corr.Status.EventCount)
					}
				}
			}
		}
	}

	objectiveStatus = observabilityv1alpha1.ObjectiveStatus{
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
	}

	slo.Status.Objective = objectiveStatus
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
