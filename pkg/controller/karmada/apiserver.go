package karmada

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	aggregator "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/util"
)

func (ctrl *KarmadaController) initKarmadaAPIServer(karmada *installv1alpha1.Karmada) error {
	klog.Info("create etcd")
	etcdService := makeEtcdService(karmada)
	controllerutil.SetOwnerReference(karmada, etcdService, scheme.Scheme)
	if err := ctrl.CreateService(etcdService); err != nil {
		return err
	}
	etcdStatefulset := makeETCDStatefulSet(karmada)
	controllerutil.SetOwnerReference(karmada, etcdStatefulset, scheme.Scheme)
	if _, err := ctrl.client.AppsV1().StatefulSets(karmada.Namespace).Create(context.TODO(), etcdStatefulset, metav1.CreateOptions{}); err != nil {
		klog.Warning(err)
	}

	klog.Info("create karmada ApiServer Deployment")
	karmadaAPIServerService := makeKarmadaAPIServerService(karmada)
	controllerutil.SetOwnerReference(karmada, karmadaAPIServerService, scheme.Scheme)
	if err := ctrl.CreateService(karmadaAPIServerService); err != nil {
		return err
	}
	karmadaAPIServerDeployment := makeKarmadaAPIServerDeployment(karmada)
	controllerutil.SetOwnerReference(karmada, karmadaAPIServerDeployment, scheme.Scheme)
	if _, err := ctrl.client.AppsV1().Deployments(karmada.Namespace).Create(context.TODO(), karmadaAPIServerDeployment, metav1.CreateOptions{}); err != nil {
		klog.Warning(err)
	}

	kubeconfigSecret, err := ctrl.client.CoreV1().Secrets(karmada.Namespace).Get(context.TODO(), fmt.Sprintf("%s-kubeconfig", karmada.Name), metav1.GetOptions{})
	if err != nil {
		return err
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfigSecret.Data["kubeconfig"])
	if err != nil {
		return err
	}
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	waiter := util.NewKubeWaiter(kubeClient, 10*time.Second)
	if err := waiter.WaitForKubeAPI(); err != nil {
		return err
	}

	klog.Info("kube api ready")

	// Create karmada-aggregated-apiserver
	// https://github.com/karmada-io/karmada/blob/master/artifacts/deploy/karmada-aggregated-apiserver.yaml
	klog.Info("create karmada aggregated apiserver Deployment")
	aggregatedAPIServerService := karmadaAggregatedAPIServerService(karmada)
	controllerutil.SetOwnerReference(karmada, aggregatedAPIServerService, scheme.Scheme)
	if err := ctrl.CreateService(aggregatedAPIServerService); err != nil {
		return err
	}
	aggregatedAPIServerDeployment := makeKarmadaAggregatedAPIServerDeployment(karmada)
	controllerutil.SetOwnerReference(karmada, aggregatedAPIServerDeployment, scheme.Scheme)
	if _, err := ctrl.client.AppsV1().Deployments(karmada.Namespace).Create(context.TODO(), aggregatedAPIServerDeployment, metav1.CreateOptions{}); err != nil {
		klog.Warning(err)
	}
	waiter = util.NewKubeWaiter(ctrl.client, 10*time.Second)

	if err := waiter.WaitForPodsWithLabel(karmada.Namespace, fmt.Sprintf("app=%s", ComponentName(KarmadaComponentAggregratedAPIServer, karmada.Name))); err != nil {
		return err
	}

	_, err = kubeClient.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "karmada-system"}}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	if err := ctrl.initAPIService(kubeClient, restConfig, karmada); err != nil {
		klog.ErrorS(err, "init karmada aggregated apiserver")
		return err
	}

	if err := ctrl.initcrds(kubeClient, restConfig, karmada.Namespace); err != nil {
		klog.ErrorS(err, "init karmada crds")
		return err
	}

	if err := ctrl.initWebhook(kubeClient, restConfig, karmada); err != nil {
		klog.ErrorS(err, "init karmada webhook")
		return err
	}

	return nil
}

