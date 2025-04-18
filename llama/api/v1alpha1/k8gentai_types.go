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

// ComponentReference defines a reference to a component definition
type ComponentReference struct {
	// DefinitionName is the name of the ComponentDefinition to use
	//DefinitionName string `json:"definitionName"`

	// Name is the instance name for this component
	Name string `json:"name"`

	// Namespace to deploy the component in
	Namespace string `json:"namespace"`

	// Parameters are instance-specific parameters that override or add to the defaults
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`
}

// ComponentStatus defines the observed state of a Component
type ComponentStatus struct {
	// Status is the current status of the component
	Status string `json:"status"`

	// Message provides additional information about the component status
	// +optional
	Message string `json:"message,omitempty"`

	// LastUpdateTime is the last time this status was updated
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

// AgenticPlatformSpec defines the desired state of AgenticPlatform
type K8gentaiSpec struct {
	// Mode is the deployment mode (dev, staging, prod)
	// +optional
	Mode string `json:"mode,omitempty"`

	// Components is a list of component references to deploy
	Components []ComponentReference `json:"components"`
}

// AgenticPlatformStatus defines the observed state of AgenticPlatform
type K8gentaiStatus struct {
	// ComponentStatuses is a map of component name to status
	// +optional
	ComponentStatuses map[string]ComponentStatus `json:"componentStatuses,omitempty"`

	// PlatformStatus represents the overall status of the agentic platform
	// +optional
	PlatformStatus string `json:"platformStatus,omitempty"`

	// PlatformMessage provides additional information about the platform status
	// +optional
	PlatformMessage string `json:"platformMessage,omitempty"`

	// InfrastructureReady indicates whether all infrastructure components are ready
	// +optional
	InfrastructureReady bool `json:"infrastructureReady,omitempty"`

	// LastUpdateTime is the last time the status was updated
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// Conditions represents the latest available observations of the platform's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.platformStatus`
// +kubebuilder:printcolumn:name="Infra-Ready",type=boolean,JSONPath=`.status.infrastructureReady`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AgenticPlatform is the Schema for the agenticplatforms API
type K8gentai struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   K8gentaiSpec   `json:"spec,omitempty"`
	Status K8gentaiStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgenticPlatformList contains a list of AgenticPlatform
type K8gentaiList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K8gentai `json:"items"`
}

func init() {
	SchemeBuilder.Register(&K8gentai{}, &K8gentaiList{})
}
