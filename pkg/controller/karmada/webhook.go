package karmada

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
)

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
    timeoutSeconds: 3`, karmada.Namespace, caBundle, ComponentName(KarmadaComponentWebhook, karmada.Name))
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
    timeoutSeconds: 3`, karmada.Namespace, caBundle, ComponentName(KarmadaComponentWebhook, karmada.Name))
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
