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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// helpers

func makeComposition(compType string, objectives []observabilityv1alpha1.SLORef, params *observabilityv1alpha1.CompositionParams) *observabilityv1alpha1.SLOComposition {
	return &observabilityv1alpha1.SLOComposition{
		ObjectMeta: metav1.ObjectMeta{Name: "test-composition", Namespace: "default"},
		Spec: observabilityv1alpha1.SLOCompositionSpec{
			Target:     99.9,
			Window:     "30d",
			Objectives: objectives,
			Composition: observabilityv1alpha1.Composition{
				Type:   compType,
				Params: params,
			},
		},
	}
}

func weightedParams(routes []observabilityv1alpha1.Route) *observabilityv1alpha1.CompositionParams {
	return &observabilityv1alpha1.CompositionParams{Routes: routes}
}

var _ = Describe("SLOComposition Webhook", func() {
	var validator SLOCompositionCustomValidator

	BeforeEach(func() {
		validator = SLOCompositionCustomValidator{}
	})

	Context("When creating or updating a WEIGHTED_ROUTES SLOComposition", func() {

		It("Should admit a valid composition with unique route names and known aliases", func() {
			obj := makeComposition("WEIGHTED_ROUTES",
				[]observabilityv1alpha1.SLORef{
					{Name: "base", Ref: observabilityv1alpha1.SLOObjective{Name: "base-slo"}},
					{Name: "payments", Ref: observabilityv1alpha1.SLOObjective{Name: "payments-slo"}},
				},
				weightedParams([]observabilityv1alpha1.Route{
					{Name: "no-coupon", Weight: 0.9, Chain: []string{"base", "payments"}},
					{Name: "with-coupon", Weight: 0.1, Chain: []string{"base", "payments"}},
				}),
			)
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny creation when two routes share the same name", func() {
			obj := makeComposition("WEIGHTED_ROUTES",
				[]observabilityv1alpha1.SLORef{
					{Name: "base", Ref: observabilityv1alpha1.SLOObjective{Name: "base-slo"}},
				},
				weightedParams([]observabilityv1alpha1.Route{
					{Name: "main", Weight: 0.5, Chain: []string{"base"}},
					{Name: "main", Weight: 0.5, Chain: []string{"base"}},
				}),
			)
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate route name"))
			Expect(err.Error()).To(ContainSubstring("main"))
		})

		It("Should deny creation when a chain references an alias not in objectives", func() {
			obj := makeComposition("WEIGHTED_ROUTES",
				[]observabilityv1alpha1.SLORef{
					{Name: "base", Ref: observabilityv1alpha1.SLOObjective{Name: "base-slo"}},
				},
				weightedParams([]observabilityv1alpha1.Route{
					{Name: "bad-route", Weight: 1.0, Chain: []string{"base", "ghost"}},
				}),
			)
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown alias"))
			Expect(err.Error()).To(ContainSubstring("ghost"))
		})

		It("Should admit the same valid composition on update", func() {
			obj := makeComposition("WEIGHTED_ROUTES",
				[]observabilityv1alpha1.SLORef{
					{Name: "api", Ref: observabilityv1alpha1.SLOObjective{Name: "api-slo"}},
				},
				weightedParams([]observabilityv1alpha1.Route{
					{Name: "main", Weight: 1.0, Chain: []string{"api"}},
				}),
			)
			_, err := validator.ValidateUpdate(ctx, obj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny update when a chain alias becomes unknown", func() {
			old := makeComposition("WEIGHTED_ROUTES",
				[]observabilityv1alpha1.SLORef{
					{Name: "api", Ref: observabilityv1alpha1.SLOObjective{Name: "api-slo"}},
				},
				weightedParams([]observabilityv1alpha1.Route{
					{Name: "main", Weight: 1.0, Chain: []string{"api"}},
				}),
			)
			updated := makeComposition("WEIGHTED_ROUTES",
				[]observabilityv1alpha1.SLORef{
					{Name: "api", Ref: observabilityv1alpha1.SLOObjective{Name: "api-slo"}},
				},
				weightedParams([]observabilityv1alpha1.Route{
					{Name: "main", Weight: 1.0, Chain: []string{"api", "missing"}},
				}),
			)
			_, err := validator.ValidateUpdate(ctx, old, updated)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing"))
		})
	})

	Context("When creating an AND_MIN SLOComposition", func() {
		It("Should skip WEIGHTED_ROUTES validation entirely", func() {
			obj := makeComposition("AND_MIN",
				[]observabilityv1alpha1.SLORef{
					{Name: "slo-a", Ref: observabilityv1alpha1.SLOObjective{Name: "slo-a"}},
				},
				nil,
			)
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
