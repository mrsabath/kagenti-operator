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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type WorkspaceManager struct {
	client client.Client
	scheme *runtime.Scheme
	Log    logr.Logger
}

func NewWorkspaceManager(c client.Client, scheme *runtime.Scheme, log logr.Logger) *WorkspaceManager {
	return &WorkspaceManager{
		client: c,
		scheme: scheme,
		Log:    log,
	}
}
func (wm *WorkspaceManager) CreateWorkspacePVC(ctx context.Context, agentBuild *agentv1alpha1.AgentBuild) error {
	wm.Log.Info("CreateWorkspacePVC", "name", agentBuild.Name, "namespace", agentBuild.Namespace)
	pvcName := fmt.Sprintf("%s-workspace", agentBuild.Name)
	pvc := &corev1.PersistentVolumeClaim{}
	err := wm.client.Get(ctx, types.NamespacedName{
		Name:      pvcName,
		Namespace: agentBuild.Namespace,
	}, pvc)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check if workspace PVC exists: %w", err)
	}
	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: agentBuild.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of":   "kagenti-operator",
				"app.kubernetes.io/component": agentBuild.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(agentBuild, pvc, wm.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference on workspace PVC: %w", err)
	}
	err = wm.client.Create(ctx, pvc)
	if err != nil {
		return fmt.Errorf("failed to create workspace PVC: %w", err)
	}
	return nil
}
func (wm *WorkspaceManager) GetWorkspaceBindings(agentBuild *agentv1alpha1.AgentBuild) []tektonv1.WorkspaceBinding {
	return []tektonv1.WorkspaceBinding{
		{
			Name: "shared-workspace",
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: fmt.Sprintf("%s-workspace", agentBuild.Name),
			},
		},
	}
}
func (wm *WorkspaceManager) ListComponentWorkspaces(ctx context.Context, namespace string) (*corev1.PersistentVolumeClaimList, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}
	err := wm.client.List(ctx, pvcList,
		client.InNamespace(namespace),
		client.MatchingLabels{"app.kubernetes.io/part-of": "kagenti-operator"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list agentBuild workspaces: %w", err)
	}
	return pvcList, nil
}
func (wm *WorkspaceManager) CleanupUnusedWorkspaces(ctx context.Context, namespace string) error {
	agentBuilds := &agentv1alpha1.AgentBuildList{}
	err := wm.client.List(ctx, agentBuilds, client.InNamespace(namespace))
	if err != nil {
		return fmt.Errorf("failed to list agentbuild: %w", err)

	}
	activeAgentBuilds := make(map[string]bool)
	for _, agentBuild := range agentBuilds.Items {
		activeAgentBuilds[agentBuild.Name] = true
	}
	workspaces, err := wm.ListComponentWorkspaces(ctx, namespace)
	if err != nil {
		return err
	}
	for _, pvc := range workspaces.Items {
		agentBuildName, ok := pvc.Labels["app.kubernetes.io/component"]
		if !ok {
			continue
		}
		if !activeAgentBuilds[agentBuildName] {
			err := wm.client.Delete(ctx, &pvc)
			if err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete unused workspace: %w", err)
			}
		}
	}
	return nil
}
