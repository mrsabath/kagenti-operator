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

package kubernetes

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceStatus represents the status of a deployed resource
type ResourceStatus struct {
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	Kind      string                 `json:"kind"`
	Group     string                 `json:"group"`
	Version   string                 `json:"version"`
	Ready     bool                   `json:"ready"`
	Phase     string                 `json:"phase"`
	Reason    string                 `json:"reason,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	LastCheck time.Time              `json:"lastCheck"`
}

// DeploymentStatus represents overall deployment status
type DeploymentStatus struct {
	TotalResources int              `json:"totalResources"`
	ReadyResources int              `json:"readyResources"`
	Resources      []ResourceStatus `json:"resources"`
	OverallReady   bool             `json:"overallReady"`
	LastUpdated    time.Time        `json:"lastUpdated"`
}

type StatusChecker struct {
	client.Client
	Log logr.Logger
}

func NewStatusChecker(client client.Client, log logr.Logger) *StatusChecker {
	return &StatusChecker{Client: client, Log: log}
}

func (sc *StatusChecker) CheckResourceStatus(ctx context.Context, obj client.Object) (*ResourceStatus, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	sc.Log.Info("CheckResourceStatus", "GVK", gvk)
	status := &ResourceStatus{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Kind:      gvk.Kind,
		Group:     gvk.Group,
		Version:   gvk.Version,
		LastCheck: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Get the current state of the resource
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}

	switch gvk.Kind {
	case "Deployment":
		return sc.checkDeploymentStatus(ctx, key, status)
	case "StatefulSet":
		return sc.checkStatefulSetStatus(ctx, key, status)
	case "DaemonSet":
		return sc.checkDaemonSetStatus(ctx, key, status)
	case "ReplicaSet":
		return sc.checkReplicaSetStatus(ctx, key, status)
	case "Job":
		return sc.checkJobStatus(ctx, key, status)
	case "Service":
		return sc.checkServiceStatus(ctx, key, status)
	case "Pod":
		return sc.checkPodStatus(ctx, key, status)
	case "PersistentVolumeClaim":
		return sc.checkPVCStatus(ctx, key, status)
	case "ConfigMap", "Secret":
		return sc.checkBasicResourceStatus(ctx, key, status)
	default:
		return sc.checkCustomResourceStatus(ctx, obj, status)
	}
}

func (sc *StatusChecker) checkDeploymentStatus(ctx context.Context, key types.NamespacedName, status *ResourceStatus) (*ResourceStatus, error) {
	sc.Log.Info("checkDeploymentStatus")
	deployment := &appsv1.Deployment{}
	if err := sc.Get(ctx, key, deployment); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	desired := *deployment.Spec.Replicas
	ready := deployment.Status.ReadyReplicas
	available := deployment.Status.AvailableReplicas

	status.Details["desiredReplicas"] = desired
	status.Details["readyReplicas"] = ready
	status.Details["availableReplicas"] = available
	status.Details["updatedReplicas"] = deployment.Status.UpdatedReplicas

	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			status.Details["progressing"] = condition.Status == corev1.ConditionTrue
			if condition.Status != corev1.ConditionTrue {
				status.Reason = condition.Reason
				status.Message = condition.Message
			}
		}
		if condition.Type == appsv1.DeploymentAvailable {
			status.Details["available"] = condition.Status == corev1.ConditionTrue
		}
	}

	if ready == desired && available == desired {
		status.Ready = true
		status.Phase = "Ready"
	} else if ready > 0 {
		status.Ready = false
		status.Phase = "PartiallyReady"
	} else {
		status.Ready = false
		status.Phase = "NotReady"
	}

	sc.Log.Info("checkDeploymentStatus", "status", status)
	return status, nil
}

func (sc *StatusChecker) checkStatefulSetStatus(ctx context.Context, key types.NamespacedName, status *ResourceStatus) (*ResourceStatus, error) {
	sts := &appsv1.StatefulSet{}
	if err := sc.Get(ctx, key, sts); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	desired := *sts.Spec.Replicas
	ready := sts.Status.ReadyReplicas
	current := sts.Status.CurrentReplicas

	status.Details["desiredReplicas"] = desired
	status.Details["readyReplicas"] = ready
	status.Details["currentReplicas"] = current
	status.Details["updatedReplicas"] = sts.Status.UpdatedReplicas

	if ready == desired && current == desired {
		status.Ready = true
		status.Phase = "Ready"
	} else if ready > 0 {
		status.Ready = false
		status.Phase = "PartiallyReady"
	} else {
		status.Ready = false
		status.Phase = "NotReady"
	}
	sc.Log.Info("checkStatefulSetStatus", "status", status)
	return status, nil
}

func (sc *StatusChecker) checkReplicaSetStatus(ctx context.Context, key types.NamespacedName, status *ResourceStatus) (*ResourceStatus, error) {
	rs := &appsv1.ReplicaSet{}
	if err := sc.Get(ctx, key, rs); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	desired := *rs.Spec.Replicas
	ready := rs.Status.ReadyReplicas
	available := rs.Status.AvailableReplicas

	status.Details["desiredReplicas"] = desired
	status.Details["readyReplicas"] = ready
	status.Details["availableReplicas"] = available
	status.Details["fullyLabeledReplicas"] = rs.Status.FullyLabeledReplicas
	status.Details["observedGeneration"] = rs.Status.ObservedGeneration

	for _, condition := range rs.Status.Conditions {
		switch condition.Type {
		case appsv1.ReplicaSetReplicaFailure:
			if condition.Status == corev1.ConditionTrue {
				status.Details["replicaFailure"] = true
				status.Reason = condition.Reason
				status.Message = condition.Message
			}
		}
	}

	// Determine readiness based on replica counts
	if ready == desired && available == desired {
		status.Ready = true
		status.Phase = "Ready"
	} else if ready > 0 {
		status.Ready = false
		status.Phase = "PartiallyReady"
		if desired > 0 {
			status.Message = fmt.Sprintf("%d/%d replicas ready", ready, desired)
		}
	} else {
		status.Ready = false
		status.Phase = "NotReady"
		if desired > 0 {
			status.Message = fmt.Sprintf("0/%d replicas ready", desired)
		}
	}

	// Additional checks for edge cases
	if desired == 0 {
		status.Ready = true
		status.Phase = "ScaledDown"
		status.Message = "ReplicaSet scaled to 0 replicas"
	}
	sc.Log.Info("checkReplicaSetStatus", "status", status)

	return status, nil
}

func (sc *StatusChecker) checkDaemonSetStatus(ctx context.Context, key types.NamespacedName, status *ResourceStatus) (*ResourceStatus, error) {
	ds := &appsv1.DaemonSet{}
	if err := sc.Get(ctx, key, ds); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	desired := ds.Status.DesiredNumberScheduled
	ready := ds.Status.NumberReady
	available := ds.Status.NumberAvailable

	status.Details["desiredNumberScheduled"] = desired
	status.Details["numberReady"] = ready
	status.Details["numberAvailable"] = available
	status.Details["numberUnavailable"] = ds.Status.NumberUnavailable

	if ready == desired && available == desired {
		status.Ready = true
		status.Phase = "Ready"
	} else if ready > 0 {
		status.Ready = false
		status.Phase = "PartiallyReady"
	} else {
		status.Ready = false
		status.Phase = "NotReady"
	}
	sc.Log.Info("checkDaemonSetStatus", "status", status)
	return status, nil
}

func (sc *StatusChecker) checkJobStatus(ctx context.Context, key types.NamespacedName, status *ResourceStatus) (*ResourceStatus, error) {
	job := &batchv1.Job{}
	if err := sc.Get(ctx, key, job); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	status.Details["active"] = job.Status.Active
	status.Details["succeeded"] = job.Status.Succeeded
	status.Details["failed"] = job.Status.Failed

	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			status.Ready = true
			status.Phase = "Complete"
			return status, nil
		}
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			status.Ready = false
			status.Phase = "Failed"
			status.Reason = condition.Reason
			status.Message = condition.Message
			return status, nil
		}
	}

	if job.Status.Active > 0 {
		status.Ready = false
		status.Phase = "Running"
	} else {
		status.Ready = false
		status.Phase = "Pending"
	}
	sc.Log.Info("checkJobStatus", "status", status)
	return status, nil
}

func (sc *StatusChecker) checkServiceStatus(ctx context.Context, key types.NamespacedName, status *ResourceStatus) (*ResourceStatus, error) {
	service := &corev1.Service{}
	if err := sc.Get(ctx, key, service); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	status.Details["type"] = string(service.Spec.Type)
	status.Details["clusterIP"] = service.Spec.ClusterIP

	if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(service.Status.LoadBalancer.Ingress) > 0 {
			status.Ready = true
			status.Phase = "Ready"
			status.Details["loadBalancerIngress"] = service.Status.LoadBalancer.Ingress
		} else {
			status.Ready = false
			status.Phase = "Pending"
			status.Message = "Waiting for LoadBalancer IP"
		}
	} else {
		// For ClusterIP and NodePort services, they're ready when created
		status.Ready = true
		status.Phase = "Ready"
	}
	sc.Log.Info("checkServiceStatus", "status", status)
	return status, nil
}

func (sc *StatusChecker) checkPodStatus(ctx context.Context, key types.NamespacedName, status *ResourceStatus) (*ResourceStatus, error) {
	pod := &corev1.Pod{}
	if err := sc.Get(ctx, key, pod); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	status.Phase = string(pod.Status.Phase)
	status.Details["restartCount"] = 0

	// Calculate total restart count
	for _, containerStatus := range pod.Status.ContainerStatuses {
		status.Details["restartCount"] = status.Details["restartCount"].(int) + int(containerStatus.RestartCount)
	}

	switch pod.Status.Phase {
	case corev1.PodRunning:
		// Check if all containers are ready
		allReady := true
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if !containerStatus.Ready {
				allReady = false
				break
			}
		}
		status.Ready = allReady
		if allReady {
			status.Phase = "Ready"
		} else {
			status.Phase = "NotReady"
		}
	case corev1.PodSucceeded:
		status.Ready = true
		status.Phase = "Succeeded"
	case corev1.PodFailed:
		status.Ready = false
		status.Phase = "Failed"
		status.Reason = pod.Status.Reason
		status.Message = pod.Status.Message
	default:
		status.Ready = false
	}
	sc.Log.Info("checkPodStatus", "status", status)
	return status, nil
}

func (sc *StatusChecker) checkPVCStatus(ctx context.Context, key types.NamespacedName, status *ResourceStatus) (*ResourceStatus, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	if err := sc.Get(ctx, key, pvc); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	status.Phase = string(pvc.Status.Phase)
	status.Details["capacity"] = pvc.Status.Capacity
	status.Details["accessModes"] = pvc.Status.AccessModes

	if pvc.Status.Phase == corev1.ClaimBound {
		status.Ready = true
		status.Phase = "Bound"
	} else {
		status.Ready = false
	}
	sc.Log.Info("checkPVCStatus", "status", status)
	return status, nil
}

func (sc *StatusChecker) checkBasicResourceStatus(ctx context.Context, key types.NamespacedName, status *ResourceStatus) (*ResourceStatus, error) {
	// Create unstructured object to check existence
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(status.GetGVK())

	if err := sc.Get(ctx, key, obj); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	// These resources are ready when they exist
	status.Ready = true
	status.Phase = "Ready"
	sc.Log.Info("checkBasicResourceStatus", "status", status)
	return status, nil
}

// Helper method for ResourceStatus to get GVK
func (rs *ResourceStatus) GetGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   rs.Group,
		Version: rs.Version,
		Kind:    rs.Kind,
	}
}

func (sc *StatusChecker) checkCustomResourceStatus(ctx context.Context, obj client.Object, status *ResourceStatus) (*ResourceStatus, error) {
	// Use unstructured to access custom resource status
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())

	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	if err := sc.Get(ctx, key, u); err != nil {
		status.Ready = false
		status.Phase = "NotFound"
		status.Reason = "ResourceNotFound"
		status.Message = err.Error()
		return status, nil
	}

	// Try to extract common status fields
	statusObj, found, err := unstructured.NestedMap(u.Object, "status")
	if err != nil || !found {
		// No status field, assume ready if exists
		status.Ready = true
		status.Phase = "Ready"
		return status, nil
	}

	// Look for common status indicators
	if phase, found, _ := unstructured.NestedString(statusObj, "phase"); found {
		status.Phase = phase
		status.Ready = (phase == "Ready" || phase == "Active" || phase == "Running")
	}

	if conditions, found, _ := unstructured.NestedSlice(statusObj, "conditions"); found {
		status.Details["conditions"] = conditions

		// Look for Ready condition
		for _, conditionObj := range conditions {
			if condition, ok := conditionObj.(map[string]interface{}); ok {
				if condType, _ := condition["type"].(string); condType == "Ready" {
					if condStatus, _ := condition["status"].(string); condStatus == "True" {
						status.Ready = true
						status.Phase = "Ready"
					}
				}
			}
		}
	}

	// If no specific indicators found, assume ready
	if status.Phase == "" {
		status.Ready = true
		status.Phase = "Ready"
	}
	sc.Log.Info("checkCustomResourceStatus", "status", status)
	return status, nil
}
