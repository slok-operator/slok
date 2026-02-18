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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	sloklog "github.com/federicolepera/slok/internal/log"
	"github.com/federicolepera/slok/internal/prometheus"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

// SLOCompositionReconciler reconciles a SLOComposition object
type SLOCompositionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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

	desideredRule, _ := prometheus.CreateAggregatedPrometheusRule(sloComposition.Name, sloComposition.Namespace, sloComposition.Spec, sloList)
	existingRule := &monitoringv1.PrometheusRule{}
	existingRule.Name = desideredRule.Name
	existingRule.Namespace = desideredRule.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, existingRule, func() error {
		existingRule.Labels = desideredRule.Labels
		existingRule.Spec = desideredRule.Spec
		return controllerutil.SetControllerReference(&slo, existingRule, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "unable to create or update Prometheus rule", "prometheus_rule", desideredRule.Name)
	}

	
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SLOCompositionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&observabilityv1alpha1.SLOComposition{}).
		Named("slocomposition").
		Complete(r)
}
