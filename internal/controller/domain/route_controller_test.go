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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	domainv1 "github.com/amoniacou/mailgun-operator/api/domain/v1"
)

var _ = Describe("Route Controller", func() {

	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		duration = time.Second * 10
	)

	Context("When reconciling a resource", func() {
		It("should create mailgun route correctly and store ID of record", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())

			router := newFakeRouter(namespace)

			doRouterLookup := types.NamespacedName{Name: router.Name, Namespace: namespace}
			createdRouter := &domainv1.Route{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doRouterLookup, createdRouter)
				if err == nil {
					return len(createdRouter.Status.RouteID) > 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdRouter.Status.RouteID).Should(Equal("myrouter"))
		})

		It("should delete mailgun route correctly", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())

			router := newFakeRouter(namespace)

			doRouterLookup := types.NamespacedName{Name: router.Name, Namespace: namespace}
			createdRouter := &domainv1.Route{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doRouterLookup, createdRouter)
				if err == nil {
					return len(createdRouter.Status.RouteID) > 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdRouter.Status.RouteID).Should(Equal("myrouter"))

			err := k8sClient.Delete(ctx, createdRouter)
			Expect(err).ToNot(HaveOccurred())

			routerList := domainv1.RouteList{}

			Eventually(func() bool {
				err := k8sClient.List(ctx, &routerList, client.InNamespace(namespace))
				if err == nil {
					return len(routerList.Items) == 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			err = k8sClient.Get(ctx, doRouterLookup, createdRouter)
			Expect(err).To(HaveOccurred())
		})

		It("should not remove finalizer if unable to remove route from mailgun", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())

			router := newFakeRouter(namespace)

			doRouterLookup := types.NamespacedName{Name: router.Name, Namespace: namespace}
			createdRouter := &domainv1.Route{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doRouterLookup, createdRouter)
				if err == nil {
					return len(createdRouter.Status.RouteID) > 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdRouter.Status.RouteID).Should(Equal("myrouter"))

			mgm.FailRoutes("delete")

			err := k8sClient.Delete(ctx, createdRouter)
			Expect(err).ToNot(HaveOccurred())

			routerList := domainv1.RouteList{}

			Eventually(func() bool {
				err := k8sClient.List(ctx, &routerList, client.InNamespace(namespace))
				if err == nil {
					return len(routerList.Items) == 1
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdRouter.Finalizers).ToNot(BeEmpty())
		})
	})
})
