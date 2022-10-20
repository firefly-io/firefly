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
	restclient "k8s.io/client-go/rest"

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
