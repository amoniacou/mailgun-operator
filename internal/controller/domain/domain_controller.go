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
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/external-dns/endpoint"

	domainv1 "github.com/amoniacou/mailgun-operator/api/domain/v1"
	"github.com/amoniacou/mailgun-operator/internal/configuration"
	"github.com/mailgun/mailgun-go/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	finalizerName    = "domain.mailgun.com/finalizer"
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

	log.V(1).Info("Start to reconcile domain", "domain", domainName)

	mg := r.Config.MailgunClient(domainName)

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
			if err := r.deleteDomain(ctx, mailgunDomain, mg); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// If its a new domain we should create it on mailgun and update status
	if len(mailgunDomain.Status.State) == 0 {
		// Set domain as processing
		log.V(1).Info("Change status to processing", "domain", domainName)
		mailgunDomain.Status.State = domainv1.DomainProcessing

		// Update status
		if err := r.Status().Update(ctx, mailgunDomain); err != nil {
			log.Error(err, "unable to update Domain status")
			return ctrl.Result{}, err
		}

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
			mailgunDomain.Status.State = domainv1.DomainFailed
			mailgunDomain.Status.MailgunError = errorMgs
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

		if mailgunDomain.Spec.ExternalDNS != nil && *mailgunDomain.Spec.ExternalDNS {
			log.V(1).Info("Create external dns records", "domain", domainName)
			err := r.createExternalDNSEntity(ctx, mailgunDomain)
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{
			RequeueAfter: time.Duration(r.Config.DomainVerifyDuration) * time.Second,
		}, nil
	}

	// its not a new record and a new tick of the loop
	if mailgunDomain.Status.State == domainv1.DomainFailed {
		log.V(1).Info("Domain state failed - no continue", "domain", domainName)
		// if the state is failed and we have an error message, return it
		return ctrl.Result{}, nil
	}

	// verify domain
	log.V(1).Info("Call a verifying domain on mailgun", "domain", domainName)
	status, err := mg.VerifyDomain(ctx, domainName)
	if err != nil {
		log.Error(err, "unable to verify domain", "domain", domainName)
		return ctrl.Result{
			RequeueAfter: time.Duration(r.Config.DomainVerifyDuration) * time.Second,
		}, err
	}

	if status == "active" {
		log.V(1).Info("Domain activated on mailgun", "domain", domainName)
		mailgunDomain.Status.State = domainv1.DomainActivated
		if err := r.Status().Update(ctx, mailgunDomain); err != nil {
			log.Error(err, "unable to update Domain status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Domain is activated on mailgun - calling for a next tick", "domain", domainName)

	return ctrl.Result{
		RequeueAfter: time.Duration(r.Config.DomainVerifyDuration) * time.Second,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&domainv1.Domain{}).
		Complete(r)
}

func (r *DomainReconciler) createExternalDNSEntity(ctx context.Context, mailgunDomain *domainv1.Domain) error {
	log := log.FromContext(ctx)
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

	if err := ctrl.SetControllerReference(mailgunDomain, dnsEntrypoint, r.Scheme); err != nil {
		log.Error(err, "unable to construct new DNSEndpoint", "domain", domainName)
		return err
	}

	if err := r.Create(ctx, dnsEntrypoint); err != nil {
		log.Error(err, "unable to create DNSEndpoint!", "entrypoint", dnsEntrypoint)
		return err
	}
	entrypointRef, err := ref.GetReference(r.Scheme, dnsEntrypoint)
	if err != nil {
		log.Error(err, "unable to make a reference to DNSEntrypoint", "entrypoint", dnsEntrypoint)
		return err
	}

	mailgunDomain.Status.DnsEntrypoint = *entrypointRef
	if err := r.Status().Update(ctx, mailgunDomain); err != nil {
		log.Error(err, "unable to update Domain status")
		return err
	}
	return nil
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
	domain.Status.SendingDnsRecords = mgDNSRecordsToDnsRecords(domainResponse.SendingDNSRecords)
	domain.Status.ReceivingDnsRecords = mgDNSRecordsToDnsRecords(domainResponse.ReceivingDNSRecords)
	domain.Status.DomainState = domainResponse.Domain.State
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

	// if we have a linked dns entrypoint then we should to delete it.
	if len(domain.Status.DnsEntrypoint.Name) > 0 {
		// lets get the dns enpoint
		dnsEndpoint := &endpoint.DNSEndpoint{}
		dnsEndpointLookup := types.NamespacedName{
			Name:      domain.Status.DnsEntrypoint.Name,
			Namespace: domain.Status.DnsEntrypoint.Namespace,
		}
		if err := r.Get(ctx, dnsEndpointLookup, dnsEndpoint); err != nil {
			log.Error(err, "unable to fetch linked Endpoint", "endpoint", domain.Status.DnsEntrypoint)
			return err
		}
		if err := r.Delete(ctx, dnsEndpoint); err != nil {
			log.Error(err, "unable to delete linked Endpoint", "endpoint", dnsEndpointLookup)
			domain.Status.MailgunError = fmt.Sprintf(
				"Unable to delete DNS endpoint %s: %v",
				domain.Status.DnsEntrypoint.Name,
				err,
			)
			if err := r.Update(ctx, domain); err != nil {
				return err
			}
		} else {
			r.Recorder.Eventf(domain, "Normal", "DeletedEndpoint",
				"Deleted linked Endpoint %s from DNS", dnsEndpointLookup,
			)
			domain.Status.DnsEntrypoint = corev1.ObjectReference{}
			if err := r.Status().Update(ctx, domain); err != nil {
				return err
			}
		}
	}
	if err := mg.DeleteDomain(ctx, domainName); err != nil {
		// if fail to delete the external dependency here, return with error
		// so that it can be retried.
		log.Error(err, "Unable to delete domain from mailgun")
		r.Recorder.Eventf(domain, "Warning", "DeletingDomainFailed",
			"Deleting domain %s from mailgun is failed", domainName,
		)
		domain.Status.State = domainv1.DomainFailed
		domain.Status.MailgunError = err.Error()
		if err := r.Status().Update(ctx, domain); err != nil {
			return err
		}
		return err
	}

	// remove our finalizer from the list and update it.
	controllerutil.RemoveFinalizer(domain, finalizerName)
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
