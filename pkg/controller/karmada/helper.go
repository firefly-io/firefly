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
