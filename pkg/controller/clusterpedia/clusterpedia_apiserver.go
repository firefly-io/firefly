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
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	aggregator "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/scheme"
	"github.com/carlory/firefly/pkg/util"
	clientutil "github.com/carlory/firefly/pkg/util/client"
	maputil "github.com/carlory/firefly/pkg/util/map"
	utilresource "github.com/carlory/firefly/pkg/util/resource"
)

// EnsureAPIServer ensures the clusterpedia-apiserver component.
func (ctrl *ClusterpediaController) EnsureAPIServer(clusterpedia *installv1alpha1.Clusterpedia) error {
	hasProvider, err := ctrl.IsControllPlaneProviderExists(clusterpedia)
	if err != nil {
		return err
	}

	if !hasProvider {
		return fmt.Errorf("unsupported without provider")
	}

	if err := ctrl.EnsureAPIServerService(clusterpedia); err != nil {
		return err
	}
	if err := ctrl.EnsureAPIServerDeployment(clusterpedia); err != nil {
		return err
	}
	if err := ctrl.EnsureClusterpediaAPIService(clusterpedia); err != nil {
		return err
	}
	return nil
}

// EnsureAPIServerService ensures the clusterpedia-apiserver service exists.
func (ctrl *ClusterpediaController) EnsureAPIServerService(clusterpedia *installv1alpha1.Clusterpedia) error {
	componentName := constants.ClusterpediaComponentAPIServer
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: clusterpedia.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": componentName},
			Ports: []corev1.ServicePort{
				{
					Name:     "server",
					Protocol: corev1.ProtocolTCP,
					Port:     443,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 6443,
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(clusterpedia, svc, scheme.Scheme)
	return clientutil.CreateOrUpdateService(ctrl.client, svc)
}

// EnsureAPIServerDeployment ensures the clusterpedia-apiserver deployment exists.
func (ctrl *ClusterpediaController) EnsureAPIServerDeployment(clusterpedia *installv1alpha1.Clusterpedia) error {
	componentName := constants.ClusterpediaComponentAPIServer
	server := clusterpedia.Spec.APIServer
	repository := clusterpedia.Spec.ImageRepository
	tag := clusterpedia.Spec.Version
	if server.ImageRepository != "" {
		repository = server.ImageRepository
	}
	if server.ImageTag != "" {
		tag = server.ImageTag
	}

	defaultArgs := map[string]string{
		// Port numbers from 0 to 1023 are reserved for common TCP/IP applications and are called well-known ports.
		// More info: https://www.sciencedirect.com/topics/computer-science/registered-port
		// When this component runs on the high security-ceritical container platform like openshift which enables pod security policy by default,
		// it will fail to start without any privilege rights. so we use port 6443 instead of 443.
		"secure-port":               "6443",
		"kubeconfig":                "/etc/kubeconfig",
		"authentication-kubeconfig": "/etc/kubeconfig",
		"authorization-kubeconfig":  "/etc/kubeconfig",
		"storage-config":            "/etc/clusterpedia/storage/internalstorage-config.yaml",
		"v":                         "3",
	}
	featureGates := maputil.MergeBoolMaps(clusterpedia.Spec.FeatureGates, server.FeatureGates)
	for feature, enabled := range featureGates {
		if defaultArgs["feature-gates"] == "" {
			defaultArgs["feature-gates"] = fmt.Sprintf("%s=%t", feature, enabled)
		} else {
			defaultArgs["feature-gates"] = fmt.Sprintf("%s,%s=%t", defaultArgs["feature-gates"], feature, enabled)
		}
	}
	computedArgs := maputil.MergeStringMaps(defaultArgs, server.ExtraArgs)
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
			Replicas: server.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "apiserver",
							Image:           util.ComponentImageName(repository, "apiserver", tag),
							ImagePullPolicy: "IfNotPresent",
							Command:         []string{"/usr/local/bin/apiserver"},
							Args:            args,
							Resources:       server.Resources,
							LivenessProbe: &corev1.Probe{
								FailureThreshold: 8,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/livez",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 6443,
										},
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								TimeoutSeconds:      15,
							},
							ReadinessProbe: &corev1.Probe{
								FailureThreshold: 3,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 6443,
										},
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    1,
								SuccessThreshold: 1,
								TimeoutSeconds:   15,
							},
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

func (ctrl *ClusterpediaController) EnsureClusterpediaAPIService(clusterpedia *installv1alpha1.Clusterpedia) error {
	kubeconfigSecretName, err := ctrl.KubeConfigSecretNameFromProvider(clusterpedia)
	if err != nil {
		return err
	}
	clientConfig, err := utilresource.GetClientConfigFromKubeConfigSecret(ctrl.client, clusterpedia.Namespace, kubeconfigSecretName, userAgentName)
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return err
	}
	aaClient, err := aggregator.NewForConfig(clientConfig)
	if err != nil {
		return err
	}

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.ClusterpediaComponentAPIServer,
			Namespace: constants.ClusterpediaSystemNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: fmt.Sprintf("%s.%s.svc", constants.ClusterpediaComponentAPIServer, clusterpedia.Namespace),
		},
	}
	if err = clientutil.CreateOrUpdateService(kubeClient, svc); err != nil {
		return err
	}

	apisvc := &apiregistrationv1.APIService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "APIService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "v1beta1.clusterpedia.io",
		},
		Spec: apiregistrationv1.APIServiceSpec{
			InsecureSkipTLSVerify: true,
			Group:                 "clusterpedia.io",
			GroupPriorityMinimum:  1000,
			Service: &apiregistrationv1.ServiceReference{
				Name:      constants.ClusterpediaComponentAPIServer,
				Namespace: constants.ClusterpediaSystemNamespace,
			},
			Version:         "v1beta1",
			VersionPriority: 100,
		},
	}
	return clientutil.CreateOrUpdateAPIService(aaClient, apisvc)
}

func (ctrl *ClusterpediaController) RemoveClusterpediaAPIService(clusterpedia *installv1alpha1.Clusterpedia) error {
	kubeconfigSecretName, err := ctrl.KubeConfigSecretNameFromProvider(clusterpedia)
	if err != nil {
		return err
	}
	clientConfig, err := utilresource.GetClientConfigFromKubeConfigSecret(ctrl.client, clusterpedia.Namespace, kubeconfigSecretName, userAgentName)
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return err
	}
	aaClient, err := aggregator.NewForConfig(clientConfig)
	if err != nil {
		return err
	}

	err = kubeClient.CoreV1().Services(constants.ClusterpediaSystemNamespace).Delete(context.TODO(), constants.ClusterpediaComponentAPIServer, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	err = aaClient.ApiregistrationV1().APIServices().Delete(context.TODO(), "v1beta1.clusterpedia.io", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}
