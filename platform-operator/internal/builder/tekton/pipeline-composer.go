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
	platformv1alpha1 "github.com/kagenti/operator/platform/api/v1alpha1"
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
	Parameters []platformv1alpha1.ParameterSpec
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
func (pc *PipelineComposer) ComposePipelineSpec(ctx context.Context, component *platformv1alpha1.Component) (*tektonv1.PipelineSpec, error) {

	// Load and validate all steps
	steps, order, err := pc.loadSteps(ctx, component)
	if err != nil {
		return nil, fmt.Errorf("failed to load pipeline steps: %w", err)
	}

	// Create ordered pipeline tasks with embedded specs
	pipelineTasks, err := pc.createPipelineTasks(steps, order) //pc.collectPipelineParams(component))
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
func (pc *PipelineComposer) getBuildSpec(component *platformv1alpha1.Component) (*platformv1alpha1.BuildSpec, error) {
	if component.Spec.Agent != nil && component.Spec.Agent.Build != nil {
		return component.Spec.Agent.Build, nil
	} else if component.Spec.Tool != nil && component.Spec.Tool.Build != nil {
		return component.Spec.Tool.Build, nil
	}
	return nil, fmt.Errorf("Invalid BuildSpec for component %s ", component.Name)
}

func (pc *PipelineComposer) loadSteps(ctx context.Context, component *platformv1alpha1.Component) (map[string]*StepDefinition, []string, error) {
	steps := make(map[string]*StepDefinition)
	var order []string

	buildSpec, err := pc.getBuildSpec(component)
	if err != nil {

	}
	for _, stepSpec := range buildSpec.Pipeline.Steps {
		// Skip disabled steps
		if stepSpec.Enabled != nil && !*stepSpec.Enabled {
			continue
		}
		order = append(order, stepSpec.Name)

		// Load step ConfigMap
		configMap := &corev1.ConfigMap{}
		err := pc.client.Get(ctx, types.NamespacedName{
			Name:      stepSpec.ConfigMap,
			Namespace: component.Namespace,
		}, configMap)

		if err != nil {
			if errors.IsNotFound(err) {
				return nil, nil, fmt.Errorf("step ConfigMap %s not found", stepSpec.ConfigMap)
			}
			return nil, nil, fmt.Errorf("failed to get step ConfigMap %s: %w", stepSpec.ConfigMap, err)
		}

		// Extract task spec definition
		taskSpecYaml, ok := configMap.Data["task-spec.yaml"]
		if !ok {
			return nil, nil, fmt.Errorf("task-spec.yaml not found in ConfigMap %s", stepSpec.ConfigMap)
		}

		// Parse task spec
		taskSpec := &tektonv1.TaskSpec{}
		if err := yaml.Unmarshal([]byte(taskSpecYaml), taskSpec); err != nil {
			return nil, nil, fmt.Errorf("failed to parse task spec definition: %w", err)
		}

		// Create step definition
		step := &StepDefinition{
			Name:       stepSpec.Name,
			ConfigMap:  stepSpec.ConfigMap,
			TaskSpec:   taskSpec,
			Parameters: pc.mergeTaskParameters(taskSpec.Params, buildSpec.Pipeline.Parameters),
		}

		steps[stepSpec.Name] = step
	}

	return steps, order, nil
}

func (pc *PipelineComposer) createPipelineTasks(steps map[string]*StepDefinition, order []string) ([]tektonv1.PipelineTask, error) {

	tasks := make([]tektonv1.PipelineTask, 0, len(order))
	for i, stepName := range order {
		stepDefinition := steps[stepName]

		//	stepParameters := pc.getTaskParams(stepDefinition.Parameters)

		// Create task with embedded spec using EmbeddedTask
		task := tektonv1.PipelineTask{
			Name: stepDefinition.Name,
			TaskSpec: &tektonv1.EmbeddedTask{
				TaskSpec: tektonv1.TaskSpec{
					Params:     pc.getTaskParams(stepDefinition.Parameters),
					Steps:      stepDefinition.TaskSpec.Steps,
					Workspaces: stepDefinition.TaskSpec.Workspaces,
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
func (pc *PipelineComposer) mergeTaskParameters(taskParams []tektonv1.ParamSpec, parameterList []platformv1alpha1.ParameterSpec) []platformv1alpha1.ParameterSpec {
	var mergedParams []platformv1alpha1.ParameterSpec

	for _, p := range parameterList {
		pc.Logger.Info("mergeTaskParameters", "parameterList param name", p.Name, "Value", p.Value)
	}
	for _, taskParam := range taskParams {
		// LOG: Right after taskParam is assigned from the range
		pc.Logger.Info("Processing taskParam",
			"name", taskParam.Name,
			"type", string(taskParam.Type),
			"description", taskParam.Description,
			"hasDefault", taskParam.Default != nil)

		// Additional detailed logging of the Default field
		if taskParam.Default != nil {
			pc.Logger.Info("TaskParam default details",
				"name", taskParam.Name,
				"defaultType", string(taskParam.Default.Type),
				"stringVal", taskParam.Default.StringVal,
				"hasArrayVal", taskParam.Default.ArrayVal != nil,
				"arrayValLen", len(taskParam.Default.ArrayVal),
				"hasObjectVal", taskParam.Default.ObjectVal != nil,
				"objectValLen", len(taskParam.Default.ObjectVal))
		} else {
			pc.Logger.Info("TaskParam has nil Default", "name", taskParam.Name)
		}
		// Create a copy of the task parameter
		//		mergedParam := platformv1alpha1.ParameterSpec{
		//			Name:        taskParam.Name,
		//			Description: taskParam.Description,
		//			Value:       taskParam.Default.StringVal,
		//		}

		// Create a copy of the task parameter
		mergedParam := platformv1alpha1.ParameterSpec{
			Name:        taskParam.Name,
			Description: taskParam.Description,
		}

		// FIX: Check if Default exists before accessing StringVal
		//	if taskParam.Default != nil {
		//		mergedParam.Value = taskParam.Default.StringVal
		//		pc.Logger.Info("Using default value",
		//			"param", taskParam.Name,
		//			"defaultValue", taskParam.Default.StringVal)
		//	} else {
		//		mergedParam.Value = "" // Safe fallback
		//		pc.Logger.Info("No default value, using empty string", "param", taskParam.Name)
		//	}

		param := pc.getTaskParam(parameterList, taskParam.Name)
		if param != nil {
			mergedParam.Value = param.Value
			pc.Logger.Info("param has value", "name", taskParam.Name, "value", mergedParam.Value)
		} else {
			pc.Logger.Info("param has nil value", "name", taskParam.Name)
			mergedParam.Value = ""
		}
		mergedParams = append(mergedParams, mergedParam)
	}
	return mergedParams
}

func (pc *PipelineComposer) getTaskParam(params []platformv1alpha1.ParameterSpec, paramName string) *platformv1alpha1.ParameterSpec {

	for _, param := range params {
		if param.Name == paramName {
			return &param
		}
	}
	return nil
}
func (pc *PipelineComposer) getTaskParams(params []platformv1alpha1.ParameterSpec) []tektonv1.ParamSpec {
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
	/*
		params := []tektonv1.ParamSpec{

			{
				Name:        "git-url",
				Type:        tektonv1.ParamTypeString,
				Description: "Git repository URL",
				Default: &tektonv1.ParamValue{
					Type:      tektonv1.ParamTypeString,
					StringVal: buildSpec.SourceRepository,
				},
			},
			{
				Name:        "git-revision",
				Type:        tektonv1.ParamTypeString,
				Description: "Git revision (branch, tag, commit)",
				Default: &tektonv1.ParamValue{
					Type:      tektonv1.ParamTypeString,
					StringVal: buildSpec.SourceRevision,
				},
			},
			{
				Name:        "component-path",
				Type:        tektonv1.ParamTypeString,
				Description: "Path to the component code within the repository",
				Default: &tektonv1.ParamValue{
					Type:      tektonv1.ParamTypeString,
					StringVal: buildSpec.SourceSubfolder,
				},
			},
			{
				Name:        "image-name",
				Type:        tektonv1.ParamTypeString,
				Description: "Name for the built image",
				Default: &tektonv1.ParamValue{
					Type:      tektonv1.ParamTypeString,
					StringVal: fmt.Sprintf("%s:%s", component.Name, "latest"),
				},
			},
		}
	*/
	return taskParams
}

func (pc *PipelineComposer) collectPipelineParams(component *platformv1alpha1.Component) []tektonv1.ParamSpec {
	buildSpec, err := pc.getBuildSpec(component)
	if err != nil {
		return nil
	}
	// Standard parameters
	params := []tektonv1.ParamSpec{
		{
			Name:        "git-url",
			Type:        tektonv1.ParamTypeString,
			Description: "Git repository URL",
			Default: &tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: buildSpec.SourceRepository,
			},
		},
		{
			Name:        "git-revision",
			Type:        tektonv1.ParamTypeString,
			Description: "Git revision (branch, tag, commit)",
			Default: &tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: buildSpec.SourceRevision,
			},
		},
		{
			Name:        "component-path",
			Type:        tektonv1.ParamTypeString,
			Description: "Path to the component code within the repository",
			Default: &tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: buildSpec.SourceSubfolder,
			},
		},
		{
			Name:        "image-name",
			Type:        tektonv1.ParamTypeString,
			Description: "Name for the built image",
			Default: &tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: fmt.Sprintf("%s:%s", component.Name, "latest"),
			},
		},
	}

	// Add custom parameters from component spec
	for _, param := range buildSpec.Pipeline.Parameters {
		params = append(params, tektonv1.ParamSpec{
			Name: param.Name,
			Type: tektonv1.ParamTypeString,
			Default: &tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: param.Value,
			},
		})
	}
	/*
		// Add boolean flags for BuildOptions
		if buildSpec.BuildOptions.EnableSigning {
			params = append(params, tektonv1.ParamSpec{
				Name: "enable-signing",
				Type: tektonv1.ParamTypeString,
				Default: &tektonv1.ArrayOrString{
					Type:      tektonv1.ParamTypeString,
					StringVal: "true",
				},
			})
		}

		if component.Spec.BuildOptions.GenerateSBOM {
			params = append(params, tektonv1.ParamSpec{
				Name: "generate-sbom",
				Type: tektonv1.ParamTypeString,
				Default: &tektonv1.ArrayOrString{
					Type:      tektonv1.ParamTypeString,
					StringVal: "true",
				},
			})
		}
	*/
	return params
}
