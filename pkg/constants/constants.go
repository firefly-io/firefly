package constants

import "time"

const (
	// APICallRetryInterval defines how long kubeadm should wait before retrying a failed API operation
	APICallRetryInterval = 500 * time.Millisecond

	// KarmadaSystemNamespace defines the leader selection namespace for karmada components
	KarmadaSystemNamespace = "karmada-system"
	// KarmadaComponentEtcd defines the name of the built-in etcd cluster component
	KarmadaComponentEtcd = "etcd"
	// KarmadaComponentKubeAPIServer defines the name of the karmada-apiserver component
	KarmadaComponentKubeAPIServer = "karmada-apiserver"
	// KarmadaComponentAggregratedAPIServer defines the name of the karmada-aggregated-apiserver component
	KarmadaComponentAggregratedAPIServer = "karmada-aggregated-apiserver"
	// KarmadaComponentKubeControllerManager defines the name of the karmada-kube-controller-manager component
	KarmadaComponentKubeControllerManager = "karmada-kube-controller-manager"
	// KarmadaComponentScheduler defines the name of the karmada-scheduler component
	KarmadaComponentScheduler = "karmada-scheduler"
	// KarmadaComponentDescheduler defines the name of the karmada-descheduler component
	KarmadaComponentDescheduler = "karmada-descheduler"
	// KarmadaComponentControllerManager defines the name of the karmada-controller-manager component
	KarmadaComponentControllerManager = "karmada-controller-manager"
	// KarmadaComponentWebhook defines the name of the karmada-webhook component
	KarmadaComponentWebhook = "karmada-webhook"
	// KarmadaComponentSchedulerEstimator defines the name of the karmada-scheduler-estimator component
	KarmadaComponentSchedulerEstimator = "karmada-scheduler-estimator"
	// FireflyComponentKarmadaManager defines the name of the karmada-karmada-manager component
	FireflyComponentKarmadaManager = "firefly-karmada-manager"
)
