/*
Copyright 2020 The Kubernetes Authors.

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
package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// ClusterTemplateSpec describes the configuration for a set of identically configured clusters.
type ClusterTemplateSpec struct {
	// Spec is the same as cluster spec but expects templatized infrastructure resources for cloning.
	Template ClusterTemplateResource `json:"template"`
}

// ClusterTemplateResource describes the cloneable content of a cluster
type ClusterTemplateResource struct {
	Spec clusterv1.ClusterSpec `json:"spec"`
}

// ClusterTemplateStatus describes the status of a set of identically configured clusters.
type ClusterTemplateStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clustertemplates,shortName=ct,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// ClusterTemplate is the Schema for the clustertemplates API
type ClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterTemplateSpec   `json:"spec,omitempty"`
	Status ClusterTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterTemplateList contains a list of ClusterTemplate
type ClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterTemplate{}, &ClusterTemplateList{})
}
