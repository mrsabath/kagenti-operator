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
	"encoding/json"
	"time"

	agentv1alpha1 "github.com/kagenti/operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// KagentiAgentReconciler reconciles a KagentiAgent object
type KagentiAgentReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	EnableClientRegistration bool
}

var (
	logger = ctrl.Log.WithName("controller").WithName("KagentiAgent")
)

const AGENT_FINALIZER = "agent.kagenti.dev/finalizer"

// +kubebuilder:rbac:groups=agent.kagenti.dev,resources=kagentiagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.kagenti.dev,resources=kagentiagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.kagenti.dev,resources=kagentiagents/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *KagentiAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger.Info("Reconciling KagentiAgent", "namespacedName", req.NamespacedName)

	agent := &agentv1alpha1.KagentiAgent{}
	err := r.Get(ctx, req.NamespacedName, agent)
	if err != nil {
		// The object might have been deleted after the reconcile request.
		// Return and don't requeue
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !agent.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, agent)
	}

	if !controllerutil.ContainsFinalizer(agent, AGENT_FINALIZER) {
		controllerutil.AddFinalizer(agent, AGENT_FINALIZER)
		if err := r.Update(ctx, agent); err != nil {
			logger.Error(err, "Unable to add finalizer to Agent")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	deploymentResult, err := r.reconcileAgentDeployment(ctx, agent)
	if err != nil {
		return deploymentResult, err
	}

	serviceResult, err := r.reconcileAgentService(ctx, agent)
	if err != nil {
		return serviceResult, err
	}
	return ctrl.Result{}, nil
}

func (r *KagentiAgentReconciler) reconcileAgentDeployment(ctx context.Context, agent *agentv1alpha1.KagentiAgent) (ctrl.Result, error) {
	deploymentName := agent.Name
	deployment := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: agent.Namespace}, deployment)
	if err != nil && errors.IsNotFound(err) {
		deployment = r.createDeploymentForAgent(agent)
		logger.Info("Creating KagentiAgent Deployment", "deploymentName", deploymentName)

		data, err := json.MarshalIndent(deployment, "", "  ")
		if err != nil {
			logger.Error(err, "Unable to marshal deployment spec to JSON")
			return ctrl.Result{}, err
		}
		logger.Info("Deployment spec: " + string(data))

		if err := controllerutil.SetControllerReference(agent, deployment, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference for KagentiAgent Deployment")
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, deployment); err != nil {
			logger.Error(err, "Unable to create KagentiAgent Deployment")
			return ctrl.Result{}, err
		}

		if err := r.initializeComponentStatus(ctx, agent); err != nil {
			logger.Error(err, "Unable to initialize KagentiAgent status after deployment creation")
			return ctrl.Result{}, err
		}
		//		if err := r.Status().Update(ctx, agent); err != nil {
		//			logger.Error(err, "Unable to update KagentiAgent status after deployment creation")
		//			return ctrl.Result{}, err
		//		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get KagentiAgent Deployment")
		return ctrl.Result{}, err
	}

	if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
		if agent.Status.DeploymentStatus.Phase != agentv1alpha1.PhaseReady {
			agent.Status.DeploymentStatus.Phase = agentv1alpha1.PhaseReady

			meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "DeploymentReady",
				Message:            "Deployment has minimum availability",
			})

			if err := r.Status().Update(ctx, agent); err != nil {
				logger.Error(err, "Failed to update Agent status for ready deployment")
				return ctrl.Result{}, err
			}

		}
	} else if agent.Status.DeploymentStatus != nil {
		if agent.Status.DeploymentStatus.Phase != agentv1alpha1.PhaseDeploying {
			agent.Status.DeploymentStatus.Phase = agentv1alpha1.PhaseDeploying

			meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             "DeploymentNotReady",
				Message:            "Deployment does not have minimum availability",
			})

			if err := r.Status().Update(ctx, agent); err != nil {
				logger.Error(err, "Failed to update KagentiAgent status for not ready deployment")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}
