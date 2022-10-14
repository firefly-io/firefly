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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	utilresource "github.com/carlory/firefly/pkg/util/resource"
)

// IsControllPlaneProviderExists check if the given provider exists.
func (ctrl *ClusterpediaController) IsControllPlaneProviderExists(clusterpedia *installv1alpha1.Clusterpedia) (bool, error) {
	provider := clusterpedia.Spec.ControlplaneProvider
	if provider == nil {
		return false, nil
	}
	if provider.Karmada != nil {
		karmadaName := provider.Karmada.Name
		karmada, err := ctrl.fireflyClient.InstallV1alpha1().Karmadas(clusterpedia.Namespace).Get(context.TODO(), karmadaName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if karmada.DeletionTimestamp != nil {
			return false, fmt.Errorf("the karmada %s is terminating", karmadaName)
		}
		return true, nil
	}
	return false, nil
}

// GetControlplaneClient returns the client of the controlplane.
func (ctrl *ClusterpediaController) GetControlplaneClient(clusterpedia *installv1alpha1.Clusterpedia) (kubernetes.Interface, error) {
	hasProvider, err := ctrl.IsControllPlaneProviderExists(clusterpedia)
	if err != nil {
		return nil, err
	}

	if !hasProvider {
		return ctrl.client, nil
	}
	return ctrl.GetControlplaneClientFromProvider(clusterpedia)
}

// GetControlplaneClientFromProvider returns the client of the controlplane according to a provider.
func (ctrl *ClusterpediaController) GetControlplaneClientFromProvider(clusterpedia *installv1alpha1.Clusterpedia) (kubernetes.Interface, error) {
	kubeconfigSecretName, err := ctrl.KubeConfigSecretNameFromProvider(clusterpedia)
	if err != nil {
		return nil, err
	}
	clientConfig, err := utilresource.GetClientConfigFromKubeConfigSecret(ctrl.client, clusterpedia.Namespace, kubeconfigSecretName, userAgentName)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(clientConfig)
}

// KubeConfigSecretNameFromProvider returns the name of a kubeconfig secret according to the given provider.
func (ctrl *ClusterpediaController) KubeConfigSecretNameFromProvider(clusterpedia *installv1alpha1.Clusterpedia) (string, error) {
	provider := clusterpedia.Spec.ControlplaneProvider
	if provider == nil {
		return "", fmt.Errorf("no provider found")
	}
	if provider.Karmada != nil {
		return ctrl.KubeConfigSecretNameFromKarmada(provider.Karmada)
	}
	return "", nil
}

func (ctrl *ClusterpediaController) KubeConfigSecretNameFromKarmada(provider *installv1alpha1.ClusterpediaControlplaneProviderKarmada) (string, error) {
	karmadaName := provider.Name
	secretName := fmt.Sprintf("%s-kubeconfig", karmadaName)
	return secretName, nil
}
