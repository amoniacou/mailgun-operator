package v1alpha1

import (
	"sigs.k8s.io/external-dns/endpoint"
)

func init() {
	SchemeBuilder.Register(&endpoint.DNSEndpoint{}, &endpoint.DNSEndpointList{})
}
