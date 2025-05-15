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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PlatformSpec defines the desired state of a Platform
type PlatformSpec struct {
	// Description of the platform
	// +optional
	Description string `json:"description,omitempty"`

	// Infrastructure components required by the platform
	// +optional
	Infrastructure []PlatformComponentRef `json:"infrastructure,omitempty"`

	// Tools required by the platform
	// +optional
	Tools []PlatformComponentRef `json:"tools,omitempty"`

	// Agents that will run on the platform
	// +optional
	Agents []PlatformComponentRef `json:"agents,omitempty"`

	// Global configurations that apply to all components
	// +optional
	GlobalConfig GlobalConfig `json:"globalConfig,omitempty"`
}

// PlatformComponentRef defines a reference to a component
type PlatformComponentRef struct {
	// Name of the component in the platform
	Name string `json:"name"`

	// Reference to the component resource
	ComponentReference ComponentReference `json:"componentReference"`
}

// ComponentReference identifies a component resource
type ComponentReference struct {
	// Name of the component resource
	Name string `json:"name"`

	// Kind of the component (Component)
	Kind string `json:"kind"`

	// API version of the component
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Namespace of the component
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// GlobalConfig defines global configuration for all components
type GlobalConfig struct {
	// Namespace for all components
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Annotations to apply to all components
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to apply to all components
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// PlatformStatus defines the observed state of a Platform
type PlatformStatus struct {
	// Phase of the platform deployment
	// +optional
	Phase string `json:"phase,omitempty"`

	// Status of all components
	// +optional
	Components ComponentsStatus `json:"components,omitempty"`

	// Conditions representing the current state of the platform
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ComponentsStatus contains the status of all components
type ComponentsStatus struct {
	// Status of infrastructure components
	// +optional
	Infrastructure []DeploymentStatus `json:"infrastructure,omitempty"`

	// Status of tool components
	// +optional
	Tools []DeploymentStatus `json:"tools,omitempty"`

	// Status of agent components
	// +optional
	Agents []DeploymentStatus `json:"agents,omitempty"`
}

// ComponentStatus defines the status of a referenced component
type DeploymentStatus struct {
	// Name of the component in the platform
	Name string `json:"name"`

	// Status of the component
	Status string `json:"status"`

	// Error message if any
	// +optional
	Error string `json:"error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// Platform is the Schema for the platforms API
type Platform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlatformSpec   `json:"spec,omitempty"`
	Status PlatformStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PlatformList contains a list of Platform
type PlatformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Platform `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Platform{}, &PlatformList{})
}
