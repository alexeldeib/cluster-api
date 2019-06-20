/*
Copyright 2018 The Kubernetes Authors.

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

package clusterdeployer

import (
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/provider"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/providercomponents"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type factory struct {
}

func NewProviderComponentsStoreFactory() provider.ComponentsStoreFactory {
	return &factory{}
}

func (f *factory) NewFromCoreClientset(clientset ctrlclient.Client) (provider.ComponentsStore, error) {
	return providercomponents.NewFromClientset(clientset)
}
