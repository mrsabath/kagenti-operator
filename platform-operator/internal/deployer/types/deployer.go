package types

import (
	"context"

	platformv1alpha1 "github.com/kagenti/operator/platform/api/v1alpha1"
)

type ComponentDeployer interface {
	// Deploy component into the k8s cluster
	Deploy(ctx context.Context, component *platformv1alpha1.Component) error
	// Update existing component
	Update(ctx context.Context, component *platformv1alpha1.Component) error
	// Delete existing component
	Delete(ctx context.Context, component *platformv1alpha1.Component) error
	// Return status of the component
	GetStatus(ctx context.Context, component *platformv1alpha1.Component) (platformv1alpha1.ComponentDeploymentStatus, error)
}
