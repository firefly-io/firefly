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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
)

func (ctrl *KarmadaController) GenerateClientConfig(karmada *installv1alpha1.Karmada) (*restclient.Config, error) {
	kubeconfigSecret, err := ctrl.client.CoreV1().Secrets(karmada.Namespace).Get(context.TODO(), fmt.Sprintf("%s-kubeconfig", karmada.Name), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	config, err := clientcmd.NewClientConfigFromBytes(kubeconfigSecret.Data["kubeconfig"])
	if err != nil {
		return nil, err
	}
	clientConfig, err := config.ClientConfig()
	if err != nil {
		return nil, err
	}
	return restclient.AddUserAgent(clientConfig, "karmada-controller"), nil
}
