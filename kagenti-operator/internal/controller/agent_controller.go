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
	"fmt"
	"time"

	agentv1alpha1 "github.com/kagenti/operator/api/v1alpha1"
	"github.com/kagenti/operator/internal/distribution"
	rbac "github.com/kagenti/operator/internal/rbac"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	EnableClientRegistration bool
	Distribution             distribution.Type
}

var (
	logger = ctrl.Log.WithName("controller").WithName("Agent")
)

const (
	CLIENT_REGISTRATION_NAME = "kagenti-client-registration"
	SPIFFY_HELPER_NAME       = "spiffe-helper"
	AGENT_FINALIZER          = "agent.kagenti.dev/finalizer"
)

// +kubebuilder:rbac:groups=agent.kagenti.dev,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.kagenti.dev,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.kagenti.dev,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/log,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger.Info("Reconciling Agent", "namespacedName", req.NamespacedName)

	agent := &agentv1alpha1.Agent{}
	err := r.Get(ctx, req.NamespacedName, agent)
	if err != nil {
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
	// if image is not set in the spec, try to fetch it from the AgentBuild object
	image := agent.Spec.ImageSource.Image
	if image == nil {
		// Get the image for the main container
		image, err := r.getContainerImage(ctx, agent)
		if err != nil || image == "" {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	deploymentResult, err := r.reconcileAgentDeployment(ctx, agent)
	if err != nil {
		return deploymentResult, err
	}

	serviceResult, err := r.reconcileAgentService(ctx, agent)
	if err != nil {
		return serviceResult, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *AgentReconciler) reconcileAgentDeployment(ctx context.Context, agent *agentv1alpha1.Agent) (ctrl.Result, error) {
	deploymentName := agent.Name
	deployment := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: agent.Namespace}, deployment)
	if err != nil && errors.IsNotFound(err) {
		deployment, err = r.createDeploymentForAgent(ctx, agent)
		if err != nil {
			logger.Error(err, "Unable to create deployment spec for Agent")
			return ctrl.Result{}, err
		}
		logger.Info("Creating Agent Deployment", "deploymentName", deploymentName)
		if agent.Annotations != nil {
			deployment.ObjectMeta.Annotations = agent.Annotations
		}
		data, err := json.MarshalIndent(deployment, "", "  ")
		if err != nil {
			logger.Error(err, "Unable to marshal deployment spec to JSON")
			return ctrl.Result{}, err
		}
		logger.Info("Deployment spec: " + string(data))

		if err := controllerutil.SetControllerReference(agent, deployment, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference for Agent Deployment")
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, deployment); err != nil {
			logger.Error(err, "Unable to create Agent Deployment")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Agent Deployment")
		return ctrl.Result{}, err
	}

	logger.Info("Deployment status",
		"name", deploymentName,
		"namespace", agent.Namespace,
		"desiredReplicas", deployment.Spec.Replicas,
		"statusReplicas", deployment.Status.Replicas,
		"readyReplicas", deployment.Status.ReadyReplicas,
		"availableReplicas", deployment.Status.AvailableReplicas,
		"updatedReplicas", deployment.Status.UpdatedReplicas,
		"unavailableReplicas", deployment.Status.UnavailableReplicas,
		"conditions", deployment.Status.Conditions)

	if agent.Status.DeploymentStatus == nil {
		agent.Status.DeploymentStatus = &agentv1alpha1.DeploymentStatus{}
	}

	agent.Status.DeploymentStatus.DeploymentMessage = fmt.Sprintf(
		"Replicas: %d/%d ready, %d updated, %d available",
		deployment.Status.ReadyReplicas,
		deployment.Status.Replicas,
		deployment.Status.UpdatedReplicas,
		deployment.Status.AvailableReplicas,
	)

	meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
		Type:               "DeploymentAvailable",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "DeploymentExists",
		Message:            fmt.Sprintf("Deployment %s exists with %d desired replicas", deployment.Name, *deployment.Spec.Replicas),
	})

	// Track pod scheduling and availability
	if deployment.Status.UnavailableReplicas > 0 {
		meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               "PodsScheduled",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "PodsUnscheduled",
			Message:            fmt.Sprintf("%d of %d pods are unavailable", deployment.Status.UnavailableReplicas, deployment.Status.Replicas),
		})
	} else if deployment.Status.Replicas > 0 {
		meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               "PodsScheduled",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "AllPodsScheduled",
			Message:            fmt.Sprintf("All %d pods are scheduled and available", deployment.Status.AvailableReplicas),
		})
	}

	// Update Ready condition and Phase based on deployment readiness
	if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
		agent.Status.DeploymentStatus.Phase = agentv1alpha1.PhaseReady

		meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "DeploymentReady",
			Message:            fmt.Sprintf("All %d/%d replicas are ready", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas),
		})
	} else {
		agent.Status.DeploymentStatus.Phase = agentv1alpha1.PhaseDeploying

		meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "DeploymentNotReady",
			Message:            fmt.Sprintf("Waiting for replicas: %d/%d ready, %d available", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas, deployment.Status.AvailableReplicas),
		})
	}

	// retry on conflict to avoid error: "the object has been modified; please apply your changes to the latest version"
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {

		latestAgent := &agentv1alpha1.Agent{}
		if err := r.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, latestAgent); err != nil {
			return err
		}

		if latestAgent.Status.DeploymentStatus == nil {
			latestAgent.Status.DeploymentStatus = &agentv1alpha1.DeploymentStatus{}
		}

		latestAgent.Status.DeploymentStatus.DeploymentMessage = agent.Status.DeploymentStatus.DeploymentMessage
		latestAgent.Status.DeploymentStatus.Phase = agent.Status.DeploymentStatus.Phase

		for _, condition := range agent.Status.Conditions {
			meta.SetStatusCondition(&latestAgent.Status.Conditions, condition)
		}

		return r.Status().Update(ctx, latestAgent)
	}); err != nil {
		logger.Error(err, "Failed to update Agent status after retries")
		return ctrl.Result{}, err
	}

	// Continue reconciling if not all replicas are ready
	if deployment.Status.ReadyReplicas < *deployment.Spec.Replicas {
		logger.Info("Requeuing: not all replicas ready",
			"ready", deployment.Status.ReadyReplicas,
			"desired", *deployment.Spec.Replicas)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	logger.Info("Deployment is ready", "replicas", deployment.Status.ReadyReplicas)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *AgentReconciler) fetchImageFromAgentBuild(ctx context.Context, agent *agentv1alpha1.Agent, agentBuildRef string) (string, error) {
	logger.Info("Fetching AgentBuild", "Name", agentBuildRef)
	agentBuild := &agentv1alpha1.AgentBuild{}
	err := r.Get(ctx, types.NamespacedName{Name: agentBuildRef, Namespace: agent.Namespace}, agentBuild)
	if err != nil {
		return "", err
	}
	if agentBuild.Status.Phase != agentv1alpha1.BuildPhaseSucceeded {
		return "", nil
	}
	if agentBuild.Status.BuiltImage == "" {
		return "", nil
	}
	return agentBuild.Status.BuiltImage, nil
}

