/*
Copyright 2024 Amoniac OU.

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

package domain

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Domain Controller", func() {

	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		duration = time.Second * 10
	)
	Context("When reconciling a resource", func() {

		It("should create mailgun domain correctly", func() {
			// ctx := context.Background()
			namespace := newFakeNamespace()
			domainName := "example.com"
			doDomain := newDigitalOceanDomain(namespace, domainName)
			Expect(doDomain.Spec.Domain).Should(Equal(domainName))

			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.

			// doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			// createdDODomain := &domainv1.Domain{}
			// Eventually(func() bool {
			// 	err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
			// 	return err == nil
			// }, timeout, interval).Should(BeTrue())

			// Expect(createdDODomain.Spec.Domain).Should(Equal(domainName))
		})
	})
})
