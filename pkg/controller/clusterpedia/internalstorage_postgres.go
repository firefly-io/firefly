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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/scheme"
	"github.com/carlory/firefly/pkg/util"
	clientutil "github.com/carlory/firefly/pkg/util/client"
)

func (ctrl *ClusterpediaController) EnsurePostgres(clusterpedia *installv1alpha1.Clusterpedia) error {
	if err := ctrl.EnsurePostgresService(clusterpedia); err != nil {
		return err
	}
	if err := ctrl.EnsurePostgresSecret(clusterpedia); err != nil {
		return err
	}
	if err := ctrl.EnsurePostgresConfigMap(clusterpedia); err != nil {
		return err
	}
	return ctrl.EnsurePostgresDeployment(clusterpedia)
}

// EnsurePostgresService ensures the clusterpedia-internalstorage-postgres service exists.
func (ctrl *ClusterpediaController) EnsurePostgresService(clusterpedia *installv1alpha1.Clusterpedia) error {
	componentName := util.ComponentName(constants.ClusterpediaComponentInternalStoragePostgres, clusterpedia.Name)
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
				"internalstorage.clusterpedia.io/type": "postgres",
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "server",
					Protocol: corev1.ProtocolTCP,
					Port:     5432,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 5432,
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(clusterpedia, svc, scheme.Scheme)
	return clientutil.CreateOrUpdateService(ctrl.client, svc)
}

// EnsurePostgresSecret ensures the clusterpedia-internalstorage-postgres secret exists.
func (ctrl *ClusterpediaController) EnsurePostgresSecret(clusterpedia *installv1alpha1.Clusterpedia) error {
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

// EnsurePostgresConfigMap ensures the clusterpedia-internalstorage-postgres configmap exists.
func (ctrl *ClusterpediaController) EnsurePostgresConfigMap(clusterpedia *installv1alpha1.Clusterpedia) error {
	svcName := util.ComponentName(constants.ClusterpediaComponentInternalStoragePostgres, clusterpedia.Name)
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
			type: "postgres"
			host: %q
			port: 5432
			user: postgres
			database: "clusterpedia"`, svcName),
		},
	}
	controllerutil.SetOwnerReference(clusterpedia, cm, scheme.Scheme)
	return clientutil.CreateOrUpdateConfigMap(ctrl.client, cm)
}

// EnsurePostgresDeployment ensures the clusterpedia-internalstorage-postgres deployment exists.
func (ctrl *ClusterpediaController) EnsurePostgresDeployment(clusterpedia *installv1alpha1.Clusterpedia) error {
	componentName := util.ComponentName(constants.ClusterpediaComponentInternalStoragePostgres, clusterpedia.Name)
	image := clusterpedia.Spec.Storage.Postgres.Local

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
					"internalstorage.clusterpedia.io/type": "postgres",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                                  componentName,
						"internalstorage.clusterpedia.io/type": "postgres",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "postgres",
							Image:           fmt.Sprintf("%s/%s:%s", image.ImageRepository, image.ImageName, image.ImageTag),
							ImagePullPolicy: "IfNotPresent",
							Env: []corev1.EnvVar{
								{
									Name:  "POSTGRES_DB",
									Value: "clusterpedia",
								},
								{
									Name: "POSTGRES_PASSWORD",
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
									Name:          "postgres",
									ContainerPort: 5432,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/postgresql/data",
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
