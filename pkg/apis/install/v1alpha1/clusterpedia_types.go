package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterapi "github.com/clusterpedia-io/api/cluster/v1alpha2"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:path=clusterpedias

// Clusterpedia is a specification for a Clusterpedia resource
type Clusterpedia struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the Clusterpedia.
	// +optional
	Spec ClusterpediaSpec `json:"spec"`
	// Most recently observed status of the Clusterpedia.
	// +optional
	Status ClusterpediaStatus `json:"status"`
}

// ClusterpediaSpec is the spec for a Clusterpedia resource
type ClusterpediaSpec struct {
	// ControlplaneProvider represents where the clusterpedia crds will be deployed on.
	// If unset, means that the clusterpedia and its crds will be installed on the host
	// cluster.
	// +optional
	ControlplaneProvider *ClusterpediaControlplaneProvider `json:"controlplaneProvider,omitempty"`

	// Version is the target version of the clusterpedia component.
	// If empty, `latest` will be used by default.
	// +optional
	Version string `json:"version,omitempty"`

	// Storage contains extra settings for the clusterpedia-storage component
	// If empty, firefly will choose the internal postgres as default value.
	// +optional
	Storage ClusterpediaStorageComponent `json:"storage,omitempty"`

	// APIServer contains extra settings for the clusterpedia-apiserver component
	// +optional
	APIServer ClusterpediaAPIServerComponent `json:"apiServer,omitempty"`

	// ControllerManager contains extra settings for the clusterpedia-controller-manager component
	ControllerManager ClusterpediaControllerManagerComponent `json:"controllerManager,omitempty"`

	// ClusterSynchroManager contains extra settings for the clustersynchro-manager component
	ClusterpediaSynchroManager ClusterSynchroManagerComponent `json:"clusterSynchroManager,omitempty"`

	// ImageRepository sets the container registry to pull images from.
	// If empty, `ghcr.io/clusterpedia-io/clusterpedia` will be used by default.
	// +optional
	ImageRepository string `json:"imageRepository,omitempty"`

	// FeatureGates enabled by the user.
	// If you don't know that a feature gate should be applied to which components, you can
	// use this field to enable or disable the feature gate for all the components of the clusterpedia instance.
	// It can be overridden by the component-specific feature gate settings.
	// Note: the clusterpedia community doesn't support this field now. Please use component-specific feature gate settings.
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
}

// ControlplaneProvider represents where the clusterpedia crds will be deployed on.
type ClusterpediaControlplaneProvider struct {
	// SyncAllCustomResources indicates whether to sync all the custom resources of member clusters to clusterpedia.
	// +optional
	SyncAllCustomResources bool `json:"syncAllCustomResources,omitempty"`

	// SyncResources represents which resources will be synced to clusterpedia from member clusters.
	// If empty, firefly won't auto-create clusterimportpolicy and nothing will be synced into clusterpedia by default.
	// +optional
	SyncResources []clusterapi.ClusterGroupResources `json:"syncResources,omitempty"`

	// Karmada represents the karmada control plane.
	// +optional
	Karmada *ClusterpediaControlplaneProviderKarmada `json:"karmada,omitempty"`
}

// KarmadaControlplaneProviderKarmada represents the karmada controlplane provider
type ClusterpediaControlplaneProviderKarmada struct {
	corev1.LocalObjectReference `json:",inline"`
}

// ClusterpediaStorageComponent holds settings to clusterpedia-storage component of the clusterpeida.
type ClusterpediaStorageComponent struct {
	// Postgres holds settings to clusterpedia-storage-postgres component of the clusterpeida.
	Postgres *Postgres `json:"postgres,omitempty"`
	//MySQL holds settings to clusterpedia-storage-mysql component of the clusterpeida.
	MySQL *MySQL `json:"mysql,omitempty"`
}

// Postgres holds settings to clusterpedia-storage-postgres component of the clusterpeida.
type Postgres struct {
	// Local provides configuration knobs for configuring the built-in postgres instance
	// Local and External are mutually exclusive
	// +optional
	Local *LocalPostgres `json:"local,omitempty"`
}

// LocalPostgres describes that firefly should run a postgres cluster in a host cluster.
type LocalPostgres struct {
	// ImageMeta allows to customize the container used for postgres
	// If empty, `docker.io/library/postgres:10` will be used by default.
	ImageMeta `json:",inline"`
}

//MySQL holds settings to clusterpedia-storage-mysql component of the clusterpeida.
type MySQL struct {
	// Local provides configuration knobs for configuring the built-in mysql instance
	// Local and External are mutually exclusive
	// +optional
	Local *LocalPostgres `json:"local,omitempty"`
}

// LocalMySQL describes that firefly should run a mysql cluster in a host cluster.
type LocalMySQL struct {
	// ImageMeta allows to customize the container used for mysql
	// If empty, `docker.io/library/mysql:8` will be used by default.
	ImageMeta `json:",inline"`
}

// ClusterpediaAPIServerComponent holds settings to clusterpedia-apiserver component of the clusterpeida.
type ClusterpediaAPIServerComponent struct {
	// ImageMeta allows to customize the image used for the clusterpedia-apiserver component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the clusterpedia-apiserver component or
	// override. A key in this map is the flag name as it appears on the command line except
	// without leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the
	// clusterpedia-apiserver component. In the future, we will provide a more structured way
	// to configure the component. Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://github.com/clusterpedia-io/clusterpedia/blob/main/cmd/apiserver/app/options/options.go
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// FeatureGates enabled by the user.
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
}

// ClusterpediaControllerManagerComponent holds settings to clusterpedia-controller-manager component of the karmada.
type ClusterpediaControllerManagerComponent struct {
	// ImageMeta allows to customize the image used for the clusterpedia-controller-manager component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the clusterpedia-controller-manager component or
	// override. A key in this map is the flag name as it appears on the command line except
	// without leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the
	// clusterpedia-controller-manager component. In the future, we will provide a more structured way
	// to configure the component. Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://github.com/clusterpedia-io/clusterpedia/blob/main/cmd/controller-manager/app/options/options.go
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// FeatureGates enabled by the user.
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
}

// ClusterSynchroManagerComponent holds settings to clustersynchro-manager component of the karmada.
type ClusterSynchroManagerComponent struct {
	// ImageMeta allows to customize the image used for the clustersynchro-manager component
	ImageMeta `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the clustersynchro-manager component or
	// override. A key in this map is the flag name as it appears on the command line except
	// without leading dash(es).
	//
	// Note: This is a temporary solution to allow for the configuration of the
	// clustersynchro-manager component. In the future, we will provide a more structured way
	// to configure the component. Once that is done, this field will be discouraged to be used.
	// Incorrect settings on this feild maybe lead to the corresponding component in an unhealthy
	// state. Before you do it, please confirm that you understand the risks of this configuration.
	//
	// For supported flags, please see
	// https://github.com/clusterpedia-io/clusterpedia/blob/main/cmd/clustersynchro-manager/app/options/options.go
	// for details.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// Compute Resources required by this component.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// FeatureGates enabled by the user.
	// More info: https://github.com/clusterpedia-io/clusterpedia/blob/main/pkg/synchromanager/features/features.go
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
}

// ClusterpediaStatus is the status for a Clusterpedia resource
type ClusterpediaStatus struct {
	// observedGeneration is the most recent generation observed for this Clusterpedia. It corresponds to the
	// Clusterpedia's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Represents the latest available observations of a clusterpedia's current state.
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterpediaList is a list of Clusterpedia resources
type ClusterpediaList struct {
	metav1.TypeMeta `json:",inline"`
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata"`

	Items []Clusterpedia `json:"items"`
}
