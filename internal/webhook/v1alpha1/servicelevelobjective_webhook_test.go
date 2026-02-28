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
)

var _ = Describe("ServiceLevelObjective Webhook", func() {
	var (
		obj       *observabilityv1alpha1.ServiceLevelObjective
		oldObj    *observabilityv1alpha1.ServiceLevelObjective
		validator ServiceLevelObjectiveCustomValidator
	)

	BeforeEach(func() {
		obj = &observabilityv1alpha1.ServiceLevelObjective{}
		oldObj = &observabilityv1alpha1.ServiceLevelObjective{}
		validator = ServiceLevelObjectiveCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	Context("When creating or updating ServiceLevelObjective under Validating Webhook", func() {
	})

})
