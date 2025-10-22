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
	"fmt"

	"github.com/go-logr/logr"
	agentv1alpha1 "github.com/kagenti/operator/api/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type StepDefinition struct {
	// Name is the identifier for the step
	Name string

	// ConfigMap is the name of the ConfigMap containing the step definition
	ConfigMap string

	// TaskSpec is the embedded task definition
	TaskSpec *tektonv1.TaskSpec

	// Parameters contains step-specific parameters
	Parameters []agentv1alpha1.ParameterSpec
}

// PipelineComposer handles composition of pipelines from individual steps
type PipelineComposer struct {
	client client.Client
	Logger logr.Logger
}

func NewPipelineComposer(c client.Client, log logr.Logger) *PipelineComposer {
	return &PipelineComposer{
		client: c,
		Logger: log,
	}
}

// ComposePipelineSpec builds a Tekton PipelineSpec object from individual step ConfigMaps
func (pc *PipelineComposer) ComposePipelineSpec(ctx context.Context, agentBuild *agentv1alpha1.AgentBuild) (*tektonv1.PipelineSpec, error) {

	// Collect parameters auto-generated from spec
	specParams := pc.collectPipelineParams(agentBuild)

	// Merge with user-provided parameters (user overrides spec)
	mergedParams := pc.mergeParameters(specParams, agentBuild.Spec.Pipeline.Parameters)

	// Load steps with merged parameters
	steps, order, err := pc.loadStepsWithMergedParams(ctx, agentBuild, mergedParams)
	if err != nil {
		return nil, fmt.Errorf("failed to load pipeline steps: %w", err)
	}

	// Create ordered pipeline tasks with embedded specs
	pipelineTasks, err := pc.createPipelineTasks(steps, order)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline tasks: %w", err)
	}

	pipelineSpec := &tektonv1.PipelineSpec{
		Tasks: pipelineTasks,
		Workspaces: []tektonv1.PipelineWorkspaceDeclaration{
			{
				Name:        "shared-workspace",
				Description: "Workspace for source code and build artifacts",
			},
		},
	}

	return pipelineSpec, nil
}

// mergeParameters merges spec-generated params with user params (user takes precedence)
func (pc *PipelineComposer) mergeParameters(specParams, userParams []agentv1alpha1.ParameterSpec) []agentv1alpha1.ParameterSpec {
	// Start with spec params
	merged := make([]agentv1alpha1.ParameterSpec, len(specParams))
	copy(merged, specParams)

	// Override with user params
	for _, userParam := range userParams {
		found := false
		for i, specParam := range merged {
			if specParam.Name == userParam.Name {
				merged[i] = userParam // User value overrides
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, userParam) // Add new user param
		}
	}

	return merged
}
func (pc *PipelineComposer) loadStepsWithMergedParams(ctx context.Context, agentBuild *agentv1alpha1.AgentBuild, globalParams []agentv1alpha1.ParameterSpec) (map[string]*StepDefinition, []string, error) {
	steps := make(map[string]*StepDefinition)
	var order []string

	for _, stepSpec := range agentBuild.Spec.Pipeline.Steps {
		if stepSpec.Enabled != nil && !*stepSpec.Enabled {
			continue
		}
		order = append(order, stepSpec.Name)

		configMap := &corev1.ConfigMap{}
		err := pc.client.Get(ctx, types.NamespacedName{
			Name:      stepSpec.ConfigMap,
			Namespace: agentBuild.Spec.Pipeline.Namespace,
		}, configMap)

		if err != nil {
			if errors.IsNotFound(err) {
				return nil, nil, fmt.Errorf("step ConfigMap %s not found", stepSpec.ConfigMap)
			}
			return nil, nil, fmt.Errorf("failed to get step ConfigMap %s: %w", stepSpec.ConfigMap, err)
		}

		taskSpecYaml, ok := configMap.Data["task-spec.yaml"]
		if !ok {
			return nil, nil, fmt.Errorf("task-spec.yaml not found in ConfigMap %s", stepSpec.ConfigMap)
		}

		taskSpec := &tektonv1.TaskSpec{}
		if err := yaml.Unmarshal([]byte(taskSpecYaml), taskSpec); err != nil {
			return nil, nil, fmt.Errorf("failed to parse task spec definition: %w", err)
		}

		//Use merged global parameters instead of just Pipeline.Parameters
		step := &StepDefinition{
			Name:       stepSpec.Name,
			ConfigMap:  stepSpec.ConfigMap,
			TaskSpec:   taskSpec,
			Parameters: pc.mergeTaskParameters(taskSpec.Params, globalParams),
		}

		steps[stepSpec.Name] = step
	}

	return steps, order, nil
}

