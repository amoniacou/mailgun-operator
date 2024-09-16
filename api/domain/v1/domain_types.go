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

package v1

import (
	"github.com/mailgun/mailgun-go/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DnsRecord struct {
	Name       string `json:"name,omitempty"`
	Priority   string `json:"priority,omitempty"`
	RecordType string `json:"record_type"`
	Valid      string `json:"valid"`
	Value      string `json:"value"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type WebSchemeType string
type DomainState string
type ExportType string

const (
	HTTP                  WebSchemeType = "http"
	HTTPS                 WebSchemeType = "https"
	DomainStateCreated    DomainState   = "created"
	DomainStateFailed     DomainState   = "failed"
	DomainStateProcessing DomainState   = "processing"
	DomainStateActivated  DomainState   = "activated"
	ExportTypeSMTP        ExportType    = "smtp"
	ExportTypeAPI         ExportType    = "api"
)

// DomainSpec defines the desired state of Domain
type DomainSpec struct {
	// Domain is a domain name which we need to create on Mailgun
	Domain string `json:"domain"`
	// Support for External-DNS
	ExternalDNS *bool `json:"external_dns,omitempty"`
	// See https://documentation.mailgun.com/en/latest/api-domains.html#domains
	WebScheme          *WebSchemeType `json:"web_scheme,omitempty"`
	DKIMKeySize        *int           `json:"dkim_key_size,omitempty"`
	ForceDKIMAuthority *bool          `json:"force_dkim_authority,omitempty"`
	Wildcard           *bool          `json:"wildcard,omitempty"`
	// +kubebuilder:validation:MinItems=0
	// +listType=set
	IPS        []string            `json:"ips,omitempty"`
	SpamAction *mailgun.SpamAction `json:"spam_action,omitempty"`

	// Export SMTP or API credentials to a secret
	ExportCredentials *bool `json:"export_credentials,omitempty"`
	// Export SMTP credentials to a secret
	// +kubebuilder:validation:RequiredIf=ExportCredentials==true
	ExportSecretName *string `json:"export_secret_name,omitempty"`
	// Export secret key for login
	// +kubebuilder:validation:RequiredIf=ExportCredentials==true
	ExportSecretLoginKey *string `json:"export_secret_login_key,omitempty"`
	// Export secret key for password
	// +kubebuilder:validation:RequiredIf=ExportCredentials==true
	ExportSecretPasswordKey *string `json:"export_secret_password_key,omitempty"`

	// Force validation of MX records for receiving mail
	// +optional
	ForceMXCheck *bool `json:"force_mx_check,omitempty"`
}

// DomainStatus defines the observed state of Domain
type DomainStatus struct {
	// Global state of the record
	State DomainState `json:"state"`

	// If domain is not managed by this operator (e.g. created manually)
	NotManaged bool `json:"not_managed,omitempty"`

	// list of DNS records for sending emails
	SendingDnsRecords []DnsRecord `json:"sending_dns_records,omitempty"`

	// list of DNS records for receiving emails
	ReceivingDnsRecords []DnsRecord `json:"receiving_dns_records,omitempty"`

	// State of the domain on Mailgun
	DomainState string `json:"domain_state"`

	// Mailgun errors
	MailgunError *string `json:"mailgun_error,omitempty"`

	// Time when we last time requested a Validation of domain on Mailgun
	LastDomainValidationTime *metav1.Time `json:"last_domain_validation_time,omitempty"`

	// Domain validation counts
	DomainValidationCount *int `json:"validation_count,omitempty"`

	// A pointer to ExternalDNS Entrypoint
	DnsEntrypointCreated bool `json:"dns_entrypoint_created,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Domain is the Schema for the domains API
type Domain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DomainSpec   `json:"spec,omitempty"`
	Status DomainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DomainList contains a list of Domain
type DomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Domain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Domain{}, &DomainList{})
}
