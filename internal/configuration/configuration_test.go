package configuration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Configutation parsing", func() {
	It("should parse configMap", func() {
		namespace := newFakeNamespace()
		configMap := newConfigMap(namespace)
		newConfig := NewConfiguration()
		newConfig.OperatorNamespace = namespace
		err := newConfig.LoadConfiguration(ctx, k8sClient, configMap.Name, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(newConfig.APIToken).To(Equal("another-valid-token"))
		Expect(newConfig.DomainVerifyDuration).To(Equal(40))
	})

	It("should parse secret", func() {
		namespace := newFakeNamespace()
		secret := newSecret(namespace)
		newConfig := NewConfiguration()
		newConfig.OperatorNamespace = namespace
		err := newConfig.LoadConfiguration(ctx, k8sClient, "", secret.Name)
		Expect(err).ToNot(HaveOccurred())
		Expect(newConfig.APIToken).To(Equal("valid-token"))
		Expect(newConfig.DomainVerifyDuration).To(Equal(30))
	})

	It("should not return error if no configMap or secret is provided", func() {
		namespace := newFakeNamespace()
		newConfig := NewConfiguration()
		newConfig.OperatorNamespace = namespace
		err := newConfig.LoadConfiguration(ctx, k8sClient, "", "")
		Expect(err).ToNot(HaveOccurred())
		// default values
		Expect(newConfig.APIToken).To(Equal(""))
		Expect(newConfig.DomainVerifyDuration).To(Equal(300))
	})

	It("should not return error if no configMap or secret is found", func() {
		namespace := newFakeNamespace()
		newConfig := NewConfiguration()
		newConfig.OperatorNamespace = namespace
		err := newConfig.LoadConfiguration(ctx, k8sClient, "ttt", "ddd")
		Expect(err).ToNot(HaveOccurred())
		// default values
		Expect(newConfig.APIToken).To(Equal(""))
		Expect(newConfig.DomainVerifyDuration).To(Equal(300))
	})

	It("should return error if ConfigMap or Secret is provided but operatorNamespace is not set", func() {
		newConfig := NewConfiguration()
		newConfig.OperatorNamespace = ""
		err := newConfig.LoadConfiguration(ctx, k8sClient, "ttt", "ddd")
		Expect(err).To(HaveOccurred())
	})

	It("should panic if ConfigMap have broken fields", func() {
		namespace := newFakeNamespace()
		newConfig := NewConfiguration()
		newConfig.OperatorNamespace = namespace
		configMap := newFailConfigMap(namespace)
		Expect(func() { newConfig.LoadConfiguration(ctx, k8sClient, configMap.Name, "") }).To(Panic())
	})
})
