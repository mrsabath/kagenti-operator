package helm

import (
	"context"

	"github.com/go-logr/logr"
	platformv1alpha1 "github.com/kagenti/operator/platform/api/v1alpha1"
	"github.com/kagenti/operator/platform/internal/deployer/types"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.ComponentDeployer = (*HelmDeployer)(nil)

type HelmDeployer struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func NewHelmDeployer(client client.Client, log logr.Logger, scheme *runtime.Scheme) *HelmDeployer {
	log.Info("NewHelmDeployer -------------- ")
	return &HelmDeployer{
		Client: client,
		Log:    log,
		Scheme: scheme,
	}
}
func (b *HelmDeployer) GetName() string {
	return "helm"
}
func (b *HelmDeployer) Deploy(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

// Update existing component
func (b *HelmDeployer) Update(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

// Delete existing component
func (b *HelmDeployer) Delete(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

// Return status of the component

func (b *HelmDeployer) GetStatus(ctx context.Context, component *platformv1alpha1.Component) (platformv1alpha1.ComponentDeploymentStatus, error) {

	return platformv1alpha1.ComponentDeploymentStatus{}, nil
}
func (d *HelmDeployer) CheckComponentStatus(ctx context.Context, component *platformv1alpha1.Component) (bool, string, error) {

	return false, "Not implemented", nil
}
