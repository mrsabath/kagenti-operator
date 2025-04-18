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
	"sort"
	"strconv"
	"strings"
	"time"

	k8gentaiv1alpha1 "aiplatform/k8gentai-operator.git/api/v1alpha1"

	helm "github.com/kubestellar/kubeflex/pkg/helm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ChartSpec struct {
	Namespace   string
	RepoURL     string
	RepoName    string
	ChartName   string
	ReleaseName string
	Version     string
	Parameters  map[string]string
}

type K8gentaiReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ComponentInstanceInfo combines component reference with its definition
type ComponentInstanceInfo struct {
	Reference  k8gentaiv1alpha1.ComponentReference
	Definition k8gentaiv1alpha1.ComponentDefinition
	Status     ComponentStatus
	Message    string
	DependsOn  []string
}

// ComponentStatus represents the status of a component
type ComponentStatus string

const (
	ComponentStatusPending       ComponentStatus = "Pending"
	ComponentStatusDeploying     ComponentStatus = "Deploying"
	ComponentStatusRunning       ComponentStatus = "Running"
	ComponentStatusFailed        ComponentStatus = "Failed"
	ComponentStatusDependsFailed ComponentStatus = "DependsFailed"
)

var ReconcileLimit time.Duration = time.Duration(10) * time.Second

// +kubebuilder:rbac:groups=agenticplatform.applatform.io,resources=agenticplatforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agenticplatform.applatform.io,resources=agenticplatforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agenticplatform.applatform.io,resources=agenticplatforms/finalizers,verbs=update
// +kubebuilder:rbac:groups=agenticplatform.applatform.io,resources=componentdefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;persistentvolumeclaims;pods;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
func (r *K8gentaiReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "namespacedName", req.NamespacedName)

	ap := &k8gentaiv1alpha1.K8gentai{}
	if err := r.Get(ctx, req.NamespacedName, ap); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get K8gentai")
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(ap, "k8gentai.applatform.io/finalizer") {
		controllerutil.AddFinalizer(ap, "k8gentai.applatform.io/finalizer")
		if err := r.Update(ctx, ap); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if !ap.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, ap)
	}

	componentInfos, err := r.getComponentDefinitions(ctx, ap)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to resolve component definitions")
		return ctrl.Result{}, err
	}

	// Sort components based on dependencies and type (infra first, then apps). Need to reconcile (and install)
	// infra components first.
	sortedComponents, err := r.sortComponentsByDependencies(ctx, componentInfos)
	if err != nil {
		logger.Error(err, "Failed to sort components by dependencies")
		return ctrl.Result{}, err
	}

	if ap.Status.ComponentStatuses == nil {
		ap.Status.ComponentStatuses = make(map[string]k8gentaiv1alpha1.ComponentStatus)
	}
	// Check if all infra components are ready
	allInfraReady := true
	infraComponentsMap := make(map[string]bool)

	// Process each component in dependency order,first infra components than app
	for _, compInfo := range sortedComponents {
		// Skip components whose dependencies aren't yet ready
		dependsFailed := false
		// Each component has a dependency list defined (can be empty)
		for _, depName := range compInfo.DependsOn {
			depStatus, exists := ap.Status.ComponentStatuses[depName]
			if !exists || depStatus.Status != string(ComponentStatusRunning) {
				logger.Info("Skipping component as dependency not ready",
					"component", compInfo.Reference.Name,
					"dependency", depName)

				now := metav1.Now()
				ap.Status.ComponentStatuses[compInfo.Reference.Name] = k8gentaiv1alpha1.ComponentStatus{
					Status:         string(ComponentStatusDependsFailed),
					Message:        fmt.Sprintf("Dependency '%s' not ready", depName),
					LastUpdateTime: &now,
				}
				dependsFailed = true
				break
			}
		}
		if dependsFailed {
			continue
		}
		now := metav1.Now()
		// If this is an app component, ensure all infra components are ready first
		if compInfo.Definition.Spec.Type == "app" && !allInfraReady {
			logger.Info("Skipping app component as not all infra components are ready",
				"component", compInfo.Reference.Name)

			ap.Status.ComponentStatuses[compInfo.Reference.Name] = k8gentaiv1alpha1.ComponentStatus{
				Status:         string(ComponentStatusPending),
				Message:        "Waiting for all infrastructure components to be ready",
				LastUpdateTime: &now,
			}
			continue
		}
		status, err := r.reconcileComponent(ctx, ap, compInfo)
		if err != nil {
			logger.Error(err, "Failed to reconcile component",
				"component", compInfo.Reference.Name)

			ap.Status.ComponentStatuses[compInfo.Reference.Name] = k8gentaiv1alpha1.ComponentStatus{
				Status:         string(ComponentStatusFailed),
				Message:        err.Error(),
				LastUpdateTime: &now,
			}

			// If an infra component fails, mark infra not ready
			if compInfo.Definition.Spec.Type == "infra" {
				allInfraReady = false
			}

			if err := r.Status().Update(ctx, ap); err != nil {
				logger.Error(err, "Failed to update status after component failure")
			}

			return ctrl.Result{RequeueAfter: ReconcileLimit}, nil
		}

		ap.Status.ComponentStatuses[compInfo.Reference.Name] = k8gentaiv1alpha1.ComponentStatus{
			Status:         string(status),
			Message:        fmt.Sprintf("Component %s is %s", compInfo.Reference.Name, status),
			LastUpdateTime: &now,
		}

		// If this is an infra component and it's not running, update the allInfraReady flag
		if compInfo.Definition.Spec.Type == "infra" {
			infraComponentsMap[compInfo.Reference.Name] = true
			if status != ComponentStatusRunning {
				allInfraReady = false
			}
		}
	}

	// Update the overall platform status
	r.updateK8gentaiStatus(ctx, ap, allInfraReady)

	if err := r.Status().Update(ctx, ap); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: ReconcileLimit}, nil
}

