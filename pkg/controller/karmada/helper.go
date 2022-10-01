package karmada

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	aggregator "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
)

// CreateOrUpdateService creates or updates a service
func CreateOrUpdateService(client kubernetes.Interface, svc *corev1.Service) error {
	got, err := client.CoreV1().Services(svc.Namespace).Get(context.TODO(), svc.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		_, err = client.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
		return err
	}
	svc.ResourceVersion = got.ResourceVersion
	_, err = client.CoreV1().Services(svc.Namespace).Update(context.TODO(), svc, metav1.UpdateOptions{})
	return err
}

// CreateOrUpdateDeployment creates or updates a deployment
func CreateOrUpdateDeployment(client kubernetes.Interface, deployment *appsv1.Deployment) error {
	got, err := client.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		_, err = client.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
		return err
	}
	deployment.ResourceVersion = got.ResourceVersion
	_, err = client.AppsV1().Deployments(deployment.Namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
	return err
}

// CreateOrUpdateStatefulSet creates or updates a statefulset
func CreateOrUpdateStatefulSet(client kubernetes.Interface, statefulset *appsv1.StatefulSet) error {
	got, err := client.AppsV1().StatefulSets(statefulset.Namespace).Get(context.TODO(), statefulset.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		_, err = client.AppsV1().StatefulSets(statefulset.Namespace).Create(context.TODO(), statefulset, metav1.CreateOptions{})
		return err
	}
	statefulset.ResourceVersion = got.ResourceVersion
	_, err = client.AppsV1().StatefulSets(statefulset.Namespace).Update(context.TODO(), statefulset, metav1.UpdateOptions{})
	return err
}

// CreateOrUpdateAPIService creates or updates an apiservice
func CreateOrUpdateAPIService(client aggregator.Interface, apisvc *apiregistrationv1.APIService) error {
	got, err := client.ApiregistrationV1().APIServices().Get(context.TODO(), apisvc.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		_, err = client.ApiregistrationV1().APIServices().Create(context.TODO(), apisvc, metav1.CreateOptions{})
		return err
	}
	apisvc.ResourceVersion = got.ResourceVersion
	_, err = client.ApiregistrationV1().APIServices().Update(context.TODO(), apisvc, metav1.UpdateOptions{})
	return err
}

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
