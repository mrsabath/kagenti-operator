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

package controller

import (
	"context"
	"fmt"
	"time"

	agenticplatformv1alpha1 "aiplatform/k8gentai-operator.git/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconciles ComponentDefinition objects
type ComponentDefinitionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agenticplatform.applatform.io,resources=componentdefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agenticplatform.applatform.io,resources=componentdefinitions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agenticplatform.applatform.io,resources=componentdefinitions/finalizers,verbs=update
func (r *ComponentDefinitionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ComponentDefinition", "request", req.NamespacedName)

	compDef := &agenticplatformv1alpha1.ComponentDefinition{}
	if err := r.Get(ctx, req.NamespacedName, compDef); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ComponentDefinition")
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(compDef, "componentdefinition.applatform.io/finalizer") {
		controllerutil.AddFinalizer(compDef, "componentdefinition.applatform.io/finalizer")

		// Fetch K8gentaiInstance object referencing this ComponentDefinition object
		ap, err := r.getReferencingK8gentaiInstance(ctx, compDef)
		if err != nil {
			logger.Error(err, "Failed to get K8gentai instance")
			return ctrl.Result{}, err
		}
		if err := controllerutil.SetControllerReference(ap, compDef, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, compDef); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if !compDef.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, compDef)
	}
	// check if component defition is valid
	if err := r.validateComponentDefinition(ctx, compDef); err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "Invalid component definition")
		}
		r.updateComponentDefinitionStatus(ctx, compDef, "Invalid", err.Error())
		return ctrl.Result{}, err
	}
	// verifies if all pods associated with a given ComponentDefinition are in a ready state
	ready, err := r.checkComponentPodsReady(ctx, compDef)
	if err != nil {
		logger.Error(err, "Pod Check For Ready Failed")
		return ctrl.Result{}, err
	}
	podsReady := "NotReady"
	if ready {
		podsReady = "Ready"
	}

	logger.Info("Reconcile", "Component", compDef.Name, "Readines state", ready)

	// updates the status of a given ComponentDefinition with a ready condition
	r.updateComponentDefinitionStatus(ctx, compDef, podsReady, "Component definition is ready")

	if err := r.reconcileReferencingK8gentaiInstances(ctx, compDef); err != nil {
		logger.Error(err, "Failed to reconcile referencing AgenticPlatforms")
		return ctrl.Result{}, err
	}
	logger.Info("Reconcile", "Component", compDef.Name, "RequeueAfter", ReconcileLimit.Seconds())
	return ctrl.Result{RequeueAfter: ReconcileLimit}, nil

}

func (r *ComponentDefinitionReconciler) checkComponentPodsReady(ctx context.Context, compDef *agenticplatformv1alpha1.ComponentDefinition) (bool, error) {
	lbls := map[string]string{"app.kubernetes.io/component": "primary"}
	selector := labels.SelectorFromSet(labels.Set(lbls))

	podList := &corev1.PodList{}

	if err := r.Client.List(ctx, podList, client.InNamespace(compDef.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return false, fmt.Errorf("failed to list Pods: %v", err)
	}
	if len(podList.Items) == 0 {
		return false, nil
	}
	for _, pod := range podList.Items {
		isReady := false
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.ContainersReady && condition.Status == corev1.ConditionTrue {
				isReady = true
				break
			}
		}
		if !isReady {
			return false, nil
		}
	}
	return true, nil

}

func (r *ComponentDefinitionReconciler) validateComponentDefinition(ctx context.Context, compDef *agenticplatformv1alpha1.ComponentDefinition) error {
	// Validate component type
	if compDef.Spec.Type != "infra" && compDef.Spec.Type != "app" {
		return fmt.Errorf("invalid component type: %s. Must be 'infra' or 'app'", compDef.Spec.Type)
	}

	// Validate installer type
	if compDef.Spec.Installer != "helm" && compDef.Spec.Installer != "deployment" {
		return fmt.Errorf("invalid installer type: %s. Must be 'helm' or 'deployment'", compDef.Spec.Installer)
	}

	// Validate helm-specific fields if installer is helm
	if compDef.Spec.Installer == "helm" {
		if compDef.Spec.RepoUrl == "" {
			return fmt.Errorf("repoUrl must be specified for helm installer")
		}
		if compDef.Spec.ChartName == "" {
			return fmt.Errorf("chartName must be specified for helm installer")
		}
	}

	// check state of dependent components
	for _, depName := range compDef.Spec.DependsOn {
		depDef := &agenticplatformv1alpha1.ComponentDefinition{}

		err := r.Get(ctx, client.ObjectKey{Name: depName, Namespace: compDef.Namespace}, depDef)
		if err != nil {
			if errors.IsNotFound(err) {
				return err
			}
			return fmt.Errorf("error checking dependency '%s': %v", depName, err)
		}
		// Prevent circular dependencies
		if err := r.checkCircularDependency(ctx, compDef.Name, depName, []string{}); err != nil {
			return err
		}
	}

	return nil
}

