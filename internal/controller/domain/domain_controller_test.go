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
	"fmt"
	"time"

	domainv1 "github.com/amoniacou/mailgun-operator/api/domain/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Domain Controller", func() {

	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		duration = time.Second * 10
	)
	Context("When reconciling a resource", func() {

		It("should create mailgun domain correctly and store DNS records", func() {
			// ctx := context.Background()
			namespace := newFakeNamespace()
			fmt.Printf("namespace name: %s", namespace)
			Expect(namespace).ToNot(BeNil())
			domainName := "example.com"
			doDomain := newDigitalOceanDomain(namespace, domainName)

			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			time.Sleep(10 * time.Second)

			Expect(createdDODomain.Spec.Domain).Should(Equal(domainName))
			Expect(createdDODomain.Status.DomainState).Should(Equal("pending"))
		})
	})
})
