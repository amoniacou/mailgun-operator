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

	domainv1 "github.com/amoniacou/mailgun-operator/api/domain/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/external-dns/endpoint"
)

var _ = Describe("Domain Controller", func() {

	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		duration = time.Second * 10
	)
	Context("When reconciling a resource", func() {

		It("should create mailgun domain correctly and store DNS records", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "example.com"
			doDomain := newDigitalOceanDomain(namespace, domainName, false)

			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Spec.Domain).Should(Equal(domainName))
			Expect(createdDODomain.Status.DomainState).Should(Equal("unverified"))
			Expect(createdDODomain.Status.ReceivingDnsRecords).Should(HaveLen(2))
			Expect(createdDODomain.Status.SendingDnsRecords).Should(HaveLen(3))
			Expect(createdDODomain.Status.DnsEntrypoint.Name).To(BeEmpty())
		})

		It("should fail to create mailgun domain as its already exists", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "fail.com"
			mgm.AddDomain(domainName)
			doDomain := newDigitalOceanDomain(namespace, domainName, false)
			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainFailed
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Spec.Domain).Should(Equal(domainName))
			Expect(createdDODomain.Status.NotManaged).Should(BeTrue())
			Expect(createdDODomain.Status.MailgunError).Should(Equal("Domain already exists on Mailgun"))
		})

		It("should create mailgun domain, store DNS records and create external DNS entities", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "another.com"
			doDomain := newDigitalOceanDomain(namespace, domainName, true)

			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Spec.Domain).Should(Equal(domainName))
			Expect(createdDODomain.Status.DomainState).Should(Equal("unverified"))
			Expect(createdDODomain.Status.ReceivingDnsRecords).Should(HaveLen(2))
			Expect(createdDODomain.Status.SendingDnsRecords).Should(HaveLen(3))
			Expect(createdDODomain.Status.DnsEntrypoint).ToNot(BeNil())
			dnsEndpoint := &endpoint.DNSEndpoint{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      createdDODomain.Status.DnsEntrypoint.Name,
				Namespace: createdDODomain.Status.DnsEntrypoint.Namespace,
			}, dnsEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(dnsEndpoint.Spec.Endpoints).Should(HaveLen(5))
			// mx records
			Expect(dnsEndpoint.Spec.Endpoints[0].RecordType).To(Equal("MX"))
			Expect(dnsEndpoint.Spec.Endpoints[0].DNSName).To(Equal(domainName))
			Expect(dnsEndpoint.Spec.Endpoints[0].Targets).To(Equal(endpoint.Targets{"10 mxa.mailgun.org"}))
			Expect(dnsEndpoint.Spec.Endpoints[1].RecordType).To(Equal("MX"))
			Expect(dnsEndpoint.Spec.Endpoints[1].DNSName).To(Equal(domainName))
			Expect(dnsEndpoint.Spec.Endpoints[1].Targets).To(Equal(endpoint.Targets{"10 mxb.mailgun.org"}))
			Expect(dnsEndpoint.Spec.Endpoints[2].RecordType).To(Equal("TXT"))
			Expect(dnsEndpoint.Spec.Endpoints[2].DNSName).To(Equal(domainName))
			Expect(dnsEndpoint.Spec.Endpoints[2].Targets).To(Equal(endpoint.Targets{"v=spf1 include:mailgun.org ~all"}))
			Expect(dnsEndpoint.Spec.Endpoints[3].RecordType).To(Equal("TXT"))
			Expect(dnsEndpoint.Spec.Endpoints[3].DNSName).To(Equal("d.mail." + domainName))
			Expect(dnsEndpoint.Spec.Endpoints[3].Targets).To(Equal(endpoint.Targets{"k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUA..."}))
			Expect(dnsEndpoint.Spec.Endpoints[4].RecordType).To(Equal("CNAME"))
			Expect(dnsEndpoint.Spec.Endpoints[4].DNSName).To(Equal("email." + domainName))
			Expect(dnsEndpoint.Spec.Endpoints[4].Targets).To(Equal(endpoint.Targets{"mailgun.org"}))
		})

		It("should create mailgun domain and change state to activated", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "success.com"
			doDomain := newDigitalOceanDomain(namespace, domainName, true)

			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainCreated))
			By("Activate domain on fake mailgun")

			mgm.ActivateDomain(domainName)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainActivated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainActivated))
		})

		It("should not be able to create mailgun domain if mailgun returns error", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "failed.com"
			mgm.FailedDomain(domainName)
			doDomain := newDigitalOceanDomain(namespace, domainName, true)

			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainFailed
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainFailed))
			Expect(createdDODomain.Status.MailgunError).ToNot(BeEmpty())
		})

		It("should fail domain if we unable to open secret", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "failed-secret.com"
			doDomain := newDigitalOceanDomain(namespace, domainName, true)
			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainFailed
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainFailed))
			Expect(createdDODomain.Status.MailgunError).To(Equal("Secret \"failedSecret\" not found"))
		})

		It("should fail if secret exist, but no api key", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "second-failed-secret.com"
			doDomain := newDigitalOceanDomain(namespace, domainName, true)
			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainFailed
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainFailed))
			Expect(createdDODomain.Status.MailgunError).To(Equal("no api-key key inside secret"))
		})

		It("should not remove finalizer if unable to remove domain from mailgun", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "finalizer-fail.com"
			doDomain := newDigitalOceanDomain(namespace, domainName, true)
			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainCreated))
			Expect(createdDODomain.Finalizers).ToNot(BeEmpty())

			mgm.DeleteDomain(domainName)

			err := k8sClient.Delete(ctx, doDomain)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainFailed
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainFailed))
			Expect(createdDODomain.Status.MailgunError).To(MatchRegexp("UnexpectedResponseError"))
			Expect(createdDODomain.Finalizers).ToNot(BeEmpty())
		})

		It("should remove domain completely", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "full-finalizer.com"
			doDomain := newDigitalOceanDomain(namespace, domainName, true)
			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainCreated))
			Expect(createdDODomain.Finalizers).ToNot(BeEmpty())

			err := k8sClient.Delete(ctx, doDomain)
			Expect(err).ToNot(HaveOccurred())

			domainList := domainv1.DomainList{}

			Eventually(func() bool {
				err := k8sClient.List(ctx, &domainList, client.InNamespace(namespace))
				if err == nil {
					return len(domainList.Items) == 0
				}
				return false
			}, timeout, interval).Should(BeTrue())

			err = k8sClient.Get(ctx, doDomainLookup, createdDODomain)
			Expect(err).To(HaveOccurred())
		})

	})
})
