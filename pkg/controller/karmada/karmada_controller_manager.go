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
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/firefly-io/firefly/pkg/apis/install/v1alpha1"
	"github.com/firefly-io/firefly/pkg/constants"
	"github.com/firefly-io/firefly/pkg/scheme"
	"github.com/firefly-io/firefly/pkg/util"
	clientutil "github.com/firefly-io/firefly/pkg/util/client"
	maputil "github.com/firefly-io/firefly/pkg/util/map"
)

func (ctrl *KarmadaController) EnsureKarmadaControllerManager(karmada *installv1alpha1.Karmada) error {
	return ctrl.EnsureKarmadaControllerManagerDeployment(karmada)
}

func (ctrl *KarmadaController) EnsureKarmadaControllerManagerDeployment(karmada *installv1alpha1.Karmada) error {
	componentName := constants.KarmadaComponentControllerManager
	kcm := karmada.Spec.ControllerManager.KarmadaControllerManager

	repository := karmada.Spec.ImageRepository
	if kcm.ImageRepository != "" {
		repository = kcm.ImageRepository
	}

	imageName := constants.KarmadaComponentControllerManager
	if kcm.ImageName != "" {
		imageName = kcm.ImageName
	}

	tag := karmada.Spec.KarmadaVersion
	if kcm.ImageTag != "" {
		tag = kcm.ImageTag
	}

	defaultArgs := map[string]string{
		"bind-address":                    "0.0.0.0",
		"kubeconfig":                      "/etc/kubeconfig",
		"cluster-status-update-frequency": "10s",
		"secure-port":                     "10357",
		"v":                               "4",
	}
	if kcm.Controllers != nil {
		defaultArgs["controllers"] = strings.Join(kcm.Controllers, ",")
	}
	featureGates := karmada.Spec.FeatureGates
	for feature, enabled := range featureGates {
		if defaultArgs["feature-gates"] == "" {
			defaultArgs["feature-gates"] = fmt.Sprintf("%s=%t", feature, enabled)
		} else {
			defaultArgs["feature-gates"] = fmt.Sprintf("%s,%s=%t", defaultArgs["feature-gates"], feature, enabled)
		}
	}
	computedArgs := maputil.MergeStringMaps(defaultArgs, kcm.ExtraArgs)
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
			Replicas: kcm.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "karmada-controller-manager",
							Image:           util.ComponentImageName(repository, imageName, tag),
							ImagePullPolicy: "IfNotPresent",
							Command:         []string{"/bin/karmada-controller-manager"},
							Args:            args,
							Resources:       kcm.Resources,
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
