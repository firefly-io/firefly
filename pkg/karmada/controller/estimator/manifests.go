package estimator

import (
	"context"
	"fmt"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/util"
	clientutil "github.com/carlory/firefly/pkg/util/client"
	maputil "github.com/carlory/firefly/pkg/util/map"
)

func (ctrl *EstimatorController) KubeConfigFromSecret(ctx context.Context, cluster *clusterv1alpha1.Cluster) (*clientcmdapi.Config, error) {
	credentials, err := ctrl.karmadaKubeClient.CoreV1().Secrets(cluster.Spec.SecretRef.Namespace).Get(ctx, cluster.Spec.SecretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	cfg := &clientcmdapi.Config{
		CurrentContext: cluster.Name,
		Contexts: []clientcmdapi.NamedContext{
			{
				Name: cluster.Name,
				Context: clientcmdapi.Context{
					Cluster:  cluster.Name,
					AuthInfo: cluster.Name,
				},
			},
		},
		Clusters: []clientcmdapi.NamedCluster{
			{
				Name: cluster.Name,
				Cluster: clientcmdapi.Cluster{
					Server:                   cluster.Spec.APIEndpoint,
					InsecureSkipTLSVerify:    cluster.Spec.InsecureSkipTLSVerification,
					CertificateAuthorityData: credentials.Data["caBundle"],
					ProxyURL:                 cluster.Spec.ProxyURL,
				},
			},
		},
		AuthInfos: []clientcmdapi.NamedAuthInfo{
			{
				Name: cluster.Name,
				AuthInfo: clientcmdapi.AuthInfo{
					Token: string(credentials.Data["token"]),
				},
			},
		},
	}
	return cfg, nil
}

func (ctrl *EstimatorController) EnsureEstimatorKubeconfigSecret(ctx context.Context, karmada *installv1alpha1.Karmada, cluster *clusterv1alpha1.Cluster) error {
	kubeconfig, err := ctrl.KubeConfigFromSecret(ctx, cluster)
	if err != nil {
		return err
	}

	kubeconfigData, err := yaml.Marshal(kubeconfig)
	if err != nil {
		return err
	}
	estimatorName := GenerateEstimatorServiceName(karmada.Name, "karmada-scheduler-estimator", cluster.Name)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-kubeconfig", estimatorName),
			Namespace: karmada.Namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfigData,
		},
	}
	controllerutil.SetOwnerReference(karmada, secret, scheme.Scheme)
	return clientutil.CreateOrUpdateSecret(ctrl.fireflyKubeClient, secret)
}

func (ctrl *EstimatorController) EnsureEstimatorService(ctx context.Context, karmada *installv1alpha1.Karmada, cluster *clusterv1alpha1.Cluster) error {
	estimatorName := GenerateEstimatorServiceName(karmada.Name, "karmada-scheduler-estimator", cluster.Name)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      estimatorName,
			Namespace: karmada.Namespace,
			Labels: map[string]string{
				"app": estimatorName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "estimator",
					Port:       10352,
					TargetPort: intstr.FromInt(10352),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": estimatorName,
			},
		},
	}
	controllerutil.SetOwnerReference(karmada, svc, scheme.Scheme)
	return clientutil.CreateOrUpdateService(ctrl.fireflyKubeClient, svc)
}

func (ctrl *EstimatorController) EnsureEstimatorDeployment(ctx context.Context, karmada *installv1alpha1.Karmada, cluster *clusterv1alpha1.Cluster) error {
	estimatorName := GenerateEstimatorServiceName(karmada.Name, "karmada-scheduler-estimator", cluster.Name)
	repository := karmada.Spec.ImageRepository
	version := karmada.Spec.KarmadaVersion
	estimator := karmada.Spec.Scheduler.KarmadaSchedulerEstimator

	defaultArgs := map[string]string{
		"kubeconfig":   "/etc/kuberentes/kubeconfig",
		"cluster-name": cluster.Name,
	}
	computedArgs := maputil.MergeStringMaps(defaultArgs, estimator.ExtraArgs)
	args := maputil.ConvertToCommandOrArgs(computedArgs)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      estimatorName,
			Namespace: karmada.Namespace,
			Labels: map[string]string{
				"app": estimatorName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": estimatorName,
				},
			},
			Replicas: estimator.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": estimatorName,
					},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-kubeconfig", estimatorName),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:    estimatorName,
							Image:   util.ComponentImageName(repository, constants.KarmadaComponentSchedulerEstimator, version),
							Command: []string{"/bin/karmada-scheduler-estimator"},
							Args:    args,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kubeconfig",
									MountPath: "/etc/kuberentes/kubeconfig",
									SubPath:   "kubeconfig",
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/healthz",
										Port:   intstr.FromInt(10351),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								FailureThreshold:    3,
								InitialDelaySeconds: 15,
								PeriodSeconds:       15,
								TimeoutSeconds:      5,
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "estimator",
									ContainerPort: 10352,
									Protocol:      corev1.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(karmada, deployment, scheme.Scheme)
	return clientutil.CreateOrUpdateDeployment(ctrl.fireflyKubeClient, deployment)
}

// GenerateEstimatorServiceName generates the gRPC scheduler estimator service name which belongs to a cluster.
func GenerateEstimatorServiceName(karmadaName, estimatorServicePrefix, clusterName string) string {
	return fmt.Sprintf("%s-%s", estimatorServicePrefix, clusterName)
}
