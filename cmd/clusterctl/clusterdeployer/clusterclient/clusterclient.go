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

package clusterclient

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // nolint
	tcmd "k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/util"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultAPIServerPort        = "443"
	retryIntervalKubectlApply   = 10 * time.Second
	retryIntervalResourceReady  = 10 * time.Second
	retryIntervalResourceDelete = 10 * time.Second
	timeoutKubectlApply         = 15 * time.Minute
	timeoutResourceReady        = 15 * time.Minute
	timeoutMachineReady         = 30 * time.Minute
	timeoutResourceDelete       = 15 * time.Minute
	machineClusterLabelName     = "cluster.k8s.io/cluster-name"
)

const (
	TimeoutMachineReady = "CLUSTER_API_MACHINE_READY_TIMEOUT"
)

// Provides interaction with a cluster
type Client interface {
	Apply(string) error
	Close() error
	CreateClusterObject(*clusterv1.Cluster) error
	CreateMachineClass(*clusterv1.MachineClass) error
	CreateMachineDeployments([]*clusterv1.MachineDeployment, string) error
	CreateMachineSets([]*clusterv1.MachineSet, string) error
	CreateMachines([]*clusterv1.Machine, string) error
	Delete(string) error
	DeleteClusters(string) error
	DeleteNamespace(string) error
	DeleteMachineClasses(string) error
	DeleteMachineClass(namespace, name string) error
	DeleteMachineDeployments(string) error
	DeleteMachineSets(string) error
	DeleteMachines(string) error
	ForceDeleteCluster(namespace, name string) error
	ForceDeleteMachine(namespace, name string) error
	ForceDeleteMachineSet(namespace, name string) error
	ForceDeleteMachineDeployment(namespace, name string) error
	EnsureNamespace(string) error
	GetClusters(string) ([]*clusterv1.Cluster, error)
	GetCluster(string, string) (*clusterv1.Cluster, error)
	GetContextNamespace() string
	GetMachineClasses(namespace string) ([]*clusterv1.MachineClass, error)
	GetMachineDeployment(namespace, name string) (*clusterv1.MachineDeployment, error)
	GetMachineDeploymentsForCluster(*clusterv1.Cluster) ([]*clusterv1.MachineDeployment, error)
	GetMachineDeployments(string) ([]*clusterv1.MachineDeployment, error)
	GetMachineSet(namespace, name string) (*clusterv1.MachineSet, error)
	GetMachineSets(namespace string) ([]*clusterv1.MachineSet, error)
	GetMachineSetsForCluster(*clusterv1.Cluster) ([]*clusterv1.MachineSet, error)
	GetMachineSetsForMachineDeployment(*clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error)
	GetMachines(namespace string) ([]*clusterv1.Machine, error)
	GetMachinesForCluster(*clusterv1.Cluster) ([]*clusterv1.Machine, error)
	GetMachinesForMachineSet(*clusterv1.MachineSet) ([]*clusterv1.Machine, error)
	ScaleStatefulSet(namespace, name string, scale int32) error
	WaitForClusterV1alpha1Ready() error
	UpdateClusterObjectEndpoint(string, string, string) error
	WaitForResourceStatuses() error
}

type client struct {
	clientSet       clientset.Interface
	kubeconfigFile  string
	configOverrides tcmd.ConfigOverrides
	closeFn         func() error
}

// New creates and returns a Client, the kubeconfig argument is expected to be the string representation
// of a valid kubeconfig.
func New(kubeconfig string) (*client, error) { //nolint
	f, err := createTempFile(kubeconfig)
	if err != nil {
		return nil, err
	}
	defer ifErrRemove(&err, f)
	c, err := NewFromDefaultSearchPath(f, clientcmd.NewConfigOverrides())
	if err != nil {
		return nil, err
	}
	c.closeFn = c.removeKubeconfigFile
	return c, nil
}

func (c *client) removeKubeconfigFile() error {
	return os.Remove(c.kubeconfigFile)
}

func (c *client) EnsureNamespace(namespaceName string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}

	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	namespace := apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}
	err = clientset.Create(context.Background(), &namespace)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (c *client) ScaleStatefulSet(ns string, name string, scale int32) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}

	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	var ss appsv1.StatefulSet
	err = clientset.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &ss)
	if err != nil {
		// IsNotFound would be a real error here, since we are only trying to scale.
		return err
	}
	ss.Spec.Replicas = &scale
	err = clientset.Update(context.Background(), &ss)
	if err != nil {
		return err
	}
	return nil
}

func (c *client) DeleteNamespace(namespaceName string) error {
	if namespaceName == apiv1.NamespaceDefault {
		return nil
	}
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}

	ns := apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}

	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	err = clientset.Delete(context.Background(), &ns)

	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// NewFromDefaultSearchPath creates and returns a Client.  The kubeconfigFile argument is expected to be the path to a
