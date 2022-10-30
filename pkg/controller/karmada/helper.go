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
	"encoding/json"
	"fmt"
	"os"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	utilresource "github.com/carlory/firefly/pkg/util/resource"
)

const (
	// the user-agent name is used when talking to karmada apiserver
	userAgentName = "karmada-controller"
)

func (ctrl *KarmadaController) GenerateClientConfig(karmada *installv1alpha1.Karmada) (*restclient.Config, error) {
	secretName := "karmada-kubeconfig"
	return utilresource.GetClientConfigFromKubeConfigSecret(ctrl.client, karmada.Namespace, secretName, userAgentName)
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
