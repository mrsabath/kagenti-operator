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
	"encoding/base64"
	"encoding/json"
	"fmt"

	"time"

	"github.com/go-logr/logr"
	beeaiv1 "github.com/kagenti/kagenti-operator/api/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

type AgentBuildReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const agentBuildFinalizer = "beeai.dev/finalizer"

const (
	agentBuilderPipeline = "agent-builder-pipeline"
	pullTaskName         = "git-clone"
	buildTaskName        = "build-and-push"
	pushTaskName         = "docker-push"
)

//+kubebuilder:rbac:groups=beeai.dev,resources=agentbuilds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=beeai.dev,resources=agentbuilds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=beeai.dev,resources=agentbuilds/finalizers,verbs=update
//+kubebuilder:rbac:groups=beeai.dev,resources=agents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=tekton.dev,resources=taskruns,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AgentBuild object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *AgentBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("- Reconciling AgentBuild - ", "namespacedName", req.NamespacedName)

	agentBuild := &beeaiv1.AgentBuild{}
	err := r.Get(ctx, req.NamespacedName, agentBuild)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("AgentBuild resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get AgentBuild")
		return ctrl.Result{}, err
	}

	// Add initiaial AgentBuild status when an object is first created
	if agentBuild.Status.ObservedGeneration == 0 {
		now := metav1.Now()
		agentBuild.Status.ObservedGeneration = agentBuild.Generation
		agentBuild.Status.BuildStatus = "Pending"
		agentBuild.Status.StartTime = &now

		if err := r.Status().Update(ctx, agentBuild); err != nil {
			logger.Error(err, "Failed to update initial AgentBuild status")
			return ctrl.Result{}, err
		}
	}

	if !agentBuild.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, agentBuild, logger)
	}

	if !controllerutil.ContainsFinalizer(agentBuild, agentBuildFinalizer) {
		controllerutil.AddFinalizer(agentBuild, agentBuildFinalizer)
		if err := r.Update(ctx, agentBuild); err != nil {
			logger.Error(err, "Failed to add finalizer to AgentBuild")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if agentBuild.Status.PipelineRunName == "" {
		return r.createPipelineRun(ctx, agentBuild, logger)
	}

	pipelineRun := &tektonv1.PipelineRun{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      agentBuild.Status.PipelineRunName,
		Namespace: agentBuild.Namespace,
	}, pipelineRun)

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("PipelineRun not found, creating a new one", "pipelineRunName", agentBuild.Status.PipelineRunName)
			agentBuild.Status.PipelineRunName = ""
			agentBuild.Status.BuildStatus = "Pending"
			if err := r.Status().Update(ctx, agentBuild); err != nil {
				logger.Error(err, "Failed to update AgentBuild status after PipelineRun not found")
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to get PipelineRun")
		return ctrl.Result{}, err
	}

	if pipelineRun.Status.CompletionTime != nil {
		agentBuild.Status.CompletionTime = pipelineRun.Status.CompletionTime

		if isPipelineRunSuccessful(pipelineRun) {
			if agentBuild.Status.BuildStatus != "Completed" {
				agentBuild.Status.BuildStatus = "Completed"

				imageRef := fmt.Sprintf("%s/%s:%s",
					agentBuild.Spec.ImageRegistry,
					agentBuild.Spec.Image,
					agentBuild.Spec.ImageTag)
				agentBuild.Status.BuiltImage = imageRef

				if err := r.Status().Update(ctx, agentBuild); err != nil {
					logger.Error(err, "Failed to update AgentBuild status after successful build")
					return ctrl.Result{}, err
				}
				if agentBuild.Spec.DeployAfterBuild {
					token, _, _, err := r.fetchSecretDataAndKey(ctx, "SOURCE_REPO_SECRET", agentBuild, logger)
					if err != nil {
						logger.Error(err, "Failed to fetch secret for the source repository, check if SOURCE_REPO_TOKEN environment variable is defined",
							"pipelineRunName", agentBuild.Status.PipelineRunName)
						return ctrl.Result{}, err
					}
					//
					pullRepoSecret, err := r.createSecret(ctx, agentBuild, ".dockerconfigjson", token, corev1.SecretTypeDockerConfigJson, "docker")
					if err != nil {
						logger.Error(err, "Failed to create secret for the image repository", "pipelineRunName", agentBuild.Status.PipelineRunName)
						return ctrl.Result{}, err
					}
					return r.createAgentCR(ctx, agentBuild, pullRepoSecret, logger)
				}
			} else if agentBuild.Status.BuildStatus == "Completed" && agentBuild.Spec.CleanupAfterBuild {
				logger.Info("Reconciling AgentBuild", "Cleaning Up Tekton Pods", req.NamespacedName)
				if err := r.cleanupTektonCompletedPods(ctx, agentBuild, logger); err != nil {
					logger.Error(err, "Failed to cleanup tekton pods",
						"pipelineRunName", agentBuild.Status.PipelineRunName)
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, nil
		} else {
			if agentBuild.Status.BuildStatus != "Failed" {
				agentBuild.Status.BuildStatus = "Failed"
				if err := r.Status().Update(ctx, agentBuild); err != nil {
					logger.Error(err, "Failed to update AgentBuild status after failed build")
					return ctrl.Result{}, err
				}
			}

			return ctrl.Result{}, nil
		}
	} else {
		if agentBuild.Status.BuildStatus != "Building" {
			agentBuild.Status.BuildStatus = "Building"
			if err := r.Status().Update(ctx, agentBuild); err != nil {
				logger.Error(err, "Failed to update AgentBuild status to Building")
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

func (r *AgentBuildReconciler) cleanupTektonCompletedPods(ctx context.Context, agentBuild *beeaiv1.AgentBuild, logger logr.Logger) error {
	// Get the pods associated with the PipelineRun
	pipelineRunName := agentBuild.Status.PipelineRunName

	// List the pods with the PipelineRun label
	podList := &corev1.PodList{}
	err := r.List(ctx, podList, client.MatchingLabels{"tekton.dev/pipelineRun": pipelineRunName})
	if err != nil {
		logger.Error(err, "Failed to list pods for PipelineRun", "pipelineRunName", pipelineRunName)
		return err
	}

	// Filter for completed pods (Succeeded)
	var completedPods []corev1.Pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodSucceeded { //|| pod.Status.Phase == corev1.PodFailed {
			completedPods = append(completedPods, pod)
		}
	}

	// Delete completed pods
	for _, pod := range completedPods {
		logger.Info("Deleting completed pod", "podName", pod.Name)
		err := r.Delete(ctx, &pod)
		if err != nil {
			logger.Error(err, "Failed to delete completed pod", "podName", pod.Name)
			// Continue with other pods rather than failing the cleanup
		}
	}

	return nil
}

func (r *AgentBuildReconciler) handleDeletion(ctx context.Context, agentBuild *beeaiv1.AgentBuild, logger logr.Logger) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(agentBuild, agentBuildFinalizer) {

		if agentBuild.Status.PipelineRunName != "" {
			pipelineRun := &tektonv1.PipelineRun{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      agentBuild.Status.PipelineRunName,
				Namespace: agentBuild.Namespace,
			}, pipelineRun)

			if err == nil {
				logger.Info("Deleting PipelineRun for AgentBuild", "pipelineRunName", agentBuild.Status.PipelineRunName)
				if err := r.Delete(ctx, pipelineRun); err != nil && !errors.IsNotFound(err) {
					logger.Error(err, "Failed to delete PipelineRun for AgentBuild", "pipelineRunName", agentBuild.Status.PipelineRunName)
					return ctrl.Result{}, err
				}
			} else if !errors.IsNotFound(err) {
				logger.Error(err, "Failed to get PipelineRun for deletion", "pipelineRunName", agentBuild.Status.PipelineRunName)
				return ctrl.Result{}, err
			}
		}

		if agentBuild.Status.AgentName != "" {
			logger.Info("Not deleting related Agent CR as it might be in use", "agentName", agentBuild.Status.AgentName)
		}

		controllerutil.RemoveFinalizer(agentBuild, agentBuildFinalizer)
		if err := r.Update(ctx, agentBuild); err != nil {
			logger.Error(err, "Failed to remove finalizer from AgentBuild")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully removed finalizer from AgentBuild")
	}

	return ctrl.Result{}, nil
}

func (r *AgentBuildReconciler) createPipelineRun(ctx context.Context, agentBuild *beeaiv1.AgentBuild, logger logr.Logger) (ctrl.Result, error) {
	imageReference := fmt.Sprintf("%s/%s:%s",
		agentBuild.Spec.ImageRegistry,
		agentBuild.Spec.Image,
		agentBuild.Spec.ImageTag)

	// get a token for the source repository
	token, _, _, err := r.fetchSecretDataAndKey(ctx, "SOURCE_REPO_SECRET", agentBuild, logger)
	if err != nil {
		logger.Error(err, "Failed to fetch secret for the source repository, check if SOURCE_REPO_TOKEN environment variable is defined",
			"pipelineRunName", agentBuild.Status.PipelineRunName)
		return ctrl.Result{}, err
	}
	pushRepoSecret, err := r.createSecret(ctx, agentBuild, "config.json", token, corev1.SecretTypeOpaque, "kaniko")
	if err != nil {
		logger.Error(err, "Failed to create secret for the image repository",
			"pipelineRunName", agentBuild.Status.PipelineRunName)
		return ctrl.Result{}, err
	}
	logger.Info("createPipelineRun", "created push secret", pushRepoSecret)

	pipelineRunName := fmt.Sprintf("%s-%s", agentBuild.Name, generateShortUID())
	pv, err := r.createPersistentVolume(ctx, agentBuild, pipelineRunName, "5Gi", "ReadWriteOnce", "/mnt/data")
	if err != nil {
		return ctrl.Result{}, err
	}
	pvc, err := r.createPersistentVolumeClaim(ctx, agentBuild, pv.Name, agentBuild.Namespace, "5Gi", "ReadWriteOnce", "standard")

	if err != nil {
		return ctrl.Result{}, err
	}

	// Define the subfolder we want to build from
	subFolder := ""
	if agentBuild.Spec.SourceSubfolder != "" {
		subFolder = agentBuild.Spec.SourceSubfolder
	}
	pipelineRun := &tektonv1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PipelineRun",
			APIVersion: "tekton.dev/v1",
		},
		ObjectMeta: metav1.ObjectMeta{

			Name:      pipelineRunName,
			Namespace: agentBuild.Namespace,
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineSpec: &tektonv1.PipelineSpec{
				Tasks: []tektonv1.PipelineTask{
					{
						Name: pullTaskName,
						TaskSpec: &tektonv1.EmbeddedTask{
							TaskSpec: tektonv1.TaskSpec{
								Params: []tektonv1.ParamSpec{
									{
										Name:        "GITHUB_TOKEN",
										Type:        tektonv1.ParamTypeString,
										Description: "The Git repository URL with the token",
										Default: &tektonv1.ParamValue{
											Type:      tektonv1.ParamTypeString,
											StringVal: string(token),
										},
									},
								},
								Steps: []tektonv1.Step{
									{
										Name:  "git-clone",
										Image: "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/git-init:latest",
										Command: []string{
											"/ko-app/git-init",
										},
										Args: []string{
											"-url",
											fmt.Sprintf("https://%s@%s", "$(params.GITHUB_TOKEN)", agentBuild.Spec.RepoURL),
											"-revision",
											agentBuild.Spec.Revision,

											"-path",
											"/workspace/source",
										},
									},
								},

								Workspaces: []tektonv1.WorkspaceDeclaration{
									{
										Name: "source",
									},
								},
							},
						},

						Workspaces: []tektonv1.WorkspacePipelineTaskBinding{
							{
								Name:      "source",
								Workspace: "source-workspace",
							},
						},
					},

					{
						Name:     "check-subfolder",
						RunAfter: []string{pullTaskName},
						TaskSpec: &tektonv1.EmbeddedTask{
							TaskSpec: tektonv1.TaskSpec{
								Steps: []tektonv1.Step{
									{
										Name:  "check-dir",
										Image: "alpine:latest",
										Command: []string{
											"sh", "-c",
										},
										Args: []string{
											fmt.Sprintf("if [ ! -d \"$(workspaces.source.path)/%s\" ]; then echo \"Subfolder %s not found\"; exit 1; else echo \"Subfolder %s exists\"; fi",
												subFolder, subFolder, subFolder),
										},
									},
								},
								Workspaces: []tektonv1.WorkspaceDeclaration{
									{
										Name: "source",
									},
								},
							},
						},
						Workspaces: []tektonv1.WorkspacePipelineTaskBinding{
							{
								Name:      "source",
								Workspace: "source-workspace",
							},
						},
					},

					{
						Name:     buildTaskName,
						RunAfter: []string{"check-subfolder"},
						TaskSpec: &tektonv1.EmbeddedTask{
							TaskSpec: tektonv1.TaskSpec{
								Steps: []tektonv1.Step{
									{
										Name:  "docker-build",
										Image: "gcr.io/kaniko-project/executor:v1.9.1",

										Args: []string{
											"--dockerfile=$(params.DOCKERFILE)",
											fmt.Sprintf("--context=$(workspaces.source.path)/%s", subFolder),
											"--destination=$(params.IMAGE)",
											"--skip-tls-verify=$(params.SKIP_TLS_VERIFY)",
											"--verbosity=trace",
										},
										Env: []corev1.EnvVar{

											{
												Name:  "DOCKER_CONFIG",
												Value: "/kaniko/.docker",
											},
										},

										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "ghcr-token",
												MountPath: "/kaniko/.docker",
											},
										},
									},
								},
								Params: []tektonv1.ParamSpec{
									{
										Name: "DOCKERFILE",
										Type: tektonv1.ParamTypeString,
										Default: &tektonv1.ParamValue{
											Type:      tektonv1.ParamTypeString,
											StringVal: "Dockerfile",
										},
									},
									{
										Name: "IMAGE",
										Type: tektonv1.ParamTypeString,
										Default: &tektonv1.ParamValue{
											Type:      tektonv1.ParamTypeString,
											StringVal: imageReference,
										},
									},

									{
										Name: "SKIP_TLS_VERIFY",
										Type: tektonv1.ParamTypeString,
										Default: &tektonv1.ParamValue{
											Type:      tektonv1.ParamTypeString,
											StringVal: "false",
										},
									},
								},
								Workspaces: []tektonv1.WorkspaceDeclaration{
									{
										Name: "source",
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "ghcr-token",
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: pushRepoSecret.Name,
											},
										},
									},
									{
										Name: "docker-config",
										VolumeSource: corev1.VolumeSource{
											EmptyDir: &corev1.EmptyDirVolumeSource{},
										},
									},
								},
							},
						},
						Params: []tektonv1.Param{
							{
								Name: "IMAGE",
								Value: tektonv1.ParamValue{
									Type:      tektonv1.ParamTypeString,
									StringVal: imageReference,
								},
							},
						},
						Workspaces: []tektonv1.WorkspacePipelineTaskBinding{
							{
								Name:      "source",
								Workspace: "source-workspace",
							},
						},
					},
				},
				Workspaces: []tektonv1.PipelineWorkspaceDeclaration{
					{
						Name: "source-workspace",
					},
				},
			},
			Workspaces: []tektonv1.WorkspaceBinding{
				{
					Name: "source-workspace",
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.Name,
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(agentBuild, pipelineRun, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference for PipelineRun")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, pipelineRun); err != nil {
		logger.Error(err, "Failed to create PipelineRun")
		return ctrl.Result{}, err
	}

	agentBuild.Status.PipelineRunName = pipelineRunName
	agentBuild.Status.BuildStatus = "Building"

	if err := r.Status().Update(ctx, agentBuild); err != nil {
		logger.Error(err, "Failed to update AgentBuild status with PipelineRun name")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
func (r *AgentBuildReconciler) createSecret(ctx context.Context, agentBuild *beeaiv1.AgentBuild, secretKey string, secretData []byte, secretType corev1.SecretType, suffix string) (*corev1.Secret, error) {

	secretName := agentBuild.Name + "-" + suffix
	foundPV := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: secretName, Namespace: agentBuild.Namespace}, foundPV)
	if err != nil {
		if errors.IsNotFound(err) {
			// Base64 encode the "username:password" string
			authString := base64.StdEncoding.EncodeToString([]byte(agentBuild.Name + ":" + string(secretData)))

			// Create the dockerconfig.json structure
			dockerConfig := map[string]interface{}{
				"auths": map[string]interface{}{
					"ghcr.io": map[string]interface{}{
						"username": agentBuild.Name,
						"password": secretData,
						"auth":     authString,
					},
				},
			}

			dockerConfigJSON, err := json.Marshal(dockerConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal docker config: %w", err)
			}

			secret := &corev1.Secret{

				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: agentBuild.Namespace,
				},
				Type: secretType,
				Data: map[string][]byte{
					secretKey: dockerConfigJSON,
				},
			}
			if err := ctrl.SetControllerReference(agentBuild, secret, r.Scheme); err != nil {
				return nil, fmt.Errorf("failed to set owner reference for Secret: %w", err)
			}
			err = r.Client.Create(ctx, secret)
			if err != nil {
				return nil, fmt.Errorf("failed to create Secret: %w", err)
			}

			fmt.Printf("Secret '%s' created successfully in namespace '%s'\n", agentBuild.Name, agentBuild.Namespace)
			return secret, nil
		}

	}

	return foundPV, nil
}

func (r *AgentBuildReconciler) createPersistentVolume(ctx context.Context, agentBuild *beeaiv1.AgentBuild, pvName, capacity, accessMode, hostPath string) (*corev1.PersistentVolume, error) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(capacity),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.PersistentVolumeAccessMode(accessMode),
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath,
				},
			},
		},
	}

	foundPV := &corev1.PersistentVolume{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: pvName}, foundPV)
	if err != nil {
		if errors.IsNotFound(err) {

			if err := r.Client.Create(ctx, pv); err != nil {
				return nil, fmt.Errorf("failed to create PersistentVolume: %w", err)
			}
			fmt.Printf("PersistentVolume created: %s\n", pvName)
			return pv, nil
		} else {

			return nil, fmt.Errorf("failed to get PersistentVolume: %w", err)
		}
	} else {

		fmt.Printf("PersistentVolume already exists: %s\n", pvName)
		return foundPV, nil
	}

}

