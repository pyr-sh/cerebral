package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePoolSpec   `json:"spec"`
	Status NodePoolStatus `json:"status,omitempty"`
}

type NodePoolStatus struct {
	CurrentNodes  int `json:"currentNodes"`
	UpToDateNodes int `json:"upToDateNodes"`
	Available     int `json:"availableNodes"`
}

type NodePoolSpec struct {
	Flavor  string `json:"flavor"`
	Desired int    `json:"desiredNodes"`
	Min     int    `json:"minNodes"`
	Max     int    `json:"maxNodes"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []NodePool `json:"items"`
}
