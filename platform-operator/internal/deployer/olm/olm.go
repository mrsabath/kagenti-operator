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
	log.Info("NewOLMDeployer -------------- ")
	return &OLMDeployer{
		Client: client,
		Log:    log,
		Scheme: scheme,
	}
}
func (b *OLMDeployer) GetName() string {
	return "olm"
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

func (d *OLMDeployer) CheckComponentStatus(ctx context.Context, component *platformv1alpha1.Component) (bool, string, error) {

	return false, "Not implemented", nil
}