// fetch all ComponentDefinitions referenced by the K8gentai object
func (r *K8gentaiReconciler) getComponentDefinitions(ctx context.Context, ap *k8gentaiv1alpha1.K8gentai) (map[string]ComponentInstanceInfo, error) {
	logger := log.FromContext(ctx)
	componentInfos := make(map[string]ComponentInstanceInfo)

	for _, compRef := range ap.Spec.Components {
		compDef := &k8gentaiv1alpha1.ComponentDefinition{}
		err := r.Get(ctx, client.ObjectKey{Name: compRef.Name, Namespace: compRef.Namespace}, compDef)
		if err != nil {
			logger.Error(err, "Failed to get ComponentDefinition",
				"definition", compRef.Name)
			return nil, fmt.Errorf("failed to get ComponentDefinition '%s': %w",
				compRef.Name, err)
		}

		componentInfos[compRef.Name] = ComponentInstanceInfo{
			Reference:  compRef,
			Definition: *compDef,
			Status:     ComponentStatusPending,
			Message:    "Pending reconciliation",
			DependsOn:  compDef.Spec.DependsOn,
		}
	}

	return componentInfos, nil
}

// sorts a collection of ComponentInstanceInfo objects based on their dependencies and types. It implements a topological
// sort algorithm to determine the correct order for processing or deploying these components. Infra components need to be
// installed first before the app components are deployed.
func (r *K8gentaiReconciler) sortComponentsByDependencies(ctx context.Context, componentInfos map[string]ComponentInstanceInfo) ([]ComponentInstanceInfo, error) {
	// Create maps to track component dependencies and incoming dependency counts.
	dependencyGraph := make(map[string][]string)
	incomingEdges := make(map[string]int)

	// Initialize with no incoming edges
	for name := range componentInfos {
		dependencyGraph[name] = []string{}
		incomingEdges[name] = 0
	}

	// Build the graph
	for name, info := range componentInfos {
		for _, depName := range info.DependsOn {
			// Check if dependency is in our component list
			if _, exists := componentInfos[depName]; exists {
				dependencyGraph[depName] = append(dependencyGraph[depName], name)
				incomingEdges[name]++
			}
		}
	}

	// Topological sort
	var sortedComponents []ComponentInstanceInfo
	var queue []string

	// Begin with nodes which have no dependencies
	for name, count := range incomingEdges {
		if count == 0 {
			queue = append(queue, name)
		}
	}

	// Process the queue
	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]

		// Add to result
		sortedComponents = append(sortedComponents, componentInfos[current])

		// Remove this node from the graph
		for _, dependent := range dependencyGraph[current] {
			incomingEdges[dependent]--
			if incomingEdges[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Check for circular dependencies
	if len(sortedComponents) != len(componentInfos) {
		return nil, fmt.Errorf("circular dependency detected in component graph")
	}

	// Finally, ensure infra components come before app components
	sort.SliceStable(sortedComponents, func(i, j int) bool {
		// If types are different, infra comes first
		if sortedComponents[i].Definition.Spec.Type != sortedComponents[j].Definition.Spec.Type {
			return sortedComponents[i].Definition.Spec.Type == "infra"
		}
		// Otherwise, keep the dependency-based order
		return true
	})

	return sortedComponents, nil
}

func (r *K8gentaiReconciler) reconcileComponent(ctx context.Context, ap *k8gentaiv1alpha1.K8gentai, compInfo ComponentInstanceInfo) (ComponentStatus, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling component",
		"component", compInfo.Reference.Name,
		"type", compInfo.Definition.Spec.Type,
		"installer", compInfo.Definition.Spec.Installer)

	comp := compInfo.Reference
	compDef := compInfo.Definition.Spec

	// Check if component is already deployed and running
	currentStatus, exists := ap.Status.ComponentStatuses[comp.Name]
	if exists && currentStatus.Status == string(ComponentStatusRunning) {
		// Check if component is still running
		if isRunning, err := r.isComponentRunning(ctx, compInfo); err != nil {
			return ComponentStatusFailed, err
		} else if isRunning {
			return ComponentStatusRunning, nil
		}
	}

	// Deploy the component based on the installer type
	switch compDef.Installer {
	case "helm":
		return r.reconcileHelmComponent(ctx, ap, compInfo)
	case "deployment":
		return r.reconcileDeploymentComponent(ctx, ap, compInfo)
	default:
		return ComponentStatusFailed, fmt.Errorf("unsupported installer type: %s", compDef.Installer)
	}
}

