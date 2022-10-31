/*
Copyright 2022 The Firefly Authors.

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

package app

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/controller-manager/controller"

	"github.com/firefly-io/firefly/pkg/karmada/controller/estimator"
	"github.com/firefly-io/firefly/pkg/karmada/controller/foo"
	"github.com/firefly-io/firefly/pkg/karmada/controller/kubean"
	"github.com/firefly-io/firefly/pkg/karmada/controller/node"
)

func startEstimatorController(ctx context.Context, controllerContext ControllerContext) (controller.Interface, bool, error) {
	if !controllerContext.HostClusterAvailableResources[schema.GroupVersionResource{Group: "install.firefly.io", Version: "v1alpha1", Resource: "karmadas"}] {
		return nil, false, nil
	}

	ctrl, err := estimator.NewEstimatorController(
		controllerContext.KarmadaClientBuilder.ClientOrDie("firefly-estimator-controller"),
		controllerContext.KarmadaClientBuilder.KarmadaClientOrDie("firefly-estimator-controller"),
		controllerContext.KarmadaInformerFactory.Cluster().V1alpha1().Clusters(),
		controllerContext.EstimatorNamespace,
		controllerContext.KarmadaName,
		controllerContext.FireflyClientBuilder.ClientOrDie("firefly-estimator-controller"),
		controllerContext.FireflyInformerFactory.Install().V1alpha1().Karmadas(),
	)
	if err != nil {
		return nil, true, fmt.Errorf("failed to start the estimator controller: %v", err)
	}
	go ctrl.Run(ctx, 1)
	return nil, true, nil
}

func startNodeController(ctx context.Context, controllerContext ControllerContext) (controller.Interface, bool, error) {
	ctrl, err := node.NewNodeController(
		controllerContext.KarmadaClientBuilder.ClientOrDie("firefly-node-controller"),
		controllerContext.FireflyKubeInformerFactory.Core().V1().Nodes(),
		controllerContext.KarmadaKubeInformerFactory.Core().V1().Nodes(),
	)
	if err != nil {
		return nil, true, fmt.Errorf("failed to start the node controller: %v", err)
	}
	go ctrl.Run(ctx, 1)
	return nil, true, nil
}

func startFooController(ctx context.Context, controllerContext ControllerContext) (controller.Interface, bool, error) {
	clientConfig := controllerContext.KarmadaClientBuilder.ConfigOrDie("firefly-foo-controller")
	dynamicClient := dynamic.NewForConfigOrDie(clientConfig)

	ctrl, err := foo.NewFooController(
		controllerContext.RESTMapper,
		dynamicClient,
		controllerContext.KarmadaClientBuilder.ClientOrDie("firefly-foo-controller"),
		controllerContext.KarmadaClientBuilder.KarmadaClientOrDie("firefly-foo-controller"),
		controllerContext.KarmadaClientBuilder.KarmadaFireflyClientOrDie("firefly-foo-controller"),
		controllerContext.KarmadaFireflyInformerFactory.Toolkit().V1alpha1().Foos(),
		controllerContext.KarmadaInformerFactory.Cluster().V1alpha1().Clusters(),
		controllerContext.KarmadaInformerFactory.Work().V1alpha1().Works(),
	)
	if err != nil {
		return nil, true, fmt.Errorf("failed to start the foo controller: %v", err)
	}
	go ctrl.Run(ctx, 1)
	return nil, true, nil
}

func startKubeanController(ctx context.Context, controllerContext ControllerContext) (controller.Interface, bool, error) {
	gvrs := []schema.GroupVersionResource{
		{Group: "kubean.io", Version: "v1alpha1", Resource: "clusteroperations"},
		{Group: "kubean.io", Version: "v1alpha1", Resource: "clusters"},
		{Group: "kubean.io", Version: "v1alpha1", Resource: "localartifactsets"},
		{Group: "kubean.io", Version: "v1alpha1", Resource: "manifests"},
	}
	for _, gvr := range gvrs {
		if !controllerContext.HostClusterAvailableResources[gvr] {
			return nil, false, nil
		}
		if !controllerContext.AvailableResources[gvr] {
			return nil, false, nil
		}
	}

	clusterInformers := controllerContext.KarmadaDynamicInformerFactory.ForResource(schema.GroupVersionResource{Group: "kubean.io", Version: "v1alpha1", Resource: "clusters"})
	hostClusterInformers := controllerContext.FireflyDynamicInformerFactory.ForResource(schema.GroupVersionResource{Group: "kubean.io", Version: "v1alpha1", Resource: "clusters"})

	clusterctrl, err := kubean.NewClusterController(
		controllerContext.EstimatorNamespace,
		controllerContext.KarmadaClientBuilder.ClientOrDie("kubean-cluster-controller"),
		controllerContext.KarmadaClientBuilder.DynamicClientOrDie("kubean-cluster-controller"),
		clusterInformers,
		controllerContext.FireflyClientBuilder.DynamicClientOrDie("kubean-cluster-controller"),
		hostClusterInformers,
	)
	if err != nil {
		return nil, true, fmt.Errorf("failed to start the kubean cluster controller: %v", err)
	}

	go clusterctrl.Run(ctx, 1)

	clusterrefctl, err := kubean.NewClusterRefController(
		controllerContext.KarmadaClientBuilder.ClientOrDie("kubean-clusterref-controller"),
		clusterInformers,
		controllerContext.KarmadaKubeInformerFactory.Core().V1().ConfigMaps(),
		controllerContext.EstimatorNamespace,
		controllerContext.FireflyClientBuilder.ClientOrDie("kubean-clusterref-controller"),
	)
	if err != nil {
		return nil, true, fmt.Errorf("failed to start the kubean clusterref controller: %v", err)
	}

	go clusterrefctl.Run(ctx, 1)

	manifestInformers := controllerContext.KarmadaDynamicInformerFactory.ForResource(schema.GroupVersionResource{Group: "kubean.io", Version: "v1alpha1", Resource: "manifests"})
	hostManifestInformers := controllerContext.FireflyDynamicInformerFactory.ForResource(schema.GroupVersionResource{Group: "kubean.io", Version: "v1alpha1", Resource: "manifests"})

	manifestctrl, err := kubean.NewManifestController(
		controllerContext.KarmadaClientBuilder.ClientOrDie("kubean-manifest-controller"),
		controllerContext.KarmadaClientBuilder.DynamicClientOrDie("kubean-manifest-controller"),
		manifestInformers,
		hostManifestInformers,
	)
	if err != nil {
		return nil, true, fmt.Errorf("failed to start the kubean manifest controller: %v", err)
	}

	go manifestctrl.Run(ctx, 1)

	return nil, true, nil
}