// valid kubeconfig file.
func NewFromDefaultSearchPath(kubeconfigFile string, overrides tcmd.ConfigOverrides) (*client, error) { //nolint
	c, err := clientcmd.NewClusterAPIClientForDefaultSearchPath(kubeconfigFile, overrides)
	if err != nil {
		return nil, err
	}

	return &client{
		kubeconfigFile:  kubeconfigFile,
		clientSet:       c,
		configOverrides: overrides,
	}, nil
}

// Close frees resources associated with the cluster client
func (c *client) Close() error {
	if c.closeFn != nil {
		return c.closeFn()
	}
	return nil
}

func (c *client) Delete(manifest string) error {
	return c.kubectlDelete(manifest)
}

func (c *client) Apply(manifest string) error {
	return c.waitForKubectlApply(manifest)
}

func (c *client) GetContextNamespace() string {
	if c.configOverrides.Context.Namespace == "" {
		return apiv1.NamespaceDefault
	}
	return c.configOverrides.Context.Namespace
}

func (c *client) GetCluster(name, ns string) (*clusterv1.Cluster, error) {
	clustersInNamespace, err := c.GetClusters(ns)
	if err != nil {
		return nil, err
	}
	var cluster *clusterv1.Cluster
	for _, nc := range clustersInNamespace {
		if nc.Name == name {
			cluster = nc
			break
		}
	}
	return cluster, nil
}

// ForceDeleteCluster removes the finalizer for a Cluster prior to deleting, this is used during pivot
func (c *client) ForceDeleteCluster(namespace, name string) error {
	cluster, err := c.clientSet.ClusterV1alpha1().Clusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "error getting cluster %s/%s", namespace, name)
	}

	cluster.ObjectMeta.SetFinalizers([]string{})

	if _, err := c.clientSet.ClusterV1alpha1().Clusters(namespace).Update(cluster); err != nil {
		return errors.Wrapf(err, "error removing finalizer on cluster %s/%s", namespace, name)
	}

	if err := c.clientSet.ClusterV1alpha1().Clusters(namespace).Delete(name, &metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "error deleting cluster %s/%s", namespace, name)
	}

	return nil
}

func (c *client) GetClusters(namespace string) ([]*clusterv1.Cluster, error) {
	clusters := &clusterv1.ClusterList{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return []*clusterv1.Cluster{}, errors.Wrap(err, "error creating config for core clientset")
	}

	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	opts := &ctrlclient.ListOptions{
		Namespace: namespace,
	}
	err = clientset.List(context.Background(), clusters, ctrlclient.UseListOptions(opts))
	if err != nil {
		return nil, errors.Wrapf(err, "error listing cluster objects in namespace %q", namespace)
	}
	var clusterList []*clusterv1.Cluster
	for i := 0; i < len(clusters.Items); i++ {
		clusterList = append(clusterList, &clusters.Items[i])
	}

	return clusterList, nil
}

