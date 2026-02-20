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
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	"github.com/federicolepera/slok/internal/burnrate"
	"github.com/federicolepera/slok/internal/errorbudget"
	sloklog "github.com/federicolepera/slok/internal/log"
	"github.com/federicolepera/slok/internal/prometheus"
	"github.com/federicolepera/slok/internal/slostatus"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

// SLOCompositionReconciler reconciles a SLOComposition object
type SLOCompositionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	PrometheusClient prometheus.PrometheusClient
	PrometheusURL    string
}

// +kubebuilder:rbac:groups=observability.slok.io,resources=slocompositions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.slok.io,resources=slocompositions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.slok.io,resources=slocompositions/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SLOComposition object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *SLOCompositionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := sloklog.New(logf.FromContext(ctx)).WithValues("slo", req.NamespacedName)

	var sloComposition observabilityv1alpha1.SLOComposition
	if err := r.Get(ctx, req.NamespacedName, &sloComposition); err != nil {
		logger.Error(err, "unable to fetch SLOComposition")
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

	var slo observabilityv1alpha1.ServiceLevelObjective
	sloList := make([]observabilityv1alpha1.ServiceLevelObjective, 0, len(sloComposition.Spec.Objectives))
	for _, obj := range sloComposition.Spec.Objectives {
		logger.Info("Processing objective", "name", obj.Name, "namespace", obj.Namespace)
		if obj.Namespace == "" {
			logger.Info("Objective namespace not specified, using SLOComposition namespace", "namespace", sloComposition.Namespace)
			obj.Namespace = sloComposition.Namespace
		}
		if err := r.Get(ctx, client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, &slo); err != nil {
			logger.Error(err, "unable to fetch SLO", "name", obj.Name, "namespace", obj.Namespace)
			meta.SetStatusCondition(&sloComposition.Status.Conditions, metav1.Condition{
				Type:    "ObjectiveFetchFailed",
				Status:  metav1.ConditionFalse,
				Reason:  "ObjectiveNotFound",
				Message: "Failed to fetch objective " + obj.Name + " in namespace " + obj.Namespace,
			})
			if err := r.Status().Update(ctx, &sloComposition); err != nil {
				logger.Error(err, "Failed to update SLOComposition status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		sloList = append(sloList, slo)
		logger.Info("Successfully fetched SLO", "name", obj.Name, "namespace", obj.Namespace)
	}

	desiredRule, err := prometheus.CreateAggregatedPrometheusRule(sloComposition.Name, sloComposition.Namespace, sloComposition.Spec, sloList)
	if err != nil {
		logger.Error(err, "unable to create Prometheus rule", "objective_name", sloComposition.Name)
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
	sliErrorRate5mQuery := fmt.Sprintf("slok:sli_error_composition_rate:5m{slo_composition_name=\"%s\",slo_composition_namespace=\"%s\"}", sloComposition.Name, sloComposition.Namespace)
	sliErrorRate5m, err := r.PrometheusClient.QuerySLI(ctx, sliErrorRate5mQuery)
	if err != nil {
		logger.Error(err, "unable to query SLI error rate for 5m window", "sli_query", sliErrorRate5mQuery)
		sloComposition.Status.ObjectiveComposition = slostatus.BuildUnknownStatus(sloComposition.Name, sloComposition.Spec.Tartget)
		sloComposition.Status.LastUpdateTime = metav1.Now()
		meta.SetStatusCondition(&sloComposition.Status.Conditions, metav1.Condition{
			Type:   "Available",
			Status: metav1.ConditionFalse,
			Reason: "QueryFailed",
		})
		if err := r.Status().Update(ctx, &sloComposition); err != nil {
			logger.Error(err, "Failed to update SLO status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}
	logger.Info("SLI error rate for 5m window", "sloCompositionName", sloComposition.Name, "sli_error_rate_5m", sliErrorRate5m)

	sliBurnRateWindowedQuery := fmt.Sprintf("slok:burn_rate_composition:%s{slo_composition_name=\"%s\",slo_composition_namespace=\"%s\"}", sloComposition.Spec.Window, sloComposition.Name, sloComposition.Namespace)
	sliBurnRateWindowed, err := r.PrometheusClient.QuerySLI(ctx, sliBurnRateWindowedQuery)
	if err != nil {
		logger.Error(err, "unable to query SLI burn rate windowed", "sli_query", sliBurnRateWindowedQuery)
		sloComposition.Status.ObjectiveComposition = slostatus.BuildUnknownStatus(sloComposition.Name, sloComposition.Spec.Tartget)
		sloComposition.Status.LastUpdateTime = metav1.Now()
		meta.SetStatusCondition(&sloComposition.Status.Conditions, metav1.Condition{
			Type:   "Available",
			Status: metav1.ConditionFalse,
			Reason: "QueryFailed",
		})
		if err := r.Status().Update(ctx, &sloComposition); err != nil {
			logger.Error(err, "Failed to update SLO status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	budget, sliValue, err := errorbudget.Calculate(sloComposition.Spec.Window, sliBurnRateWindowed, sliErrorRate5m)
	if err != nil {
		logger.Error(err, "unable to calculate error budget", "objective_name", sloComposition.Name)
		sloComposition.Status.ObjectiveComposition = slostatus.BuildUnknownStatus(sloComposition.Name, sloComposition.Spec.Tartget)
		sloComposition.Status.LastUpdateTime = metav1.Now()
		meta.SetStatusCondition(&sloComposition.Status.Conditions, metav1.Condition{
			Type:   "Available",
			Status: metav1.ConditionFalse,
			Reason: "CalculationFailed",
		})
		if err := r.Status().Update(ctx, &sloComposition); err != nil {
			logger.Error(err, "Failed to update SLO status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	var burnRates []burnrate.BurnRate
	for _, preset := range defaultBurnRatePresets {
		sliBurnRateShortQuery := fmt.Sprintf("slok:burn_rate_composition:%s{slo_composition_name=\"%s\",slo_composition_namespace=\"%s\"}", preset.ShortWindow, sloComposition.Name, sloComposition.Namespace)
		sliBurnRateShortLongQuery := fmt.Sprintf("slok:burn_rate_composition:%s{slo_composition_name=\"%s\",slo_composition_namespace=\"%s\"}", preset.LongWindow, sloComposition.Name, sloComposition.Namespace)
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

	status := errorbudget.DetermineStatus(sloComposition.Spec.Tartget, sliValue, budget.PercentRemaining, burnRates)
	sloComposition.Status.ObjectiveComposition = slostatus.BuildSuccessStatus(sloComposition.Name, sloComposition.Spec.Tartget, sliValue, budget, status, slostatus.BuildBurnRateStatuses(burnRates))
	sloComposition.Status.LastUpdateTime = metav1.Now()

	meta.SetStatusCondition(&sloComposition.Status.Conditions, metav1.Condition{
		Type:   "Available",
		Status: metav1.ConditionTrue,
		Reason: "Reconciled",
	})
	if err := r.Status().Update(ctx, &sloComposition); err != nil {
		logger.Error(err, "Failed to update SLOComposition status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled SLOComposition")

	// Requeue after 1 minute
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SLOCompositionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&observabilityv1alpha1.SLOComposition{}).
		Named("slocomposition").
		Complete(r)
}
