package olm

import (
	"context"

	"github.com/go-logr/logr"
	platformv1alpha1 "github.com/kagenti/operator/platform/api/v1alpha1"
	"github.com/kagenti/operator/platform/internal/deployer/types"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.ComponentDeployer = (*OLMDeployer)(nil)

type OLMDeployer struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func NewOLMDeployer(client client.Client, log logr.Logger, scheme *runtime.Scheme) *OLMDeployer {
	return &OLMDeployer{
		Client: client,
		Log:    log,
		Scheme: scheme,
	}
}
func (b *OLMDeployer) Deploy(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

// Update existing component
func (b *OLMDeployer) Update(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

// Delete existing component
func (b *OLMDeployer) Delete(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

// Return status of the component

func (b *OLMDeployer) GetStatus(ctx context.Context, component *platformv1alpha1.Component) (platformv1alpha1.ComponentDeploymentStatus, error) {

	return platformv1alpha1.ComponentDeploymentStatus{}, nil
}
