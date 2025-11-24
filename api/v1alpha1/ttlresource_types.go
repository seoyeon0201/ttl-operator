/*
Copyright 2025 seoyeon.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TTLResourceSpec defines the desired state of TTLResource.
type TTLResourceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of TTLResource. Edit ttlresource_types.go to remove/update
	// Foo string `json:"foo,omitempty"`

	TTLSeconds int `json:"ttlSeconds"` // TTL 시간 (초)
}

// TTLResourceStatus defines the observed state of TTLResource.
type TTLResourceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	
	Expired bool `json:"expired"` // TTL 시간이 만료되었는지 여부
	CreatedAt metav1.Time  `json:"createdAt"` // 리소스가 실제로 생성된 시각
	ExpiredAt *metav1.Time `json:"expiredAt,omitempty"` // TTL 만료 시각
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// TTLResource is the Schema for the ttlresources API.
type TTLResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TTLResourceSpec   `json:"spec,omitempty"`
	Status TTLResourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TTLResourceList contains a list of TTLResource.
type TTLResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TTLResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TTLResource{}, &TTLResourceList{})
}
