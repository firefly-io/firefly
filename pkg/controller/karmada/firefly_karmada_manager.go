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
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/scheme"
	"github.com/carlory/firefly/pkg/util"
	maputil "github.com/carlory/firefly/pkg/util/map"
	utilresource "github.com/carlory/firefly/pkg/util/resource"
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
	if err := ctrl.EnsureFireflyKarmadaManagerDeployment(karmada); err != nil {
		return err
	}
	return ctrl.EnsureFireflyKarmadaManagerCRDs(karmada)
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

	fkm := karmada.Spec.ControllerManager.FireflyKarmadaManager
	tag := "latest"
	if fkm.ImageRepository != "" {
		repository = fkm.ImageRepository
	}
	if fkm.ImageTag != "" {
		tag = fkm.ImageTag
	}

	defaultArgs := map[string]string{
		"karmada-kubeconfig":        "/etc/karmada/kubeconfig",
		"authentication-kubeconfig": "/etc/karmada/kubeconfig",
		"authorization-kubeconfig":  "/etc/karmada/kubeconfig",
		"estimator-namespace":       karmada.Namespace,
		"karmada-name":              karmada.Name,
		"v":                         "4",
	}
	if fkm.Controllers != nil {
		defaultArgs["controllers"] = strings.Join(fkm.Controllers, ",")
	}

	args := maputil.ConvertToCommandOrArgs(defaultArgs)

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
							Image:           util.ComponentImageName(repository, constants.FireflyComponentKarmadaManager, tag),
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"firefly-karmada-manager"},
							Args:            args,
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

func (ctrl *KarmadaController) EnsureFireflyKarmadaManagerCRDs(karmada *installv1alpha1.Karmada) error {
	clientConfig, err := ctrl.GenerateClientConfig(karmada)
	if err != nil {
		return err
	}
	result := utilresource.NewBuilder(clientConfig).
		Unstructured().
		FilenameParam(false, &resource.FilenameOptions{Recursive: false, Filenames: []string{"./pkg/karmada/crds"}}).
		Flatten().Do()
	return result.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		_, err1 := resource.NewHelper(info.Client, info.Mapping).Create(info.Namespace, true, info.Object)
		if err1 != nil {
			if !errors.IsAlreadyExists(err1) {
				return err1
			}
		}
		return nil
	})
}
