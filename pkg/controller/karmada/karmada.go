package karmada

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/util"
)

func karmadaAggregatedAPIServerService(karmada *installv1alpha1.Karmada) *corev1.Service {
	componentName := util.ComponentName(constants.KarmadaComponentAggregratedAPIServer, karmada.Name)
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": componentName},
			Ports: []corev1.ServicePort{
				{
					Protocol: corev1.ProtocolTCP,
					Port:     443,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 443,
					},
				},
			},
		},
	}
}

func makeKarmadaAggregatedAPIServerDeployment(karmada *installv1alpha1.Karmada) *appsv1.Deployment {
	repository := karmada.Spec.ImageRepository
	version := karmada.Spec.KarmadaVersion
	componentName := util.ComponentName(constants.KarmadaComponentAggregratedAPIServer, karmada.Name)
	apiServer := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": componentName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "karmada-aggregated-apiserver",
							Image:           util.ComponentImageName(repository, constants.KarmadaComponentAggregratedAPIServer, version),
							ImagePullPolicy: "IfNotPresent",
							Command: []string{
								"/bin/karmada-aggregated-apiserver",
								"--kubeconfig=/etc/kubeconfig",
								"--authentication-kubeconfig=/etc/kubeconfig",
								"--authorization-kubeconfig=/etc/kubeconfig",
								"--etcd-cafile=/etc/kubernetes/pki/etcd-ca.crt",
								"--etcd-certfile=/etc/kubernetes/pki/etcd-client.crt",
								"--etcd-keyfile=/etc/kubernetes/pki/etcd-client.key",
								fmt.Sprintf("--etcd-servers=https://%s.%s.svc:2379", util.ComponentName(constants.KarmadaComponentEtcd, karmada.Name), karmada.Namespace),
								"--audit-log-path=-",
								"--feature-gates=APIPriorityAndFairness=false",
								"--audit-log-maxage=0",
								"--audit-log-maxbackup=0",
								"--tls-cert-file=/etc/kubernetes/pki/apiserver.crt",
								"--tls-private-key-file=/etc/kubernetes/pki/apiserver.key",
							},
							Resources: karmada.Spec.APIServer.KarmadaAggregratedAPIServer.Resources,
							LivenessProbe: &corev1.Probe{
								FailureThreshold: 8,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/livez",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 443,
										},
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								TimeoutSeconds:      15,
							},
							ReadinessProbe: &corev1.Probe{
								FailureThreshold: 3,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 443,
										},
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    1,
								SuccessThreshold: 1,
								TimeoutSeconds:   15,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "k8s-certs",
									MountPath: "/etc/kubernetes/pki",
									ReadOnly:  true,
								},
								{
									Name:      "kubeconfig",
									MountPath: "/etc/kubeconfig",
									SubPath:   "kubeconfig",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "k8s-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-cert", util.ComponentName("karmada", karmada.Name)),
								},
							},
						},
						{
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-kubeconfig", karmada.Name),
								},
							},
						},
					},
				},
			},
		},
	}
	return apiServer
}

func makeKarmadaKubeControllerManagerDeployment(karmada *installv1alpha1.Karmada) *appsv1.Deployment {
	repository := karmada.Spec.ImageRepository
	version := karmada.Spec.KubernetesVersion

	componentName := util.ComponentName(constants.KarmadaComponentKubeControllerManager, karmada.Name)
	apiServer := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": componentName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "kube-controller-manager",
							Image:           util.ComponentImageName(repository, "kube-controller-manager", version),
							ImagePullPolicy: "IfNotPresent",
							Command: []string{
								"kube-controller-manager",
								"--allocate-node-cidrs=true",
								"--authentication-kubeconfig=/etc/kubeconfig",
								"--authorization-kubeconfig=/etc/kubeconfig",
								"--bind-address=0.0.0.0",
								"--client-ca-file=/etc/kubernetes/pki/ca.crt",
								"--cluster-cidr=10.244.0.0/16",
								"--cluster-name=kubernetes",
								"--cluster-signing-cert-file=/etc/kubernetes/pki/ca.crt",
								"--cluster-signing-key-file=/etc/kubernetes/pki/ca.key",
								fmt.Sprintf("--controllers=%s", strings.Join(karmada.Spec.ControllerManager.KubeControllerManager.Controllers, ",")),
								"--kubeconfig=/etc/kubeconfig",
								"--leader-elect=true",
								"--node-cidr-mask-size=24",
								"--port=0",
								"--root-ca-file=/etc/kubernetes/pki/ca.crt",
								"--service-account-private-key-file=/etc/kubernetes/pki/karmada.key",
								fmt.Sprintf("--service-cluster-ip-range=%s", karmada.Spec.Networking.ServiceSubnet),
								"--use-service-account-credentials=true",
								"--v=4",
							},
							Resources: karmada.Spec.ControllerManager.KubeControllerManager.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "k8s-certs",
									MountPath: "/etc/kubernetes/pki",
									ReadOnly:  true,
								},
								{
									Name:      "kubeconfig",
									MountPath: "/etc/kubeconfig",
									SubPath:   "kubeconfig",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "k8s-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-cert", util.ComponentName("karmada", karmada.Name)),
								},
							},
						},
						{
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-kubeconfig", karmada.Name),
								},
							},
						},
					},
				},
			},
		},
	}
	return apiServer
}

