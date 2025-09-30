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

	agentv1alpha1 "github.com/kagenti/operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KagentiAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.KagentiAgent{}).
		Named("kagentiagent").
		Complete(r)
}
