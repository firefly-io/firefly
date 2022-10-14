package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

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
type ClusterpediaSpec struct{}

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

	Items []Karmada `json:"items"`
}
