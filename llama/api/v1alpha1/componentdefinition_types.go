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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ComponentDefinitionSpec defines the desired state of a component template
type ComponentDefinitionSpec struct {
	//DefinitionName string `json:"definitionName"`

	// Name of the component
	Name string `json:"name"`

	// Namespace of the component
	Namespace string `json:"namespace"`

	// Type of component (infra, app)
	Type string `json:"type"`

	// Installer type (helm, deployment, etc.)
	Installer string `json:"installer"`

	// RepoUrl is the Helm repository URL (for helm installer)
	// +optional
	RepoUrl string `json:"repoUrl,omitempty"`

	// RepoName is the name of the Helm repository (for helm installer)
	// +optional
	RepoName string `json:"repoName,omitempty"`

	// RepoName is the name of the Helm repository (for helm installer)
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// Image Version for the component
	Version string `json:"version,omitempty"`

	// ChartName is the name of the Helm chart (for helm installer)
	// +optional
	ChartName string `json:"chartName,omitempty"`

	// DefaultParameters are the default parameters for this component
	// +optional
	DefaultParameters map[string]string `json:"defaultParameters,omitempty"`

	// Env is a list of environment variables to use during the build process
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// DependsOn is a list of component names this component depends on
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`
}

// ComponentDefinitionStatus defines the observed state of ComponentDefinition
type ComponentDefinitionStatus struct {
	// Conditions represents the latest available observations of the component definition's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastUpdateTime is the last time the component definition was updated
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Installer",type=string,JSONPath=`.spec.installer`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ComponentDefinition is the Schema for ComponentDefinition API
type ComponentDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentDefinitionSpec   `json:"spec,omitempty"`
	Status ComponentDefinitionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentDefinitionList contains a list of ComponentDefinition
type ComponentDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentDefinition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentDefinition{}, &ComponentDefinitionList{})
}

/*
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ComponentDefinitionSpec defines the desired state of ComponentDefinition.
type ComponentDefinitionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ComponentDefinition. Edit componentdefinition_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ComponentDefinitionStatus defines the observed state of ComponentDefinition.
type ComponentDefinitionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ComponentDefinition is the Schema for the componentdefinitions API.
type ComponentDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentDefinitionSpec   `json:"spec,omitempty"`
	Status ComponentDefinitionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentDefinitionList contains a list of ComponentDefinition.
type ComponentDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentDefinition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentDefinition{}, &ComponentDefinitionList{})
}

*/