func (pc *PipelineComposer) createPipelineTasks(steps map[string]*StepDefinition, order []string) ([]tektonv1.PipelineTask, error) {

	tasks := make([]tektonv1.PipelineTask, 0, len(order))
	for i, stepName := range order {
		stepDefinition := steps[stepName]
		// Create task with embedded spec using EmbeddedTask
		task := tektonv1.PipelineTask{
			Name: stepDefinition.Name,
			TaskSpec: &tektonv1.EmbeddedTask{
				TaskSpec: tektonv1.TaskSpec{
					Params:     pc.getTaskParams(stepDefinition.Parameters),
					Steps:      stepDefinition.TaskSpec.Steps,
					Workspaces: stepDefinition.TaskSpec.Workspaces,
					Volumes:    stepDefinition.TaskSpec.Volumes,
				},
			},
			Workspaces: []tektonv1.WorkspacePipelineTaskBinding{
				{
					Name:      "source",
					Workspace: "shared-workspace",
				},
			},
		}
		if i > 0 {
			previousTaskName := order[i-1]
			task.RunAfter = []string{previousTaskName}
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// mergeTaskParameters merges task parameters with component parameters
func (pc *PipelineComposer) mergeTaskParameters(taskParams []tektonv1.ParamSpec, parameterList []agentv1alpha1.ParameterSpec) []agentv1alpha1.ParameterSpec {
	var mergedParams []agentv1alpha1.ParameterSpec

	for _, p := range parameterList {
		pc.Logger.Info("mergeTaskParameters", "parameterList param name", p.Name, "Value", p.Value)
	}
	for _, taskParam := range taskParams {
		defaultValue := ""

		pc.Logger.Info("Processing taskParam",
			"name", taskParam.Name,
			"type", string(taskParam.Type),
			"description", taskParam.Description,
			"hasDefault", taskParam.Default != nil)

		if taskParam.Default != nil {
			pc.Logger.Info("TaskParam default details",
				"name", taskParam.Name,
				"defaultType", string(taskParam.Default.Type),
				"stringVal", taskParam.Default.StringVal,
				"hasArrayVal", taskParam.Default.ArrayVal != nil,
				"arrayValLen", len(taskParam.Default.ArrayVal),
				"hasObjectVal", taskParam.Default.ObjectVal != nil,
				"objectValLen", len(taskParam.Default.ObjectVal))
			defaultValue = taskParam.Default.StringVal
		} else {
			pc.Logger.Info("TaskParam has nil Default", "name", taskParam.Name)
		}

		mergedParam := agentv1alpha1.ParameterSpec{
			Name:        taskParam.Name,
			Description: taskParam.Description,
		}

		param := pc.getTaskParam(parameterList, taskParam.Name)
		if param != nil {
			mergedParam.Value = param.Value
			pc.Logger.Info("param has value", "name", taskParam.Name, "value", mergedParam.Value)
		} else {
			pc.Logger.Info("param has nil value", "name", taskParam.Name)
			mergedParam.Value = defaultValue
		}
		mergedParams = append(mergedParams, mergedParam)
	}
	return mergedParams
}

func (pc *PipelineComposer) getTaskParam(params []agentv1alpha1.ParameterSpec, paramName string) *agentv1alpha1.ParameterSpec {

	for _, param := range params {
		if param.Name == paramName {
			return &param
		}
	}
	return nil
}
func (pc *PipelineComposer) getTaskParams(params []agentv1alpha1.ParameterSpec) []tektonv1.ParamSpec {
	taskParams := make([]tektonv1.ParamSpec, 0, len(params))

	for _, param := range params {
		p := tektonv1.ParamSpec{
			Name: param.Name,
			Default: &tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: param.Value,
			},
		}
		taskParams = append(taskParams, p)

	}

	return taskParams
}
func (pc *PipelineComposer) collectPipelineParams(agentBuild *agentv1alpha1.AgentBuild) []agentv1alpha1.ParameterSpec {
	params := []agentv1alpha1.ParameterSpec{}

	// Git source parameters from SourceSpec
	if agentBuild.Spec.SourceSpec.SourceRepository != "" {
		params = append(params, agentv1alpha1.ParameterSpec{
			Name:  "repo-url",
			Value: agentBuild.Spec.SourceSpec.SourceRepository,
		})
	}

	if agentBuild.Spec.SourceSpec.SourceRevision != "" {
		params = append(params, agentv1alpha1.ParameterSpec{
			Name:  "revision",
			Value: agentBuild.Spec.SourceSpec.SourceRevision,
		})
	}

	if agentBuild.Spec.SourceSpec.SourceSubfolder != "" {
		params = append(params, agentv1alpha1.ParameterSpec{
			Name:  "subfolder-path",
			Value: agentBuild.Spec.SourceSpec.SourceSubfolder,
		})
	}

	// Source credentials secret
	if agentBuild.Spec.SourceSpec.SourceCredentials != nil {
		params = append(params, agentv1alpha1.ParameterSpec{
			Name:  "SOURCE_REPO_SECRET",
			Value: agentBuild.Spec.SourceSpec.SourceCredentials.Name,
		})
	}

	// Build output parameters from BuildOutput
	if agentBuild.Spec.BuildOutput != nil {
		// Combined image:tag parameter
		imageURL := fmt.Sprintf("%s/%s:%s",
			agentBuild.Spec.BuildOutput.ImageRegistry,
			agentBuild.Spec.BuildOutput.Image,
			agentBuild.Spec.BuildOutput.ImageTag,
		)
		params = append(params, agentv1alpha1.ParameterSpec{
			Name:  "image",
			Value: imageURL,
		})

		// Registry credentials secret
		if agentBuild.Spec.BuildOutput.ImageRepoCredentials != nil {
			params = append(params, agentv1alpha1.ParameterSpec{
				Name:  "registry-secret",
				Value: agentBuild.Spec.BuildOutput.ImageRepoCredentials.Name,
			})
		}
	}

	return params
}
