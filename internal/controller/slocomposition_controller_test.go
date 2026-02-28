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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
)

var _ = Describe("SLOComposition Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		slocomposition := &observabilityv1alpha1.SLOComposition{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind SLOComposition")
			err := k8sClient.Get(ctx, typeNamespacedName, slocomposition)
			if err != nil && errors.IsNotFound(err) {
				resource := &observabilityv1alpha1.SLOComposition{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: observabilityv1alpha1.SLOCompositionSpec{
						Target: 99.9,
						Window: "30d",
						Objectives: []observabilityv1alpha1.SLORef{
							{
								Name: "test-slo",
								Ref:  observabilityv1alpha1.SLOObjective{Name: "availability"},
							},
						},
						Composition: observabilityv1alpha1.Composition{
							Type: "AND_MIN",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance SLOComposition")
			resource := &observabilityv1alpha1.SLOComposition{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &SLOCompositionReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: NewMockPrometheusClient(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
