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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/util/sets"
	utilpointer "k8s.io/utils/pointer"
)

var KubeControllersEnabledByDefaults = sets.NewString(
	"namespace",
	"garbagecollector",
	"serviceaccount-token",
	"ttl-after-finished",
	"bootstrapsigner",
	"csrapproving",
	"csrcleaner",
	"csrsigning",
)

func SetDefaults_Karmada(obj *Karmada) {
	if obj.Spec.KubernetesVersion == "" {
		obj.Spec.KubernetesVersion = "v1.21.7"
	}
	if obj.Spec.KarmadaVersion == "" {
		obj.Spec.KarmadaVersion = "v1.2.0"
	}

	if obj.Spec.ImageRepository == "" {
		obj.Spec.ImageRepository = "swr.ap-southeast-1.myhuaweicloud.com/karmada"
	}

	if obj.Spec.KubeImageRepository == "" {
		obj.Spec.KubeImageRepository = "registry.k8s.io"
	}

	if obj.Spec.FireflyImageRepository == "" {
		obj.Spec.FireflyImageRepository = "ghcr.io/firefly-io"
	}

	network := &obj.Spec.Networking
	if network.DNSDomain == "" {
		network.DNSDomain = "cluster.local"
	}
	if network.ServiceSubnet == "" {
		network.ServiceSubnet = "10.96.0.0/12"
	}

	apiServer := &obj.Spec.APIServer
	if apiServer.KubeAPIServer.Replicas == nil {
		apiServer.KubeAPIServer.Replicas = utilpointer.Int32(1)
	}
	if apiServer.KarmadaAggregratedAPIServer.Replicas == nil {
		apiServer.KarmadaAggregratedAPIServer.Replicas = utilpointer.Int32(1)
	}

	webhook := &obj.Spec.Webhook
	if webhook.KarmadaWebhook.Replicas == nil {
		webhook.KarmadaWebhook.Replicas = utilpointer.Int32(1)
	}

	controllerManager := &obj.Spec.ControllerManager
	if controllerManager.KubeControllerManager.Replicas == nil {
		controllerManager.KubeControllerManager.Replicas = utilpointer.Int32(1)
	}
	if controllerManager.KubeControllerManager.Controllers == nil {
		controllerManager.KubeControllerManager.Controllers = KubeControllersEnabledByDefaults.List()
	}
	if controllerManager.KarmadaControllerManager.Replicas == nil {
		controllerManager.KarmadaControllerManager.Replicas = utilpointer.Int32(1)
	}
	if controllerManager.FireflyKarmadaManager.Replicas == nil {
		controllerManager.FireflyKarmadaManager.Replicas = utilpointer.Int32(1)
	}

	scheduler := &obj.Spec.Scheduler
	if scheduler.KarmadaScheduler.Replicas == nil {
		scheduler.KarmadaScheduler.Replicas = utilpointer.Int32(1)
	}
	if scheduler.KarmadaDescheduler.Enable == nil {
		scheduler.KarmadaDescheduler.Enable = utilpointer.Bool(false)
	}
	if scheduler.KarmadaDescheduler.Replicas == nil {
		scheduler.KarmadaDescheduler.Replicas = utilpointer.Int32(1)
	}
	if scheduler.KarmadaSchedulerEstimator.Replicas == nil {
		scheduler.KarmadaSchedulerEstimator.Replicas = utilpointer.Int32(1)
	}
}
