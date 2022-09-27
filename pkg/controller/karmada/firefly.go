package karmada

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/util"
)

func (ctrl *KarmadaController) EnsureFireflyKarmadaManager(karmada *installv1alpha1.Karmada) error {
	if err := ctrl.EnsureFireflyKarmadaManagerServiceAccount(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureFireflyKarmadaManagerClusterRoleBinding(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureFireflyKarmadaManagerRoleBinding(karmada); err != nil {
		return err
	}
	return ctrl.EnsureFireflyKarmadaManagerDeployment(karmada)
}

func (ctrl *KarmadaController) EnsureFireflyKarmadaManagerServiceAccount(karmada *installv1alpha1.Karmada) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.ComponentName(constants.FireflyComponentKarmadaManager, karmada.Name),
			Namespace: karmada.Namespace,
		},
	}
	controllerutil.SetOwnerReference(karmada, sa, scheme.Scheme)
	_, err := ctrl.client.CoreV1().ServiceAccounts(karmada.Namespace).Create(context.TODO(), sa, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) EnsureFireflyKarmadaManagerClusterRoleBinding(karmada *installv1alpha1.Karmada) error {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.ComponentName(constants.FireflyComponentKarmadaManager, karmada.Name),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      util.ComponentName(constants.FireflyComponentKarmadaManager, karmada.Name),
				Namespace: karmada.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "firefly:karmada-manager",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	controllerutil.SetOwnerReference(karmada, crb, scheme.Scheme)
	_, err := ctrl.client.RbacV1().ClusterRoleBindings().Create(context.TODO(), crb, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) EnsureFireflyKarmadaManagerRoleBinding(karmada *installv1alpha1.Karmada) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.ComponentName(constants.FireflyComponentKarmadaManager, karmada.Name),
			Namespace: karmada.Namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      util.ComponentName(constants.FireflyComponentKarmadaManager, karmada.Name),
				Namespace: karmada.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "admin",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	controllerutil.SetOwnerReference(karmada, rb, scheme.Scheme)
	_, err := ctrl.client.RbacV1().RoleBindings(karmada.Namespace).Create(context.TODO(), rb, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) EnsureFireflyKarmadaManagerDeployment(karmada *installv1alpha1.Karmada) error {
	componentName := util.ComponentName(constants.FireflyComponentKarmadaManager, karmada.Name)
	repository := karmada.Spec.ImageRepository

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": componentName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": componentName,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: componentName,
					Volumes: []corev1.Volume{
						{
							Name: "karmada-kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-kubeconfig", karmada.Name),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            componentName,
							Image:           util.ComponentImageName(repository, constants.FireflyComponentKarmadaManager, "v1.2.0"),
							ImagePullPolicy: corev1.PullAlways,
							Command: []string{
								"/bin/firefly-karmada-manager",
								"--karmada-kubeconfig",
								"/etc/karmada/kubeconfig",
								"--authentication-kubeconfig",
								"/etc/karmada/kubeconfig",
								"--authorization-kubeconfig",
								"/etc/karmada/kubeconfig",
								"--estimator-namespace",
								karmada.Namespace,
								"--karmada-name",
								karmada.Name,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "karmada-kubeconfig",
									MountPath: "/etc/karmada",
								},
							},
						},
					},
				},
			},
		},
	}

	controllerutil.SetOwnerReference(karmada, deployment, scheme.Scheme)
	_, err := ctrl.client.AppsV1().Deployments(karmada.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
