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
	"fmt"

	"github.com/MakeNowJust/heredoc"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/util"
	clientutil "github.com/carlory/firefly/pkg/util/client"
)

func (ctrl *ClusterpediaController) EnsureMySQL(clusterpedia *installv1alpha1.Clusterpedia) error {
	if err := ctrl.EnsureMySQLService(clusterpedia); err != nil {
		return err
	}
	if err := ctrl.EnsureMySQLSecret(clusterpedia); err != nil {
		return err
	}
	if err := ctrl.EnsureMySQLConfigMap(clusterpedia); err != nil {
		return err
	}
	return ctrl.EnsureMySQLDeployment(clusterpedia)
}

// EnsureMySQLService ensures the clusterpedia-internalstorage-mysql service exists.
func (ctrl *ClusterpediaController) EnsureMySQLService(clusterpedia *installv1alpha1.Clusterpedia) error {
	componentName := util.ComponentName(constants.ClusterpediaComponentInternalStorageMySQL, clusterpedia.Name)
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: clusterpedia.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app":                                  componentName,
				"internalstorage.clusterpedia.io/type": "mysql",
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "server",
					Protocol: corev1.ProtocolTCP,
					Port:     3306,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 3306,
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(clusterpedia, svc, scheme.Scheme)
	return clientutil.CreateOrUpdateService(ctrl.client, svc)
}

// EnsureMySQLSecret ensures the clusterpedia-internalstorage-mysql secret exists.
func (ctrl *ClusterpediaController) EnsureMySQLSecret(clusterpedia *installv1alpha1.Clusterpedia) error {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateDatabaseSecretName(clusterpedia),
			Namespace: clusterpedia.Namespace,
		},
		Data: map[string][]byte{
			"password": []byte("dangerous0"),
		},
	}
	controllerutil.SetOwnerReference(clusterpedia, secret, scheme.Scheme)
	return clientutil.CreateOrUpdateSecret(ctrl.client, secret)
}

// EnsureMySQLConfigMap ensures the clusterpedia-internalstorage-mysql configmap exists.
func (ctrl *ClusterpediaController) EnsureMySQLConfigMap(clusterpedia *installv1alpha1.Clusterpedia) error {
	svcName := util.ComponentName(constants.ClusterpediaComponentInternalStorageMySQL, clusterpedia.Name)
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateDatabaseConfigMapName(clusterpedia),
			Namespace: clusterpedia.Namespace,
		},
		Data: map[string]string{
			"internalstorage-config.yaml": heredoc.Docf(`
			type: "mysql"
			host: %q
			port: 3306
			user: root
			database: "clusterpedia"`, svcName),
		},
	}
	controllerutil.SetOwnerReference(clusterpedia, cm, scheme.Scheme)
	return clientutil.CreateOrUpdateConfigMap(ctrl.client, cm)
}

// EnsureMySQLDeployment ensures the clusterpedia-internalstorage-mysql deployment exists.
func (ctrl *ClusterpediaController) EnsureMySQLDeployment(clusterpedia *installv1alpha1.Clusterpedia) error {
	componentName := util.ComponentName(constants.ClusterpediaComponentInternalStorageMySQL, clusterpedia.Name)
	image := clusterpedia.Spec.Storage.MySQL.Local

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: clusterpedia.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":                                  componentName,
					"internalstorage.clusterpedia.io/type": "mysql",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                                  componentName,
						"internalstorage.clusterpedia.io/type": "mysql",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "mysql",
							Image:           fmt.Sprintf("%s/%s:%s", image.ImageRepository, image.ImageName, image.ImageTag),
							ImagePullPolicy: "IfNotPresent",
							Args: []string{
								"--default-authentication-plugin=mysql_native_password",
							},
							Env: []corev1.EnvVar{
								{
									Name:  "MYSQL_DATABASE",
									Value: "clusterpedia",
								},
								{
									Name: "MYSQL_ROOT_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: GenerateDatabaseSecretName(clusterpedia),
											},
											Key: "password",
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "mysql",
									ContainerPort: 3306,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/mysql",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(clusterpedia, deployment, scheme.Scheme)
	return clientutil.CreateOrUpdateDeployment(ctrl.client, deployment)
}
