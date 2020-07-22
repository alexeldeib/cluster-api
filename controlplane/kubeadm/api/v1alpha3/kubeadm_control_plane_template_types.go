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
)

// KubeadmControlPlaneTemplateSpec describes the configuration for a set of identically configured clusters.
type KubeadmControlPlaneTemplateSpec struct {
	// Spec is the same as cluster spec but expects templatized infrastructure resources for cloning.
	template KubeadmControlPlaneTemplateResource `json:"template"`
}

type KubeadmControlPlaneTemplateResource struct {
	spec KubeadmControlPlaneSpec `json:"spec"`
}

// KubeadmControlPlaneTemplateStatus describes the status of a set of identically configured clusters.
type KubeadmControlPlaneTemplateStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeadmcontrolplanetemplates,shortName=kcpt,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// KubeadmControlPlaneTemplate is the Schema for the KubeadmControlPlaneTemplates API
type KubeadmControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeadmControlPlaneTemplateSpec   `json:"spec,omitempty"`
	Status KubeadmControlPlaneTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubeadmControlPlaneTemplateList contains a list of KubeadmControlPlaneTemplate
type KubeadmControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeadmControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeadmControlPlaneTemplate{}, &KubeadmControlPlaneTemplateList{})
}