func makeKarmadaSchedulerDeployment(karmada *installv1alpha1.Karmada) *appsv1.Deployment {
	repository := karmada.Spec.ImageRepository
	version := karmada.Spec.KarmadaVersion

	componentName := util.ComponentName(constants.KarmadaComponentScheduler, karmada.Name)
	apiServer := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": componentName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "karmada-scheduler",
							Image:           util.ComponentImageName(repository, constants.KarmadaComponentScheduler, version),
							ImagePullPolicy: "IfNotPresent",
							Command: []string{
								"/bin/karmada-scheduler",
								"--bind-address=0.0.0.0",
								"--kubeconfig=/etc/kubeconfig",
								"--secure-port=10351",
								"--feature-gates=Failover=true",
								"--enable-scheduler-estimator=false",
								"--v=4",
							},
							Resources: karmada.Spec.Scheduler.KarmadaScheduler.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kubeconfig",
									MountPath: "/etc/kubeconfig",
									SubPath:   "kubeconfig",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-kubeconfig", karmada.Name),
								},
							},
						},
					},
				},
			},
		},
	}
	return apiServer
}

func makeKarmadaControllerManagerDeployment(karmada *installv1alpha1.Karmada) *appsv1.Deployment {
	repository := karmada.Spec.ImageRepository
	version := karmada.Spec.KarmadaVersion

	componentName := util.ComponentName(constants.KarmadaComponentControllerManager, karmada.Name)
	apiServer := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": componentName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "karmada-controller-manager",
							Image:           util.ComponentImageName(repository, constants.KarmadaComponentControllerManager, version),
							ImagePullPolicy: "IfNotPresent",
							Command: []string{
								"/bin/karmada-controller-manager",
								"--bind-address=0.0.0.0",
								"--kubeconfig=/etc/kubeconfig",
								"--cluster-status-update-frequency=10s",
								"--secure-port=10357",
								"--feature-gates=PropagateDeps=true",
								"--v=4",
							},
							Resources: karmada.Spec.ControllerManager.KarmadaControllerManager.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kubeconfig",
									MountPath: "/etc/kubeconfig",
									SubPath:   "kubeconfig",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-kubeconfig", karmada.Name),
								},
							},
						},
					},
				},
			},
		},
	}
	return apiServer
}

func karmadaWebhookService(karmada *installv1alpha1.Karmada) *corev1.Service {
	componentName := util.ComponentName(constants.KarmadaComponentWebhook, karmada.Name)
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": componentName},
			Ports: []corev1.ServicePort{
				{
					Protocol: corev1.ProtocolTCP,
					Port:     443,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 8443,
					},
				},
			},
		},
	}
}

func makeKarmadaWebhookDeployment(karmada *installv1alpha1.Karmada) *appsv1.Deployment {
	repository := karmada.Spec.ImageRepository
	version := karmada.Spec.KarmadaVersion

	componentName := util.ComponentName(constants.KarmadaComponentWebhook, karmada.Name)
	apiServer := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: karmada.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": componentName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": componentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "karmada-webhook",
							Image:           util.ComponentImageName(repository, constants.KarmadaComponentWebhook, version),
							ImagePullPolicy: "IfNotPresent",
							Command: []string{
								"/bin/karmada-webhook",
								"--bind-address=0.0.0.0",
								"--kubeconfig=/etc/kubeconfig",
								"--secure-port=8443",
								"--cert-dir=/var/serving-cert",
								"--v=4",
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8443,
								},
							},
							Resources: karmada.Spec.Webhook.KarmadaWebhook.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kubeconfig",
									MountPath: "/etc/kubeconfig",
									SubPath:   "kubeconfig",
								},
								{
									Name:      "cert",
									MountPath: "/var/serving-cert",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-kubeconfig", karmada.Name),
								},
							},
						},
						{
							Name: "cert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-cert", componentName),
								},
							},
						},
					},
				},
			},
		},
	}
	return apiServer
}
