package deployer

import (
	//"context"
	"fmt"

	"github.com/go-logr/logr"
	platformv1alpha1 "github.com/kagenti/operator/platform/api/v1alpha1"
	"github.com/kagenti/operator/platform/internal/deployer/helm"
	"github.com/kagenti/operator/platform/internal/deployer/kubernetes"
	"github.com/kagenti/operator/platform/internal/deployer/olm"
	"github.com/kagenti/operator/platform/internal/deployer/types"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DeployerFactory struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger

	KubeDeployer types.ComponentDeployer
	HelmDeployer types.ComponentDeployer
	OLMDeployer  types.ComponentDeployer
}

func NewDeployerFactory(client client.Client, log logr.Logger, scheme *runtime.Scheme) *DeployerFactory {
	factory := &DeployerFactory{
		Client: client,
		Log:    log,
		Scheme: scheme,
	}
	factory.KubeDeployer = kubernetes.NewKubernetesDeployer(client, log, scheme)
	factory.HelmDeployer = helm.NewHelmDeployer(client, log, scheme)
	factory.OLMDeployer = olm.NewOLMDeployer(client, log, scheme)

	return factory
}

func (d *DeployerFactory) GetDeployer(component *platformv1alpha1.Component) (types.ComponentDeployer, error) {
	if component == nil {
		return nil, fmt.Errorf("unable to get Deployer for undefined (nil) component")
	}
	if component.Spec.Deployer.Kubernetes != nil {

		return d.KubeDeployer, nil
	}
	if component.Spec.Deployer.Helm != nil {

		return d.HelmDeployer, nil
	}
	if component.Spec.Deployer.Olm != nil {

		return d.OLMDeployer, nil
	}
	return nil, fmt.Errorf("No valid deployer found for component %s/%s", component.Namespace, component.Name)
}

/*
// DeployComponent is a convenience method to deploy a component using the appropriate deployer
func (f *DeployerFactory) DeployComponent(ctx context.Context, component *platformv1alpha1.Component) error {
	deployer, err := f.getDeployer(component)
	if err != nil {
		return err
	}
	return deployer.Deploy(ctx, component)
}

// UpdateComponent is a convenience method to update a component using the appropriate deployer
func (f *DeployerFactory) UpdateComponent(ctx context.Context, component *platformv1alpha1.Component) error {
	deployer, err := f.getDeployer(component)
	if err != nil {
		return err
	}
	return deployer.Update(ctx, component)
} // DeleteComponent is a convenience method to delete a component using the appropriate deployer
func (f *DeployerFactory) DeleteComponent(ctx context.Context, component *platformv1alpha1.Component) error {
	deployer, err := f.getDeployer(component)
	if err != nil {
		return err
	}
	return deployer.Delete(ctx, component)
}

// GetComponentStatus is a convenience method to get the status of a component using the appropriate deployer
func (f *DeployerFactory) GetComponentStatus(ctx context.Context, component *platformv1alpha1.Component) (platformv1alpha1.ComponentDeploymentStatus, error) {
	deployer, err := f.getDeployer(component)
	if err != nil {
		return platformv1alpha1.ComponentDeploymentStatus{}, err
	}
	return deployer.GetStatus(ctx, component)
}
*/
