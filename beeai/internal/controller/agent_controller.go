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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	beeaiv1 "github.com/kagenti/kagenti-operator/api/v1"
)

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const agentFinalizer = "beeai.dev/finalizer"

//+kubebuilder:rbac:groups=beeai.beeai.dev,resources=agents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=beeai.beeai.dev,resources=agents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=beeai.beeai.dev,resources=agents/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Agent", "namespacedName", req.NamespacedName)

	agent := &beeaiv1.Agent{}
	err := r.Get(ctx, req.NamespacedName, agent)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Agent instance not found")
		return ctrl.Result{}, err
	}

	if !agent.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, agent, logger)
	}

	if !controllerutil.ContainsFinalizer(agent, agentFinalizer) {
		controllerutil.AddFinalizer(agent, agentFinalizer)
		if err := r.Update(ctx, agent); err != nil {
			logger.Error(err, "Unable to add finalizer to Agent")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	deploymentResult, err := r.reconcileAgentDeployment(ctx, agent, logger)
	if err != nil {
		return deploymentResult, err
	}

	serviceResult, err := r.reconcileAgentService(ctx, agent, logger)
	if err != nil {
		return serviceResult, err
	}

	return ctrl.Result{}, nil
}

func (r *AgentReconciler) reconcileAgentDeployment(ctx context.Context, agent *beeaiv1.Agent, logger logr.Logger) (ctrl.Result, error) {
	deploymentName := agent.Name
	deployment := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: agent.Namespace}, deployment)
	if err != nil && errors.IsNotFound(err) {
		deployment = r.createDeploymentForAgent(agent)
		logger.Info("Creating Deployment", "deploymentName", deploymentName)

		if err := controllerutil.SetControllerReference(agent, deployment, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference for Deployment")
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, deployment); err != nil {
			logger.Error(err, "Unable to create Deployment")
			return ctrl.Result{}, err
		}
		agent.Status.DeploymentStatus = "Creating"
		if err := r.Status().Update(ctx, agent); err != nil {
			logger.Error(err, "Unable to update Agent status after deployment creation")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
		if !agent.Status.Ready || agent.Status.DeploymentStatus != "Ready" {
			agent.Status.Ready = true
			agent.Status.DeploymentStatus = "Ready"

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
	} else {
		if agent.Status.Ready || agent.Status.DeploymentStatus != "NotReady" {
			agent.Status.Ready = false
			agent.Status.DeploymentStatus = "NotReady"

			meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             "DeploymentNotReady",
				Message:            "Deployment does not have minimum availability",
			})

			if err := r.Status().Update(ctx, agent); err != nil {
				logger.Error(err, "Failed to update Agent status for not ready deployment")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *AgentReconciler) createDeploymentForAgent(agent *beeaiv1.Agent) *appsv1.Deployment {
	labels := map[string]string{
		"app":        agent.Name,
		"controller": "agent-operator",
	}

	imageRepoSecretName := ""
	for _, envVar := range agent.Spec.Env {
		if envVar.Name == "IMAGE_REPO_SECRET" {
			imageRepoSecretName = envVar.Value
			break
		}
	}
	replicas := int32(1)

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
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "agent",
							Image:           agent.Spec.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,

							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8000,
									Name:          "http",
								},
							},

							Env:       agent.Spec.Env,
							Resources: agent.Spec.Resources,
						},
					},

					ImagePullSecrets: []corev1.LocalObjectReference{
						{
							Name: imageRepoSecretName,
						},
					},
				},
			},
		},
	}
}

func (r *AgentReconciler) reconcileAgentService(ctx context.Context, agent *beeaiv1.Agent, logger logr.Logger) (ctrl.Result, error) {
	serviceName := agent.Name
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

func (r *AgentReconciler) createServiceForAgent(agent *beeaiv1.Agent) *corev1.Service {
	labels := map[string]string{
		"app":        agent.Name,
		"controller": "agent-operator",
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       8000,
					TargetPort: intstr.FromInt(8000),
					Name:       "http",
				},
			},
		},
	}
}
func (r *AgentReconciler) handleDeletion(ctx context.Context, agent *beeaiv1.Agent, logger logr.Logger) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
		// Delete Deployment and associated Service objects
		deployment := &appsv1.Deployment{}
		deploymentName := agent.Name
		err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: agent.Namespace}, deployment)
		if err == nil {
			logger.Info("Deleting deployment for Agent", "deploymentName", deploymentName)
			if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete deployment for Agent", "deploymentName", deploymentName)
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
			logger.Info("Deleting service for Agent", "serviceName", serviceName)
			if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete service for Agent", "serviceName", serviceName)
				return ctrl.Result{}, err
			}
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to get service for deletion", "serviceName", serviceName)
			return ctrl.Result{}, err
		}
		controllerutil.RemoveFinalizer(agent, agentFinalizer)
		if err := r.Update(ctx, agent); err != nil {
			logger.Error(err, "Failed to remove finalizer from Agent")
			return ctrl.Result{}, err
		}
		logger.Info("Removed finalizer from Agent")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&beeaiv1.Agent{}).
		Named("agent").
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
