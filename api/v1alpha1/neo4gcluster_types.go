/*
Copyright 2026.

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Neo4gClusterSpec defines the desired state of a Neo4g database cluster.
type Neo4gClusterSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Replicas int32 `json:"replicas"`

	// +kubebuilder:default="ghcr.io/neo4g/neo4g:latest"
	Image string `json:"image,omitempty"`

	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// +optional
	Config *Neo4gConfig `json:"config,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Gateway tuning — only relevant when replicas > 1 (gateway is auto-deployed).
	// +optional
	Gateway *GatewaySpec `json:"gateway,omitempty"`
}

type StorageSpec struct {
	// +kubebuilder:default="10Gi"
	Size resource.Quantity `json:"size,omitempty"`

	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

type Neo4gConfig struct {
	// +kubebuilder:default=1024
	// +optional
	PoolSize *int32 `json:"poolSize,omitempty"`

	// +kubebuilder:default=true
	// +optional
	WALEnabled *bool `json:"walEnabled,omitempty"`

	// +kubebuilder:default=false
	// +optional
	WALNoSync *bool `json:"walNoSync,omitempty"`

	// +kubebuilder:default="500ms"
	// +optional
	ReplPollInterval *string `json:"replPollInterval,omitempty"`
}

type GatewaySpec struct {
	// +kubebuilder:default="ghcr.io/neo4g/gateway:latest"
	// +optional
	Image string `json:"image,omitempty"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// +kubebuilder:default="2s"
	// +optional
	HeartbeatInterval *string `json:"heartbeatInterval,omitempty"`

	// +kubebuilder:default="6s"
	// +optional
	HeartbeatTimeout *string `json:"heartbeatTimeout,omitempty"`

	// +kubebuilder:default="1s"
	// +optional
	ElectionDelay *string `json:"electionDelay,omitempty"`
}

type ClusterPhase string

const (
	PhasePending ClusterPhase = "Pending"
	PhaseRunning ClusterPhase = "Running"
	PhaseFailed  ClusterPhase = "Failed"
)

// Neo4gClusterStatus defines the observed state of Neo4gCluster.
type Neo4gClusterStatus struct {
	// +optional
	Phase ClusterPhase `json:"phase,omitempty"`

	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Stable endpoint for downstream apps: <name>.<namespace>.svc:7474
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Neo4gCluster is the Schema for the neo4gclusters API.
type Neo4gCluster struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec Neo4gClusterSpec `json:"spec"`

	// +optional
	Status Neo4gClusterStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// Neo4gClusterList contains a list of Neo4gCluster.
type Neo4gClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Neo4gCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Neo4gCluster{}, &Neo4gClusterList{})
}
