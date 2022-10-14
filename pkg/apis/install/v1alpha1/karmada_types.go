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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Karmada is a specification for a Karmada resource
type Karmada struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the Karmada.
	// +optional
	Spec KarmadaSpec `json:"spec"`
	// Most recently observed status of the Karmada.
	// +optional
	Status KarmadaStatus `json:"status"`
}

// KarmadaSpec is the spec for a Karmada resource
type KarmadaSpec struct {
	// Etcd holds configuration for etcd.
	// +optional
	Etcd Etcd `json:"etcd,omitempty"`

	// Networking holds configuration for the networking topology of the cluster.
	// +optional
	Networking Networking `json:"networking,omitempty"`

	// KubernetesVersion is the target version of the kube-apiserver component.
	// +optional
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`

	// KarmadaVersion is the target version of the karmada.
	// +optional
	KarmadaVersion string `json:"karmadaVersion,omitempty"`

	// ControlPlaneEndpoint sets a stable IP address or DNS name for the control plane; it
	// can be a valid IP address or a RFC-1123 DNS subdomain, both with optional TCP port.
	// In case the ControlPlaneEndpoint is not specified, the AdvertiseAddress + BindPort
	// are used; in case the ControlPlaneEndpoint is specified but without a TCP port,
	// the BindPort is used.
	// Possible usages are:
	// e.g. In a cluster with more than one control plane instances, this field should be
	// assigned the address of the external load balancer in front of the
	// control plane instances.
	// e.g.  in environments with enforced node recycling, the ControlPlaneEndpoint
	// could be used for assigning a stable DNS to the control plane.
	// +optional
	ControlPlaneEndpoint string `json:"controlPlaneEndpoint,omitempty"`

	// APIServer contains extra settings for the API server control plane component
	// +optional
	APIServer APIServerComponent `json:"apiServer,omitempty"`

	// Webhook contains extra settings for the webhook component
	// +optional
	Webhook WebhookComponent `json:"webhook,omitempty"`

	// ControllerManager contains extra settings for the controller manager control plane component
	// +optional
	ControllerManager ControllerManagerComponent `json:"controllerManager,omitempty"`

	// Scheduler contains extra settings for the scheduler control plane component
	// +optional
	Scheduler SchedulerComponent `json:"scheduler,omitempty"`

	// ImageRepository sets the container registry to pull images from.
	// If empty, `ghcr.io/carlory` will be used by default.
	// +optional
	ImageRepository string `json:"imageRepository,omitempty"`

	// FeatureGates enabled by the user.
	// If you don't know that a feature gate should be applied to which components, you can
	// use this field to enable or disable the feature gate for all the components of the karmada instance.
	// - Failover: https://karmada.io/docs/userguide/failover/#failover
	// - GracefulEviction: https://karmada.io/docs/userguide/failover/#graceful-eviction-feature
	// - PropagateDeps: https://karmada.io/docs/userguide/scheduling/propagate-dependencies
	// - CustomizedClusterResourceModeling: https://karmada.io/docs/userguide/scheduling/cluster-resources#start-to-use-cluster-resource-models
	// More info: https://github.com/karmada-io/karmada/blob/master/pkg/features/features.go
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
}

// Etcd contains elements describing Etcd configuration.
type Etcd struct {
	// Local provides configuration knobs for configuring the built-in etcd instance
	// Local and External are mutually exclusive
	// +optional
	Local *LocalEtcd `json:"local,omitempty"`

	// External describes how to connect to an external etcd cluster
	// Local and External are mutually exclusive
	// +optional
	External *ExternalEtcd `json:"external,omitempty"`
}

// LocalEtcd describes that firefly should run an etcd cluster in a host cluster.
type LocalEtcd struct {
	// ImageMeta allows to customize the container used for etcd
	ImageMeta `json:",inline"`

	// DataVolume is the volume etcd will place its data.
	// If empty, etcd will use an emptyDir.
	// +optional
	DataVolume *corev1.PersistentVolumeClaimTemplate `json:"dataVolume"`

	// ServerCertSANs sets extra Subject Alternative Names for the etcd server signing cert.
	// +optional
	ServerCertSANs []string `json:"serverCertSANs,omitempty"`

	// PeerCertSANs sets extra Subject Alternative Names for the etcd peer signing cert.
	// +optional
	PeerCertSANs []string `json:"peerCertSANs,omitempty"`
}

// ExternalEtcd describes an external etcd cluster.
// Firefly has no knowledge of where certificate files live and they must be supplied.
type ExternalEtcd struct {
	// Endpoints of etcd members. Required for ExternalEtcd.
	Endpoints []string `json:"endpoints"`

	// CAData is an SSL Certificate Authority file used to secure etcd communication.
	// Required if using a TLS connection.
	CAData []byte `json:"caData"`

	// CertData is an SSL certification file used to secure etcd communication.
	// Required if using a TLS connection.
	CertData []byte `json:"certData"`

	// KeyData is an SSL key file used to secure etcd communication.
	// Required if using a TLS connection.
	KeyData []byte `json:"keyData"`
}

// Networking contains elements describing cluster's networking configuration
type Networking struct {
	// ServiceSubnet is the subnet used by k8s services. Defaults to "10.96.0.0/12".
	// +optional
	ServiceSubnet string `json:"serviceSubnet,omitempty"`

	// DNSDomain is the dns domain used by k8s services. Defaults to "cluster.local".
	// +optional
	DNSDomain string `json:"dnsDomain,omitempty"`
}

// APIServerComponent holds settings necessary for API server deployments in the karmada
type APIServerComponent struct {
	// KubeAPIServerComponent holds settings to kube-apiserver component of the kubernetes.
	// Karmada uses it as it's own apiserver in order to provide Kubernetes-native APIs.
	KubeAPIServer KubeAPIServerComponent `json:"kubeAPIServer,omitempty"`

	// KarmadaAggregratedAPIServerComponent holds settings to karmada-aggregated-apiserver component of the karmada.
	KarmadaAggregratedAPIServer KarmadaAggregratedAPIServerComponent `json:"karmadaAggregratedAPIServer,omitempty"`
}

// KubeAPIServerComponent holds settings to kube-apiserver component of the kubernetes.
// Karmada uses it as it's own apiserver in order to provide Kubernetes-native APIs.
type KubeAPIServerComponent struct {
	// ImageMeta allows to customize the image used for the kube-apiserver component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the kube-apiserver component or
	// override. A key in this map is the flag name as it appears on the command line except
	// without leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the
	// kube-apiserver component. In the future, we will provide a more structured way
	// to configure the component. Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// CertSANs sets extra Subject Alternative Names for the API Server signing cert.
	// +optional
	CertSANs []string `json:"certSANs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// FeatureGates enabled by the user.
	// More info: https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
}