func (r *K8gentaiReconciler) reconcileHelmComponent(ctx context.Context, ap *k8gentaiv1alpha1.K8gentai, compInfo ComponentInstanceInfo) (ComponentStatus, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Helm component", "component", compInfo.Reference.Name)

	comp := compInfo.Reference
	compDef := compInfo.Definition.Spec

	// Merge default parameters with instance-specific parameters
	parameters := make(map[string]string)
	for k, v := range compDef.DefaultParameters {
		parameters[k] = v
	}
	for k, v := range comp.Parameters {
		parameters[k] = v
	}

	// Deploy or update the component using HelmChartDeployer
	err := r.InstallHelmChart(ctx, ChartSpec{
		Namespace:   comp.Namespace,
		RepoURL:     compDef.RepoUrl,
		Version:     compDef.Version,
		RepoName:    compDef.RepoName,
		ChartName:   compDef.ChartName,
		ReleaseName: comp.Name,
		Parameters:  parameters,
	})

	if err != nil {
		return ComponentStatusFailed, err
	}

	isRunning, err := r.isComponentRunning(ctx, compInfo)
	if err != nil {
		return ComponentStatusFailed, err
	}

	if isRunning {
		return ComponentStatusRunning, nil
	}

	return ComponentStatusDeploying, nil
}

func (r *K8gentaiReconciler) reconcileDeploymentComponent(ctx context.Context, ap *k8gentaiv1alpha1.K8gentai, compInfo ComponentInstanceInfo) (ComponentStatus, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Deployment component", "component", compInfo.Reference.Name)

	comp := compInfo.Reference
	compDef := compInfo.Definition.Spec

	// Merge default parameters with instance-specific parameters
	parameters := make(map[string]string)
	for k, v := range compDef.DefaultParameters {
		parameters[k] = v
	}
	for k, v := range comp.Parameters {
		parameters[k] = v
	}

	// Check if deployment already exists
	deployment := &appsv1.Deployment{}
	deploymentName := types.NamespacedName{
		Name:      comp.Name,
		Namespace: comp.Namespace,
	}

	err := r.Get(ctx, deploymentName, deployment)
	if err != nil && !errors.IsNotFound(err) {
		return ComponentStatusFailed, err
	}

	if errors.IsNotFound(err) {
		// Create deployment
		if err := r.createComponentDeployment(ctx, ap, compInfo, parameters); err != nil {
			return ComponentStatusFailed, err
		}

		// Create service if port is defined
		if _, ok := parameters["port"]; ok {
			if err := r.createComponentService(ctx, ap, compInfo, parameters); err != nil {
				return ComponentStatusFailed, err
			}
		}

		logger.Info("Deployment created for component", "component", comp.Name)
		return ComponentStatusDeploying, nil
	}
	// Check if the component is now running
	isRunning, err := r.isComponentRunning(ctx, compInfo)
	if err != nil {
		return ComponentStatusFailed, err
	}

	if isRunning {
		return ComponentStatusRunning, nil
	}

	return ComponentStatusDeploying, nil
}

