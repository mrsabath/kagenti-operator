/*
Copyright 2025.

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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AgentSpec defines the desired state of Agent.
type AgentSpec struct {
	// Description is a human-readable description of the agent
	// +optional
	Description string `json:"description,omitempty"`

	// Image is the container image to use for the agent
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// Env is a list of environment variables to set in the container
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources is the compute resources required by the container
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// AgentStatus defines the observed state of Agent.
type AgentStatus struct {
	// Conditions represent the latest available observations of an Agent's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed for this Agent
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// DeploymentStatus represents the current status of the Agent deployment
	// +optional
	DeploymentStatus string `json:"deploymentStatus,omitempty"`

	// Ready indicates if the agent is ready to serve requests
	// +optional
	Ready bool `json:"ready,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image"
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Agent is the Schema for the agents API.
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentList contains a list of Agent.
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
