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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RouteSpec defines the desired state of Route
type RouteSpec struct {
	// Description of the route
	// +kubebuilder:validation:required
	Description string `json:"description"`
	// Matching expression for the route
	// +kubebuilder:validation:required
	Expression string `json:"expression"`
	// Action to be taken when the route matches an incoming email
	// +kubebuilder:validation:required
	Actions []string `json:"actions"`
	// Priority of route
	// Smaller number indicates higher priority. Higher priority routes are handled first.
	// +optional
	Priority *int `json:"priority"`
}

// RouteStatus defines the observed state of Route
type RouteStatus struct {
	// ID of created route on mailgun
	RouteID *string `json:"route_id,omitempty"`
	// Mailgun error message if any error occurred during route creation or deletion
	MailgunError *string `json:"mailgun_error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Route is the Schema for the routes API
type Route struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RouteSpec   `json:"spec,omitempty"`
	Status RouteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RouteList contains a list of Route
type RouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Route `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Route{}, &RouteList{})
}