func (c *client) GetMachineClasses(namespace string) ([]*clusterv1.MachineClass, error) {
	machineClasses := &clusterv1.MachineClassList{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return []*clusterv1.MachineClass{}, errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	err = clientset.List(context.Background(), machineClasses)
	if err != nil {
		return nil, errors.Wrapf(err, "error listing machine class objects in namespace %q", namespace)
	}

	var machineClassesList []*clusterv1.MachineClass
	for i := 0; i < len(machineClasses.Items); i++ {
		machineClassesList = append(machineClassesList, &(machineClasses.Items[i]))
	}

	return machineClassesList, nil
}

func (c *client) GetMachineDeployment(namespace, name string) (*clusterv1.MachineDeployment, error) {
	machineDeployment := &clusterv1.MachineDeployment{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return &clusterv1.MachineDeployment{}, errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	err = clientset.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, machineDeployment)
	if err != nil {
		return nil, errors.Wrapf(err, "error listing machine deployment objects in namespace %q", namespace)
	}

	return machineDeployment, nil
}

func (c *client) GetMachineDeploymentsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineDeployment, error) {
	machineDeploymentList := &clusterv1.MachineDeploymentList{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return []*clusterv1.MachineDeployment{}, errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	listOpts := &ctrlclient.ListOptions{}
	err = listOpts.SetLabelSelector(fmt.Sprintf("%s=%s", machineClusterLabelName, cluster.Name))
	if err != nil {
		return nil, errors.Wrapf(err, "error settings label selector '%s=%s' for Cluster %s/%s", machineClusterLabelName, cluster.Name, cluster.Namespace, cluster.Name)
	}

	err = clientset.List(context.Background(), machineDeploymentList, ctrlclient.UseListOptions(listOpts))
	if err != nil {
		return nil, errors.Wrapf(err, "error listing MachineDeployments for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	var machineDeployments []*clusterv1.MachineDeployment
	for idx, md := range machineDeploymentList.Items {
		for _, or := range md.GetOwnerReferences() {
			if or.Kind == cluster.Kind && or.Name == cluster.Name {
				machineDeployments = append(machineDeployments, machineDeployments[idx])
				continue
			}
		}
	}

	return machineDeployments, nil
}

func (c *client) GetMachineDeployments(namespace string) ([]*clusterv1.MachineDeployment, error) {
	machineDeployments := clusterv1.MachineDeploymentList{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return []*clusterv1.MachineDeployment{}, errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	err = clientset.List(context.Background(), &machineDeployments)
	if err != nil {
		return nil, errors.Wrapf(err, "error listing machine deployment objects in namespace %q", namespace)
	}

	var machineDeploymentsList []*clusterv1.MachineDeployment
	for i := 0; i < len(machineDeployments.Items); i++ {
		machineDeploymentsList = append(machineDeploymentsList, &machineDeployments.Items[i])
	}

	return machineDeploymentsList, nil
}

func (c *client) GetMachineSet(namespace, name string) (*clusterv1.MachineSet, error) {
	machineSet := &clusterv1.MachineSet{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return &clusterv1.MachineSet{}, errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	err = clientset.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, machineSet)
	if err != nil {
		return nil, errors.Wrapf(err, "error listing machine deployment objects in namespace %q", namespace)
	}
	return machineSet, nil
}

func (c *client) GetMachineSets(namespace string) ([]*clusterv1.MachineSet, error) {
	machineSets := clusterv1.MachineSetList{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return []*clusterv1.MachineSet{}, errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	err = clientset.List(context.Background(), &machineSets)
	if err != nil {
		return nil, errors.Wrapf(err, "error listing machine deployment objects in namespace %q", namespace)
	}

	var machineSetsList []*clusterv1.MachineSet
	for i := 0; i < len(machineSets.Items); i++ {
		machineSetsList = append(machineSetsList, &machineSets.Items[i])
	}

	return machineSetsList, nil
}

func (c *client) GetMachineSetsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineSet, error) {
	machineSetList := clusterv1.MachineSetList{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return []*clusterv1.MachineSet{}, errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	listOpts := &ctrlclient.ListOptions{}
	err = listOpts.SetLabelSelector(fmt.Sprintf("%s=%s", machineClusterLabelName, cluster.Name))
	if err != nil {
		return nil, errors.Wrapf(err, "error settings label selector '%s=%s' for Cluster %s/%s", machineClusterLabelName, cluster.Name, cluster.Namespace, cluster.Name)
	}
	err = clientset.List(context.Background(), &machineSetList, ctrlclient.UseListOptions(listOpts))
	if err != nil {
		return nil, errors.Wrapf(err, "error listing MachineSets for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	var machineSets []*clusterv1.MachineSet
	for idx, md := range machineSetList.Items {
		for _, or := range md.GetOwnerReferences() {
			if or.Kind == cluster.Kind && or.Name == cluster.Name {
				machineSets = append(machineSets, &machineSetList.Items[idx])
				continue
			}
		}
	}
	return machineSets, nil
}

func (c *client) GetMachineSetsForMachineDeployment(md *clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error) {
	machineSets, err := c.GetMachineSets(md.Namespace)
	if err != nil {
		return nil, err
	}
	var controlledMachineSets []*clusterv1.MachineSet
	for _, ms := range machineSets {
		if metav1.GetControllerOf(ms) != nil && metav1.IsControlledBy(ms, md) {
			controlledMachineSets = append(controlledMachineSets, ms)
		}
	}
	return controlledMachineSets, nil
}

func (c *client) GetMachines(namespace string) ([]*clusterv1.Machine, error) {
	machines := clusterv1.MachineList{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return []*clusterv1.Machine{}, errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	err = clientset.List(context.Background(), &machines)
	if err != nil {
		return nil, errors.Wrapf(err, "error listing machine objects in namespace %q", namespace)
	}
	var machinesList []*clusterv1.Machine
	for i := 0; i < len(machines.Items); i++ {
		machinesList = append(machinesList, &machines.Items[i])
	}

	return machinesList, nil
}

func (c *client) GetMachinesForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.Machine, error) {
	machineslist := clusterv1.MachineList{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return []*clusterv1.Machine{}, errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	listOpts := &ctrlclient.ListOptions{}
	err = listOpts.SetLabelSelector(fmt.Sprintf("%s=%s", machineClusterLabelName, cluster.Name))
	if err != nil {
		return nil, errors.Wrapf(err, "error settings label selector '%s=%s' for Cluster %s/%s", machineClusterLabelName, cluster.Name, cluster.Namespace, cluster.Name)
	}
	err = clientset.List(context.Background(), &machineslist, ctrlclient.UseListOptions(listOpts))
	if err != nil {
		return nil, errors.Wrapf(err, "error listing Machines for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	var machines []*clusterv1.Machine
	for idx, md := range machineslist.Items {
		for _, or := range md.GetOwnerReferences() {
			if or.Kind == cluster.Kind && or.Name == cluster.Name {
				machines = append(machines, &machineslist.Items[idx])
				continue
			}
		}
	}
	return machines, nil
}

func (c *client) GetMachinesForMachineSet(ms *clusterv1.MachineSet) ([]*clusterv1.Machine, error) {
	machines, err := c.GetMachines(ms.Namespace)
	if err != nil {
		return nil, err
	}
	var controlledMachines []*clusterv1.Machine
	for _, m := range machines {
		if metav1.GetControllerOf(m) != nil && metav1.IsControlledBy(m, ms) {
			controlledMachines = append(controlledMachines, m)
		}
	}
	return controlledMachines, nil
}

func (c *client) CreateMachineClass(machineClass *clusterv1.MachineClass) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	if err = clientset.Create(context.Background(), machineClass); err != nil {
		return errors.Wrapf(err, "error listing machine set object %s in namespace %q", machineClass.Namespace, machineClass.Name)
	}
	return nil
}

func (c *client) DeleteMachineClass(namespace, name string) error {
	machineClass := &clusterv1.MachineClass{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	if err = clientset.Delete(context.Background(), machineClass); err != nil {
		return errors.Wrapf(err, "error deleting MachineClass %s/%s", namespace, name)
	}
	return nil
}

func (c *client) CreateClusterObject(cluster *clusterv1.Cluster) error {
	namespace := c.GetContextNamespace()
	if cluster.Namespace == "" {
		cluster.Namespace = namespace
	}

	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	if err = clientset.Create(context.Background(), cluster); err != nil {
		return errors.Wrapf(err, "error listing machine set object %s in namespace %q", cluster.Namespace, cluster.Name)
	}
	return nil
}

func (c *client) CreateMachineDeployments(deployments []*clusterv1.MachineDeployment, namespace string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	for _, deploy := range deployments {
		deploy.Namespace = namespace
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		if err = clientset.Create(context.Background(), deploy); err != nil {
			return errors.Wrapf(err, "error creating a machine deployment object in namespace %q", namespace)
		}
		return nil
	}
	return nil
}

func (c *client) CreateMachineSets(machineSets []*clusterv1.MachineSet, namespace string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	for _, ms := range machineSets {
		ms.Namespace = namespace
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		if err = clientset.Create(context.Background(), ms); err != nil {
			return errors.Wrapf(err, "error creating a machine set object in namespace %q", namespace)
		}
		return nil
	}
	return nil
}

func (c *client) CreateMachines(machines []*clusterv1.Machine, namespace string) error {
	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		gerr    error
	)
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	// The approach to concurrency here comes from golang.org/x/sync/errgroup.
	for _, machine := range machines {
		wg.Add(1)

		go func(machine *clusterv1.Machine) {
			defer wg.Done()
			var createdMachine *clusterv1.Machine
			machine.Namespace = namespace
			err = clientset.Create(context.Background(), machine)
			if err != nil {
				errOnce.Do(func() {
					gerr = errors.Wrapf(err, "error creating a machine object in namespace %v", namespace)
				})
				return
			}

			if err := waitForMachineReady(c.clientSet, createdMachine); err != nil {
				errOnce.Do(func() { gerr = err })
			}
		}(machine)
	}
	wg.Wait()
	return gerr
}

// DeleteClusters deletes all Clusters in a namespace. If the namespace is empty then all Clusters in all namespaces are deleted.
func (c *client) DeleteClusters(namespace string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	seen := make(map[string]bool)
	clustersToDelete := make(map[string]*clusterv1.ClusterList)

	if namespace != "" {
		seen[namespace] = true
	} else {
		clusters := &clusterv1.ClusterList{}
		err = clientset.List(context.Background(), clusters)
		if err != nil {
			return errors.Wrap(err, "error listing Clusters in all namespaces")
		}
		for _, cluster := range clusters.Items {
			if _, ok := seen[cluster.Namespace]; !ok {
				seen[cluster.Namespace] = true
				clustersToDelete[cluster.Namespace].Items = append(clustersToDelete[cluster.Namespace].Items, cluster)
			}
		}
	}
	for ns := range seen {
		err = clientset.Delete(context.Background(), clustersToDelete[ns])
		if err != nil {
			return errors.Wrapf(err, "error deleting Clusters in namespace %q", ns)
		}
		err = c.waitForClusterDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for Cluster(s) deletion to complete in namespace %q", ns)
		}
	}

	return nil
}

// DeleteMachineClasses deletes all MachineClasses in a namespace. If the namespace is empty then all MachineClasses in all namespaces are deleted.
func (c *client) DeleteMachineClasses(namespace string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	seen := make(map[string]bool)
	machineClassesToDelete := make(map[string]*clusterv1.MachineClassList)

	if namespace != "" {
		seen[namespace] = true
	} else {
		machineClasses := &clusterv1.MachineClassList{}
		err := clientset.List(context.Background(), machineClasses)
		if err != nil {
			return errors.Wrap(err, "error listing MachineClasss in all namespaces")
		}
		for _, cluster := range machineClasses.Items {
			if _, ok := seen[cluster.Namespace]; !ok {
				seen[cluster.Namespace] = true
				machineClassesToDelete[cluster.Namespace].Items = append(machineClassesToDelete[cluster.Namespace].Items, cluster)
			}
		}
	}
	for ns := range seen {
		if err := c.DeleteMachineClasses(ns); err != nil {
			return err
		}
		if err := clientset.Delete(context.Background(), machineClassesToDelete[ns]); err != nil {
			return errors.Wrapf(err, "error deleting MachineClasses in namespace %q", ns)
		}
		err := c.waitForMachineClassesDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for MachineClass(es) deletion to complete in ns %q", ns)
		}
	}

	return nil
}

// DeleteMachineDeployments deletes all MachineDeployments in a namespace. If the namespace is empty then all MachineDeployments in all namespaces are deleted.
func (c *client) DeleteMachineDeployments(namespace string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	seen := make(map[string]bool)
	machineDeploymentsToDelete := make(map[string]*clusterv1.MachineDeploymentList)

	if namespace != "" {
		seen[namespace] = true
	} else {
		machineDeployments := &clusterv1.MachineDeploymentList{}
		err := clientset.List(context.Background(), machineDeployments)
		if err != nil {
			return errors.Wrap(err, "error listing MachineDeployments in all namespaces")
		}
		for _, cluster := range machineDeployments.Items {
			if _, ok := seen[cluster.Namespace]; !ok {
				seen[cluster.Namespace] = true
				machineDeploymentsToDelete[cluster.Namespace].Items = append(machineDeploymentsToDelete[cluster.Namespace].Items, cluster)
			}
		}
	}
	for ns := range seen {
		err = clientset.Delete(context.Background(), machineDeploymentsToDelete[ns])
		if err != nil {
			return errors.Wrapf(err, "error deleting MachineDeployments in namespace %q", ns)
		}
		err = c.waitForMachineDeploymentsDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for MachineDeployment(s) deletion to complete in namespace %q", ns)
		}
	}
	return nil
}

// DeleteMachineSets deletes all MachineSets in a namespace. If the namespace is empty then all MachineSets in all namespaces are deleted.
func (c *client) DeleteMachineSets(namespace string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	seen := make(map[string]bool)
	machineSetsToDelete := make(map[string]*clusterv1.MachineSetList)

	if namespace != "" {
		seen[namespace] = true
	} else {
		machineSets := &clusterv1.MachineSetList{}
		err := clientset.List(context.Background(), machineSets)
		if err != nil {
			return errors.Wrap(err, "error listing MachineSets in all namespaces")
		}
		for _, ms := range machineSets.Items {
			if _, ok := seen[ms.Namespace]; !ok {
				seen[ms.Namespace] = true
				machineSetsToDelete[ms.Namespace].Items = append(machineSetsToDelete[ms.Namespace].Items, ms)
			}
		}
	}
	for ns := range seen {
		err = clientset.Delete(context.Background(), machineSetsToDelete[ns])
		if err != nil {
			return errors.Wrapf(err, "error deleting MachineSets in namespace %q", ns)
		}
		err = c.waitForMachineSetsDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for MachineSet(s) deletion to complete in namespace %q", ns)
		}
	}

	return nil
}

// DeleteMachines deletes all Machines in a namespace. If the namespace is empty then all Machines in all namespaces are deleted.
func (c *client) DeleteMachines(namespace string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	seen := make(map[string]bool)
	machinesToDelete := make(map[string]*clusterv1.MachineList)

	if namespace != "" {
		seen[namespace] = true
	} else {
		machines := &clusterv1.MachineList{}
		err := clientset.List(context.Background(), machines)
		if err != nil {
			return errors.Wrap(err, "error listing Machines in all namespaces")
		}
		for _, ms := range machines.Items {
			if _, ok := seen[ms.Namespace]; !ok {
				seen[ms.Namespace] = true
				machinesToDelete[ms.Namespace].Items = append(machinesToDelete[ms.Namespace].Items, ms)
			}
		}
	}
	for ns := range seen {
		err = clientset.Delete(context.Background(), machinesToDelete[ns])
		if err != nil {
			return errors.Wrapf(err, "error deleting Machines in namespace %q", ns)
		}
		err = c.waitForMachineSetsDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for Machine(s) deletion to complete in namespace %q", ns)
		}
	}

	return nil
}

func (c *client) ForceDeleteMachine(namespace, name string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	machine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	err = clientset.Get(context.Background(), namespacedName, &machine)
	if err != nil {
		return errors.Wrapf(err, "error getting Machine %s/%s", namespace, name)
	}
	machine.SetFinalizers([]string{})
	if err := clientset.Update(context.Background(), &machine); err != nil {
		return errors.Wrapf(err, "error removing finalizer for Machine %s/%s", namespace, name)
	}
	if err := clientset.Delete(context.Background(), &machine, ctrlclient.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return errors.Wrapf(err, "error deleting Machine %s/%s", namespace, name)
	}
	return nil
}

func (c *client) ForceDeleteMachineSet(namespace, name string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	machineSet := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	err = clientset.Get(context.Background(), namespacedName, &machineSet)
	if err != nil {
		return errors.Wrapf(err, "error getting Machine %s/%s", namespace, name)
	}
	machineSet.SetFinalizers([]string{})
	if err := clientset.Update(context.Background(), &machineSet); err != nil {
		return errors.Wrapf(err, "error removing finalizer for Machine %s/%s", namespace, name)
	}
	if err := clientset.Delete(context.Background(), &machineSet, ctrlclient.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return errors.Wrapf(err, "error deleting Machine %s/%s", namespace, name)
	}
	return nil
}

func (c *client) ForceDeleteMachineDeployment(namespace, name string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	machineDeployment := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	err = clientset.Get(context.Background(), namespacedName, &machineDeployment)
	if err != nil {
		return errors.Wrapf(err, "error getting Machine %s/%s", namespace, name)
	}
	machineDeployment.SetFinalizers([]string{})
	if err := clientset.Update(context.Background(), &machineDeployment); err != nil {
		return errors.Wrapf(err, "error removing finalizer for Machine %s/%s", namespace, name)
	}
	if err := clientset.Delete(context.Background(), &machineDeployment, ctrlclient.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return errors.Wrapf(err, "error deleting Machine %s/%s", namespace, name)
	}
	return nil
}

// UpdateClusterObjectEndpoint updates the status of a cluster API endpoint, clusterEndpoint
// can be passed as hostname or hostname:port, if port is not present the default port 443 is applied.
// TODO: Test this function
func (c *client) UpdateClusterObjectEndpoint(clusterEndpoint, clusterName, namespace string) error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	cluster, err := c.GetCluster(clusterName, namespace)
	if err != nil {
		return err
	}
	endpointHost, endpointPort, err := net.SplitHostPort(clusterEndpoint)
	if err != nil {
		// We rely on provider.GetControlPlaneEndpoint to provide a correct hostname/IP, no
		// further validation is done.
		endpointHost = clusterEndpoint
		endpointPort = defaultAPIServerPort
	}
	endpointPortInt, err := strconv.Atoi(endpointPort)
	if err != nil {
		return errors.Wrapf(err, "error while converting cluster endpoint port %q", endpointPort)
	}
	cluster.Status.APIEndpoints = append(cluster.Status.APIEndpoints,
		clusterv1.APIEndpoint{
			Host: endpointHost,
			Port: endpointPortInt,
		})
	err = clientset.Status().Update(context.Background(), cluster)
	return err
}

func (c *client) WaitForClusterV1alpha1Ready() error {
	return waitForClusterResourceReady(c.clientSet)
}

func (c *client) WaitForResourceStatuses() error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}

	clientset, err := ctrlclient.New(config, ctrlclient.Options{})
	deadline := time.Now().Add(timeoutResourceReady)

	timeout := time.Until(deadline)
	return util.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		klog.V(2).Info("Waiting for Cluster API resources to have statuses...")
		clusters := &clusterv1.ClusterList{}
		err = clientset.List(context.Background(), clusters)
		clusters, err := c.clientSet.ClusterV1alpha1().Clusters("").List(metav1.ListOptions{})
		if err != nil {
			klog.V(10).Infof("retrying: failed to list clusters: %v", err)
			return false, nil
		}
		for _, cluster := range clusters.Items {
			if reflect.DeepEqual(clusterv1.ClusterStatus{}, cluster.Status) {
				klog.V(10).Info("retrying: cluster status is empty")
				return false, nil
			}
			if cluster.Status.ProviderStatus == nil {
				klog.V(10).Info("retrying: cluster.Status.ProviderStatus is not set")
				return false, nil
			}
		}
		machineDeployments, err := c.clientSet.ClusterV1alpha1().MachineDeployments("").List(metav1.ListOptions{})
		if err != nil {
			klog.V(10).Infof("retrying: failed to list machine deployment: %v", err)
			return false, nil
		}
		for _, md := range machineDeployments.Items {
			if reflect.DeepEqual(clusterv1.MachineDeploymentStatus{}, md.Status) {
				klog.V(10).Info("retrying: machine deployment status is empty")
				return false, nil
			}
		}
		machineSets, err := c.clientSet.ClusterV1alpha1().MachineSets("").List(metav1.ListOptions{})
		if err != nil {
			klog.V(10).Infof("retrying: failed to list machinesets: %v", err)
			return false, nil
		}
		for _, ms := range machineSets.Items {
			if reflect.DeepEqual(clusterv1.MachineSetStatus{}, ms.Status) {
				klog.V(10).Info("retrying: machineset status is empty")
				return false, nil
			}
		}
		machines, err := c.clientSet.ClusterV1alpha1().Machines("").List(metav1.ListOptions{})
		if err != nil {
			klog.V(10).Infof("retrying: failed to list machines: %v", err)
			return false, nil
		}
		for _, m := range machines.Items {
			if reflect.DeepEqual(clusterv1.MachineStatus{}, m.Status) {
				klog.V(10).Info("retrying: machine status is empty")
				return false, nil
			}
			if m.Status.ProviderStatus == nil {
				klog.V(10).Info("retrying: machine.Status.ProviderStatus is not set")
				return false, nil
			}
		}

		return true, nil
	})
}

