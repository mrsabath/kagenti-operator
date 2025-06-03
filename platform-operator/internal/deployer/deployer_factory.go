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
