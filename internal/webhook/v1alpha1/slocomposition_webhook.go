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
)

var slocompositionlog = logf.Log.WithName("slocomposition-resource")

// SetupSLOCompositionWebhookWithManager registers the webhook for SLOComposition in the manager.
func SetupSLOCompositionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&observabilityv1alpha1.SLOComposition{}).
		WithValidator(&SLOCompositionCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-observability-slok-io-v1alpha1-slocomposition,mutating=false,failurePolicy=fail,sideEffects=None,groups=observability.slok.io,resources=slocompositions,verbs=create;update,versions=v1alpha1,name=vslocomposition-v1alpha1.kb.io,admissionReviewVersions=v1

// SLOCompositionCustomValidator struct is responsible for validating the SLOComposition resource
// when it is created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type SLOCompositionCustomValidator struct{}

var _ webhook.CustomValidator = &SLOCompositionCustomValidator{}

// ValidateCreate implements webhook.CustomValidator.
func (v *SLOCompositionCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	composition, ok := obj.(*observabilityv1alpha1.SLOComposition)
	if !ok {
		return nil, fmt.Errorf("expected a SLOComposition object but got %T", obj)
	}
	slocompositionlog.Info("Validation for SLOComposition upon creation", "name", composition.GetName())

	return nil, validateSLOComposition(composition)
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *SLOCompositionCustomValidator) ValidateUpdate(_ context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	composition, ok := newObj.(*observabilityv1alpha1.SLOComposition)
	if !ok {
		return nil, fmt.Errorf("expected a SLOComposition object for the newObj but got %T", newObj)
	}
	slocompositionlog.Info("Validation for SLOComposition upon update", "name", composition.GetName())

	return nil, validateSLOComposition(composition)
}

// ValidateDelete implements webhook.CustomValidator.
func (v *SLOCompositionCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	composition, ok := obj.(*observabilityv1alpha1.SLOComposition)
	if !ok {
		return nil, fmt.Errorf("expected a SLOComposition object but got %T", obj)
	}
	slocompositionlog.Info("Validation for SLOComposition upon deletion", "name", composition.GetName())

	return nil, nil
}

// validateSLOComposition runs all validation checks for create and update.
func validateSLOComposition(composition *observabilityv1alpha1.SLOComposition) error {
	if composition.Spec.Composition.Type != "WEIGHTED_ROUTES" {
		return nil
	}
	if composition.Spec.Composition.Params == nil {
		return nil
	}

	routes := composition.Spec.Composition.Params.Routes

	// Check 1: route names must be unique.
	routeNames := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if _, seen := routeNames[route.Name]; seen {
			return fmt.Errorf("duplicate route name %q: route names must be unique", route.Name)
		}
		routeNames[route.Name] = struct{}{}
	}

	// Check 2: every alias referenced in a chain must exist in objectives.
	objectiveAliases := make(map[string]struct{}, len(composition.Spec.Objectives))
	for _, obj := range composition.Spec.Objectives {
		objectiveAliases[obj.Name] = struct{}{}
	}
	for _, route := range routes {
		for _, alias := range route.Chain {
			if _, ok := objectiveAliases[alias]; !ok {
				return fmt.Errorf("route %q references unknown alias %q: alias must match an entry in objectives", route.Name, alias)
			}
		}
	}

	return nil
}
