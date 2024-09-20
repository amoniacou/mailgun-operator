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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
					return createdRouter.Status.RouteID != nil && len(*createdRouter.Status.RouteID) > 0
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(*createdRouter.Status.RouteID).ToNot(BeEmpty())
			Expect(createdRouter.Status.MailgunError).To(BeNil())
		})

		It("should fail create of mailgun route", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())

			routerName := "test-router"
			router := &domainv1.Route{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "domain.mydomain.com/v1",
					Kind:       "Route",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      routerName,
					Namespace: namespace,
				},
				Spec: domainv1.RouteSpec{
					Description: "fail",
					Expression:  "match_recipient('.*@example.com')",
					Actions: []string{
						"stop()",
					},
				},
			}
			err := k8sClient.Create(ctx, router)
			Expect(err).ToNot(HaveOccurred())

			doRouterLookup := types.NamespacedName{Name: router.Name, Namespace: namespace}
			createdRouter := &domainv1.Route{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, doRouterLookup, createdRouter)
				if err == nil {
					return createdRouter.Status.MailgunError != nil && len(*createdRouter.Status.MailgunError) > 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(*createdRouter.Status.MailgunError).To(Equal("Unable to create route"))

			err = k8sClient.Delete(ctx, router)
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

		It("should delete mailgun route correctly", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())

			router := newFakeRouter(namespace)

			doRouterLookup := types.NamespacedName{Name: router.Name, Namespace: namespace}
			createdRouter := &domainv1.Route{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doRouterLookup, createdRouter)
				if err == nil {
					return createdRouter.Status.RouteID != nil && len(*createdRouter.Status.RouteID) > 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			err := k8sClient.Delete(ctx, router)
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
					return createdRouter.Status.RouteID != nil && len(*createdRouter.Status.RouteID) > 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(*createdRouter.Status.RouteID).ToNot(BeEmpty())
			Expect(createdRouter.Finalizers).To(ContainElement(routeFinalizer))
			By("Make the delete route to fail")

			mgm.FailRoutes("delete", *createdRouter.Status.RouteID)

			err := k8sClient.Delete(ctx, router)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, doRouterLookup, createdRouter)
				if err == nil {
					return createdRouter.Status.MailgunError != nil && len(*createdRouter.Status.MailgunError) > 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(*createdRouter.Status.MailgunError).To(Equal("Unable to delete route from Mailgun"))
			Expect(createdRouter.Finalizers).ToNot(BeEmpty())
		})

		It("should update route on mailgun", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			routerName := "test-router"
			router := &domainv1.Route{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "domain.mydomain.com/v1",
					Kind:       "Route",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      routerName,
					Namespace: namespace,
				},
				Spec: domainv1.RouteSpec{
					Description: "test-router",
					Expression:  "match_recipient('.*@example.com')",
					Actions: []string{
						"stop()",
					},
				},
			}
			err := k8sClient.Create(ctx, router)
			Expect(err).ToNot(HaveOccurred())

			doRouterLookup := types.NamespacedName{Name: router.Name, Namespace: namespace}
			createdRouter := &domainv1.Route{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, doRouterLookup, createdRouter)
				if err == nil {
					return createdRouter.Status.RouteID != nil && len(*createdRouter.Status.RouteID) > 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(*createdRouter.Status.RouteID).ToNot(BeEmpty())

			createdRouter.Spec.Expression = "match_recipient('.*@updated.com')"

			err = k8sClient.Update(ctx, createdRouter)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Get(ctx, doRouterLookup, createdRouter)
			Expect(err).ToNot(HaveOccurred())
			Expect(createdRouter.Status.MailgunError).To(BeNil())
		})
	})
})
