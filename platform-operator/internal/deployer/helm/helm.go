package helm

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	platformv1alpha1 "github.com/kagenti/operator/platform/api/v1alpha1"
	"github.com/kagenti/operator/platform/internal/deployer/types"
	helm "github.com/kubestellar/kubeflex/pkg/helm"
	//appsv1 "k8s.io/api/apps/v1"
	//	corev1 "k8s.io/api/core/v1"
	//	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	//	k8stypes "k8s.io/apimachinery/pkg/types"
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

	reqLogger := b.Log
	parameters := []string{}

	for _, parameter := range component.Spec.Deployer.Helm.Parameters {
		parameters = append(parameters, fmt.Sprintf("%s=%s", parameter.Name, parameter.Value))
	}

	reqLogger.Info("Deploying ", "Infra Component", component.Name)
	h := &helm.HelmHandler{
		URL:         component.Spec.Deployer.Helm.ChartRepoUrl,
		Version:     component.Spec.Deployer.Helm.ChartVersion,
		RepoName:    component.Spec.Deployer.Helm.ChartRepoName,
		ChartName:   component.Spec.Deployer.Helm.ChartName,
		Namespace:   component.Namespace,
		ReleaseName: component.Name,
		Args:        map[string]string{"set": strings.Join(parameters, ",")},
	}
	if err := helm.Init(ctx, h); err != nil {
		reqLogger.Error(err, "Failed to call Helm.Init()")
		return err
	}

	if !h.IsDeployed() {
		if err := h.Install(); err != nil {
			reqLogger.Error(err, "Failed to call Helm.Install()")
			return err
		}
	} else {
		reqLogger.Info("Helm Chart Already Installed", "Component", component.Name, "Namespace", component.Namespace)
	}
	reqLogger.Info("Helm Chart Installed")
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

	d.Log.WithValues("component", component.Name, "namespace", component.Namespace)
	reqLogger := d.Log
	parameters := []string{}

	for _, parameter := range component.Spec.Deployer.Helm.Parameters {
		parameters = append(parameters, fmt.Sprintf("%s=%s", parameter.Name, parameter.Value))
	}

	reqLogger.Info("Deploying ", "Infra Component", component.Name)
	h := &helm.HelmHandler{
		URL:         component.Spec.Deployer.Helm.ChartRepoUrl,
		Version:     component.Spec.Deployer.Helm.ChartVersion,
		RepoName:    component.Spec.Deployer.Helm.ChartRepoName,
		ChartName:   component.Spec.Deployer.Helm.ChartName,
		Namespace:   component.Spec.Deployer.Namespace,
		ReleaseName: component.Name,
		Args:        map[string]string{"set": strings.Join(parameters, ",")},
	}
	if err := helm.Init(ctx, h); err != nil {
		reqLogger.Error(err, "Failed to call Helm.Init()")
		return false, "Not ready", err
	}
	release, _ := h.CheckStatus()
	reqLogger.Info("Helm >>>>>> ", "Release status", release.Info.Status.String())

	if h.IsDeployed() {
		return true, "Component is ready", nil
	}

	return false, "Not ready", nil
}