func (c *client) waitForClusterDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for Clusters to be deleted...")
		clusters := &clusterv1.ClusterList{}
		config, err := ctrl.GetConfig()
		if err != nil {
			return false, errors.Wrap(err, "error creating config for core clientset")
		}

		clientset, err := ctrlclient.New(config, ctrlclient.Options{})

		if err = clientset.List(context.Background(), clusters); err != nil {
			return false, errors.Wrapf(err, "error listing cluster objects in namespace %q", namespace)
		}

		if len(clusters.Items) > 0 {
			return false, nil
		}

		return true, nil
	})
}

func (c *client) waitForMachineClassesDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for MachineClasses to be deleted...")
		machineClasses := &clusterv1.MachineClassList{}
		config, err := ctrl.GetConfig()
		if err != nil {
			return false, errors.Wrap(err, "error creating config for core clientset")
		}

		clientset, err := ctrlclient.New(config, ctrlclient.Options{})

		if err = clientset.List(context.Background(), machineClasses); err != nil {
			return false, nil
		}

		if len(machineClasses.Items) > 0 {
			return false, nil
		}

		return true, nil
	})
}

func (c *client) waitForMachineDeploymentsDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for MachineDeployments to be deleted...")
		machineDeployments := &clusterv1.MachineDeploymentList{}
		config, err := ctrl.GetConfig()
		if err != nil {
			return false, errors.Wrap(err, "error creating config for core clientset")
		}

		clientset, err := ctrlclient.New(config, ctrlclient.Options{})

		if err = clientset.List(context.Background(), machineDeployments); err != nil {
			return false, nil
		}
		if len(machineDeployments.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *client) waitForMachineSetsDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for MachineSets to be deleted...")
		machineSets := &clusterv1.MachineSetList{}
		config, err := ctrl.GetConfig()
		if err != nil {
			return false, errors.Wrap(err, "error creating config for core clientset")
		}

		clientset, err := ctrlclient.New(config, ctrlclient.Options{})

		if err = clientset.List(context.Background(), machineSets); err != nil {
			return false, nil
		}
		if len(machineSets.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *client) waitForMachinesDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for Machines to be deleted...")
		machines := &clusterv1.MachineList{}
		config, err := ctrl.GetConfig()
		if err != nil {
			return false, errors.Wrap(err, "error creating config for core clientset")
		}

		clientset, err := ctrlclient.New(config, ctrlclient.Options{})

		if err = clientset.List(context.Background(), machines); err != nil {
			return false, nil
		}
		if len(machines.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *client) waitForMachineDelete(namespace, name string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for Machine %s/%s to be deleted...", namespace, name)
		machine := &clusterv1.Machine{}
		config, err := ctrl.GetConfig()
		if err != nil {
			return false, errors.Wrap(err, "error creating config for core clientset")
		}

		clientset, err := ctrlclient.New(config, ctrlclient.Options{})

		if err = clientset.List(context.Background(), machine); err != nil && !apierrors.IsNotFound(err) {
			return false, errors.Wrap(err, "error checking machine for deletion status ")
		}
		return true, nil
	})
}

