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
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/external-dns/endpoint"

	domainv1 "github.com/amoniacou/mailgun-operator/api/domain/v1"
	"github.com/amoniacou/mailgun-operator/internal/configuration"
	"github.com/amoniacou/mailgun-operator/internal/utils"
	"github.com/mailgun/mailgun-go/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	domainFinalizer  = "domain.mailgun.com/finalizer"
	endpointOwnerKey = ".metadata.controller"
)

// DomainReconciler reconciles a Domain object
type DomainReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Config   *configuration.Data
}

// +kubebuilder:rbac:groups=domain.mailgun.com,resources=domains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=domain.mailgun.com,resources=domains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=domain.mailgun.com,resources=domains/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=externaldns.k8s.io,resources=dnsendpoints,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *DomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var mailgunDomain = &domainv1.Domain{}

	// lookup for item
	if err := r.Get(ctx, req.NamespacedName, mailgunDomain); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Domain")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	domainName := mailgunDomain.Spec.Domain

	log.V(1).Info("Start to reconcile domain", "domain", domainName)

	mg := r.Config.MailgunClient(domainName)

	// examine DeletionTimestamp to determine if object is under deletion
	if mailgunDomain.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(mailgunDomain, domainFinalizer) {
			controllerutil.AddFinalizer(mailgunDomain, domainFinalizer)
			if err := r.Update(ctx, mailgunDomain); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(mailgunDomain, domainFinalizer) {
			if err := r.deleteDomain(ctx, mailgunDomain, mg); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// If its a new domain we should create it on mailgun and update status
	if len(mailgunDomain.Status.State) == 0 {
		// try to search domain on Mailgun
		log.V(1).Info("Get domain from mailgun", "domain", domainName)
		_, err := mg.GetDomain(ctx, domainName)
		if err == nil {
			errorMgs := "Domain already exists on Mailgun"
			log.WithValues("domain", domainName).Info(errorMgs)
			r.Recorder.Eventf(mailgunDomain, "Warning", "DomainExisted",
				"Domain %s is already exists on mailgun", domainName,
			)
			mailgunDomain.Status.NotManaged = true
			mailgunDomain.Status.State = domainv1.DomainStateFailed
			mailgunDomain.Status.MailgunError = &errorMgs
			if err := r.Status().Update(ctx, mailgunDomain); err != nil {
				log.Error(err, "unable to update Domain status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		log.V(1).Info("Create domain on mailgun", "domain", domainName)
		err = r.createDomain(ctx, mailgunDomain, mg)
		if err != nil {
			log.Error(err, "Unable to create domain on mailgun")
			mailgunDomain.Status.State = domainv1.DomainStateFailed
			mailgunError := err.Error()
			mailgunDomain.Status.MailgunError = &mailgunError
			if err := r.Status().Update(ctx, mailgunDomain); err != nil {
				log.Error(err, "unable to update Domain status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		mailgunDomain.Status.State = domainv1.DomainStateCreated
	}

	// No need to continue if domain already failed or activated
	if mailgunDomain.Status.State == domainv1.DomainStateFailed ||
		mailgunDomain.Status.State == domainv1.DomainStateActivated {
		log.V(1).Info("Domain state check", "domain", domainName, "state", mailgunDomain.Status.State)
		return ctrl.Result{}, nil
	}

	// we should to try create external DNS records if they are not created yet
	if mailgunDomain.Spec.ExternalDNS != nil &&
		*mailgunDomain.Spec.ExternalDNS &&
		!mailgunDomain.Status.DnsEntrypointCreated {
		log.V(1).Info("Create external dns records", "domain", domainName)
		err := r.createExternalDNSEntity(ctx, mailgunDomain)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// try to search domain on Mailgun
	log.V(1).Info("Get domain from mailgun before verification", "domain", domainName)
	resp, err := mg.GetDomain(ctx, domainName)
	if err != nil {
		log.Error(err, "unable to get domain from mailgun", "domain", domainName)
		mailgunDomain.Status.State = domainv1.DomainStateFailed
		errorMessage := "Domain not found on Mailgun"
		mailgunDomain.Status.MailgunError = &errorMessage
		if err := r.Client.Status().Update(ctx, mailgunDomain); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	mailgunDomain.Status.ReceivingDnsRecords = mgDNSRecordsToDnsRecords(resp.ReceivingDNSRecords)
	mailgunDomain.Status.SendingDnsRecords = mgDNSRecordsToDnsRecords(resp.SendingDNSRecords)

	// trying to verify domain on Mailgun
	log.V(1).Info("Call a verifying domain on mailgun", "domain", domainName)
	mailgunStatus, err := mg.VerifyDomain(ctx, domainName)
	if err != nil {
		log.Error(err, "unable to verify domain", "domain", domainName)
		return ctrl.Result{
			RequeueAfter: time.Duration(r.Config.DomainVerifyDuration) * time.Second,
		}, err
	}

	log.V(1).Info("Domain verification result", "domain", domainName, "status", mailgunStatus)

	// set mailgun status
	mailgunDomain.Status.DomainState = mailgunStatus

	// Update last validation time and count
	// Such info for now just to understanding the counts and last validation time
	mailgunDomain.Status.LastDomainValidationTime = &metav1.Time{Time: time.Now()}
	if mailgunDomain.Status.DomainValidationCount == nil {
		mailgunDomain.Status.DomainValidationCount = new(int)
		*mailgunDomain.Status.DomainValidationCount = 0
	}
	*mailgunDomain.Status.DomainValidationCount++

	// Mailgun returns "active" if the domain is verified and ready for sending emails
	if mailgunStatus == "active" {
		log.V(1).Info("Domain is activated on mailgun for sending", "domain", domainName, "status", mailgunStatus)

		// Check if we need to force MX check
		if mailgunDomain.Spec.ForceMXCheck != nil && *mailgunDomain.Spec.ForceMXCheck {
			r.checkMXRecordsAndSetState(ctx, mg, mailgunDomain)
		} else {
			mailgunDomain.Status.State = domainv1.DomainStateActivated
		}
	}

	// Store the status
	if err := r.Status().Update(ctx, mailgunDomain); err != nil {
		log.Error(err, "unable to update Domain status")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Domain is not activated on mailgun - calling for a next tick", "domain", domainName)

	return ctrl.Result{
		RequeueAfter: time.Duration(r.Config.DomainVerifyDuration) * time.Second,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	pred := predicate.GenerationChangedPredicate{}
	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(pred).
		For(&domainv1.Domain{}).
		Complete(r)
}

// export SMTP credentials to a given Secret
func (r *DomainReconciler) exportCredentialsToSecret(ctx context.Context, domain *domainv1.Domain, mg *mailgun.MailgunImpl, login, password string) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Exporting SMTP credentials to a Secret")
	secret := &corev1.Secret{}
	secretLookupKey := types.NamespacedName{Namespace: domain.Namespace, Name: *domain.Spec.ExportSecretName}

	log.V(1).Info("Checking if secret already exists", "secret", *domain.Spec.ExportSecretName)
	err := r.Client.Get(ctx, secretLookupKey, secret)
	// we not get secret by some other reason (not found), we should return an error
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("error getting secret: %w", err)
		} else {
			log.V(1).Info("Creating new secret", "secret", *domain.Spec.ExportSecretName)
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      *domain.Spec.ExportSecretName,
					Namespace: domain.Namespace,
				},
				Data: map[string][]byte{},
			}
		}
	} else {
		log.V(1).Info("Secret already exists", "secret", *domain.Spec.ExportSecretName)
	}

	// default export keys
	passwordKey := "smtp-password"
	loginKey := "smtp-login"

	if domain.Spec.ExportSecretPasswordKey != nil && len(*domain.Spec.ExportSecretPasswordKey) > 0 {
		passwordKey = *domain.Spec.ExportSecretPasswordKey
	}
	if domain.Spec.ExportSecretLoginKey != nil && len(*domain.Spec.ExportSecretLoginKey) > 0 {
		loginKey = *domain.Spec.ExportSecretLoginKey
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}

		secret.Data[passwordKey] = []byte(password)
		secret.Data[loginKey] = []byte(login)
		return nil
	}); err != nil {
		log.Error(err, "unable to create or update secret", "secret", secret.Name)
		return err
	}
	return nil
}

// check the MX records state and set the state of domain
func (r *DomainReconciler) checkMXRecordsAndSetState(ctx context.Context, mg *mailgun.MailgunImpl, mailgunDomain *domainv1.Domain) {
	log := log.FromContext(ctx)
	domainResponse, err := mg.GetDomain(ctx, mailgunDomain.Spec.Domain)
	if err != nil {
		log.Error(err, "unable to get domain from mailgun")
		return
	}

	log.V(1).Info("Receiving response from mailgun", "domain", domainResponse.ReceivingDNSRecords)

	if len(domainResponse.ReceivingDNSRecords) == 0 {
		log.Error(err, "no receiving DNS records for domain in Mailgun response")
		return
	}

	for _, record := range domainResponse.ReceivingDNSRecords {
		log.V(1).Info("Checking MX record", "record", record.RecordType, "value", record.Value, "valid", record.Valid)
		if record.Valid != "valid" {
			return
		}
	}
	log.V(1).Info("All MX records are valid", "domain", mailgunDomain.Spec.Domain)
	mailgunDomain.Status.State = domainv1.DomainStateActivated
}

// create external DNS entity for the domain
func (r *DomainReconciler) createExternalDNSEntity(ctx context.Context, mailgunDomain *domainv1.Domain) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Creating external DNS entity for domain", "domain", mailgunDomain.Spec.Domain)
	log.V(1).Info("Spec:", "domain", mailgunDomain.Spec)
	domainName := mailgunDomain.Spec.Domain
	// create receive records
	dnsEntrypoint := &endpoint.DNSEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mailgunDomain.Name,
			Namespace: mailgunDomain.GetNamespace(),
		},
		Spec: endpoint.DNSEndpointSpec{
			Endpoints: []*endpoint.Endpoint{},
		},
	}

	records := slices.Concat(mailgunDomain.Status.ReceivingDnsRecords, mailgunDomain.Status.SendingDnsRecords)

	mxRecords := []string{}
	for _, record := range records {
		// skip the already valid records. Looks like they already have been created on DNS so no need to create again
		if record.Valid == "valid" {
			continue
		}
		dnsName := record.Name
		target := record.Value
		// sending record
		if record.RecordType == "MX" {
			mxRecords = append(mxRecords, fmt.Sprintf("%s %s", record.Priority, record.Value))
		} else {
			dnsRecordEndpoint := endpoint.NewEndpoint(dnsName, record.RecordType, target)
			dnsEntrypoint.Spec.Endpoints = append(dnsEntrypoint.Spec.Endpoints, dnsRecordEndpoint)
		}
	}

	if len(mxRecords) > 0 {
		dnsRecordEndpoint := endpoint.NewEndpoint(domainName, "MX", mxRecords...)
		dnsEntrypoint.Spec.Endpoints = append(dnsEntrypoint.Spec.Endpoints, dnsRecordEndpoint)
	}

	if len(mailgunDomain.Spec.ExternalDNSRecords) > 0 {
		log.V(1).Info("Adding external DNS records to the domain", "domain", domainName)
		for _, record := range mailgunDomain.Spec.ExternalDNSRecords {
			dnsRecordEndpoint := endpoint.NewEndpoint(record.DNSName, record.RecordType, record.Targets...)
			dnsEntrypoint.Spec.Endpoints = append(dnsEntrypoint.Spec.Endpoints, dnsRecordEndpoint)
		}
		log.V(1).Info("External DNS records added to the domain", "domain", domainName, "entrypoint", dnsEntrypoint.Spec.Endpoints)
	}

	if err := r.Create(ctx, dnsEntrypoint); err != nil {
		log.Error(err, "unable to create DNSEndpoint!", "entrypoint", dnsEntrypoint)
		return err
	}

	mailgunDomain.Status.DnsEntrypointCreated = true
	if err := r.Status().Update(ctx, mailgunDomain); err != nil {
		log.Error(err, "unable to update Domain status")
		return err
	}
	return nil
}

// Create Domain helper
func (r *DomainReconciler) createDomain(ctx context.Context, domain *domainv1.Domain, mg *mailgun.MailgunImpl) error {
	log := log.FromContext(ctx)
	options := mailgun.CreateDomainOptions{}
	if domain.Spec.DKIMKeySize != nil && *domain.Spec.DKIMKeySize > 0 {
		options.DKIMKeySize = *domain.Spec.DKIMKeySize
	}
	if domain.Spec.ForceDKIMAuthority != nil && *domain.Spec.ForceDKIMAuthority {
		options.ForceDKIMAuthority = true
	}
	if domain.Spec.SpamAction != nil && len(*domain.Spec.SpamAction) > 0 {
		options.SpamAction = *domain.Spec.SpamAction
	}
	if domain.Spec.Wildcard != nil && *domain.Spec.Wildcard {
		options.Wildcard = true
	}
	// Generate a random password
	password, err := utils.RandomHex(16)
	if err != nil {
		log.Error(err, "unable to generate random password")
		return err
	}

	options.Password = password

	domainResponse, err := mg.CreateDomain(ctx, domain.Spec.Domain, &options)
	if err != nil {
		log.Error(err, "unable to create mailgun domain", "response", domainResponse)
		return err
	}
	domain.Status.SendingDnsRecords = mgDNSRecordsToDnsRecords(domainResponse.SendingDNSRecords)
	domain.Status.ReceivingDnsRecords = mgDNSRecordsToDnsRecords(domainResponse.ReceivingDNSRecords)
	domain.Status.DomainState = domainResponse.Domain.State
	if domain.Spec.ExportCredentials != nil && *domain.Spec.ExportCredentials {
		err := r.exportCredentialsToSecret(ctx, domain, mg, domainResponse.Domain.SMTPLogin, options.Password)

		if err != nil {
			log.Error(err, "unable to export credentials to secret")
			return err
		}
	}
	if err := r.Status().Update(ctx, domain); err != nil {
		log.Error(err, "unable to update Domain status")
		return err
	}
	return nil
}

// func Delete Domain helper
func (r *DomainReconciler) deleteDomain(ctx context.Context, domain *domainv1.Domain, mg *mailgun.MailgunImpl) error {
	log := log.FromContext(ctx)
	domainName := domain.Spec.Domain
	// our finalizer is present, so lets handle any external dependency
	r.Recorder.Eventf(domain, "Normal", "DeletingDomain",
		"Deleting domain %s from mailgun", domainName,
	)

	// lets get the dns enpoint
	dnsEndpoint := &endpoint.DNSEndpoint{}
	dnsEndpointLookup := types.NamespacedName{
		Name:      domain.Name,
		Namespace: domain.Namespace,
	}
	if err := r.Get(ctx, dnsEndpointLookup, dnsEndpoint); client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to fetch linked Endpoint")
		return err
	}
	if err := r.Delete(ctx, dnsEndpoint); err != nil {
		log.Error(err, "unable to delete linked Endpoint", "endpoint", dnsEndpointLookup)
		errMsg := fmt.Sprintf(
			"Unable to delete DNS endpoint %s: %v",
			domain.Name,
			err,
		)
		domain.Status.MailgunError = &errMsg
		if err := r.Update(ctx, domain); err != nil {
			return err
		}
	} else {
		r.Recorder.Eventf(domain, "Normal", "DeletedEndpoint",
			"Deleted linked Endpoint %s from DNS", dnsEndpointLookup,
		)
	}
	// check if domain is exists
	_, err := mg.GetDomain(ctx, domainName)
	if err != nil {
		log.V(1).Info("Mailgun domain already deleted")
	} else {
		if err := mg.DeleteDomain(ctx, domainName); err != nil {
			// if fail to delete the external dependency here, return with error
			// so that it can be retried.
			log.Error(err, "Unable to delete domain from mailgun")
			r.Recorder.Eventf(domain, "Warning", "DeletingDomainFailed",
				"Deleting domain %s from mailgun is failed", domainName,
			)
			domain.Status.State = domainv1.DomainStateFailed
			errMsg := fmt.Sprintf("Unable to delete domain: %v", err)
			domain.Status.MailgunError = &errMsg
			if err := r.Status().Update(ctx, domain); err != nil {
				return err
			}
			return err
		}
	}

	// remove our finalizer from the list and update it.
	controllerutil.RemoveFinalizer(domain, domainFinalizer)
	if err := r.Update(ctx, domain); err != nil {
		return err
	}
	return nil
}

func mgDNSRecordsToDnsRecords(records []mailgun.DNSRecord) []domainv1.DnsRecord {
	result := make([]domainv1.DnsRecord, len(records))
	for i, record := range records {
		result[i] = domainv1.DnsRecord{
			Name:       record.Name,
			Priority:   record.Priority,
			RecordType: record.RecordType,
			Valid:      record.Valid,
			Value:      record.Value,
		}
	}
	return result
}
