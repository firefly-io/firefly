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

func (ctrl *KarmadaController) EnsureKaramdaWebhook(karmada *installv1alpha1.Karmada) error {
	if err := ctrl.EnsureKarmadaWebhookConfiguration(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureKaramdaWebhookService(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureKaramdaWebhookDeployment(karmada); err != nil {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) EnsureKaramdaWebhookService(karmada *installv1alpha1.Karmada) error {
	componentName := constants.KarmadaComponentWebhook
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

func (ctrl *KarmadaController) EnsureKaramdaWebhookDeployment(karmada *installv1alpha1.Karmada) error {
	componentName := constants.KarmadaComponentWebhook
	webhook := karmada.Spec.Webhook.KarmadaWebhook

	repository := karmada.Spec.ImageRepository
	if webhook.ImageRepository != "" {
		repository = webhook.ImageRepository
	}

	imageName := constants.KarmadaComponentWebhook
	if webhook.ImageName != "" {
		imageName = webhook.ImageName
	}

	tag := karmada.Spec.KarmadaVersion
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
							Name:            "karmada-webhook",
							Image:           util.ComponentImageName(repository, imageName, tag),
							ImagePullPolicy: "IfNotPresent",
							Command:         []string{"/bin/karmada-webhook"},
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

func (ctrl *KarmadaController) EnsureKarmadaWebhookConfiguration(karmada *installv1alpha1.Karmada) error {
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
	if err := createValidatingWebhookConfiguration(client, validatingConfig(caBunlde, karmada)); err != nil {
		return err
	}

	if err := createMutatingWebhookConfiguration(client, mutatingConfig(caBunlde, karmada)); err != nil {
		return err
	}
	return nil
}

func mutatingConfig(caBundle string, karmada *installv1alpha1.Karmada) string {
	return fmt.Sprintf(`apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mutating-config
  labels:
    app: mutating-config
webhooks:
  - name: propagationpolicy.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["policy.karmada.io"]
        apiVersions: ["*"]
        resources: ["propagationpolicies"]
        scope: "Namespaced"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/mutate-propagationpolicy
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3
  - name: clusterpropagationpolicy.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["policy.karmada.io"]
        apiVersions: ["*"]
        resources: ["clusterpropagationpolicies"]
        scope: "Cluster"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/mutate-clusterpropagationpolicy
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3
  - name: overridepolicy.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["policy.karmada.io"]
        apiVersions: ["*"]
        resources: ["overridepolicies"]
        scope: "Namespaced"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/mutate-overridepolicy
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3
  - name: work.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["work.karmada.io"]
        apiVersions: ["*"]
        resources: ["works"]
        scope: "Namespaced"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/mutate-work
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3`, karmada.Namespace, caBundle, constants.KarmadaComponentWebhook)
}

func validatingConfig(caBundle string, karmada *installv1alpha1.Karmada) string {
	return fmt.Sprintf(`apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: validating-config
  labels:
    app: validating-config
webhooks:
  - name: propagationpolicy.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["policy.karmada.io"]
        apiVersions: ["*"]
        resources: ["propagationpolicies"]
        scope: "Namespaced"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/validate-propagationpolicy
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3
  - name: clusterpropagationpolicy.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["policy.karmada.io"]
        apiVersions: ["*"]
        resources: ["clusterpropagationpolicies"]
        scope: "Cluster"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/validate-clusterpropagationpolicy
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3
  - name: overridepolicy.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["policy.karmada.io"]
        apiVersions: ["*"]
        resources: ["overridepolicies"]
        scope: "Namespaced"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/validate-overridepolicy
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3
  - name: clusteroverridepolicy.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["policy.karmada.io"]
        apiVersions: ["*"]
        resources: ["clusteroverridepolicies"]
        scope: "Cluster"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/validate-clusteroverridepolicy
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3
  - name: config.karmada.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["config.karmada.io"]
        apiVersions: ["*"]
        resources: ["resourceexploringwebhookconfigurations"]
        scope: "Cluster"
    clientConfig:
      url: https://%[3]s.%[1]s.svc:443/validate-resourceexploringwebhookconfiguration
      caBundle: %[2]s
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 3`, karmada.Namespace, caBundle, constants.KarmadaComponentWebhook)
}
