//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	v4 "github.com/mailgun/mailgun-go/v4"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DnsRecord) DeepCopyInto(out *DnsRecord) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DnsRecord.
func (in *DnsRecord) DeepCopy() *DnsRecord {
	if in == nil {
		return nil
	}
	out := new(DnsRecord)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Domain) DeepCopyInto(out *Domain) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Domain.
func (in *Domain) DeepCopy() *Domain {
	if in == nil {
		return nil
	}
	out := new(Domain)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Domain) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DomainList) DeepCopyInto(out *DomainList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Domain, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DomainList.
func (in *DomainList) DeepCopy() *DomainList {
	if in == nil {
		return nil
	}
	out := new(DomainList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DomainList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DomainSpec) DeepCopyInto(out *DomainSpec) {
	*out = *in
	if in.ExternalDNS != nil {
		in, out := &in.ExternalDNS, &out.ExternalDNS
		*out = new(bool)
		**out = **in
	}
	if in.WebScheme != nil {
		in, out := &in.WebScheme, &out.WebScheme
		*out = new(WebSchemeType)
		**out = **in
	}
	if in.DKIMKeySize != nil {
		in, out := &in.DKIMKeySize, &out.DKIMKeySize
		*out = new(int)
		**out = **in
	}
	if in.ForceDKIMAuthority != nil {
		in, out := &in.ForceDKIMAuthority, &out.ForceDKIMAuthority
		*out = new(bool)
		**out = **in
	}
	if in.Wildcard != nil {
		in, out := &in.Wildcard, &out.Wildcard
		*out = new(bool)
		**out = **in
	}
	if in.IPS != nil {
		in, out := &in.IPS, &out.IPS
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.SpamAction != nil {
		in, out := &in.SpamAction, &out.SpamAction
		*out = new(v4.SpamAction)
		**out = **in
	}
	if in.ExportCredentials != nil {
		in, out := &in.ExportCredentials, &out.ExportCredentials
		*out = new(bool)
		**out = **in
	}
	if in.ExportSecretName != nil {
		in, out := &in.ExportSecretName, &out.ExportSecretName
		*out = new(string)
		**out = **in
	}
	if in.ExportSecretLoginKey != nil {
		in, out := &in.ExportSecretLoginKey, &out.ExportSecretLoginKey
		*out = new(string)
		**out = **in
	}
	if in.ExportSecretPasswordKey != nil {
		in, out := &in.ExportSecretPasswordKey, &out.ExportSecretPasswordKey
		*out = new(string)
		**out = **in
	}
	if in.ForceMXCheck != nil {
		in, out := &in.ForceMXCheck, &out.ForceMXCheck
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DomainSpec.
func (in *DomainSpec) DeepCopy() *DomainSpec {
	if in == nil {
		return nil
	}
	out := new(DomainSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DomainStatus) DeepCopyInto(out *DomainStatus) {
	*out = *in
	if in.SendingDnsRecords != nil {
		in, out := &in.SendingDnsRecords, &out.SendingDnsRecords
		*out = make([]DnsRecord, len(*in))
		copy(*out, *in)
	}
	if in.ReceivingDnsRecords != nil {
		in, out := &in.ReceivingDnsRecords, &out.ReceivingDnsRecords
		*out = make([]DnsRecord, len(*in))
		copy(*out, *in)
	}
	if in.MailgunError != nil {
		in, out := &in.MailgunError, &out.MailgunError
		*out = new(string)
		**out = **in
	}
	if in.LastDomainValidationTime != nil {
		in, out := &in.LastDomainValidationTime, &out.LastDomainValidationTime
		*out = (*in).DeepCopy()
	}
	if in.DomainValidationCount != nil {
		in, out := &in.DomainValidationCount, &out.DomainValidationCount
		*out = new(int)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DomainStatus.
func (in *DomainStatus) DeepCopy() *DomainStatus {
	if in == nil {
		return nil
	}
	out := new(DomainStatus)
	in.DeepCopyInto(out)
	return out
}