func (r *K8gentaiReconciler) createComponentDeployment(ctx context.Context, ap *k8gentaiv1alpha1.K8gentai, compInfo ComponentInstanceInfo, parameters map[string]string) error {
	logger := log.FromContext(ctx)

	comp := compInfo.Reference
	compDef := compInfo.Definition.Spec

	// Extract parameters
	image := parameters["image"]
	if image == "" {
		return fmt.Errorf("image parameter is required for deployment component")
	}

	args := parameters["args"]

	port := parameters["port"]

	var containerPorts []corev1.ContainerPort
	if port != "" {
		portInt, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("invalid port number: %s", port)
		}
		containerPorts = append(containerPorts, corev1.ContainerPort{
			Name:          "http",
			ContainerPort: int32(portInt),
			Protocol:      corev1.ProtocolTCP,
		})
	}

	envVars := compDef.Env

	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1024Mi"),
		},
	}

	replicas := int32(1)
	if replicasStr, ok := parameters["replicas"]; ok {
		replicasInt, err := strconv.Atoi(replicasStr)
		if err != nil {
			logger.Error(err, "Failed to parse replicas", "value", replicasStr)
		} else {
			replicas = int32(replicasInt)
		}
	}

	// Create deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      comp.Name,
			Namespace: comp.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       comp.Name,
				"app.kubernetes.io/instance":   ap.Name,
				"app.kubernetes.io/managed-by": "ap-operator",
				"app.kubernetes.io/component":  compInfo.Definition.Spec.Type,
				"app.kubernetes.io/part-of":    ap.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":     comp.Name,
					"app.kubernetes.io/instance": ap.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      comp.Name,
						"app.kubernetes.io/instance":  ap.Name,
						"app.kubernetes.io/component": compInfo.Definition.Spec.Type,
						"app.kubernetes.io/part-of":   ap.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            comp.Name,
							Image:           image,
							ImagePullPolicy: "IfNotPresent",
							Ports:           containerPorts,
							Env:             envVars,
							Resources:       resources,
							TTY:             true,
							Stdin:           true,
						},
					},
				},
			},
		},
	}

	// Add command args if specified
	if args != "" {
		argsSlice := strings.Split(args, " ")
		deployment.Spec.Template.Spec.Containers[0].Args = argsSlice
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(ap, deployment, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, deployment)
}

func (r *K8gentaiReconciler) createComponentService(ctx context.Context, ap *k8gentaiv1alpha1.K8gentai, compInfo ComponentInstanceInfo, parameters map[string]string) error {
	comp := compInfo.Reference

	port := parameters["port"]
	if port == "" {
		return nil
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port number: %s", port)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      comp.Name,
			Namespace: comp.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       comp.Name,
				"app.kubernetes.io/instance":   ap.Name,
				"app.kubernetes.io/managed-by": "ap-operator",
				"app.kubernetes.io/component":  compInfo.Definition.Spec.Type,
				"app.kubernetes.io/part-of":    ap.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name":     comp.Name,
				"app.kubernetes.io/instance": ap.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(portInt),
					TargetPort: intstr.FromInt(portInt),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(ap, service, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, service)
}

func (r *K8gentaiReconciler) isComponentRunning(ctx context.Context, compInfo ComponentInstanceInfo) (bool, error) {
	// This implementation depends on the component type
	switch compInfo.Definition.Spec.Installer {
	case "helm":
		return r.isHelmComponentRunning(ctx, compInfo)
	case "deployment":
		return r.isDeploymentComponentRunning(ctx, compInfo)
	default:
		return false, fmt.Errorf("unsupported installer type: %s", compInfo.Definition.Spec.Installer)
	}
}

func (r *K8gentaiReconciler) isHelmComponentRunning(ctx context.Context, compInfo ComponentInstanceInfo) (bool, error) {
	// For Helm components, check if their respective pods are running
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(compInfo.Reference.Namespace),
		client.MatchingLabels{"app.kubernetes.io/instance": compInfo.Reference.Name},
	}

	if err := r.List(ctx, podList, listOpts...); err != nil {
		return false, err
	}

	// Check if we have any pods and if they're all running
	if len(podList.Items) == 0 {
		return false, nil
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			return false, nil
		}

		// Additional check for container readiness
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if !containerStatus.Ready {
				return false, nil
			}
		}
	}

	return true, nil
}

