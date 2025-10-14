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

package tekton

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	agentv1alpha1 "github.com/kagenti/operator/api/v1alpha1"
	"github.com/kagenti/operator/internal/builder"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ builder.Builder = &TektonBuilder{}

type TektonBuilder struct {
	client.Client
	Scheme           *runtime.Scheme
	Log              logr.Logger
	PipelineComposer *PipelineComposer
	WorkspaceManager *WorkspaceManager
}

func NewTektonBuilder(client client.Client, log logr.Logger, scheme *runtime.Scheme, composer *PipelineComposer, workspaceMgr *WorkspaceManager) *TektonBuilder {
	return &TektonBuilder{
		Client:           client,
		Scheme:           scheme,
		Log:              log,
		PipelineComposer: composer,
		WorkspaceManager: workspaceMgr,
	}
}
func (b *TektonBuilder) Build(ctx context.Context, agentBuild *agentv1alpha1.AgentBuild) error {
	b.Log.Info("TektonBuilder - building Tekton PipelineRun")

	if err := b.WorkspaceManager.CreateWorkspacePVC(ctx, agentBuild); err != nil {
		b.Log.Error(err, "Failed to verify if workspace PVC is available")
		return fmt.Errorf("workspace PVC creation failed: %w", err)
	}

	err := b.createPipelineRun(ctx, agentBuild)
	if err != nil {
		return err
	}
	return nil
	/*
		if err := b.CheckStatus(ctx, agentBuild); err != nil {
			return err
	*/
}
func (b *TektonBuilder) Cleanup(ctx context.Context, agentBuild *agentv1alpha1.AgentBuild) error {
	// Check if cleanup is enabled
	if !agentBuild.Spec.CleanupAfterBuild {
		b.Log.Info("Skipping cleanup as CleanupAfterBuild is false")
		return nil
	}
	// Get the pods associated with the PipelineRun
	pipelineRunName := agentBuild.Status.PipelineRunName

	// List the pods with the PipelineRun label
	podList := &corev1.PodList{}
	err := b.List(ctx, podList, client.MatchingLabels{"tekton.dev/pipelineRun": pipelineRunName})
	if err != nil {
		b.Log.Error(err, "Failed to list pods for PipelineRun", "pipelineRunName", pipelineRunName)
		return err
	}

	// Filter for completed pods (Succeeded)
	var completedPods []corev1.Pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			completedPods = append(completedPods, pod)
		}
	}

	// Delete completed pods
	for _, pod := range completedPods {
		b.Log.Info("Deleting completed pod", "podName", pod.Name)
		err := b.Delete(ctx, &pod)
		if err != nil {
			b.Log.Error(err, "Failed to delete completed pod", "podName", pod.Name)
		}
	}
	return nil
}
func (b *TektonBuilder) createPipelineRun(ctx context.Context, agentBuild *agentv1alpha1.AgentBuild) error {
	logger := b.Log.WithValues("Tekton builder", agentBuild.Name, "Namespace", agentBuild.Namespace)
	//logger.Info("Creating new PipelineRun")
	// Check if a PipelineRun already exists for this build
	if agentBuild.Status.PipelineRunName != "" {
		existingPR := &tektonv1.PipelineRun{}
		err := b.Client.Get(ctx, types.NamespacedName{
			Name:      agentBuild.Status.PipelineRunName,
			Namespace: agentBuild.Namespace,
		}, existingPR)
		if err == nil {
			logger.Info("PipelineRun already exists, skipping creation",
				"pipelineRun", agentBuild.Status.PipelineRunName)
			return nil // Already exists, don't create duplicate
		}
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to check existing PipelineRun: %w", err)
		}
		// NotFound is OK, continue to create
	}
	// Generate unique PipelineRun name
	pipelineRunName := fmt.Sprintf("%s-%s", agentBuild.Name, time.Now().Format("20060102150405"))

	// Update status FIRST to claim this build attempt
	logger.Info("Updating status to Building before creating PipelineRun")
	agentBuild.Status.PipelineRunName = pipelineRunName
	agentBuild.Status.Phase = agentv1alpha1.PhaseBuilding
	agentBuild.Status.Message = "Creating PipelineRun"
	agentBuild.Status.LastBuildTime = &metav1.Time{Time: time.Now()}

	if err := b.Client.Status().Update(ctx, agentBuild); err != nil {
		b.Log.Error(err, "Failed to update status before creating PipelineRun")
		return err
	}

	// Now safe to create PipelineRun - status already shows Building
	logger.Info("Creating new PipelineRun", "name", pipelineRunName)
	pipelineSpec, err := b.PipelineComposer.ComposePipelineSpec(ctx, agentBuild)
	if err != nil {
		return err
	}
	allParams := b.PipelineComposer.collectPipelineParams(agentBuild)
	// Merge with user parameters
	allParams = b.PipelineComposer.mergeParameters(allParams, agentBuild.Spec.Pipeline.Parameters)
	// Convert to Tekton Param format
	pipelineParams := make([]tektonv1.Param, 0, len(allParams))
	for _, p := range allParams {
		pipelineParams = append(pipelineParams, tektonv1.Param{
			Name: p.Name,
			Value: tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: p.Value,
			},
		})
		logger.Info("PipelineRun parameter", "name", p.Name, "value", p.Value)
	}
	pp, err := json.MarshalIndent(pipelineSpec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal pipelinespec for logging: %w", err)
	} else {
		fmt.Println("PipelineSpec:::" + string(pp))
	}
	// Go uses a specific reference date for time formatting: Mon Jan 2 15:04:05 MST 2006
	// This is NOT about the actual year - it's a pattern template.
	//pipelineRunName := fmt.Sprintf("%s-%s", agentBuild.Name, time.Now().Format("20060102150405"))
	pipelineRun := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pipelineRunName,
			Namespace: agentBuild.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of":   "kagenti-operator",
				"app.kubernetes.io/component": agentBuild.Name,
			},
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineSpec: pipelineSpec,
			Workspaces:   b.WorkspaceManager.GetWorkspaceBindings(agentBuild),
			Params:       pipelineParams,
		},
	}

	if err := controllerutil.SetControllerReference(agentBuild, pipelineRun, b.Scheme); err != nil {
		b.Log.Error(err, "Failed to set owner reference on PipelineRun")
		return err
	}
	b.Log.Info("Creating a new PipelineRun", "Namespace", agentBuild.Namespace, "PipelineRun.Name", pipelineRun.Name)
	if err := b.Client.Create(ctx, pipelineRun); err != nil {
		b.Log.Error(err, "Failed to create a PipelineRun")
		return err
	}

	agentBuild.Status.PipelineRunName = pipelineRunName
	agentBuild.Status.Phase = agentv1alpha1.PhaseBuilding

	agentBuild.Status.LastBuildTime = &metav1.Time{Time: time.Now()}
	if err := b.Client.Status().Update(ctx, agentBuild); err != nil {
		// Handle AlreadyExists gracefully
		if errors.IsAlreadyExists(err) {
			logger.Info("PipelineRun already exists (race condition), continuing")
			return nil
		}
		b.Log.Error(err, "Failed to update PipelineRun status")
		return err
	}

	return nil
}
func (b *TektonBuilder) Cancel(ctx context.Context, agentBuild *agentv1alpha1.AgentBuild) error {
	return nil
}
func (b *TektonBuilder) CheckStatus(ctx context.Context, agentBuild *agentv1alpha1.AgentBuild) error {
	logger := b.Log.WithValues("Tekton builder", agentBuild.Name, "Namespace", agentBuild.Namespace)
	logger.Info("Checking build status")

	if agentBuild.Status.Phase == "" {
		return nil
	}
	pipelineRun := &tektonv1.PipelineRun{}
	err := b.Client.Get(ctx, types.NamespacedName{
		Name:      agentBuild.Status.PipelineRunName,
		Namespace: agentBuild.Namespace,
	}, pipelineRun)
	if err != nil {
		if errors.IsNotFound(err) {
			agentBuild.Status.Phase = agentv1alpha1.BuildPhaseFailed
			agentBuild.Status.Message = "Pipeline run not found"
			// Set condition for PipelineRun not found
			meta.SetStatusCondition(&agentBuild.Status.Conditions, metav1.Condition{
				Type:               "PipelineRunExists",
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             "PipelineRunNotFound",
				Message:            fmt.Sprintf("PipelineRun %s was not found", agentBuild.Status.PipelineRunName),
			})

			meta.SetStatusCondition(&agentBuild.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             "BuildFailed",
				Message:            "PipelineRun not found",
			})
			return b.Client.Status().Update(ctx, agentBuild)
		}
		return err
	}
	// Set PipelineRunExists condition
	meta.SetStatusCondition(&agentBuild.Status.Conditions, metav1.Condition{
		Type:               "PipelineRunExists",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "PipelineRunFound",
		Message:            fmt.Sprintf("PipelineRun %s exists", pipelineRun.Name),
	})

	logger.Info("tekton Pipeline Build", "phase", pipelineRun.Status.Status)
	if pipelineRun.Status.CompletionTime != nil {
		// Build is complete
		for _, condition := range pipelineRun.Status.Conditions {
			if condition.Type == "Succeeded" {
				if condition.Status == corev1.ConditionTrue {
					agentBuild.Status.Phase = agentv1alpha1.BuildPhaseSucceeded
					agentBuild.Status.Message = "Build completed successfully"
					agentBuild.Status.CompletionTime = pipelineRun.Status.CompletionTime
					// Extract built image from pipeline results
					agentBuild.Status.BuiltImage = b.extractBuiltImage(pipelineRun)
					meta.SetStatusCondition(&agentBuild.Status.Conditions, metav1.Condition{
						Type:               "BuildSucceeded",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
						Reason:             "BuildCompleted",
						Message:            "Build completed successfully",
					})

					meta.SetStatusCondition(&agentBuild.Status.Conditions, metav1.Condition{
						Type:               "Ready",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
						Reason:             "BuildReady",
						Message:            fmt.Sprintf("Image built: %s", agentBuild.Status.BuiltImage),
					})

				} else {
					agentBuild.Status.Phase = agentv1alpha1.BuildPhaseFailed
					agentBuild.Status.Message = fmt.Sprintf("Build failed: %s", condition.Message)
					meta.SetStatusCondition(&agentBuild.Status.Conditions, metav1.Condition{
						Type:               "BuildSucceeded",
						Status:             metav1.ConditionFalse,
						LastTransitionTime: metav1.Now(),
						Reason:             "BuildFailed",
						Message:            condition.Message,
					})

					meta.SetStatusCondition(&agentBuild.Status.Conditions, metav1.Condition{
						Type:               "Ready",
						Status:             metav1.ConditionFalse,
						LastTransitionTime: metav1.Now(),
						Reason:             "BuildNotReady",
						Message:            "Build failed",
					})

				}
				agentBuild.Status.LastBuildTime = &metav1.Time{Time: time.Now()}

				return b.Client.Status().Update(ctx, agentBuild)
			}
		}
	} else {
		// Build in progress - set in-progress condition
		meta.SetStatusCondition(&agentBuild.Status.Conditions, metav1.Condition{
			Type:               "BuildSucceeded",
			Status:             metav1.ConditionUnknown,
			LastTransitionTime: metav1.Now(),
			Reason:             "BuildInProgress",
			Message:            "Build is currently running",
		})
	}

	return nil
}
func (b *TektonBuilder) extractBuiltImage(pipelineRun *tektonv1.PipelineRun) string {
	// Extract from pipeline results
	for _, result := range pipelineRun.Status.Results {
		if result.Name == "IMAGE_URL" || result.Name == "image-url" {
			return result.Value.StringVal
		}
	}

	// Fallback: construct from parameters
	for _, param := range pipelineRun.Spec.Params {
		if param.Name == "IMAGE" || param.Name == "image" {
			return param.Value.StringVal
		}
	}

	return ""
}

func (b *TektonBuilder) triggerNewBuild(agentBuild *agentv1alpha1.AgentBuild) bool {
	b.Log.Info("triggerNewBuild()", "BuildStatus", agentBuild.Status)
	// Don't trigger if already building
	if agentBuild.Status.Phase == agentv1alpha1.PhaseBuilding {
		return false
	}
	if agentBuild.Status.LastBuildTime.IsZero() || agentBuild.Status.PipelineRunName == "" {
		return true
	}
	// Trigger on failure (for retry)
	if agentBuild.Status.Phase == agentv1alpha1.BuildPhaseFailed {
		return true
	}

	for _, condition := range agentBuild.Status.Conditions {
		if condition.Type == "BuildSucceeded" && condition.Status == metav1.ConditionFalse {
			return true
		}
	}

	return false
}
func (b *TektonBuilder) GetStatus(ctx context.Context, agent *agentv1alpha1.AgentBuild) (agentv1alpha1.AgentBuildStatus, error) {
	return agentv1alpha1.AgentBuildStatus{}, nil
}
