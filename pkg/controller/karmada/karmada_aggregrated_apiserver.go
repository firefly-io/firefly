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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	aggregator "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/firefly-io/firefly/pkg/apis/install/v1alpha1"
	"github.com/firefly-io/firefly/pkg/constants"
	"github.com/firefly-io/firefly/pkg/scheme"
	"github.com/firefly-io/firefly/pkg/util"
	clientutil "github.com/firefly-io/firefly/pkg/util/client"
	maputil "github.com/firefly-io/firefly/pkg/util/map"
)

func (ctrl *KarmadaController) EnsureKarmadaAggregatedAPIServer(karmada *installv1alpha1.Karmada) error {
	if err := ctrl.EnsureKarmadaAggregatedAPIServerService(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureKarmadaAggregatedAPIServerDeployment(karmada); err != nil {
		return err
	}
	podLabel := fmt.Sprintf("app=%s", constants.KarmadaComponentAggregratedAPIServer)
	err := util.NewKubeWaiter(ctrl.client, 10*time.Second).WaitForPodsWithLabel(karmada.Namespace, podLabel)
	if err != nil {
		return err
	}
	return ctrl.EnsureKarmadaAggregatedAPIServerAPIService(karmada)
}

func (ctrl *KarmadaController) EnsureKarmadaAggregatedAPIServerService(karmada *installv1alpha1.Karmada) error {
	componentName := constants.KarmadaComponentAggregratedAPIServer
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": componentName},
			Ports: []corev1.ServicePort{
				{
					Protocol: corev1.ProtocolTCP,
					Port:     443,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 443,
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(karmada, svc, scheme.Scheme)
	return clientutil.CreateOrUpdateService(ctrl.client, svc)
}

func (ctrl *KarmadaController) EnsureKarmadaAggregatedAPIServerDeployment(karmada *installv1alpha1.Karmada) error {
	componentName := constants.KarmadaComponentAggregratedAPIServer
	server := karmada.Spec.APIServer.KarmadaAggregratedAPIServer
	repository := karmada.Spec.ImageRepository
	if server.ImageRepository != "" {
		repository = server.ImageRepository
	}
	imageName := constants.KarmadaComponentAggregratedAPIServer
	if server.ImageName != "" {
		imageName = server.ImageName
	}
	tag := karmada.Spec.KarmadaVersion
	if server.ImageTag != "" {
		tag = server.ImageTag
	}

	defaultArgs := map[string]string{
		"kubeconfig":                "/etc/kubeconfig",
		"authentication-kubeconfig": "/etc/kubeconfig",
		"authorization-kubeconfig":  "/etc/kubeconfig",
		"etcd-cafile":               "/etc/kubernetes/pki/etcd-ca.crt",
		"etcd-certfile":             "/etc/kubernetes/pki/etcd-client.crt",
		"etcd-keyfile":              "/etc/kubernetes/pki/etcd-client.key",
		"etcd-servers":              fmt.Sprintf("https://%s.%s.svc:2379", constants.KarmadaComponentEtcd, karmada.Namespace),
		"audit-log-path":            "-",
		"feature-gates":             "APIPriorityAndFairness=false",
		"audit-log-maxage":          "0",
		"audit-log-maxbackup":       "0",
		"tls-cert-file":             "/etc/kubernetes/pki/apiserver.crt",
		"tls-private-key-file":      "/etc/kubernetes/pki/apiserver.key",
	}
	featureGates := karmada.Spec.FeatureGates
	for feature, enabled := range featureGates {
		if defaultArgs["feature-gates"] == "" {
			defaultArgs["feature-gates"] = fmt.Sprintf("%s=%t", feature, enabled)
		} else {
			defaultArgs["feature-gates"] = fmt.Sprintf("%s,%s=%t", defaultArgs["feature-gates"], feature, enabled)
		}
	}
	computedArgs := maputil.MergeStringMaps(defaultArgs, server.ExtraArgs)
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
			Replicas: server.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "karmada-aggregated-apiserver",
							Image:           util.ComponentImageName(repository, imageName, tag),
							ImagePullPolicy: "IfNotPresent",
							Command:         []string{"/bin/karmada-aggregated-apiserver"},
							Args:            args,
							Resources:       server.Resources,
							LivenessProbe: &corev1.Probe{
								FailureThreshold: 8,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/livez",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 443,
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
											IntVal: 443,
										},
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    1,
								SuccessThreshold: 1,
								TimeoutSeconds:   15,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "k8s-certs",
									MountPath: "/etc/kubernetes/pki",
									ReadOnly:  true,
								},
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
							Name: "k8s-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "karmada-cert",
								},
							},
						},
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

func (ctrl *KarmadaController) EnsureKarmadaAggregatedAPIServerAPIService(karmada *installv1alpha1.Karmada) error {
	clientConfig, err := ctrl.GenerateClientConfig(karmada)
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
			Name:      constants.KarmadaComponentAggregratedAPIServer,
			Namespace: constants.KarmadaSystemNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: fmt.Sprintf("%s.%s.svc", constants.KarmadaComponentAggregratedAPIServer, karmada.Namespace),
		},
	}
	if err = clientutil.CreateOrUpdateService(kubeClient, svc); err != nil {
		return err
	}

	aaAPIServiceObjName := "v1alpha1.cluster.karmada.io"
	apisvc := &apiregistrationv1.APIService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "APIService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   aaAPIServiceObjName,
			Labels: map[string]string{"app": "karmada-aggregated-apiserver", "apiserver": "true"},
		},
		Spec: apiregistrationv1.APIServiceSpec{
			InsecureSkipTLSVerify: true,
			Group:                 "cluster.karmada.io",
			GroupPriorityMinimum:  2000,
			Service: &apiregistrationv1.ServiceReference{
				Name:      constants.KarmadaComponentAggregratedAPIServer,
				Namespace: constants.KarmadaSystemNamespace,
			},
			Version:         "v1alpha1",
			VersionPriority: 10,
		},
	}
	return clientutil.CreateOrUpdateAPIService(aaClient, apisvc)
}
