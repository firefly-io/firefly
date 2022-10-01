package karmada

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/util"
)

func (ctrl *KarmadaController) EnsureEtcd(karmada *installv1alpha1.Karmada) error {
	if err := ctrl.EnsureEtcdService(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureEtcdStatefulSet(karmada); err != nil {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) EnsureEtcdService(karmada *installv1alpha1.Karmada) error {
	etcdName := util.ComponentName(constants.KarmadaComponentEtcd, karmada.Name)
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdName,
			Namespace: karmada.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": etcdName},
			ClusterIP: "None",
			Type:      corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:     "client",
					Protocol: corev1.ProtocolTCP,
					Port:     2379,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 2379,
					},
				},
				{
					Name:     "server",
					Protocol: corev1.ProtocolTCP,
					Port:     2380,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 2380,
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(karmada, svc, scheme.Scheme)
	return CreateOrUpdateService(ctrl.client, svc)
}

func (ctrl *KarmadaController) EnsureEtcdStatefulSet(karmada *installv1alpha1.Karmada) error {
	etcdName := util.ComponentName(constants.KarmadaComponentEtcd, karmada.Name)
	repository := karmada.Spec.ImageRepository

	sts := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdName,
			Namespace: karmada.Namespace,
			Labels:    map[string]string{"app": etcdName},
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": etcdName},
			},
			ServiceName: etcdName,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": etcdName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "etcd",
							Image:           util.ComponentImageName(repository, constants.KarmadaComponentEtcd, "3.4.13-0"),
							ImagePullPolicy: "IfNotPresent",
							Command: []string{
								"/usr/local/bin/etcd",
								"--name",
								"etcd0",
								"--listen-peer-urls",
								"http://0.0.0.0:2380",
								"--listen-client-urls",
								"https://0.0.0.0:2379",
								"--advertise-client-urls",
								fmt.Sprintf("https://%s.%s.svc:2379", etcdName, karmada.Namespace),
								"--initial-cluster",
								fmt.Sprintf("etcd0=http://%s-0.%s.%s.svc:2380", etcdName, etcdName, karmada.Namespace),
								"--initial-cluster-state",
								"new",
								"--cert-file=/etc/etcd/pki/etcd-server.crt",
								"--client-cert-auth=true",
								"--key-file=/etc/etcd/pki/etcd-server.key",
								"--trusted-ca-file=/etc/etcd/pki/etcd-ca.crt",
								"--data-dir=/var/lib/etcd",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "etcd-certs",
									MountPath: "/etc/etcd/pki",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "etcd-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-cert", util.ComponentName(constants.KarmadaComponentEtcd, karmada.Name)),
								},
							},
						},
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(karmada, sts, scheme.Scheme)
	return CreateOrUpdateStatefulSet(ctrl.client, sts)
}
