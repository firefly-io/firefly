package karmada

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/util"
	maputil "github.com/carlory/firefly/pkg/util/map"
)

func (ctrl *KarmadaController) EnsureKarmadaScheduler(karmada *installv1alpha1.Karmada) error {
	return ctrl.EnsureKarmadaSchedulerDeployment(karmada)
}

func (ctrl *KarmadaController) EnsureKarmadaSchedulerDeployment(karmada *installv1alpha1.Karmada) error {
	componentName := util.ComponentName(constants.KarmadaComponentScheduler, karmada.Name)
	scheduler := karmada.Spec.Scheduler.KarmadaScheduler
	repository := karmada.Spec.ImageRepository
	tag := karmada.Spec.KarmadaVersion
	if scheduler.ImageRepository != "" {
		repository = scheduler.ImageRepository
	}
	if scheduler.ImageTag != "" {
		tag = scheduler.ImageTag
	}

	defaultArgs := map[string]string{
		"bind-address":               "0.0.0.0",
		"kubeconfig":                 "/etc/kubeconfig",
		"secure-port":                "10351",
		"enable-scheduler-estimator": "false",
		"feature-gates":              "Failover=true",
		"v":                          "4",
	}
	computedArgs := maputil.MergeStringMaps(defaultArgs, scheduler.ExtraArgs)
	args := maputil.ConvertToCommandOrArgs(computedArgs)

	deployment := &appsv1.Deployment{
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
							Image:           util.ComponentImageName(repository, constants.KarmadaComponentScheduler, tag),
							ImagePullPolicy: "IfNotPresent",
							Command:         []string{"/bin/karmada-scheduler"},
							Args:            args,
							Resources:       scheduler.Resources,
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

	controllerutil.SetOwnerReference(karmada, deployment, scheme.Scheme)
	return CreateOrUpdateDeployment(ctrl.client, deployment)
}
