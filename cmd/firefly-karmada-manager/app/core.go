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

	"github.com/carlory/firefly/pkg/karmada/controller/estimator"
	"github.com/carlory/firefly/pkg/karmada/controller/foo"
	"github.com/carlory/firefly/pkg/karmada/controller/kubean"
	"github.com/carlory/firefly/pkg/karmada/controller/node"
)

func startEstimatorController(ctx context.Context, controllerContext ControllerContext) (controller.Interface, bool, error) {
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
	ctrl, err := kubean.NewClusterController(
		controllerContext.KarmadaClientBuilder.ClientOrDie("kubean-cluster-controller"),
		controllerContext.KarmadaClientBuilder.DynamicClientOrDie("kubean-cluster-controller"),
		clusterInformers,
		controllerContext.FireflyClientBuilder.DynamicClientOrDie("kubean-cluster-controller"),
		hostClusterInformers,
	)
	if err != nil {
		return nil, true, fmt.Errorf("failed to start the kubean cluster controller: %v", err)
	}
	go ctrl.Run(ctx, 1)
	return nil, true, nil
}