func (c *client) kubectlDelete(manifest string) error {
	return c.kubectlManifestCmd("delete", manifest)
}

func (c *client) kubectlApply(manifest string) error {
	return c.kubectlManifestCmd("apply", manifest)
}

func (c *client) kubectlManifestCmd(commandName, manifest string) error {
	cmd := exec.Command("kubectl", c.buildKubectlArgs(commandName)...)
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "couldn't kubectl apply, output: %s", string(out))
	}
	return nil
}

func (c *client) buildKubectlArgs(commandName string) []string {
	args := []string{commandName}
	if c.kubeconfigFile != "" {
		args = append(args, "--kubeconfig", c.kubeconfigFile)
	}
	if c.configOverrides.Context.Cluster != "" {
		args = append(args, "--cluster", c.configOverrides.Context.Cluster)
	}
	if c.configOverrides.Context.Namespace != "" {
		args = append(args, "--namespace", c.configOverrides.Context.Namespace)
	}
	if c.configOverrides.Context.AuthInfo != "" {
		args = append(args, "--user", c.configOverrides.Context.AuthInfo)
	}
	return append(args, "-f", "-")
}

func (c *client) waitForKubectlApply(manifest string) error {
	err := util.PollImmediate(retryIntervalKubectlApply, timeoutKubectlApply, func() (bool, error) {
		klog.V(2).Infof("Waiting for kubectl apply...")
		err := c.kubectlApply(manifest)
		if err != nil {
			if strings.Contains(err.Error(), io.EOF.Error()) || strings.Contains(err.Error(), "refused") || strings.Contains(err.Error(), "no such host") {
				// Connection was refused, probably because the API server is not ready yet.
				klog.V(4).Infof("Waiting for kubectl apply... server not yet available: %v", err)
				return false, nil
			}
			if strings.Contains(err.Error(), "unable to recognize") {
				klog.V(4).Infof("Waiting for kubectl apply... api not yet available: %v", err)
				return false, nil
			}
			if strings.Contains(err.Error(), "namespaces \"default\" not found") {
				klog.V(4).Infof("Waiting for kubectl apply... default namespace not yet available: %v", err)
				return false, nil
			}
			klog.Warningf("Waiting for kubectl apply... unknown error %v", err)
			return false, err
		}

		return true, nil
	})

	return err
}

