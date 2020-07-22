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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:verbs=create;update,path=/validate-exp-cluster-x-k8s-io-v1alpha3-machinedeploymenttemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=exp.cluster.x-k8s.io,resources=machinedeploymenttemplates,versions=v1alpha3,name=validation.exp.machinedeploymenttemplate.cluster.x-k8s.io,sideEffects=None

func (c *MachineDeploymentTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(c).
		Complete()
}

var _ webhook.Validator = &MachineDeploymentTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (c *MachineDeploymentTemplate) ValidateCreate() error {
	return c.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (c *MachineDeploymentTemplate) ValidateUpdate(old runtime.Object) error {
	return c.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (c *MachineDeploymentTemplate) ValidateDelete() error {
	return nil
}

func (c *MachineDeploymentTemplate) validate() error {
	var allErrs field.ErrorList

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(GroupVersion.WithKind("MachineDeploymentTemplate").GroupKind(), c.Name, allErrs)
	}

	return nil
}