func (ctrl *KarmadaController) initWebhook(karmadaClient kubernetes.Interface, restConfig *rest.Config, karmada *installv1alpha1.Karmada) error {
	karmadaCert, err := ctrl.client.CoreV1().Secrets(karmada.Namespace).Get(context.TODO(), fmt.Sprintf("%s-cert", ComponentName("karmada", karmada.Name)), metav1.GetOptions{})
	if err != nil {
		return err
	}
	caCrt := karmadaCert.Data["ca.crt"]
	caBunlde := base64.StdEncoding.EncodeToString(caCrt)
	if err := createValidatingWebhookConfiguration(karmadaClient, validatingConfig(caBunlde, karmada)); err != nil {
		return err
	}

	if err := createMutatingWebhookConfiguration(karmadaClient, mutatingConfig(caBunlde, karmada)); err != nil {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) initAPIService(karmadaClient kubernetes.Interface, restConfig *rest.Config, karmada *installv1alpha1.Karmada) error {
	aaService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "karmada-aggregated-apiserver",
			Namespace: "karmada-system",
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: fmt.Sprintf("%s.%s.svc", ComponentName(KarmadaComponentAggregratedAPIServer, karmada.Name), karmada.Namespace),
		},
	}
	_, err := karmadaClient.CoreV1().Services("karmada-system").Create(context.TODO(), aaService, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	apiRegistrationClient, err := aggregator.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	aaAPIServiceObjName := "v1alpha1.cluster.karmada.io"
	aaAPIService := &apiregistrationv1.APIService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "APIService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   aaAPIServiceObjName,
			Labels: map[string]string{"app": "karmada-aggregated-apiserver", "apiserver": "true"},
		},
		Spec: apiregistrationv1.APIServiceSpec{
			InsecureSkipTLSVerify: true,
			Group:                 "cluster.karmada.io",
			GroupPriorityMinimum:  2000,
			Service: &apiregistrationv1.ServiceReference{
				Name:      "karmada-aggregated-apiserver",
				Namespace: "karmada-system",
			},
			Version:         "v1alpha1",
			VersionPriority: 10,
		},
	}

	_, err = apiRegistrationClient.ApiregistrationV1().APIServices().Create(context.TODO(), aaAPIService, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) initKarmadaComponent(karmada *installv1alpha1.Karmada) error {
	namespace := karmada.Namespace
	kubeControllerManagerDeployment := makeKarmadaKubeControllerManagerDeployment(karmada)
	controllerutil.SetOwnerReference(karmada, kubeControllerManagerDeployment, scheme.Scheme)
	_, err := ctrl.client.AppsV1().Deployments(namespace).Create(context.TODO(), kubeControllerManagerDeployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	karmadaSchedulerDeployment := makeKarmadaSchedulerDeployment(karmada)
	controllerutil.SetOwnerReference(karmada, karmadaSchedulerDeployment, scheme.Scheme)
	_, err = ctrl.client.AppsV1().Deployments(namespace).Create(context.TODO(), karmadaSchedulerDeployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	controllerManagerDeployment := makeKarmadaControllerManagerDeployment(karmada)
	controllerutil.SetOwnerReference(karmada, controllerManagerDeployment, scheme.Scheme)
	_, err = ctrl.client.AppsV1().Deployments(namespace).Create(context.TODO(), controllerManagerDeployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	webhookService := karmadaWebhookService(karmada)
	if err := controllerutil.SetOwnerReference(karmada, webhookService, scheme.Scheme); err != nil {
		klog.ErrorS(err, "init karmada SetOwnerReference apiserver")
		return err
	}
	if err := ctrl.CreateService(webhookService); err != nil {
		return err
	}
	webhookDeployment := makeKarmadaWebhookDeployment(karmada)
	controllerutil.SetOwnerReference(karmada, webhookDeployment, scheme.Scheme)
	_, err = ctrl.client.AppsV1().Deployments(namespace).Create(context.TODO(), webhookDeployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	if err := ctrl.EnsureFireflyKarmadaManager(karmada); err != nil {
		klog.ErrorS(err, "init karmada EnsureFireflyKarmadaManager")
		return err
	}
	return nil
}
