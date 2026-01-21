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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	"github.com/federicolepera/slok/internal/validation"
)

// nolint:unused
// log is for logging in this package.
var servicelevelobjectivelog = logf.Log.WithName("servicelevelobjective-resource")

// SetupServiceLevelObjectiveWebhookWithManager registers the webhook for ServiceLevelObjective in the manager.
func SetupServiceLevelObjectiveWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&observabilityv1alpha1.ServiceLevelObjective{}).
		WithValidator(&ServiceLevelObjectiveCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-observability-slok-io-v1alpha1-servicelevelobjective,mutating=false,failurePolicy=fail,sideEffects=None,groups=observability.slok.io,resources=servicelevelobjectives,verbs=create;update,versions=v1alpha1,name=vservicelevelobjective-v1alpha1.kb.io,admissionReviewVersions=v1

// ServiceLevelObjectiveCustomValidator struct is responsible for validating the ServiceLevelObjective resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ServiceLevelObjectiveCustomValidator struct {
}

var _ webhook.CustomValidator = &ServiceLevelObjectiveCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ServiceLevelObjective.
func (v *ServiceLevelObjectiveCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	servicelevelobjective, ok := obj.(*observabilityv1alpha1.ServiceLevelObjective)
	if !ok {
		return nil, fmt.Errorf("expected a ServiceLevelObjective object but got %T", obj)
	}
	var admissionWarnings admission.Warnings
	servicelevelobjectivelog.Info("Validation for ServiceLevelObjective upon creation", "name", servicelevelobjective.GetName())
	for _, objective := range servicelevelobjective.Spec.Objectives {
		mismatches := validation.ValidateQueryWindow(objective.Sli.Query, objective.Window)
		if len(mismatches) > 0 {
			servicelevelobjectivelog.Info("WARNING: SLI query window mismatch", "mismatches", mismatches)
			for _, mismatch := range mismatches {
				warningMsg := fmt.Sprintf("SLI query window [%s] does not match objective window [%s] in objective [%s]", mismatch.QueryWindow, mismatch.ObjectiveWindow, objective.Name)
				admissionWarnings = append(admissionWarnings, warningMsg)
			}
		}
	}
	return admissionWarnings, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ServiceLevelObjective.
func (v *ServiceLevelObjectiveCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	servicelevelobjective, ok := newObj.(*observabilityv1alpha1.ServiceLevelObjective)
	if !ok {
		return nil, fmt.Errorf("expected a ServiceLevelObjective object for the newObj but got %T", newObj)
	}
	var admissionWarnings admission.Warnings
	servicelevelobjectivelog.Info("Validation for ServiceLevelObjective upon update", "name", servicelevelobjective.GetName())
	for _, objective := range servicelevelobjective.Spec.Objectives {
		mismatches := validation.ValidateQueryWindow(objective.Sli.Query, objective.Window)
		if len(mismatches) > 0 {
			servicelevelobjectivelog.Info("WARNING: SLI query window mismatch", "mismatches", mismatches)
			for _, mismatch := range mismatches {
				warningMsg := fmt.Sprintf("SLI query window [%s] does not match objective window [%s] in objective [%s]", mismatch.QueryWindow, mismatch.ObjectiveWindow, objective.Name)
				admissionWarnings = append(admissionWarnings, warningMsg)
			}
		}
	}
	return admissionWarnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ServiceLevelObjective.
func (v *ServiceLevelObjectiveCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	servicelevelobjective, ok := obj.(*observabilityv1alpha1.ServiceLevelObjective)
	if !ok {
		return nil, fmt.Errorf("expected a ServiceLevelObjective object but got %T", obj)
	}
	servicelevelobjectivelog.Info("Validation for ServiceLevelObjective upon deletion", "name", servicelevelobjective.GetName())

	return nil, nil
}
