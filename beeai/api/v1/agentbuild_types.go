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

// AgentBuildSpec defines the desired state of AgentBuild.
type AgentBuildSpec struct {
	// RepoURL is the URL of the Git repository containing the agent source code
	// +kubebuilder:validation:Required
	RepoURL string `json:"repoUrl"`

	// Name of subfolder containing the agent source code. Supports a repo with
	// many agents, each isolated in a dedicated folder
	// +optional
	SourceSubfolder string `json:"sourceSubfolder"`

	// RepoUser is the username in the Git repository containing the agent source code
	// +kubebuilder:validation:Required
	RepoUser string `json:"repoUser"`

	// Revision is the Git revision to checkout (branch, tag, commit SHA)
	// +kubebuilder:validation:Required
	Revision string `json:"revision"`

	// Image is the name of the image to build
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// ImageTag is the tag to apply to the built image
	// +kubebuilder:validation:Required
	ImageTag string `json:"imageTag"`

	// ImageRegistry is the container registry where the image will be pushed
	// +kubebuilder:validation:Required
	ImageRegistry string `json:"imageRegistry"`

	// Env is a list of environment variables to use during the build process
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// DeployAfterBuild indicates whether to automatically deploy the agent after build
	// +optional
	DeployAfterBuild bool `json:"deployAfterBuild,omitempty"`

	// Agent defines the configuration for the Agent CR that will be created
	// +optional
	Agent AgentTemplate `json:"agent,omitempty"`
}

// AgentTemplate defines the Agent CR configuration to be created after successful build
type AgentTemplate struct {
	// Name is the name for the Agent CR
	// If not specified, the AgentBuild name will be used
	// +optional
	Name string `json:"name,omitempty"`

	// Description is a human-readable description of the agent
	// +optional
	Description string `json:"description,omitempty"`

	// Env is a list of environment variables to set in the agent container
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources is the compute resources required by the container
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// AgentBuildStatus defines the observed state of AgentBuild.
type AgentBuildStatus struct {
	// Name is the name for the Agent CR
	// +optional
	AgentName string `json:"agentName,omitempty"`

	// Conditions represent the latest available observations of an AgentBuild's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed for this AgentBuild
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// BuildStatus represents the current status of the build process
	// +optional
	BuildStatus string `json:"buildStatus,omitempty"`

	// PipelineRunName is the name of the Tekton PipelineRun resource
	// +optional
	PipelineRunName string `json:"pipelineRunName,omitempty"`

	// StartTime is when the build was started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the build was completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// BuiltImage is the full reference to the built image
	// +optional
	BuiltImage string `json:"builtImage,omitempty"`

	// DeploymentStatus represents the status of the agent deployment if deployAfterBuild is true
	// +optional
	DeploymentStatus string `json:"deploymentStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.buildStatus"
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".status.builtImage"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// AgentBuild is the Schema for the agentbuilds API.
type AgentBuild struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentBuildSpec   `json:"spec,omitempty"`
	Status AgentBuildStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentBuildList contains a list of AgentBuild.
type AgentBuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentBuild `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentBuild{}, &AgentBuildList{})
}