func (r *AgentReconciler) getContainerImage(ctx context.Context, agent *agentv1alpha1.Agent) (string, error) {

	if agent.Spec.ImageSource.BuildRef != nil {
		image, err := r.fetchImageFromAgentBuild(ctx, agent, agent.Spec.ImageSource.BuildRef.Name)
		if err != nil {
			logger.Error(err, "Unable to fetch image from AgentBuild", "buildRef", agent.Spec.ImageSource.BuildRef.Name)
			return "", err
		}
		return image, nil
	} else if agent.Spec.ImageSource.Image != nil && *agent.Spec.ImageSource.Image != "" {
		return *agent.Spec.ImageSource.Image, nil
	}
	return "", nil
}

func (r *AgentReconciler) createDeploymentForAgent(ctx context.Context, agent *agentv1alpha1.Agent) (*appsv1.Deployment, error) {
	if len(agent.Spec.PodTemplateSpec.Spec.Containers) == 0 {
		return nil, fmt.Errorf("no containers defined in PodTemplateSpec")
	}
	replicas := int32(1)
	if agent.Spec.Replicas != nil {
		replicas = int32(*agent.Spec.Replicas)
	}
	podTemplateSpec := agent.Spec.PodTemplateSpec.DeepCopy()

	podTemplateSpec.ObjectMeta.ResourceVersion = ""
	podTemplateSpec.ObjectMeta.UID = ""

	rbacConfig := rbac.GetComponentRBACConfig(agent.Namespace, agent.Name, agent.Labels)
	rbacManager := rbac.NewRBACManager(r.Client, r.Scheme)
	if err := rbacManager.CreateRBACObjects(ctx, rbacConfig, agent); err != nil {
		logger.Error(err, "failed to create RBAC objects")
		return nil, fmt.Errorf("failed to create RBAC objects: %w", err)
	}
	labels := map[string]string{
		"app.kubernetes.io/name": agent.Name,
	}
	for k, v := range agent.Labels {
		if _, exists := labels[k]; !exists {
			labels[k] = v
		}
	}
	clientId := agent.Namespace + "/" + agent.Name
	mainEnvs := podTemplateSpec.Spec.Containers[0].Env
	mainEnvs = append(mainEnvs, []corev1.EnvVar{
		{
			Name:  "CLIENT_NAME",
			Value: clientId,
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

	for inx := range podTemplateSpec.Spec.Containers {
		if podTemplateSpec.Spec.Containers[inx].Env != nil {
			podTemplateSpec.Spec.Containers[inx].Env = append(podTemplateSpec.Spec.Containers[inx].Env, mainEnvs...)
		} else {
			podTemplateSpec.Spec.Containers[inx].Env = mainEnvs
		}
		// if the agent is deployed from an existing container image (not built from source) use the
		// image from 'agent.Spec.ImageSource.Image'
		if inx == 0 && agent.Spec.ImageSource.Image != nil {
			podTemplateSpec.Spec.Containers[inx].Image = *agent.Spec.ImageSource.Image
		}

		containerPorts := []corev1.ContainerPort{}
		if podTemplateSpec.Spec.Containers[inx].Ports == nil {
			containerPorts = append(containerPorts, corev1.ContainerPort{
				Name:          "http",
				ContainerPort: 8000,
				Protocol:      corev1.ProtocolTCP,
			})
		} else {
			for _, port := range podTemplateSpec.Spec.Containers[inx].Ports {
				containerPorts = append(containerPorts, corev1.ContainerPort{
					Name:          port.Name,
					ContainerPort: port.ContainerPort,
					Protocol:      port.Protocol,
				})
			}
		}
		podTemplateSpec.Spec.Containers[inx].Ports = containerPorts
	}

	// Get the image for the main container
	image, err := r.getContainerImage(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("no valid image found for Agent %s", agent.Name)
	}
	if agent.Spec.ImageSource.BuildRef != nil {
		logger.Info("Using image from AgentBuild", "buildRef", agent.Spec.ImageSource.BuildRef.Name, "image", image)

		mainContainerFound := false
		for inx := range podTemplateSpec.Spec.Containers {
			if podTemplateSpec.Spec.Containers[inx].Name == "agent" {
				podTemplateSpec.Spec.Containers[inx].Image = image
				mainContainerFound = true
				break
			}
		}
		if !mainContainerFound && len(podTemplateSpec.Spec.Containers) > 0 {
			logger.Info("No container named 'agent' found, using first container")
			podTemplateSpec.Spec.Containers[0].Image = image
		}
	} else {
		logger.Info("Using static image for Agent", "image", image)
	}

	logger.Info("Agent", "EnableClientRegistration", r.EnableClientRegistration)
	if r.EnableClientRegistration {
		if exists := r.containerExists(podTemplateSpec, CLIENT_REGISTRATION_NAME); !exists {
			logger.Info("Adding client registration container")
			err := r.addClientRegistrationContainer(podTemplateSpec, agent)
			if err != nil {
				logger.Error(err, "Unable to add keycloak client registration sidecar container")
				return nil, err
			}
		}
		if exists := r.containerExists(podTemplateSpec, SPIFFY_HELPER_NAME); !exists {
			err := r.addSpiffyHelperContainer(podTemplateSpec)
			if err != nil {
				logger.Error(err, "Unable to add spiffy-helper sidecar container")
				return nil, err
			}
		}
	}
	r.addVolumesAndMounts(podTemplateSpec)
	// Set the ServiceAccountName for the pod
	if podTemplateSpec.Spec.ServiceAccountName == "" {
		podTemplateSpec.Spec.ServiceAccountName = rbacConfig.ServiceAccountName
	}

	// Set security context for the pod (if not already specified)
	if podTemplateSpec.Spec.SecurityContext == nil {
		podSecCtx := &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(true),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}

		// On OpenShift, omit these to allow SCC admission controller to inject appropriate values
		if r.Distribution != distribution.OpenShift {
			podSecCtx.RunAsUser = ptr.To(int64(1000))
			podSecCtx.FSGroup = ptr.To(int64(1000))
		}

		podTemplateSpec.Spec.SecurityContext = podSecCtx
	}

	// Set security context for each container (only if not already specified)
	for inx := range podTemplateSpec.Spec.Containers {
		if podTemplateSpec.Spec.Containers[inx].SecurityContext == nil {
			podTemplateSpec.Spec.Containers[inx].SecurityContext = &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				Privileged:               ptr.To(false),
				ReadOnlyRootFilesystem:   ptr.To(true),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
			}
		}
	}

	if podTemplateSpec.ObjectMeta.Labels == nil {
		podTemplateSpec.ObjectMeta.Labels = make(map[string]string)
	}
	for k, v := range labels {
		podTemplateSpec.ObjectMeta.Labels[k] = v
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        agent.Name,
			Namespace:   agent.Namespace,
			Labels:      labels,
			Annotations: agent.Annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: *podTemplateSpec,
		},
	}, nil
}