func waitForClusterResourceReady(cs clientset.Interface) error {
	deadline := time.Now().Add(timeoutResourceReady)
	timeout := time.Until(deadline)
	cluster := &clusterv1.ClusterList{}
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	return util.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		klog.V(2).Info("Waiting for Cluster v1alpha resources to be listable...")
		if err = clientset.List(context.Background(), cluster); err == nil {
			return true, nil
		}
		return false, nil
	})
}

func waitForMachineReady(cs clientset.Interface, machine *clusterv1.Machine) error {
	timeout := timeoutMachineReady
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error creating config for core clientset")
	}
	clientset, err := ctrlclient.New(config, ctrlclient.Options{})

	if p := os.Getenv(TimeoutMachineReady); p != "" {
		t, err := strconv.Atoi(p)
		if err == nil {
			// only valid value will be used
			timeout = time.Duration(t) * time.Minute
			klog.V(4).Info("Setting wait for machine timeout value to ", timeout)
		}
	}

	err = util.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		klog.V(2).Infof("Waiting for Machine %v to become ready...", machine.Name)
		namespacedName := types.NamespacedName{
			Namespace: machine.Namespace,
			Name:      machine.Name,
		}
		if err = clientset.Get(context.Background(), namespacedName, machine); err != nil && !apierrors.IsNotFound(err) {
			return false, nil
		}

		// TODO: update once machine controllers have a way to indicate a machine has been provisoned. https://github.com/kubernetes-sigs/cluster-api/issues/253
		// Seeing a node cannot be purely relied upon because the provisioned control plane will not be registering with
		// the stack that provisions it.
		ready := machine.Status.NodeRef != nil || len(machine.Annotations) > 0
		return ready, nil
	})

	return err
}

