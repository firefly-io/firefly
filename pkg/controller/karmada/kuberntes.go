package karmada

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/util"
)

func makeKarmadaAPIServerService(karmada *installv1alpha1.Karmada) *corev1.Service {
	componentName := util.ComponentName(constants.KarmadaComponentKubeAPIServer, karmada.Name)
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
					Name:     "server",
					Protocol: corev1.ProtocolTCP,
					Port:     5443,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 5443,
					},
				},
			},
		},
	}
}

func makeKarmadaAPIServerDeployment(karmada *installv1alpha1.Karmada) *appsv1.Deployment {
	repository := karmada.Spec.ImageRepository
	version := karmada.Spec.KubernetesVersion

	componentName := util.ComponentName(constants.KarmadaComponentKubeAPIServer, karmada.Name)
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
							Name:            "karmada-apiserver",
							Image:           util.ComponentImageName(repository, "kube-apiserver", version),
							ImagePullPolicy: "IfNotPresent",
							Command: []string{
								"kube-apiserver",
								"--allow-privileged=true",
								"--authorization-mode=Node,RBAC",
								"--client-ca-file=/etc/kubernetes/pki/ca.crt",
								"--enable-admission-plugins=NodeRestriction",
								"--enable-bootstrap-token-auth=true",
								"--etcd-cafile=/etc/kubernetes/pki/etcd-ca.crt",
								"--etcd-certfile=/etc/kubernetes/pki/etcd-client.crt",
								"--etcd-keyfile=/etc/kubernetes/pki/etcd-client.key",
								fmt.Sprintf("--etcd-servers=https://%s.%s.svc:2379", util.ComponentName(constants.KarmadaComponentEtcd, karmada.Name), karmada.Namespace),
								"--bind-address=0.0.0.0",
								"--insecure-port=0",
								"--kubelet-client-certificate=/etc/kubernetes/pki/karmada.crt",
								"--kubelet-client-key=/etc/kubernetes/pki/karmada.key",
								"--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
								"--disable-admission-plugins=StorageObjectInUseProtection,ServiceAccount",
								"--runtime-config=",
								"--secure-port=5443",
								fmt.Sprintf("--service-account-issuer=https://kubernetes.default.svc.%s", karmada.Spec.Networking.DNSDomain),
								"--service-account-key-file=/etc/kubernetes/pki/karmada.key",
								"--service-account-signing-key-file=/etc/kubernetes/pki/karmada.key",
								"--service-cluster-ip-range=10.96.0.0/12",
								"--proxy-client-cert-file=/etc/kubernetes/pki/front-proxy-client.crt",
								"--proxy-client-key-file=/etc/kubernetes/pki/front-proxy-client.key",
								"--requestheader-allowed-names=front-proxy-client",
								"--requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt",
								"--requestheader-extra-headers-prefix=X-Remote-Extra-",
								"--requestheader-group-headers=X-Remote-Group",
								"--requestheader-username-headers=X-Remote-User",
								"--tls-cert-file=/etc/kubernetes/pki/apiserver.crt",
								"--tls-private-key-file=/etc/kubernetes/pki/apiserver.key",
							},
							Resources: karmada.Spec.APIServer.KubeAPIServer.Resources,
							LivenessProbe: &corev1.Probe{
								FailureThreshold: 8,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/livez",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 5443,
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
											IntVal: 5443,
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
					},
				},
			},
		},
	}
	return apiServer
}