func (r *KagentiAgentReconciler) initializeComponentStatus(ctx context.Context, agent *agentv1alpha1.KagentiAgent) error {
	now := metav1.Now()
	agent.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: now,
			Reason:             "Initializing",
			Message:            "Agent is being initialized",
		},
	}
	//agent.Status.LastTransitionTime = &now

	agent.Status.DeploymentStatus = &agentv1alpha1.DeploymentStatus{
		Phase:             agentv1alpha1.PhaseDeploying,
		DeploymentMessage: "Deployment is pending",
	}

	return r.Client.Status().Update(ctx, agent)
}
func (r *KagentiAgentReconciler) createDeploymentForAgent(agent *agentv1alpha1.KagentiAgent) *appsv1.Deployment {
	labels := map[string]string{
		"app":        agent.Name,
		"controller": "agent-operator",
	}

	replicas := int32(*agent.Spec.Replicas)

	clientId := agent.Namespace + "/" + agent.Name
	mainEnvs := agent.Spec.PodTemplateSpec.Spec.Containers[0].Env
	mainEnvs = append(mainEnvs, []corev1.EnvVar{
		{
			Name:  "CLIENT_NAME",
			Value: clientId,
		},
		{
			Name:  "CLIENT_ID",
			Value: "spiffe://localtest.me/sa/" + agent.Name,
		},
		{
			Name:  "NAMESPACE",
			Value: agent.Namespace,
		},
		{
			Name:  "UV_CACHE_DIR",
			Value: "/app/.cache/uv",
		},
	}...)
	for inx := range agent.Spec.PodTemplateSpec.Spec.Containers {
		agent.Spec.PodTemplateSpec.Spec.Containers[inx].Env = mainEnvs
	}

	if agent.Spec.PodTemplateSpec.Spec.InitContainers == nil {
		agent.Spec.PodTemplateSpec.Spec.InitContainers = []corev1.Container{}
	}

	if r.EnableClientRegistration {
		agent.Spec.PodTemplateSpec.Spec.InitContainers = append(agent.Spec.PodTemplateSpec.Spec.InitContainers, corev1.Container{
			Name:            "kagenti-client-registration",
			Image:           "ghcr.io/kagenti/kagenti/client-registration:latest",
			ImagePullPolicy: agent.Spec.PodTemplateSpec.Spec.Containers[0].ImagePullPolicy,
			Resources:       *agent.Spec.PodTemplateSpec.Spec.Resources,
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "cache",
					MountPath: "/app/.cache",
				},
				{
					Name:      "venv",
					MountPath: "/app/.venv",
				},
			},
			Env: []corev1.EnvVar{
				{
					Name: "KEYCLOAK_URL",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "environments",
							},
							Key:      "KEYCLOAK_URL",
							Optional: ptr.To(true),
						},
					},
				},
				{
					Name: "KEYCLOAK_REALM",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "environments",
							},
							Key: "KEYCLOAK_REALM",
						},
					},
				},
				{
					Name: "KEYCLOAK_ADMIN_USERNAME",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "environments",
							},
							Key: "KEYCLOAK_ADMIN_USERNAME",
						},
					},
				},
				{
					Name: "KEYCLOAK_ADMIN_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "environments",
							},
							Key: "KEYCLOAK_ADMIN_PASSWORD",
						},
					},
				},
				{
					Name:  "CLIENT_NAME",
					Value: agent.Name,
				},
				{
					Name:  "CLIENT_ID",
					Value: "spiffe://localtest.me/sa/" + agent.Name,
				},
				{
					Name:  "NAMESPACE",
					Value: agent.Namespace,
				},
			},
		})
	}

	addVolumesAndMounts(&agent.Spec.PodTemplateSpec.Spec, "cache", "/app/.cache")
	addVolumesAndMounts(&agent.Spec.PodTemplateSpec.Spec, "venv", "/app/.venv")

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},

			Template: corev1.PodTemplateSpec{

				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},

				Spec: agent.Spec.PodTemplateSpec.Spec,
			},
		},
	}
}
func addVolumesAndMounts(podSpec *corev1.PodSpec, volumeName string, mountPath string) {
	if !hasVolumeMounts(podSpec, volumeName) {
		for inx := range podSpec.Containers {
			podSpec.Containers[inx].VolumeMounts = append(podSpec.Containers[inx].VolumeMounts, corev1.VolumeMount{
				Name:      volumeName,
				MountPath: mountPath,
			})
		}
	}
	if !hasVolume(podSpec, volumeName) {
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}
}

func hasVolumeMounts(podSpec *corev1.PodSpec, volumeMountName string) bool {
	// Check if any container has volume mounts
	for _, container := range podSpec.Containers {
		for _, vm := range container.VolumeMounts {
			if vm.Name == volumeMountName {
				return true
			}
		}
	}
	return false
}

func hasVolume(podSpec *corev1.PodSpec, volumeName string) bool {
	// Check if any container has volume mounts
	for _, v := range podSpec.Volumes {
		if v.Name == volumeName {
			return true
		}
	}
	return false
}

func (r *KagentiAgentReconciler) reconcileAgentService(ctx context.Context, agent *agentv1alpha1.KagentiAgent) (ctrl.Result, error) {
	serviceName := agent.Name + "-service"
	service := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: agent.Namespace}, service)

	if err != nil && errors.IsNotFound(err) {
		service = r.createServiceForAgent(agent)
		logger.Info("Creating Service", "serviceName", serviceName)

		if err := controllerutil.SetControllerReference(agent, service, r.Scheme); err != nil {
			logger.Error(err, "Failed to set controller reference for Service")
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, service); err != nil {
			logger.Error(err, "Failed to create Service")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KagentiAgentReconciler) createServiceForAgent(agent *agentv1alpha1.KagentiAgent) *corev1.Service {
	labels := map[string]string{
		"app":        agent.Name,
		"controller": "agent-operator",
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name + "-service",
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Name:       "http",
				},
			},
		},
	}
}

func (r *KagentiAgentReconciler) handleDeletion(ctx context.Context, agent *agentv1alpha1.KagentiAgent) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(agent, AGENT_FINALIZER) {
		// Delete Deployment and associated Service objects
		deployment := &appsv1.Deployment{}
		deploymentName := agent.Name
		err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: agent.Namespace}, deployment)
		if err == nil {
			logger.Info("Deleting deployment for KagentiAgent", "deploymentName", deploymentName)
			if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete deployment for KagentiAgent", "deploymentName", deploymentName)
				return ctrl.Result{}, err
			}
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to get deployment for deletion", "deploymentName", deploymentName)
			return ctrl.Result{}, err
		}

		service := &corev1.Service{}
		serviceName := agent.Name
		err = r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: agent.Namespace}, service)
		if err == nil {
			logger.Info("Deleting service for KagentiAgent", "serviceName", serviceName)
			if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete service for Agent", "serviceName", serviceName)
				return ctrl.Result{}, err
			}
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to get service for deletion", "serviceName", serviceName)
			return ctrl.Result{}, err
		}
		controllerutil.RemoveFinalizer(agent, AGENT_FINALIZER)
		if err := r.Update(ctx, agent); err != nil {
			logger.Error(err, "Failed to remove finalizer from KagentiAgent")
			return ctrl.Result{}, err
		}
		logger.Info("Removed finalizer from KagentiAgent")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KagentiAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.KagentiAgent{}).
		Named("kagentiagent").
		Complete(r)
}