func (r *AgentReconciler) containerExists(podTemplateSpec *corev1.PodTemplateSpec, containerName string) bool {
	for _, container := range podTemplateSpec.Spec.Containers {
		if container.Name == containerName {
			return true
		}
	}

	return false
}

func (r *AgentReconciler) volumeExists(podTemplateSpec *corev1.PodTemplateSpec, volumeName string) bool {

	for _, vol := range podTemplateSpec.Spec.Volumes {
		if vol.Name == volumeName {
			return true
		}
	}
	return false
}

func (r *AgentReconciler) addClientRegistrationContainer(podTemplateSpec *corev1.PodTemplateSpec, agent *agentv1alpha1.Agent) error {

	containers := podTemplateSpec.Spec.Containers
	if len(containers) == 0 {
		return fmt.Errorf("no containers found in Agent spec")
	}

	imagePullPolicy := "IfNotPresent"
	resources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
	}
	clientId := agent.Namespace + "/" + agent.Name
	containers = append(containers, corev1.Container{
		Name:            CLIENT_REGISTRATION_NAME,
		Image:           "ghcr.io/kagenti/kagenti/client-registration:latest",
		ImagePullPolicy: corev1.PullPolicy(imagePullPolicy),
		Resources:       resources,
		// Wait until /opt/jwt_svid.token appears, then exec
		Command: []string{
			"/bin/sh",
			"-c",
			"while [ ! -f /opt/jwt_svid.token ]; do echo waiting for SVID; sleep 1; done; python client_registration.py; tail -f /dev/null",
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
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				// This is how client registration accesses the SVID
				Name:      "svid-output",
				MountPath: "/opt",
			},
			{
				// This is how client registration accesses the SVID
				Name:      "shared-data",
				MountPath: "/shared",
			},
		},
	})
	podTemplateSpec.Spec.Containers = containers
	return nil
}
func (r *AgentReconciler) addSpiffyHelperContainer(podTemplateSpec *corev1.PodTemplateSpec) error {

	containers := podTemplateSpec.Spec.Containers
	if len(containers) == 0 {
		return fmt.Errorf("no containers found in Agent spec")
	}

	imagePullPolicy := "IfNotPresent"
	resources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
	}

	containers = append(containers, corev1.Container{
		Name:            SPIFFY_HELPER_NAME,
		Image:           "ghcr.io/spiffe/spiffe-helper:nightly",
		ImagePullPolicy: corev1.PullPolicy(imagePullPolicy),
		Resources:       resources,
		// Wait until /opt/jwt_svid.token appears, then exec
		Command: []string{
			"/spiffe-helper",
			"-config=/etc/spiffe-helper/helper.conf",
			"run",
		},

		VolumeMounts: []corev1.VolumeMount{
			{
				// This is how client registration accesses the SVID
				Name:      "spiffe-helper-config",
				MountPath: "/etc/spiffe-helper",
			},
			{
				// This is how client registration accesses the SVID
				Name:      "spire-agent-socket",
				MountPath: "/spiffe-workload-api",
			},
			{
				// This is how client registration accesses the SVID
				Name:      "svid-output",
				MountPath: "/opt",
			},
			{
				// This is how client registration accesses the SVID
				Name:      "shared-data",
				MountPath: "/shared",
			},
		},
	})
	podTemplateSpec.Spec.Containers = containers
	return nil
}
func (r *AgentReconciler) addVolumesAndMounts(podTemplateSpec *corev1.PodTemplateSpec) {
	if !hasVolumeMounts(&podTemplateSpec.Spec, "cache") {
		if len(podTemplateSpec.Spec.Containers) > 0 {
			podTemplateSpec.Spec.Containers[0].VolumeMounts =
				append(podTemplateSpec.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
					Name:      "cache",
					MountPath: "/app/.cache",
				})
		}
	}

	if exists := r.volumeExists(podTemplateSpec, "cache"); !exists {
		podTemplateSpec.Spec.Volumes = append(podTemplateSpec.Spec.Volumes, corev1.Volume{
			Name: "cache",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}
	if exists := r.volumeExists(podTemplateSpec, "shared-data"); !exists {
		podTemplateSpec.Spec.Volumes =
			append(podTemplateSpec.Spec.Volumes, corev1.Volume{
				Name: "shared-data",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})
	}

	if exists := r.volumeExists(podTemplateSpec, "spire-agent-socket"); !exists {
		podTemplateSpec.Spec.Volumes =
			append(podTemplateSpec.Spec.Volumes, corev1.Volume{
				Name: "spire-agent-socket",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/run/spire/agent-sockets",
					},
				},
			})
	}

	if exists := r.volumeExists(podTemplateSpec, "spiffe-helper-config"); !exists {
		podTemplateSpec.Spec.Volumes =
			append(podTemplateSpec.Spec.Volumes, corev1.Volume{
				Name: "spiffe-helper-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "spiffe-helper-config",
						},
					},
				},
			})
	}

	if exists := r.volumeExists(podTemplateSpec, "svid-output"); !exists {
		podTemplateSpec.Spec.Volumes =
			append(podTemplateSpec.Spec.Volumes, corev1.Volume{
				Name: "svid-output",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})
	}
}