func createTempFile(contents string) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	defer ifErrRemove(&err, f.Name())
	if err = f.Close(); err != nil {
		return "", err
	}
	err = ioutil.WriteFile(f.Name(), []byte(contents), 0644)
	if err != nil {
		return "", err
	}
	return f.Name(), nil
}

func ifErrRemove(pErr *error, path string) {
	if *pErr != nil {
		if err := os.Remove(path); err != nil {
			klog.Warningf("Error removing file '%s': %v", path, err)
		}
	}
}

func GetClusterAPIObject(client Client, clusterName, namespace string) (*clusterv1.Cluster, *clusterv1.Machine, []*clusterv1.Machine, error) {
	machines, err := client.GetMachines(namespace)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "unable to fetch machines")
	}
	cluster, err := client.GetCluster(clusterName, namespace)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "unable to fetch cluster %s/%s", namespace, clusterName)
	}

	controlPlane, nodes, err := ExtractControlPlaneMachines(machines)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "unable to fetch control plane machine in cluster %s/%s", namespace, clusterName)
	}
	return cluster, controlPlane[0], nodes, nil
}

// ExtractControlPlaneMachines separates the machines running the control plane from the incoming machines.
// This is currently done by looking at which machine specifies the control plane version.
func ExtractControlPlaneMachines(machines []*clusterv1.Machine) ([]*clusterv1.Machine, []*clusterv1.Machine, error) {
	nodes := []*clusterv1.Machine{}
	controlPlaneMachines := []*clusterv1.Machine{}
	for _, machine := range machines {
		if util.IsControlPlaneMachine(machine) {
			controlPlaneMachines = append(controlPlaneMachines, machine)
		} else {
			nodes = append(nodes, machine)
		}
	}
	if len(controlPlaneMachines) < 1 {
		return nil, nil, errors.Errorf("expected one or more control plane machines, got: %v", len(controlPlaneMachines))
	}
	return controlPlaneMachines, nodes, nil
}