// KarmadaAggregratedAPIServerComponent holds settings to karmada-aggregated-apiserver component of the karmada.
type KarmadaAggregratedAPIServerComponent struct {
	// ImageMeta allows to customize the image used for the karmada-aggregated-apiserver component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the karmada-aggregated-apiserver component or
	// override. A key in this map is the flag name as it appears on the command line except
	// without leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the
	// karmada-aggregated-apiserver component. In the future, we will provide a more structured way
	// to configure the component. Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://github.com/karmada-io/karmada/blob/master/cmd/aggregated-apiserver/app/options/options.go
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// WebhookComponent holds settings to webhook component of the karmada.
type WebhookComponent struct {
	// KarmadaWebhook holds settings to karmada-webook component of the karmada.
	KarmadaWebhook KarmadaWebhookComponent `json:"karmadaWebhook,omitempty"`
}

// KarmadaWebhookComponent holds settings to karmada-webhook component of the karmada.
type KarmadaWebhookComponent struct {
	// ImageMeta allows to customize the image used for the karmada-webhook component component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the karmada-webhook component or
	// override. A key in this map is the flag name as it appears on the command line except
	// without leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the
	// karmada-webhook component. In the future, we will provide a more structured way
	// to configure the component. Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://github.com/karmada-io/karmada/blob/master/cmd/webhook/app/options/options.go
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ControllerManagerComponent holds settings to controller manager components of the karmada.
type ControllerManagerComponent struct {
	// KubeControllerManager holds settings to kube-controller-manager component of the karmada.
	KubeControllerManager KubeControllerManagerComponent `json:"kubeControllerManager,omitempty"`

	// KarmadaControllerManager holds settings to karmada-controller-manager component of the karmada.
	KarmadaControllerManager KarmadaControllerManagerComponent `json:"karmadaControllerManager,omitempty"`

	// FireflyKarmadaManager holds settings to firefly-karmada-manager component of the karmada.
	FireflyKarmadaManager FireflyKarmadaManagerComponent `json:"fireflyKarmadaManager,omitempty"`
}

// KubeControllerManagerComponent holds settings to kube-controller-manager component of the kubernetes.
// Karmada uses it to manage the lifecycle of the federated resources. An especial case is the garbage
// collection of the orphan resources in your karmada.
type KubeControllerManagerComponent struct {
	// ImageMeta allows to customize the image used for the karmada-scheduler component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// A list of controllers to enable. '*' enables all on-by-default controllers,
	// 'foo' enables the controller named 'foo', '-foo' disables the controller named
	// 'foo'.
	//
	// All controllers: attachdetach, bootstrapsigner, cloud-node-lifecycle,
	// clusterrole-aggregation, cronjob, csrapproving, csrcleaner, csrsigning,
	// daemonset, deployment, disruption, endpoint, endpointslice,
	// endpointslicemirroring, ephemeral-volume, garbagecollector,
	// horizontalpodautoscaling, job, namespace, nodeipam, nodelifecycle,
	// persistentvolume-binder, persistentvolume-expander, podgc, pv-protection,
	// pvc-protection, replicaset, replicationcontroller, resourcequota,
	// root-ca-cert-publisher, route, service, serviceaccount, serviceaccount-token,
	// statefulset, tokencleaner, ttl, ttl-after-finished
	// Disabled-by-default controllers: bootstrapsigner, tokencleaner (default [*])
	// Actual Supported controllers depend on the version of Kubernetes. See
	// https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/
	// for details.
	//
	// However, Karmada uses Kubernetes Native API definitions for federated resource template,
	// so it doesn't need enable some resource related controllers like daemonset, deployment etc.
	// On the other hand, Karmada leverages the capabilities of the Kubernetes controller to
	// manage the lifecycle of the federated resource, so it needs to enable some controllers.
	// For example, the `namespace` controller is used to manage the lifecycle of the namespace
	// and the `garbagecollector` controller handles automatic clean-up of redundant items in
	// your karmada.
	//
	// According to the user feedback and karmada requirements, the following controllers are
	// enabled by default: namespace, garbagecollector, serviceaccount-token, ttl-after-finished,
	// bootstrapsigner,csrapproving,csrcleaner,csrsigning. See
	// https://karmada.io/docs/administrator/configuration/configure-controllers#kubernetes-controllers
	//
	// Others are disabled by default. If you want to enable or disable other controllers, you
	// have to explicitly specify all the controllers that kube-controller-manager shoud enable
	// at startup phase.
	// +optional
	Controllers []string `json:"controllers,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the kube-controller-manager component or
	// override. A key in this map is the flag name as it appears on the command line except
	// without leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the
	// kube-controller-manager component. In the future, we will provide a more structured way
	// to configure the component. Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// FeatureGates enabled by the user.
	// More info: https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
}

// KarmadaControllerManagerComponent holds settings to the karmada-controller-manager component of the karmada.
type KarmadaControllerManagerComponent struct {
	// ImageMeta allows to customize the image used for the karmada-controller-manager component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// A list of controllers to enable. '*' enables all on-by-default controllers,
	// 'foo' enables the controller named 'foo', '-foo' disables the controller named
	// 'foo'.
	//
	// All controllers: binding, cluster, clusterStatus, endpointSlice, execution,
	// federatedResourceQuotaStatus, federatedResourceQuotaSync, hpa, namespace,
	// serviceExport, serviceImport, unifiedAuth, workStatus.
	// Disabled-by-default controllers: hpa (default [*])
	// Actual Supported controllers depend on the version of Karmada. See
	// https://karmada.io/docs/administrator/configuration/configure-controllers#configure-karmada-controllers
	// for details.
	//
	// +optional
	Controllers []string `json:"controllers,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the karmada-controller-manager component or
	// override. A key in this map is the flag name as it appears on the command line except
	// without leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the
	// karmada-controller-manager component. In the future, we will provide a more structured way
	// to configure the component. Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://github.com/karmada-io/karmada/blob/master/cmd/controller-manager/app/options/options.go
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// FireflyKarmadaManagerComponent holds settings to the firefly-karmada-manager component of the karmada.
type FireflyKarmadaManagerComponent struct {
	// ImageMeta allows to customize the image used for the firefly-karmada-manager component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// A list of controllers to enable. '*' enables all on-by-default controllers,
	// 'foo' enables the controller named 'foo', '-foo' disables the controller named
	// 'foo'.
	//
	// All controllers: estimator, node
	// Disabled-by-default controllers:  (default [*])
	//
	// +optional
	Controllers []string `json:"controllers,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// SchedulerComponent holds settings to scheduler components of the cluster.
type SchedulerComponent struct {
	// KarmadaScheduler holds settings to karmada-scheduler conponent of the karmada.
	// +optional
	KarmadaScheduler KarmadaSchedulerComponent `json:"karmadaScheduler,omitempty"`

	// KarmadaScheduler holds settings to karmada-descheduler conponent of the karmada.
	// +optional
	KarmadaDescheduler KarmadaDeschedulerComponent `json:"karmadaDescheduler,omitempty"`

	// KarmadaSchedulerEstimator holds settings to karmada-scheduler-estimator conponent of the karmada.
	// +optional
	KarmadaSchedulerEstimator KarmadaSchedulerEstimatorComponent `json:"karmadaSchedulerEstimator,omitempty"`
}

// KarmadaSchedulerComponent holds settings to karmada-scheduler conponent of the karmada.
type KarmadaSchedulerComponent struct {
	// ImageMeta allows to customize the image used for the karmada-scheduler component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the karmada-scheduler component or override.
	// A key in this map is the flag name as it appears on the command line except without
	// leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the karmada-scheduler
	// component. In the future, we will provide a more structured way to configure the component.
	// Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://github.com/karmada-io/karmada/blob/master/cmd/scheduler/app/options/options.go
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// KarmadaDeschedulerComponent holds settings to karmada-descheduler conponent of the karmada.
type KarmadaDeschedulerComponent struct {
	// Enable indicates whether the karmada-descheduler conponent should be deployed.
	// This is a pointer to distinguish between explicit zero and not specified.
	// Defaults to false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// ImageMeta allows to customize the image used for the karmada-descheduler component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the karmada-descheduler component or override.
	// A key in this map is the flag name as it appears on the command line except without
	// leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the karmada-descheduler
	// component. In the future, we will provide a more structured way to configure the component.
	// Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://github.com/karmada-io/karmada/blob/master/cmd/descheduler/app/options/options.go
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// KarmadaSchedulerEstimatorComponent holds settings to karmada-scheduler-estimator conponent of the karmada.
type KarmadaSchedulerEstimatorComponent struct {
	// ImageMeta allows to customize the image used for the karmada-scheduler-estimator component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the karmada-scheduler-estimator component or override.
	// A key in this map is the flag name as it appears on the command line except without
	// leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the karmada-scheduler-estimator
	// component. In the future, we will provide a more structured way to configure the component.
	// Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://github.com/karmada-io/karmada/blob/master/cmd/scheduler-estimator/app/options/options.go
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ImageMeta allows to customize the image used for components.
type ImageMeta struct {
	// ImageRepository sets the container registry to pull images from.
	// if not set, the ImageRepository defined in Spec will be used instead.
	// +optional
	ImageRepository string `json:"imageRepository,omitempty"`

	// ImageTag allows to specify a tag for the image.
	// In case this value is set, firefly does not change automatically the version
	// of the above components during upgrades.
	// +optional
	ImageTag string `json:"imageTag,omitempty"`

	// ImageName allows to specify a name for the image.
	// +optional
	ImageName string `json:"imageName,omitempty"`
}

// KarmadaStatus is the status for a Karmada resource
type KarmadaStatus struct {
	// observedGeneration is the most recent generation observed for this Karmada. It corresponds to the
	// Karmada's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Represents the latest available observations of a karmada's current state.
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KarmadaList is a list of Karmada resources
type KarmadaList struct {
	metav1.TypeMeta `json:",inline"`
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata"`

	Items []Karmada `json:"items"`
}