func (r *K8gentaiReconciler) isDeploymentComponentRunning(ctx context.Context, compInfo ComponentInstanceInfo) (bool, error) {
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: compInfo.Reference.Name, Namespace: compInfo.Reference.Namespace}, deployment); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Check if deployment is available and ready
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			// Check if all replicas are ready
			return deployment.Status.ReadyReplicas == *deployment.Spec.Replicas, nil
		}
	}

	return false, nil
}

func (r *K8gentaiReconciler) updateK8gentaiStatus(ctx context.Context, ap *k8gentaiv1alpha1.K8gentai, infraReady bool) {
	logger := log.FromContext(ctx)

	// Update infrastructure ready flag
	ap.Status.InfrastructureReady = infraReady

	// Count components by status
	statusCounts := make(map[string]int)
	for _, status := range ap.Status.ComponentStatuses {
		statusCounts[status.Status]++
	}

	// Determine overall K8gentai status
	if statusCounts[string(ComponentStatusFailed)] > 0 {
		ap.Status.PlatformStatus = "Degraded"
		ap.Status.PlatformMessage = fmt.Sprintf("Platform degraded: %d failed components", statusCounts[string(ComponentStatusFailed)])
	} else if statusCounts[string(ComponentStatusDeploying)] > 0 ||
		statusCounts[string(ComponentStatusPending)] > 0 ||
		statusCounts[string(ComponentStatusDependsFailed)] > 0 {
		ap.Status.PlatformStatus = "Deploying"
		ap.Status.PlatformMessage = "Platform is deploying components"
	} else if statusCounts[string(ComponentStatusRunning)] == len(ap.Status.ComponentStatuses) &&
		len(ap.Status.ComponentStatuses) > 0 {
		ap.Status.PlatformStatus = "Ready"
		ap.Status.PlatformMessage = "Platform is ready"
	} else {
		ap.Status.PlatformStatus = "Unknown"
		ap.Status.PlatformMessage = "Platform status is unknown"
	}
	now := metav1.Now()
	ap.Status.LastUpdateTime = &now

	ready := "False"
	msg := "Some infrastructure components are not ready"
	if infraReady {
		ready = "True"
		msg = "All infrastructure components are ready"
	}

	meta.SetStatusCondition(&ap.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionStatus(ready),
		Reason:             ap.Status.PlatformStatus,
		Message:            ap.Status.PlatformMessage,
		LastTransitionTime: metav1.Now(),
	})
	meta.SetStatusCondition(&ap.Status.Conditions, metav1.Condition{
		Type:    "InfrastructureReady",
		Status:  metav1.ConditionStatus(ready),
		Reason:  ready,
		Message: msg,
		//	"Some infrastructure components are not ready",
		LastTransitionTime: metav1.Now(),
	})

	logger.Info("Updated platform status",
		"status", ap.Status.PlatformStatus,
		"infraReady", ap.Status.InfrastructureReady,
	)
}

