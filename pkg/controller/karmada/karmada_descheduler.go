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

package karmada

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/firefly-io/firefly/pkg/apis/install/v1alpha1"
	"github.com/firefly-io/firefly/pkg/constants"
	"github.com/firefly-io/firefly/pkg/scheme"
	"github.com/firefly-io/firefly/pkg/util"
	clientutil "github.com/firefly-io/firefly/pkg/util/client"
	maputil "github.com/firefly-io/firefly/pkg/util/map"
)

func (ctrl *KarmadaController) EnsureKarmadaDescheduler(karmada *installv1alpha1.Karmada) error {
	var enabled bool
	if karmada.Spec.Scheduler.KarmadaDescheduler.Enable != nil {
		enabled = *karmada.Spec.Scheduler.KarmadaDescheduler.Enable
	}
	if version.CompareKubeAwareVersionStrings("v1.1.0", karmada.Spec.KarmadaVersion) < 0 {
		enabled = false
	}

	if enabled {
		return ctrl.EnsureKarmadaDeschedulerDeployment(karmada)
	}
	return ctrl.RemoveKarmadaDescheduler(karmada)
}

func (ctrl *KarmadaController) RemoveKarmadaDescheduler(karmada *installv1alpha1.Karmada) error {
	componentName := constants.KarmadaComponentDescheduler
	err := ctrl.client.AppsV1().Deployments(karmada.Namespace).Delete(context.TODO(), componentName, metav1.DeleteOptions{})
	return client.IgnoreNotFound(err)
}

func (ctrl *KarmadaController) EnsureKarmadaDeschedulerDeployment(karmada *installv1alpha1.Karmada) error {
	componentName := constants.KarmadaComponentDescheduler
	scheduler := karmada.Spec.Scheduler.KarmadaDescheduler

	repository := karmada.Spec.ImageRepository
	if scheduler.ImageRepository != "" {
		repository = scheduler.ImageRepository
	}

	imageName := constants.KarmadaComponentDescheduler
	if scheduler.ImageName != "" {
		imageName = scheduler.ImageName
	}

	tag := karmada.Spec.KarmadaVersion
	if scheduler.ImageTag != "" {
		tag = scheduler.ImageTag
	}

	defaultArgs := map[string]string{
		"bind-address": "0.0.0.0",
		"kubeconfig":   "/etc/kubeconfig",
		"v":            "4",
	}
	computedArgs := maputil.MergeStringMaps(defaultArgs, scheduler.ExtraArgs)
	args := maputil.ConvertToCommandOrArgs(computedArgs)

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": componentName},
			},
			Replicas: scheduler.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "karmada-descheduler",
							Image:           util.ComponentImageName(repository, imageName, tag),
							ImagePullPolicy: "IfNotPresent",
							Command:         []string{"/bin/karmada-descheduler"},
							Args:            args,
							Resources:       scheduler.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kubeconfig",
									MountPath: "/etc/kubeconfig",
									SubPath:   "kubeconfig",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "karmada-kubeconfig",
								},
							},
						},
					},
				},
			},
		},
	}

	controllerutil.SetOwnerReference(karmada, deployment, scheme.Scheme)
	return clientutil.CreateOrUpdateDeployment(ctrl.client, deployment)
}
