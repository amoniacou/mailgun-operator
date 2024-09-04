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
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	domainv1 "github.com/amoniacou/mailgun-operator/api/domain/v1"
	"github.com/mailgun/mailgun-go/v4"
	corev1 "k8s.io/api/core/v1"
)

const finalizerName = "domain.mailgun.com/finalizer"

// DomainReconciler reconciles a Domain object
type DomainReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=domain.mailgun.com,resources=domains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=domain.mailgun.com,resources=domains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=domain.mailgun.com,resources=domains/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=list;get;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Domain object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *DomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var mailgunDomain = &domainv1.Domain{}

	// lookup for item
	if err := r.Get(ctx, req.NamespacedName, mailgunDomain); err != nil {
		log.Error(err, "unable to fetch Domain")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	domainName := mailgunDomain.Spec.Domain

	// get mailgun API key from secret
	apiKey, err := r.getDomainAPIKey(ctx, req, mailgunDomain)

	// if no secret than just fail
	if err != nil {
		return ctrl.Result{}, err
	}

	// setup mailgun client
	mg := mailgun.NewMailgun(domainName, apiKey)
	switch mailgunDomain.Spec.APIServer {
	case "EU":
		mg.SetAPIBase(mailgun.APIBaseEU)
	case "US":
		mg.SetAPIBase(mailgun.APIBaseUS)
	default:
		mg.SetAPIBase(mailgunDomain.Spec.APIServer)
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if mailgunDomain.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(mailgunDomain, finalizerName) {
			controllerutil.AddFinalizer(mailgunDomain, finalizerName)
			if err := r.Update(ctx, mailgunDomain); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(mailgunDomain, finalizerName) {
			// our finalizer is present, so lets handle any external dependency
			r.Recorder.Eventf(mailgunDomain, "Normal", "DeletingDomain",
				"Deleting domain %s from mailgun", domainName,
			)
			if err := mg.DeleteDomain(ctx, domainName); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried.
				log.Error(err, "Unable to delete domain from mailgun")
				r.Recorder.Eventf(mailgunDomain, "Warning", "DeletingDomainFailed",
					"Deleting domain %s from mailgun is failed", domainName,
				)
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(mailgunDomain, finalizerName)
			if err := r.Update(ctx, mailgunDomain); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// if its a new record
	fmt.Printf("domain: %v\n", mailgunDomain.Status)
	if len(mailgunDomain.Status.State) == 0 {
		// Set domain as processing
		mailgunDomain.Status.State = domainv1.DomainProcessing

		// Update status
		if err := r.Status().Update(ctx, mailgunDomain); err != nil {
			log.Error(err, "unable to update Domain status")
			return ctrl.Result{}, err
		}

		// try to search domain on Mailgun
		_, err := mg.GetDomain(ctx, domainName)
		if err == nil {
			errorMgs := "Domain already exists on Mailgun"
			log.WithValues("domain", domainName).Info(errorMgs)
			r.Recorder.Eventf(mailgunDomain, "Warning", "DomainExisted",
				"Domain %s is already exists on mailgun", domainName,
			)
			mailgunDomain.Status.NotManaged = true
			mailgunDomain.Status.State = domainv1.DomainFailed
			mailgunDomain.Status.MailgunError = errorMgs
			if err := r.Status().Update(ctx, mailgunDomain); err != nil {
				log.Error(err, "unable to update Domain status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		err = r.createDomain(ctx, mailgunDomain, mg)
		if err != nil {
			log.Error(err, "Unable to create domain on mailgun")
			mailgunDomain.Status.State = domainv1.DomainFailed
			mailgunDomain.Status.MailgunError = err.Error()
			if err := r.Status().Update(ctx, mailgunDomain); err != nil {
				log.Error(err, "unable to update Domain status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		mailgunDomain.Status.State = domainv1.DomainCreated

		// update status with records
		if err := r.Status().Update(ctx, mailgunDomain); err != nil {
			log.Error(err, "unable to update Domain status")
			return ctrl.Result{}, err
		}

		// wait a 10 minutes for creating DNS records and run validation check
		return ctrl.Result{
			RequeueAfter: time.Minute * 10,
		}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&domainv1.Domain{}).
		Complete(r)
}

// Get Domain API key
func (r *DomainReconciler) getDomainAPIKey(
	ctx context.Context, req ctrl.Request, domain *domainv1.Domain,
) (string, error) {
	if len(domain.Spec.SecretName) == 0 {
		return "", errors.New("secret not defined")
	}
	log := log.FromContext(ctx)
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: domain.Spec.SecretName}, &secret); err != nil {
		log.WithValues("domain", domain.Spec.Domain, "secretName", domain.Spec.SecretName).
			Error(err, "Unable to get API key secret")
		return "", err
	}

	if _, ok := secret.Data["api-key"]; !ok {
		err := errors.New("No api-key key inside secret")
		log.WithValues("domain", domain.Spec.Domain, "secretName", domain.Spec.SecretName).
			Error(err, "Unable to get API key secret")
		return "", err
	}

	return string(secret.Data["api-key"]), nil
}

// Create Domain helper
func (r *DomainReconciler) createDomain(ctx context.Context, domain *domainv1.Domain, mg *mailgun.MailgunImpl) error {
	options := mailgun.CreateDomainOptions{}
	if domain.Spec.DKIMKeySize != nil {
		options.DKIMKeySize = *domain.Spec.DKIMKeySize
	}
	if domain.Spec.ForceDKIMAuthority != nil {
		options.ForceDKIMAuthority = *domain.Spec.ForceDKIMAuthority
	}
	domainResponse, err := mg.CreateDomain(ctx, domain.Spec.Domain, &options)
	if err != nil {
		return err
	}
	fmt.Printf("response: %v\n", domainResponse.Domain.State)
	domain.Status.SendingDnsRecords = mgDNSRecordsToDnsRecords(domainResponse.SendingDNSRecords)
	domain.Status.ReceivingDnsRecords = mgDNSRecordsToDnsRecords(domainResponse.ReceivingDNSRecords)
	domain.Status.DomainState = domainResponse.Domain.State
	return nil
}

func mgDNSRecordsToDnsRecords(records []mailgun.DNSRecord) []domainv1.DnsRecord {
	result := make([]domainv1.DnsRecord, len(records))
	for _, record := range records {
		result = append(result, domainv1.DnsRecord{
			Name:       record.Name,
			Priority:   record.Priority,
			RecordType: record.RecordType,
			Valid:      record.Valid,
			Value:      record.Value,
		})
	}
	return result
}