func (r *K8gentaiReconciler) handleDeletion(ctx context.Context, ap *k8gentaiv1alpha1.K8gentai) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling deletion of K8gentai instance")

	// Process component deletions in reverse order of dependencies
	// First get all component definitions
	componentInfos, err := r.getComponentDefinitions(ctx, ap)
	if err != nil {
		logger.Error(err, "Failed to resolve component definitions for deletion")
		// Continue with finalizer removal even if component resolution fails
	} else {
		// Sort components with dependencies
		sortedComponents, err := r.sortComponentsByDependencies(ctx, componentInfos)
		if err != nil {
			logger.Error(err, "Failed to sort components for deletion")
			// Continue with finalizer removal even if sorting fails
		} else {
			// Reverse the order for deletion (application components before infrastructure)
			for i, j := 0, len(sortedComponents)-1; i < j; i, j = i+1, j-1 {
				sortedComponents[i], sortedComponents[j] = sortedComponents[j], sortedComponents[i]
			}

			// Delete each component
			for _, compInfo := range sortedComponents {
				if err := r.deleteComponent(ctx, ap, compInfo); err != nil {
					logger.Error(err, "Failed to delete component during cleanup", "component", compInfo.Reference.Name)
				}
			}
		}
	}

	// Once everything is cleaned up, remove the finalizer
	controllerutil.RemoveFinalizer(ap, "k8gentai.applatform.io/finalizer")
	if err := r.Update(ctx, ap); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Finalizer removed, allowing deletion")
	return ctrl.Result{}, nil
}

func (r *K8gentaiReconciler) deleteComponent(ctx context.Context, ap *k8gentaiv1alpha1.K8gentai, compInfo ComponentInstanceInfo) error {
	logger := log.FromContext(ctx)
	logger.Info("Deleting component", "component", compInfo.Reference.Name)

	switch compInfo.Definition.Spec.Installer {
	case "helm":
		if err := r.Uninstall(ctx, compInfo.Reference.Namespace, compInfo.Reference.Name); err != nil {
			return fmt.Errorf("failed to uninstall Helm chart: %w", err)
		}
	case "deployment":
		// Delete the deployment
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      compInfo.Reference.Name,
				Namespace: compInfo.Reference.Namespace,
			},
		}
		if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete deployment: %w", err)
		}

		// Delete the service if it exists
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      compInfo.Reference.Name,
				Namespace: compInfo.Reference.Namespace,
			},
		}
		if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete service: %w", err)
		}
	default:
		return fmt.Errorf("unsupported installer type: %s", compInfo.Definition.Spec.Installer)
	}

	return nil
}

func (r *K8gentaiReconciler) InstallHelmChart(ctx context.Context, chart ChartSpec) error {
	reqLogger := log.FromContext(ctx)
	parameters := []string{}
	for key, value := range chart.Parameters {
		parameters = append(parameters, fmt.Sprintf("%s=%s", key, value))
	}

	reqLogger.Info("Deploying ", "Infra Component", chart.ChartName)
	h := &helm.HelmHandler{
		URL:         chart.RepoURL,
		Version:     chart.Version,
		RepoName:    chart.RepoName,
		ChartName:   chart.ChartName,
		Namespace:   chart.Namespace,
		ReleaseName: chart.ReleaseName,
		Args:        map[string]string{"set": strings.Join(parameters, ",")},
	}
	if err := helm.Init(ctx, h); err != nil {
		reqLogger.Error(err, "Failed to call Helm.Init()")
		return err
	}

	if !h.IsDeployed() {
		if err := h.Install(); err != nil {
			reqLogger.Error(err, "Failed to call Helm.Install()")
			return err
		}
	} else {
		reqLogger.Info("Helm Chart Already Installed", "Component", chart.ChartName, "Namespace", chart.Namespace)
	}
	reqLogger.Info("Helm Chart Installed")
	return nil
}

func IsHelmDeployed() bool {
	return true
}

func (r *K8gentaiReconciler) Uninstall(ctx context.Context, componentName, namespace string) error {
	return nil
}

func (r *K8gentaiReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8gentaiv1alpha1.K8gentai{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(
			&k8gentaiv1alpha1.ComponentDefinition{},
			handler.EnqueueRequestsFromMapFunc(r.findK8gentaiInstancesForComponentDefinition),
		).
		Complete(r)
}

func (r *K8gentaiReconciler) findK8gentaiInstancesForComponentDefinition(ctx context.Context, compDef client.Object) []reconcile.Request {
	apList := &k8gentaiv1alpha1.K8gentaiList{}
	if err := r.Client.List(context.Background(), apList); err != nil {
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for _, ap := range apList.Items {
		for _, comp := range ap.Spec.Components {
			if comp.Name == compDef.GetName() {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ap.GetName(),
						Namespace: ap.GetNamespace(),
					},
				})
				break
			}
		}
	}

	return requests
}
