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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	kagentioperatordevv1alpha1 "github.com/kagenti/operator/platform/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var componentlog = logf.Log.WithName("component-resource")

// SetupComponentWebhookWithManager registers the webhook for Component in the manager.
func SetupComponentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&kagentioperatordevv1alpha1.Component{}).
		WithValidator(&ComponentCustomValidator{}).
		WithDefaulter(&ComponentCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-kagenti-operator-dev-v1alpha1-component,mutating=true,failurePolicy=fail,sideEffects=None,groups=kagenti.operator.dev,resources=components,verbs=create;update,versions=v1alpha1,name=mcomponent-v1alpha1.kb.io,admissionReviewVersions=v1

type ComponentCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &ComponentCustomDefaulter{}

func (d *ComponentCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	component, ok := obj.(*kagentioperatordevv1alpha1.Component)

	if !ok {
		return fmt.Errorf("expected an Component object but got %T", obj)
	}
	componentlog.Info("Defaulting for Component", "name", component.GetName())

	// Set suspend to true by default for new Component resources
	if component.Spec.Suspend == nil {
		suspend := true
		component.Spec.Suspend = &suspend
		componentlog.Info("setting suspend to true", "name", component.Name, "namespace", component.Namespace)
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-kagenti-operator-dev-v1alpha1-component,mutating=false,failurePolicy=fail,sideEffects=None,groups=kagenti.operator.dev,resources=components,verbs=create;update,versions=v1alpha1,name=vcomponent-v1alpha1.kb.io,admissionReviewVersions=v1

// ComponentCustomValidator struct is responsible for validating the Component resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ComponentCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &ComponentCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Component.
func (v *ComponentCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	component, ok := obj.(*kagentioperatordevv1alpha1.Component)
	if !ok {
		return nil, fmt.Errorf("expected a Component object but got %T", obj)
	}
	componentlog.Info("Validation for Component upon creation", "name", component.GetName())

	v.validateComponentSpec(component)

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Component.
func (v *ComponentCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	component, ok := newObj.(*kagentioperatordevv1alpha1.Component)
	if !ok {
		return nil, fmt.Errorf("expected a Component object for the newObj but got %T", newObj)
	}
	componentlog.Info("Validation for Component upon update", "name", component.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Component.
func (v *ComponentCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	component, ok := obj.(*kagentioperatordevv1alpha1.Component)
	if !ok {
		return nil, fmt.Errorf("expected a Component object but got %T", obj)
	}
	componentlog.Info("Validation for Component upon deletion", "name", component.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}

// validateComponentSpec validates the ComponentSpec for mutually exclusive fields
func (v *ComponentCustomValidator) validateComponentSpec(component *kagentioperatordevv1alpha1.Component) []string {
	var errors []string

	// Validate component types - only one should be specified
	componentCount := 0
	if component.Spec.Agent != nil {
		componentCount++
	}
	if component.Spec.Tool != nil {
		componentCount++
	}
	if component.Spec.Infra != nil {
		componentCount++
	}

	if componentCount == 0 {
		errors = append(errors, "exactly one component type must be specified (agent, tool, or infra)")
	} else if componentCount > 1 {
		errors = append(errors, "only one component type can be specified (agent, tool, or infra are mutually exclusive)")
	}

	// Validate deployer - only one deployment method should be specified
	deployerErrors := v.validateDeployerSpec(&component.Spec.Deployer)
	errors = append(errors, deployerErrors...)

	return errors
}

// validateDeployerSpec validates the DeployerSpec for mutually exclusive deployment methods
func (v *ComponentCustomValidator) validateDeployerSpec(deployer *kagentioperatordevv1alpha1.DeployerSpec) []string {
	var errors []string

	deployerCount := 0
	if deployer.Helm != nil {
		deployerCount++
	}
	if deployer.Kubernetes != nil {
		deployerCount++
	}
	if deployer.Olm != nil {
		deployerCount++
	}

	if deployerCount == 0 {
		errors = append(errors, "exactly one deployer method must be specified (helm, kubernetes, or olm)")
	} else if deployerCount > 1 {
		errors = append(errors, "only one deployer method can be specified (helm, kubernetes, and olm are mutually exclusive)")
	}
	return errors
}
