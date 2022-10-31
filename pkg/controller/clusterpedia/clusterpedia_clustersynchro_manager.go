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

package clusterpedia

import (
	"fmt"

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

func (ctrl *ClusterpediaController) EnsureClusterSynchroManager(clusterpedia *installv1alpha1.Clusterpedia) error {
	hasProvider, err := ctrl.IsControllPlaneProviderExists(clusterpedia)
	if err != nil {
		return err
	}

	if !hasProvider {
		return fmt.Errorf("unsupported without provider")
	}

	return ctrl.EnsureClusterSynchroManagerDeployment(clusterpedia)
}

func (ctrl *ClusterpediaController) EnsureClusterSynchroManagerDeployment(clusterpedia *installv1alpha1.Clusterpedia) error {
	componentName := constants.ClusterpediaComponentClusterSynchroManager
	manager := clusterpedia.Spec.ClusterpediaSynchroManager
	repository := clusterpedia.Spec.ImageRepository
	tag := clusterpedia.Spec.Version
	if manager.ImageRepository != "" {
		repository = manager.ImageRepository
	}
	if manager.ImageTag != "" {
		tag = manager.ImageTag
	}

	defaultArgs := map[string]string{
		"kubeconfig":                      "/etc/kubeconfig",
		"leader-elect-resource-namespace": constants.ClusterpediaSystemNamespace,
		"storage-config":                  "/etc/clusterpedia/storage/internalstorage-config.yaml",
		"v":                               "4",
	}
	featureGates := maputil.MergeBoolMaps(clusterpedia.Spec.FeatureGates, manager.FeatureGates)
	for feature, enabled := range featureGates {
		if defaultArgs["feature-gates"] == "" {
			defaultArgs["feature-gates"] = fmt.Sprintf("%s=%t", feature, enabled)
		} else {
			defaultArgs["feature-gates"] = fmt.Sprintf("%s,%s=%t", defaultArgs["feature-gates"], feature, enabled)
		}
	}
	computedArgs := maputil.MergeStringMaps(defaultArgs, manager.ExtraArgs)
	args := maputil.ConvertToCommandOrArgs(computedArgs)

	kubeconfigSecretName, err := ctrl.KubeConfigSecretNameFromProvider(clusterpedia)
	if err != nil {
		return err
	}

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: clusterpedia.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": componentName},
			},
			Replicas: manager.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "manager",
							Image:           util.ComponentImageName(repository, "clustersynchro-manager", tag),
							ImagePullPolicy: "IfNotPresent",
							Command:         []string{"/usr/local/bin/clustersynchro-manager"},
							Args:            args,
							Env: []corev1.EnvVar{
								{
									Name: "DB_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: GenerateDatabaseSecretName(clusterpedia),
											},
											Key: "password",
										},
									},
								},
							},
							Resources: manager.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "internalstorage-config",
									MountPath: "/etc/clusterpedia/storage",
									ReadOnly:  true,
								},
								{
									Name:      "kubeconfig",
									MountPath: "/etc/kubeconfig",
									SubPath:   "kubeconfig",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "internalstorage-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: GenerateDatabaseConfigMapName(clusterpedia),
									},
								},
							},
						},
						{
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: kubeconfigSecretName,
								},
							},
						},
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(clusterpedia, deployment, scheme.Scheme)
	return clientutil.CreateOrUpdateDeployment(ctrl.client, deployment)
}
