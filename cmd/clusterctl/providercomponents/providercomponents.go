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

package providercomponents

import (
	"context"
	"io/ioutil"

	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	configMapName                  = "clusterctl"
	configMapProviderComponentsKey = "provider-components"
)

type Store struct {
	// If present the provider components will be loaded from and saved to this file
	ExplicitPath string
	// If present and ExplicitPath is not present, provider components will be loaded and saved to this store
	Client ctrlclient.Client
}

func NewFromConfigMap(client ctrlclient.Client) (*Store, error) {
	store := Store{
		Client: client,
	}
	return &store, nil
}

func NewFromClientset(client ctrlclient.Client) (*Store, error) {
	return NewFromConfigMap(client)
}

func (pc *Store) Save(providerComponents string) error {
	if pc.ExplicitPath == "" {
		return pc.saveToConfigMap(providerComponents)
	}
	return ioutil.WriteFile(pc.ExplicitPath, []byte(providerComponents), 0644)
}

func (pc *Store) Load() (string, error) {
	if pc.ExplicitPath == "" {
		return pc.loadFromConfigMap()
	}
	return pc.loadFromFile()
}

func (pc *Store) loadFromFile() (string, error) {
	bytes, err := ioutil.ReadFile(pc.ExplicitPath)
	if err != nil {
		return "", errors.Wrapf(err, "error when loading provider components from %q", pc.ExplicitPath)
	}
	return string(bytes), nil
}

func (pc *Store) saveToConfigMap(providerComponents string) error {
	namespacedName := types.NamespacedName{
		Namespace: core.NamespaceDefault,
		Name:      configMapName,
	}
	configMap := &core.ConfigMap{}
	err := pc.Client.Get(context.Background(), namespacedName, configMap)
	if apierrors.IsNotFound(err) {
		configMap = &core.ConfigMap{
			ObjectMeta: meta.ObjectMeta{
				Name: configMapName,
			},
		}
	} else if err != nil {
		return errors.Wrapf(err, "unable to get configmap %q", configMapName)
	}
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}
	configMap.Data[configMapProviderComponentsKey] = providerComponents
	if err == nil {
		err = pc.Client.Update(context.Background(), configMap)
		if err != nil {
			return errors.Wrapf(err, "error updating config map %q", configMapName)
		}
	} else {
		err = pc.Client.Create(context.Background(), configMap)
		if err != nil {
			return errors.Wrapf(err, "error creating config map %q", configMapName)
		}
	}
	return nil
}

func (pc *Store) loadFromConfigMap() (string, error) {
	if pc.ConfigMap == nil {
		return "", errors.New("unable to load config map: need a valid ConfigMapInterface")
	}
	configMap := &core.ConfigMap{}
	err := pc.Client.Get(context.Background(), namespacedName, configMap)
	if err != nil {
		return "", errors.Wrapf(err, "error getting configmap named %q", configMapName)
	}
	providerComponents, ok := configMap.Data[configMapProviderComponentsKey]
	if !ok {
		return "", errors.Errorf("configmap %q does not contain the provider components key %q", configMapName, configMapProviderComponentsKey)
	}
	return providerComponents, nil
}
