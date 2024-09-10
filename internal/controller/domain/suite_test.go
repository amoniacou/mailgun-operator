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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	domainv1 "github.com/amoniacou/mailgun-operator/api/domain/v1"
	entrypointv1alpha "github.com/amoniacou/mailgun-operator/api/external_dns/v1alpha1"
	"github.com/amoniacou/mailgun-operator/internal/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var k8sManager ctrl.Manager
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

var mgm *utils.MailgunMockServer

const (
	validApiToken string = "valid-token"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	testEnv = buildTestEnv()

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = domainv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = entrypointv1alpha.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Cache:  cache.Options{},
	})
	Expect(err).ToNot(HaveOccurred())

	// start reconciler
	err = (&DomainReconciler{
		Client:               k8sManager.GetClient(),
		Scheme:               k8sManager.GetScheme(),
		Recorder:             k8sManager.GetEventRecorderFor("domain-controller"),
		DomainVerifyDuration: time.Second * 5, // In tests we wait 5 seconds
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("start mailgun fake server")
	// start mailgun server
	mgm = utils.NewMailgunServer(validApiToken)
	// utils.PrettyPrint(map[string]interface{}{
	// 	"mg": mgm,
	// })
	fmt.Printf("mailgun mock server url: %s\n", mgm.URL())

	// start manager
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	// stop mailgun mock server
	mgm.Stop()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func buildTestEnv() *envtest.Environment {
	const (
		envUseExistingCluster = "USE_EXISTING_CLUSTER"
	)

	testEnvironment := &envtest.Environment{}

	if os.Getenv(envUseExistingCluster) != "true" {
		By("bootstrapping test environment")
		testEnvironment.BinaryAssetsDirectory = filepath.Join("..", "..", "..", "bin", "k8s",
			fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH))
		testEnvironment.CRDDirectoryPaths = []string{filepath.Join("..", "..", "..", "config", "crd", "bases")}
		// testEnvironment.AttachControlPlaneOutput = true
		testEnvironment.ErrorIfCRDPathMissing = true
	}

	return testEnvironment
}

func newFakeNamespace() string {
	name := rand.String(10)

	namespace := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: name,
		},
	}
	err := k8sClient.Create(context.Background(), namespace)
	Expect(err).ToNot(HaveOccurred())

	return name
}

func newApiKeySecret(namespace string) *corev1.Secret {
	name := "secret-" + rand.String(10)
	secret := &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"api-key": []byte(validApiToken),
		},
	}
	err := k8sClient.Create(context.Background(), secret)
	Expect(err).ToNot(HaveOccurred())

	return secret
}

func newBrokenKeySecret(namespace string) *corev1.Secret {
	name := "secret-" + rand.String(10)
	secret := &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"api-key-broken": []byte(validApiToken),
		},
	}
	err := k8sClient.Create(context.Background(), secret)
	Expect(err).ToNot(HaveOccurred())

	return secret
}

func newDigitalOceanDomain(namespace, domainName string, externalDNS bool) *domainv1.Domain {
	name := "domain-" + rand.String(10)
	secretName := "failedSecret"
	if domainName != "failed-secret.com" {
		secret := newApiKeySecret(namespace)
		secretName = secret.Name
	}
	if domainName == "second-failed-secret.com" {
		secret := newBrokenKeySecret(namespace)
		secretName = secret.Name
	}
	manager := &domainv1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: domainv1.DomainSpec{
			Domain:     domainName,
			SecretName: secretName,
			APIServer:  mgm.URL(),
		},
	}

	if externalDNS {
		manager.Spec.ExternalDNS = &externalDNS
	}

	err := k8sClient.Create(context.Background(), manager)
	Expect(err).ToNot(HaveOccurred())
	return manager
}
