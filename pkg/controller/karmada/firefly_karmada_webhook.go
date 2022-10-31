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
	"encoding/base64"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/firefly-io/firefly/pkg/apis/install/v1alpha1"
	"github.com/firefly-io/firefly/pkg/constants"
	"github.com/firefly-io/firefly/pkg/scheme"
	"github.com/firefly-io/firefly/pkg/util"
	clientutil "github.com/firefly-io/firefly/pkg/util/client"
	maputil "github.com/firefly-io/firefly/pkg/util/map"
)

func (ctrl *KarmadaController) EnsureFireflyKaramdaWebhook(karmada *installv1alpha1.Karmada) error {
	if err := ctrl.EnsureFireflyKarmadaWebhookConfiguration(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureFireflyKaramdaWebhookService(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureFireflyKaramdaWebhookDeployment(karmada); err != nil {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) EnsureFireflyKaramdaWebhookService(karmada *installv1alpha1.Karmada) error {
	componentName := constants.FireflyComponentKarmadaWebhook
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
						IntVal: 8443,
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(karmada, svc, scheme.Scheme)
	return clientutil.CreateOrUpdateService(ctrl.client, svc)
}

func (ctrl *KarmadaController) EnsureFireflyKaramdaWebhookDeployment(karmada *installv1alpha1.Karmada) error {
	componentName := constants.FireflyComponentKarmadaWebhook
	webhook := karmada.Spec.Webhook.FireflyKarmadaWebhook

	repository := karmada.Spec.ImageRepository
	if karmada.Spec.FireflyImageRepository != "" {
		repository = karmada.Spec.FireflyImageRepository
	}
	if webhook.ImageRepository != "" {
		repository = webhook.ImageRepository
	}

	imageName := constants.FireflyComponentKarmadaWebhook
	if webhook.ImageName != "" {
		imageName = webhook.ImageName
	}

	tag := "latest"
	if webhook.ImageTag != "" {
		tag = webhook.ImageTag
	}

	defaultArgs := map[string]string{
		"bind-address": "0.0.0.0",
		"kubeconfig":   "/etc/kubeconfig",
		"secure-port":  "8443",
		"cert-dir":     "/var/serving-cert",
		"v":            "4",
	}
	computedArgs := maputil.MergeStringMaps(defaultArgs, webhook.ExtraArgs)
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
			Replicas: webhook.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "firefly-karmada-webhook",
							Image:           util.ComponentImageName(repository, imageName, tag),
							ImagePullPolicy: "Always",
							Command:         []string{"/bin/firefly-karmada-webhook"},
							Args:            args,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8443,
								},
							},
							Resources: webhook.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kubeconfig",
									MountPath: "/etc/kubeconfig",
									SubPath:   "kubeconfig",
								},
								{
									Name:      "cert",
									MountPath: "/var/serving-cert",
									ReadOnly:  true,
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
						{
							Name: "cert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-cert", componentName),
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

func (ctrl *KarmadaController) EnsureFireflyKarmadaWebhookConfiguration(karmada *installv1alpha1.Karmada) error {
	clientConfig, err := ctrl.GenerateClientConfig(karmada)
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return err
	}

	karmadaCert, err := ctrl.client.CoreV1().Secrets(karmada.Namespace).Get(context.TODO(), "karmada-cert", metav1.GetOptions{})
	if err != nil {
		return err
	}
	caCrt := karmadaCert.Data["ca.crt"]
	caBunlde := base64.StdEncoding.EncodeToString(caCrt)
	if err := createMutatingWebhookConfiguration(client, genFireflyKarmadaWebhookMutatingConfig(caBunlde, karmada)); err != nil {
		return err
	}
	return nil
}

func genFireflyKarmadaWebhookMutatingConfig(caBundle string, karmada *installv1alpha1.Karmada) string {
	return fmt.Sprintf(`apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: zzzz-firefly-karmada-webhook
  labels:
    app: firefly-karmada-webhook
webhooks:
  - name: firefly-work.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["work.karmada.io"]
        apiVersions: ["*"]
        resources: ["works"]
        scope: "Namespaced"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/mutate-work-karmada-io-v1alpha1-work
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3`, karmada.Namespace, caBundle, constants.FireflyComponentKarmadaWebhook)
}