func (r *AgentBuildReconciler) createPersistentVolumeClaim(ctx context.Context, agentBuild *beeaiv1.AgentBuild, pvcName, namespace, capacity, accessMode, storageClassName string) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.PersistentVolumeAccessMode(accessMode),
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(capacity),
				},
			},
		},
	}

	if storageClassName != "" {
		pvc.Spec.StorageClassName = &storageClassName
	}

	foundPVC := &corev1.PersistentVolumeClaim{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: namespace}, foundPVC)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := ctrl.SetControllerReference(agentBuild, pvc, r.Scheme); err != nil {
				return nil, fmt.Errorf("failed to set owner reference for PVC: %w", err)
			}
			if err := r.Client.Create(ctx, pvc); err != nil {
				return nil, fmt.Errorf("failed to create PersistentVolumeClaim: %w", err)
			}
			fmt.Printf("PersistentVolumeClaim created: %s in namespace %s\n", pvcName, namespace)
			return pvc, nil
		} else {
			return nil, fmt.Errorf("failed to get PersistentVolumeClaim: %w", err)
		}
	} else {
		fmt.Printf("PersistentVolumeClaim already exists: %s in namespace %s\n", pvcName, namespace)
		return foundPVC, nil
	}

}

func (r *AgentBuildReconciler) createAgentCR(ctx context.Context, agentBuild *beeaiv1.AgentBuild, pullRepoSecret *corev1.Secret, logger logr.Logger) (ctrl.Result, error) {
	if agentBuild.Status.AgentName != "" {
		agent := &beeaiv1.Agent{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      agentBuild.Status.AgentName,
			Namespace: agentBuild.Namespace,
		}, agent)

		if err == nil {
			return ctrl.Result{}, nil
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to check if Agent exists")
			return ctrl.Result{}, err
		}

		logger.Info("Agent CR not found, recreating", "agentName", agentBuild.Status.AgentName)
	}

	newEnvVar := corev1.EnvVar{
		Name:  "IMAGE_REPO_SECRET",
		Value: pullRepoSecret.Name,
	}

	updatedEnvVars := append(agentBuild.Spec.Agent.Env, newEnvVar)

	agentName := agentBuild.Spec.Agent.Name
	if agentName == "" {
		agentName = agentBuild.Name
	}

	agent := &beeaiv1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName,
			Namespace: agentBuild.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of":    "agent-operator",
				"app.kubernetes.io/managed-by": "agent-build-controller",
				"app.kubernetes.io/created-by": agentBuild.Name,
			},
		},
		Spec: beeaiv1.AgentSpec{
			Description: agentBuild.Spec.Agent.Description,
			Image:       agentBuild.Status.BuiltImage,
			Env:         updatedEnvVars,
			Resources:   agentBuild.Spec.Agent.Resources,
		},
	}

	agent.OwnerReferences = append(agent.OwnerReferences, metav1.OwnerReference{
		APIVersion: agentBuild.APIVersion,
		Kind:       agentBuild.Kind,
		Name:       agentBuild.Name,
		UID:        agentBuild.UID,
	})

	if err := r.Create(ctx, agent); err != nil {
		logger.Error(err, "Failed to create Agent CR")
		return ctrl.Result{}, err
	}

	agentBuild.Status.AgentName = agentName
	if err := r.Status().Update(ctx, agentBuild); err != nil {
		logger.Error(err, "Failed to update AgentBuild status with Agent name")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *AgentBuildReconciler) fetchSecretDataAndKey(ctx context.Context, envVarName string, agentBuild *beeaiv1.AgentBuild, logger logr.Logger) ([]byte, string, string, error) {
	for _, env := range agentBuild.Spec.Env {

		if env.Name != envVarName {
			continue
		}
		if env.ValueFrom != nil {
			if env.ValueFrom.SecretKeyRef != nil {
				secretName := env.ValueFrom.SecretKeyRef.Name
				secretKey := env.ValueFrom.SecretKeyRef.Key

				secret := &corev1.Secret{}
				err := r.Get(ctx, types.NamespacedName{
					Namespace: agentBuild.Namespace,
					Name:      secretName,
				}, secret)

				if err != nil {
					return nil, "", "", fmt.Errorf("failed to find secret %s referenced by env var %s: %w",
						secretName, env.Name, err)
				}

				if _, ok := secret.Data[secretKey]; !ok {
					return nil, "", "", fmt.Errorf("key %s not found in secret %s referenced by env var %s",
						secretKey, secretName, env.Name)
				}
				return secret.Data[secretKey], secretName, secretKey, nil
			}
		}
	}
	return nil, "", "", nil
}
func (r *AgentBuildReconciler) processEnvVarsForPipeline(ctx context.Context, agentBuild *beeaiv1.AgentBuild, logger logr.Logger) ([]corev1.EnvVar, error) {
	pipelineEnv := []corev1.EnvVar{}

	for _, env := range agentBuild.Spec.Env {

		if env.Value != "" {
			pipelineEnv = append(pipelineEnv, env)
			continue
		}

		if env.ValueFrom != nil {
			if env.ValueFrom.SecretKeyRef != nil {
				secretName := env.ValueFrom.SecretKeyRef.Name
				secretKey := env.ValueFrom.SecretKeyRef.Key

				secret := &corev1.Secret{}
				err := r.Get(ctx, types.NamespacedName{
					Namespace: agentBuild.Namespace,
					Name:      secretName,
				}, secret)

				if err != nil {
					return nil, fmt.Errorf("failed to find secret %s referenced by env var %s: %w",
						secretName, env.Name, err)
				}

				if _, ok := secret.Data[secretKey]; !ok {
					return nil, fmt.Errorf("key %s not found in secret %s referenced by env var %s",
						secretKey, secretName, env.Name)
				}

				pipelineEnv = append(pipelineEnv, env)
			} else if env.ValueFrom.ConfigMapKeyRef != nil {
				configMapName := env.ValueFrom.ConfigMapKeyRef.Name
				configMapKey := env.ValueFrom.ConfigMapKeyRef.Key

				configMap := &corev1.ConfigMap{}
				err := r.Get(ctx, types.NamespacedName{
					Namespace: agentBuild.Namespace,
					Name:      configMapName,
				}, configMap)

				if err != nil {
					return nil, fmt.Errorf("failed to find ConfigMap %s referenced by env var %s: %w",
						configMapName, env.Name, err)
				}

				if _, ok := configMap.Data[configMapKey]; !ok {
					return nil, fmt.Errorf("key %s not found in ConfigMap %s referenced by env var %s",
						configMapKey, configMapName, env.Name)
				}

				pipelineEnv = append(pipelineEnv, env)
			} else if env.ValueFrom.FieldRef != nil || env.ValueFrom.ResourceFieldRef != nil {
				logger.Info("Using field reference in pipeline, which may not work as expected in Tekton",
					"envVar", env.Name)
				pipelineEnv = append(pipelineEnv, env)
			}
		}
	}

	return pipelineEnv, nil
}

func isPipelineRunSuccessful(pipelineRun *tektonv1.PipelineRun) bool {
	for _, condition := range pipelineRun.Status.Conditions {
		if condition.Type == "Succeeded" &&
			condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func generateShortUID() string {
	now := time.Now().UnixNano()
	return fmt.Sprintf("%x", now)[:8]
}

func (r *AgentBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&beeaiv1.AgentBuild{}).
		Owns(&tektonv1.PipelineRun{}).
		Complete(r)
}
