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

// MachineDeploymentTemplateSpec describes the configuration for a set of identically configured MachineDeployments.
type MachineDeploymentTemplateSpec struct {
	// Spec is the same as cluster spec but expects templatized infrastructure resources for cloning.
	Template MachineDeploymentTemplateResource `json:"template"`
}

// MachineDeploymentTemplateResource describes the cloneable content of a cluster
type MachineDeploymentTemplateResource struct {
	Spec clusterv1.MachineDeploymentSpec `json:"spec"`
}

// MachineDeploymentTemplateStatus describes the status of a machinedeployment template.
type MachineDeploymentTemplateStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machinedeploymenttemplates,shortName=mdt,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// MachineDeploymentTemplate is the Schema for the machinedeploymenttemplates API
type MachineDeploymentTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   clusterv1.MachineDeploymentSpec `json:"spec,omitempty"`
	Status MachineDeploymentTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MachineDeploymentTemplateList contains a list of MachineDeploymentTemplate
type MachineDeploymentTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineDeploymentTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachineDeploymentTemplate{}, &MachineDeploymentTemplateList{})
}