func hasVolumeMounts(podSpec *corev1.PodSpec, volumeMountName string) bool {
	for _, container := range podSpec.Containers {
		for _, vm := range container.VolumeMounts {
			if vm.Name == volumeMountName {
				return true
			}
		}
	}
	return false
}

func (r *AgentReconciler) reconcileAgentService(ctx context.Context, agent *agentv1alpha1.Agent) (ctrl.Result, error) {
	serviceName := agent.Name + "-svc"
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

func (r *AgentReconciler) createServiceForAgent(agent *agentv1alpha1.Agent) *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name": agent.Name,
	}
	servicePorts := []corev1.ServicePort{}
	if agent.Spec.ServicePorts == nil {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       "http",
			Protocol:   corev1.ProtocolTCP,
			Port:       8000,
			TargetPort: intstr.FromInt(8000),
		})
	} else {
		servicePorts = agent.Spec.ServicePorts
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        agent.Name + "-svc",
			Namespace:   agent.Namespace,
			Labels:      labels,
			Annotations: agent.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    servicePorts,
		},
	}
}

func (r *AgentReconciler) handleDeletion(ctx context.Context, agent *agentv1alpha1.Agent) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(agent, AGENT_FINALIZER) {
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
		serviceName := agent.Name + "-svc"
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
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			latestAgent := &agentv1alpha1.Agent{}
			if err := r.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, latestAgent); err != nil {
				return err
			}

			controllerutil.RemoveFinalizer(latestAgent, AGENT_FINALIZER)
			return r.Update(ctx, latestAgent)
		}); err != nil {
			logger.Error(err, "Failed to remove finalizer from Agent after retries")
			return ctrl.Result{}, err
		}
		logger.Info("Removed finalizer from Agent")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("Agent").
		Complete(r)
}
