package kubernetes

import (
	"context"

	"github.com/go-logr/logr"
	platformv1alpha1 "github.com/kagenti/operator/platform/api/v1alpha1"
	"github.com/kagenti/operator/platform/internal/deployer/types"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.ComponentDeployer = (*KubernetesDeployer)(nil)

type KubernetesDeployer struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func NewKubernetesDeployer(client client.Client, log logr.Logger, scheme *runtime.Scheme) *KubernetesDeployer {
	return &KubernetesDeployer{
		Client: client,
		Log:    log,
		Scheme: scheme,
	}
}
func (b *KubernetesDeployer) Deploy(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

// Update existing component
func (b *KubernetesDeployer) Update(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

// Delete existing component
func (b *KubernetesDeployer) Delete(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

// Return status of the component

func (b *KubernetesDeployer) GetStatus(ctx context.Context, component *platformv1alpha1.Component) (platformv1alpha1.ComponentDeploymentStatus, error) {

	return platformv1alpha1.ComponentDeploymentStatus{}, nil
}
