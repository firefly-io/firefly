package karmada

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/util"
	maputil "github.com/carlory/firefly/pkg/util/map"
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
	componentName := util.ComponentName(constants.KarmadaComponentWebhook, karmada.Name)
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
	return CreateOrUpdateService(ctrl.client, svc)
}

func (ctrl *KarmadaController) EnsureKaramdaWebhookDeployment(karmada *installv1alpha1.Karmada) error {
	componentName := util.ComponentName(constants.KarmadaComponentWebhook, karmada.Name)
	repository := karmada.Spec.ImageRepository
	version := karmada.Spec.KarmadaVersion
	webhook := karmada.Spec.Webhook.KarmadaWebhook

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
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "karmada-webhook",
							Image:           util.ComponentImageName(repository, constants.KarmadaComponentWebhook, version),
							ImagePullPolicy: "IfNotPresent",
							Command:         []string{"/bin/karmada-webhook"},
							Args:            args,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8443,
								},
							},
							Resources: karmada.Spec.Webhook.KarmadaWebhook.Resources,
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
									SecretName: fmt.Sprintf("%s-kubeconfig", karmada.Name),
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
	return CreateOrUpdateDeployment(ctrl.client, deployment)
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

	karmadaCert, err := ctrl.client.CoreV1().Secrets(karmada.Namespace).Get(context.TODO(), fmt.Sprintf("%s-cert", util.ComponentName("karmada", karmada.Name)), metav1.GetOptions{})
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
    timeoutSeconds: 3`, karmada.Namespace, caBundle, util.ComponentName(constants.KarmadaComponentWebhook, karmada.Name))
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
    timeoutSeconds: 3`, karmada.Namespace, caBundle, util.ComponentName(constants.KarmadaComponentWebhook, karmada.Name))
}

func createValidatingWebhookConfiguration(c kubernetes.Interface, staticYaml string) error {
	obj := admissionregistrationv1.ValidatingWebhookConfiguration{}

	if err := json.Unmarshal(StaticYamlToJSONByte(staticYaml), &obj); err != nil {
		klog.Errorln("Error convert json byte to admissionregistration v1 ValidatingWebhookConfiguration struct.")
		return err
	}

	_, err := c.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(context.TODO(), &obj, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			latest, err := c.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if err != nil {
				klog.Errorln("Error get validating webhook configuration.")
				return err
			}
			obj.ResourceVersion = latest.ResourceVersion
			_, err = c.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(context.TODO(), &obj, metav1.UpdateOptions{})
			if err != nil {
				klog.Errorln("Error update validating webhook configuration.")
				return err
			}
			return nil
		}
		return err
	}
	return nil
}

func createMutatingWebhookConfiguration(c kubernetes.Interface, staticYaml string) error {
	obj := admissionregistrationv1.MutatingWebhookConfiguration{}

	if err := json.Unmarshal(StaticYamlToJSONByte(staticYaml), &obj); err != nil {
		klog.Errorln("Error convert json byte to admissionregistration v1 MutatingWebhookConfiguration struct.")
		return err
	}

	_, err := c.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.TODO(), &obj, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			latest, err := c.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			obj.ResourceVersion = latest.ResourceVersion
			_, err = c.AdmissionregistrationV1().MutatingWebhookConfigurations().Update(context.TODO(), &obj, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return nil
		}
		return err
	}
	return nil
}

// StaticYamlToJSONByte  Static yaml file conversion JSON Byte
func StaticYamlToJSONByte(staticYaml string) []byte {
	jsonByte, err := yaml.YAMLToJSON([]byte(staticYaml))
	if err != nil {
		fmt.Printf("Error convert string to json byte: %v", err)
		os.Exit(1)
	}
	return jsonByte
}
