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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	observabilityv1alpha1 "github.com/federicolepera/slok/api/v1alpha1"
)

// MockPrometheusClient implements prometheus.PrometheusClient for testing
type MockPrometheusClient struct {
	SLIValues        map[string]float64
	SLIErrors        map[string]error
	ConnectionError  error
	QueryCallCount   int
	ConnectCallCount int
}

func NewMockPrometheusClient() *MockPrometheusClient {
	return &MockPrometheusClient{
		SLIValues: make(map[string]float64),
		SLIErrors: make(map[string]error),
	}
}

func (m *MockPrometheusClient) QuerySLI(ctx context.Context, query string) (float64, error) {
	m.QueryCallCount++
	if err, ok := m.SLIErrors[query]; ok {
		return 0, err
	}
	if val, ok := m.SLIValues[query]; ok {
		return val, nil
	}
	return 99.9, nil // Default value
}

func (m *MockPrometheusClient) CheckConnection(ctx context.Context) error {
	m.ConnectCallCount++
	return m.ConnectionError
}

var _ = Describe("ServiceLevelObjective Controller", func() {
	const (
		resourceName      = "test-slo"
		resourceNamespace = "default"
	)

	ctx := context.Background()

	typeNamespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: resourceNamespace,
	}

	Context("When reconciling a valid SLO resource", func() {
		var mockPrometheus *MockPrometheusClient

		BeforeEach(func() {
			mockPrometheus = NewMockPrometheusClient()

			By("Creating the custom resource for the Kind ServiceLevelObjective")
			slo := &observabilityv1alpha1.ServiceLevelObjective{}
			err := k8sClient.Get(ctx, typeNamespacedName, slo)
			if err != nil && errors.IsNotFound(err) {
				resource := &observabilityv1alpha1.ServiceLevelObjective{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: observabilityv1alpha1.ServiceLevelObjectiveSpec{
						DisplayName: "Test SLO for API Availability",
						Objectives: []observabilityv1alpha1.Objective{
							{
								Name:   "availability",
								Target: 99.9,
								Window: "30d",
								Sli: observabilityv1alpha1.SLI{
									Query: "sum(rate(http_requests_total{code=~\"2..\"}[5m])) / sum(rate(http_requests_total[5m])) * 100",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance ServiceLevelObjective")
			resource := &observabilityv1alpha1.ServiceLevelObjective{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource with met status", func() {
			By("Setting up mock to return SLI value above target")
			mockPrometheus.SLIValues["sum(rate(http_requests_total{code=~\"2..\"}[5m])) / sum(rate(http_requests_total[5m])) * 100"] = 99.95

			By("Reconciling the created resource")
			controllerReconciler := &ServiceLevelObjectiveReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockPrometheus,
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying the status was updated")
			updatedSLO := &observabilityv1alpha1.ServiceLevelObjective{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedSLO)).To(Succeed())
			Expect(updatedSLO.Status.Objectives).To(HaveLen(1))
			Expect(updatedSLO.Status.Objectives[0].Status).To(Equal("met"))
			Expect(updatedSLO.Status.Objectives[0].Actual).To(Equal(99.95))

			By("Verifying Prometheus was called")
			Expect(mockPrometheus.QueryCallCount).To(Equal(1))
			Expect(mockPrometheus.ConnectCallCount).To(Equal(1))
		})

		It("should set violated status when SLI is below target", func() {
			By("Setting up mock to return SLI value below target")
			mockPrometheus.SLIValues["sum(rate(http_requests_total{code=~\"2..\"}[5m])) / sum(rate(http_requests_total[5m])) * 100"] = 99.5

			controllerReconciler := &ServiceLevelObjectiveReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockPrometheus,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			updatedSLO := &observabilityv1alpha1.ServiceLevelObjective{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedSLO)).To(Succeed())
			Expect(updatedSLO.Status.Objectives[0].Status).To(Equal("violated"))
		})

		It("should set at-risk status when error budget is low", func() {
			By("Setting up mock to return SLI value that leaves < 10% error budget")
			// Target is 99.9%, so error budget is 0.1%
			// If actual is 99.91%, consumed is 0.09%, remaining is 0.01% = 10% of budget
			// To get < 10% remaining, actual needs to be slightly below 99.91%
			mockPrometheus.SLIValues["sum(rate(http_requests_total{code=~\"2..\"}[5m])) / sum(rate(http_requests_total[5m])) * 100"] = 99.905

			controllerReconciler := &ServiceLevelObjectiveReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockPrometheus,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			updatedSLO := &observabilityv1alpha1.ServiceLevelObjective{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedSLO)).To(Succeed())
			Expect(updatedSLO.Status.Objectives[0].Status).To(Equal("at-risk"))
		})

		It("should handle Prometheus query errors gracefully", func() {
			By("Setting up mock to return error for query")
			mockPrometheus.SLIErrors["sum(rate(http_requests_total{code=~\"2..\"}[5m])) / sum(rate(http_requests_total[5m])) * 100"] = fmt.Errorf("prometheus unavailable")

			controllerReconciler := &ServiceLevelObjectiveReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockPrometheus,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			updatedSLO := &observabilityv1alpha1.ServiceLevelObjective{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedSLO)).To(Succeed())
			Expect(updatedSLO.Status.Objectives[0].Status).To(Equal("unknown"))
		})

		It("should return error when Prometheus connection fails", func() {
			By("Setting up mock to fail connection check")
			mockPrometheus.ConnectionError = fmt.Errorf("connection refused")

			controllerReconciler := &ServiceLevelObjectiveReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockPrometheus,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})
	})

	Context("When reconciling a non-existent resource", func() {
		It("should not return an error", func() {
			mockPrometheus := NewMockPrometheusClient()
			controllerReconciler := &ServiceLevelObjectiveReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockPrometheus,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-slo",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling an SLO with multiple objectives", func() {
		const multiObjResourceName = "multi-objective-slo"

		multiObjNamespacedName := types.NamespacedName{
			Name:      multiObjResourceName,
			Namespace: resourceNamespace,
		}

		BeforeEach(func() {
			By("Creating an SLO with multiple objectives")
			slo := &observabilityv1alpha1.ServiceLevelObjective{}
			err := k8sClient.Get(ctx, multiObjNamespacedName, slo)
			if err != nil && errors.IsNotFound(err) {
				resource := &observabilityv1alpha1.ServiceLevelObjective{
					ObjectMeta: metav1.ObjectMeta{
						Name:      multiObjResourceName,
						Namespace: resourceNamespace,
					},
					Spec: observabilityv1alpha1.ServiceLevelObjectiveSpec{
						DisplayName: "Multi-Objective SLO",
						Objectives: []observabilityv1alpha1.Objective{
							{
								Name:   "availability",
								Target: 99.9,
								Window: "30d",
								Sli: observabilityv1alpha1.SLI{
									Query: "availability_query",
								},
							},
							{
								Name:   "latency",
								Target: 95.0,
								Window: "7d",
								Sli: observabilityv1alpha1.SLI{
									Query: "latency_query",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the multi-objective SLO")
			resource := &observabilityv1alpha1.ServiceLevelObjective{}
			err := k8sClient.Get(ctx, multiObjNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should reconcile all objectives independently", func() {
			mockPrometheus := NewMockPrometheusClient()
			mockPrometheus.SLIValues["availability_query"] = 99.95 // met
			mockPrometheus.SLIValues["latency_query"] = 90.0       // violated

			controllerReconciler := &ServiceLevelObjectiveReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockPrometheus,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: multiObjNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			updatedSLO := &observabilityv1alpha1.ServiceLevelObjective{}
			Expect(k8sClient.Get(ctx, multiObjNamespacedName, updatedSLO)).To(Succeed())
			Expect(updatedSLO.Status.Objectives).To(HaveLen(2))

			// Find objectives by name
			var availabilityStatus, latencyStatus *observabilityv1alpha1.ObjectiveStatus
			for i := range updatedSLO.Status.Objectives {
				switch updatedSLO.Status.Objectives[i].Name {
				case "availability":
					availabilityStatus = &updatedSLO.Status.Objectives[i]
				case "latency":
					latencyStatus = &updatedSLO.Status.Objectives[i]
				}
			}

			Expect(availabilityStatus).NotTo(BeNil())
			Expect(availabilityStatus.Status).To(Equal("met"))

			Expect(latencyStatus).NotTo(BeNil())
			Expect(latencyStatus.Status).To(Equal("violated"))

			By("Verifying both queries were made")
			Expect(mockPrometheus.QueryCallCount).To(Equal(2))
		})
	})
})
