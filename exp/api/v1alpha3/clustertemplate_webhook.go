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

// +kubebuilder:webhook:verbs=create;update,path=/validate-exp-cluster-x-k8s-io-v1alpha3-clustertemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=exp.cluster.x-k8s.io,resources=clustertemplates,versions=v1alpha3,name=validation.exp.clustertemplate.cluster.x-k8s.io,sideEffects=None

func (c *ClusterTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(c).
		Complete()
}

var _ webhook.Validator = &ClusterTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (c *ClusterTemplate) ValidateCreate() error {
	return c.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (c *ClusterTemplate) ValidateUpdate(old runtime.Object) error {
	return c.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (c *ClusterTemplate) ValidateDelete() error {
	return nil
}

func (c *ClusterTemplate) validate() error {
	templateSpecPath := field.NewPath("spec", "template", "spec")
	var allErrs field.ErrorList

	if !c.Spec.Template.Spec.ControlPlaneEndpoint.IsZero() {
		allErrs = append(
			allErrs,
			field.Invalid(
				templateSpecPath.Child("controlPlaneEndpoint"),
				c.Spec.Template.Spec.ControlPlaneEndpoint,
				"may not be populated for cluster templates",
			),
		)
	}

	if c.Spec.Template.Spec.Paused {
		allErrs = append(
			allErrs,
			field.Invalid(
				templateSpecPath.Child("controlPlaneEndpoint"),
				c.Spec.Template.Spec.Paused,
				"may not be populated for cluster templates",
			),
		)
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(GroupVersion.WithKind("ClusterTemplate").GroupKind(), c.Name, allErrs)
	}

	return nil
}