// checks for circular dependencies in ComponentDefinitions. Each component may have a list of dependent components which determine in
// which order components are installed. Infra components are installed first, then the app components
func (r *ComponentDefinitionReconciler) checkCircularDependency(ctx context.Context, originalName, currentName string, visited []string) error {
	logger := log.FromContext(ctx)
	// Check if revisiting a component which indicates circular dependency. The 'visited' is a slice of strings tracking the names of components
	// already encountered in the current dependency path.
	for _, v := range visited {
		if v == currentName {
			return fmt.Errorf("circular dependency found - components '%s' and '%s'", originalName, currentName)
		}
	}

	// Add current to visited path
	visited = append(visited, currentName)

	// Get the current component definition
	compDef := &agenticplatformv1alpha1.ComponentDefinition{}
	err := r.Get(ctx, client.ObjectKey{Name: currentName, Namespace: compDef.Namespace}, compDef)
	if err != nil {
		if errors.IsNotFound(err) {
			return err
		} else {
			logger.Error(err, "checkCircularDependency() encountered error")
			return err
		}
	}

	// If this component depends on the original, there is a circlular dependency
	for _, dep := range compDef.Spec.DependsOn {
		if dep == originalName {
			return fmt.Errorf("circular dependency detected: %s -> %s -> %s",
				originalName, currentName, originalName)
		}

		// Recurse through each dependency
		if err := r.checkCircularDependency(ctx, originalName, dep, visited); err != nil {
			return err
		}
	}

	return nil
}

func (r *ComponentDefinitionReconciler) updateComponentDefinitionStatus(ctx context.Context, compDef *agenticplatformv1alpha1.ComponentDefinition, status, message string) {
	logger := log.FromContext(ctx)
	conditionStatus := metav1.ConditionFalse
	if status == "Ready" {
		conditionStatus = metav1.ConditionTrue
	}

	meta.SetStatusCondition(&compDef.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus,
		Reason:             status,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})

	now := metav1.Now()
	compDef.Status.LastUpdateTime = &now

	if err := r.Status().Update(ctx, compDef); err != nil {
		logger.Error(err, "Failed to update ComponentDefinition status")
	}
}

func (r *ComponentDefinitionReconciler) handleDeletion(ctx context.Context, compDef *agenticplatformv1alpha1.ComponentDefinition) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling deletion of ComponentDefinition", "name", compDef.Name)

	// Check if this component is still referenced by any K8gentai instance
	apList := &agenticplatformv1alpha1.K8gentaiList{}
	if err := r.List(ctx, apList); err != nil {
		logger.Error(err, "Failed to list K8gentai instances")
		return ctrl.Result{}, err
	}

	for _, ap := range apList.Items {
		for _, comp := range ap.Spec.Components {

			if comp.Name == compDef.Name {
				// Prevent deletion if still referenced
				logger.Info("ComponentDefinition cannot be deleted while still referenced",
					"definition", compDef.Name,
					"referencedBy", ap.Name)

				meta.SetStatusCondition(&compDef.Status.Conditions, metav1.Condition{
					Type:               "DeletionBlocked",
					Status:             metav1.ConditionTrue,
					Reason:             "StillReferenced",
					Message:            fmt.Sprintf("Still referenced by K8gentai '%s'", ap.Name),
					LastTransitionTime: metav1.Now(),
				})

				if err := r.Status().Update(ctx, compDef); err != nil {
					logger.Error(err, "Failed to update status")
				}
				return ctrl.Result{RequeueAfter: ReconcileLimit}, nil
			}
		}
	}

	controllerutil.RemoveFinalizer(compDef, "componentdefinition.applatform.io/finalizer")
	if err := r.Update(ctx, compDef); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Removed finalizer from ComponentDefinition", "name", compDef.Name)
	return ctrl.Result{}, nil
}

func (r *ComponentDefinitionReconciler) reconcileReferencingK8gentaiInstances(ctx context.Context, compDef *agenticplatformv1alpha1.ComponentDefinition) error {
	logger := log.FromContext(ctx)

	apList := &agenticplatformv1alpha1.K8gentaiList{}
	if err := r.List(ctx, apList); err != nil {
		return err
	}

	// Check each K8gentai object for a reference to this ComponentDefinition
	for _, ap := range apList.Items {
		referencesThis := false
		for _, comp := range ap.Spec.Components {
			if comp.Name == compDef.Name {
				referencesThis = true
				break
			}
		}

		// If this K8gentai references this ComponentDefinition,
		// trigger a reconciliation by adding an annotation
		if referencesThis {
			logger.Info("Found K8gentai referencing this ComponentDefinition",
				"apName", ap.Name,
				"componentDefinition", compDef.Name)

			apCopy := ap.DeepCopy()
			if apCopy.Annotations == nil {
				apCopy.Annotations = make(map[string]string)
			}
			apCopy.Annotations["agenticplatform.applatform.io/last-component-update"] =
				fmt.Sprintf("%s-%d", compDef.Name, time.Now().Unix())

			if err := r.Update(ctx, apCopy); err != nil {
				logger.Error(err, "Failed to update K8gentai",
					"apName", ap.Name)
				continue
			}
		}
	}

	return nil
}

func (r *ComponentDefinitionReconciler) getReferencingK8gentaiInstance(ctx context.Context, compDef *agenticplatformv1alpha1.ComponentDefinition) (*agenticplatformv1alpha1.K8gentai, error) {
	apList := &agenticplatformv1alpha1.K8gentaiList{}
	if err := r.List(ctx, apList); err != nil {
		return nil, err
	}
	for _, ap := range apList.Items {
		for _, comp := range ap.Spec.Components {
			if comp.Name == compDef.Name {
				return &ap, nil
			}
		}
	}

	return nil, nil
}

func (r *ComponentDefinitionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agenticplatformv1alpha1.ComponentDefinition{}).
		Complete(r)
}
