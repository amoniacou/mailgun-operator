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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
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
					return createdDODomain.Status.State == domainv1.DomainStateCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Spec.Domain).Should(Equal(domainName))
			Expect(createdDODomain.Status.DomainState).Should(Equal("unverified"))
			Expect(createdDODomain.Status.ReceivingDnsRecords).Should(HaveLen(2))
			Expect(createdDODomain.Status.SendingDnsRecords).Should(HaveLen(3))
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
					return createdDODomain.Status.State == domainv1.DomainStateFailed
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Spec.Domain).Should(Equal(domainName))
			Expect(createdDODomain.Status.NotManaged).Should(BeTrue())
			Expect(*createdDODomain.Status.MailgunError).Should(Equal("Domain already exists on Mailgun"))
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
					return createdDODomain.Status.State == domainv1.DomainStateCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Spec.Domain).Should(Equal(domainName))
			Expect(createdDODomain.Status.DomainState).Should(Equal("unverified"))
			Expect(createdDODomain.Status.ReceivingDnsRecords).Should(HaveLen(2))
			Expect(createdDODomain.Status.SendingDnsRecords).Should(HaveLen(3))
			dnsEndpoint := &endpoint.DNSEndpoint{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      createdDODomain.Name,
				Namespace: createdDODomain.Namespace,
			}, dnsEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(dnsEndpoint.Spec.Endpoints).Should(HaveLen(4))
			// mx records
			Expect(dnsEndpoint.Spec.Endpoints[3].RecordType).To(Equal("MX"))
			Expect(dnsEndpoint.Spec.Endpoints[3].DNSName).To(Equal(domainName))
			Expect(dnsEndpoint.Spec.Endpoints[3].Targets).To(Equal(endpoint.Targets{"10 mxa.mailgun.org", "10 mxb.mailgun.org"}))
			Expect(dnsEndpoint.Spec.Endpoints[0].RecordType).To(Equal("TXT"))
			Expect(dnsEndpoint.Spec.Endpoints[0].DNSName).To(Equal(domainName))
			Expect(dnsEndpoint.Spec.Endpoints[0].Targets).To(Equal(endpoint.Targets{"v=spf1 include:mailgun.org ~all"}))
			Expect(dnsEndpoint.Spec.Endpoints[1].RecordType).To(Equal("TXT"))
			Expect(dnsEndpoint.Spec.Endpoints[1].DNSName).To(Equal("d.mail." + domainName))
			Expect(dnsEndpoint.Spec.Endpoints[1].Targets).To(Equal(endpoint.Targets{"k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUA..."}))
			Expect(dnsEndpoint.Spec.Endpoints[2].RecordType).To(Equal("CNAME"))
			Expect(dnsEndpoint.Spec.Endpoints[2].DNSName).To(Equal("email." + domainName))
			Expect(dnsEndpoint.Spec.Endpoints[2].Targets).To(Equal(endpoint.Targets{"mailgun.org"}))
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
					return createdDODomain.Status.State == domainv1.DomainStateCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateCreated))
			By("Activate domain on fake mailgun")

			mgm.ActivateDomain(domainName)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainStateActivated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateActivated))
		})

		It("should create mailgun domain and change state to activated and force checking MX records", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "success-two.com"
			name := "domain-" + rand.String(10)
			forceMXChecks := true
			doDomain := &domainv1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: domainv1.DomainSpec{
					Domain:       domainName,
					ForceMXCheck: &forceMXChecks,
				},
			}

			err := k8sClient.Create(ctx, doDomain)
			Expect(err).ToNot(HaveOccurred())

			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainStateCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateCreated))
			By("Activate domain on fake mailgun")

			mgm.ActivateDomain(domainName)

			Eventually(func() int {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					By("Domain validation count by " + fmt.Sprintf("%d", *createdDODomain.Status.DomainValidationCount))
					return *createdDODomain.Status.DomainValidationCount
				}
				return 0
			}, timeout, interval).Should(And(SatisfyAll(BeNumerically(">=", 2), BeNumerically("<", 5))))
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateCreated))
			By("Activate mx records on fake mailgun")

			mgm.ActivateMXDomain(domainName)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainStateActivated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateActivated))
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
					return createdDODomain.Status.State == domainv1.DomainStateFailed
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateFailed))
			Expect(*createdDODomain.Status.MailgunError).ToNot(BeEmpty())
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
					return createdDODomain.Status.State == domainv1.DomainStateCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateCreated))
			Expect(createdDODomain.Finalizers).ToNot(BeEmpty())

			mgm.FailedDomain(domainName)

			err := k8sClient.Delete(ctx, doDomain)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainStateFailed
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateFailed))
			Expect(*createdDODomain.Status.MailgunError).To(MatchRegexp("UnexpectedResponseError"))
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
					return createdDODomain.Status.State == domainv1.DomainStateCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateCreated))
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

		It("should create domain and export credentials to new secret", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "first-cred.com"
			exportSecretName := "test-secret"
			name := "domain-" + rand.String(10)
			exportCred := true
			doDomain := &domainv1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: domainv1.DomainSpec{
					Domain:            domainName,
					ExportCredentials: &exportCred,
					ExportSecretName:  &exportSecretName,
				},
			}

			err := k8sClient.Create(ctx, doDomain)
			Expect(err).ToNot(HaveOccurred())
			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainStateCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateCreated))
			secretLoolup := types.NamespacedName{Name: exportSecretName, Namespace: namespace}
			createdSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, secretLoolup, createdSecret)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(createdSecret.Data["smtp-login"])).To(Equal("postmaster@first-cred.com"))
			Expect(string(createdSecret.Data["smtp-password"])).ToNot(BeEmpty())
		})

		It("should create domain and export credentials to existing secret", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "second-cred.com"
			exportSecretName := "exists-secret"
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      exportSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"some-key": []byte("some-value"),
				},
			}
			err := k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())
			name := "domain-" + rand.String(10)
			exportCred := true
			doDomain := &domainv1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: domainv1.DomainSpec{
					Domain:            domainName,
					ExportCredentials: &exportCred,
					ExportSecretName:  &exportSecretName,
				},
			}
			err = k8sClient.Create(ctx, doDomain)
			Expect(err).ToNot(HaveOccurred())
			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainStateCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			secretLoolup := types.NamespacedName{Name: exportSecretName, Namespace: namespace}
			existSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, secretLoolup, existSecret)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(existSecret.Data["smtp-login"])).To(Equal("postmaster@second-cred.com"))
			Expect(string(existSecret.Data["smtp-password"])).ToNot(BeEmpty())
			Expect(string(existSecret.Data["some-key"])).To(Equal("some-value"))
		})

		It("should create domain and export credentials to new secret with specific keys", func() {
			namespace := newFakeNamespace()
			Expect(namespace).ToNot(BeNil())
			domainName := "third-cred.com"
			exportSecretName := "test-secret"
			name := "domain-" + rand.String(10)
			exportCred := true
			myLoginKey := "my-login"
			myPasswordKey := "my-password"
			doDomain := &domainv1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: domainv1.DomainSpec{
					Domain:                  domainName,
					ExportCredentials:       &exportCred,
					ExportSecretName:        &exportSecretName,
					ExportSecretLoginKey:    &myLoginKey,
					ExportSecretPasswordKey: &myPasswordKey,
				},
			}

			err := k8sClient.Create(ctx, doDomain)
			Expect(err).ToNot(HaveOccurred())
			doDomainLookup := types.NamespacedName{Name: doDomain.Name, Namespace: namespace}
			createdDODomain := &domainv1.Domain{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, doDomainLookup, createdDODomain)
				if err == nil {
					return createdDODomain.Status.State == domainv1.DomainStateCreated
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Expect(createdDODomain.Status.State).To(Equal(domainv1.DomainStateCreated))
			secretLoolup := types.NamespacedName{Name: exportSecretName, Namespace: namespace}
			createdSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, secretLoolup, createdSecret)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(createdSecret.Data[myLoginKey])).To(Equal("postmaster@third-cred.com"))
			Expect(string(createdSecret.Data[myPasswordKey])).ToNot(BeEmpty())
		})
	})
})
