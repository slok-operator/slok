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
	return 0, fmt.Errorf("no mock value for query: %s", query)
}

func (m *MockPrometheusClient) QuerySLINotNormalized(ctx context.Context, query string) (float64, error) {
	m.QueryCallCount++
	if err, ok := m.SLIErrors[query]; ok {
		return 0, err
	}
	if val, ok := m.SLIValues[query]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("no mock value for query: %s", query)
}

func (m *MockPrometheusClient) CheckConnection(ctx context.Context) error {
	m.ConnectCallCount++
	return m.ConnectionError
}

// sliErrorRateQuery builds the SLI error rate query string matching the controller format.
//
//nolint:unparam // sloNamespace is always "default" in tests but kept for clarity
func sliErrorRateQuery(objectiveName, sloName, sloNamespace string) string {
	return fmt.Sprintf(`slok:sli_error_rate:5m{objective_name="%s",slo_name="%s",slo_namespace="%s"}`, objectiveName, sloName, sloNamespace)
}

// burnRateQuery builds a burn rate query string for a given window.
func burnRateQuery(window, objectiveName, sloName, sloNamespace string) string {
	return fmt.Sprintf(`slok:burn_rate:%s{objective_name="%s",slo_name="%s",slo_namespace="%s"}`, window, objectiveName, sloName, sloNamespace)
}

// setBurnRateValues sets the same burn rate value for all 6 unique windows.
//
//nolint:unparam // sloNamespace is always "default" in tests but kept for clarity
func setBurnRateValues(mock *MockPrometheusClient, objectiveName, sloName, sloNamespace string, value float64) {
	for _, window := range []string{"5m", "1h", "6h", "3d", "7d", "30d"} {
		mock.SLIValues[burnRateQuery(window, objectiveName, sloName, sloNamespace)] = value
	}
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
						Objective: observabilityv1alpha1.Objective{
							Name:   "availability",
							Target: 99.9,
							Window: "30d",
							Sli: observabilityv1alpha1.SLI{
								Query: &observabilityv1alpha1.Query{
									TotalQuery: `sum(rate(http_requests_total[5m]))`,
									ErrorQuery: `sum(rate(http_requests_total{code=~"5.."}[5m]))`,
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
			By("Setting up mock to return SLI error rate producing actual above target with low burn rates")
			// sliErrorRate=0.0005 → actual=99.95 (above target 99.9)
			mockPrometheus.SLIValues[sliErrorRateQuery("availability", resourceName, resourceNamespace)] = 0.0005
			// All burn rates = 0.5 → all below thresholds → met
			setBurnRateValues(mockPrometheus, "availability", resourceName, resourceNamespace, 0.5)

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
			Expect(updatedSLO.Status.Objective.Status).To(Equal("met"))
			Expect(updatedSLO.Status.Objective.Actual).To(Equal(99.95))

			By("Verifying Prometheus was called (2 SLI queries + 4 presets x 2 burn rate = 10)")
			Expect(mockPrometheus.QueryCallCount).To(Equal(10))
			Expect(mockPrometheus.ConnectCallCount).To(Equal(1))
		})

		It("should set violated status when error budget is exhausted", func() {
			By("Setting up mock to return burn rate >= 1 on 30d window (budget exhausted)")
			mockPrometheus.SLIValues[sliErrorRateQuery("availability", resourceName, resourceNamespace)] = 0.005
			// sliBurnRateWindowed (30d) >= 1.0 → budget <= 0 → violated
			setBurnRateValues(mockPrometheus, "availability", resourceName, resourceNamespace, 1.5)

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
			Expect(updatedSLO.Status.Objective.Status).To(Equal("violated"))
		})

		It("should set critical status when 5m/1h burn rate exceeds 14x", func() {
			By("Setting up mock with high burn rate > 14 on short windows, < 1 on 30d (budget > 0)")
			mockPrometheus.SLIValues[sliErrorRateQuery("availability", resourceName, resourceNamespace)] = 0.0005
			// Short-term burn rates = 20, but 30d < 1 so budget > 0
			for _, window := range []string{"5m", "1h", "6h", "3d", "7d"} {
				mockPrometheus.SLIValues[burnRateQuery(window, "availability", resourceName, resourceNamespace)] = 20
			}
			mockPrometheus.SLIValues[burnRateQuery("30d", "availability", resourceName, resourceNamespace)] = 0.5

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
			Expect(updatedSLO.Status.Objective.Status).To(Equal("critical"))
		})

		It("should set degraded status when 1h/6h burn rate exceeds 6x", func() {
			By("Setting up mock with burn rate > 6 but < 14 on short windows, < 1 on 30d")
			mockPrometheus.SLIValues[sliErrorRateQuery("availability", resourceName, resourceNamespace)] = 0.0005
			// Burn rates = 10 on short windows, < 1 on 30d
			for _, window := range []string{"5m", "1h", "6h", "3d", "7d"} {
				mockPrometheus.SLIValues[burnRateQuery(window, "availability", resourceName, resourceNamespace)] = 10
			}
			mockPrometheus.SLIValues[burnRateQuery("30d", "availability", resourceName, resourceNamespace)] = 0.5

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
			Expect(updatedSLO.Status.Objective.Status).To(Equal("degraded"))
		})

		It("should set warning status when 6h/3d burn rate exceeds 1x", func() {
			By("Setting up mock with burn rate > 1 but < 6 on short windows, < 1 on 30d")
			mockPrometheus.SLIValues[sliErrorRateQuery("availability", resourceName, resourceNamespace)] = 0.0005
			// Burn rates = 3 on short windows, < 1 on 30d
			for _, window := range []string{"5m", "1h", "6h", "3d", "7d"} {
				mockPrometheus.SLIValues[burnRateQuery(window, "availability", resourceName, resourceNamespace)] = 3
			}
			mockPrometheus.SLIValues[burnRateQuery("30d", "availability", resourceName, resourceNamespace)] = 0.5

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
			Expect(updatedSLO.Status.Objective.Status).To(Equal("warning"))
		})

		It("should handle Prometheus query errors gracefully", func() {
			By("Setting up mock to return error for SLI error rate query")
			mockPrometheus.SLIErrors[sliErrorRateQuery("availability", resourceName, resourceNamespace)] = fmt.Errorf("prometheus unavailable")

			controllerReconciler := &ServiceLevelObjectiveReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockPrometheus,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// Controller returns early without error when SLI query fails
			Expect(err).NotTo(HaveOccurred())

			// Note: status is NOT updated because the controller returns before
			// r.Status().Update() when the SLI query fails
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

})
