package tekton

import (
	"context"

	"github.com/go-logr/logr"
	platformv1alpha1 "github.com/kagenti/operator/platform/api/v1alpha1"
	"github.com/kagenti/operator/platform/internal/builder"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ builder.Builder = &TektonBuilder{}

type TektonBuilder struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func NewTektonBuilder(client client.Client, log logr.Logger, scheme *runtime.Scheme) *TektonBuilder {
	return &TektonBuilder{
		Client: client,
		Scheme: scheme,
		Log:    log,
	}
}
func (b *TektonBuilder) Build(ctx context.Context, component *platformv1alpha1.Component) error {

	return nil
}

func (b *TektonBuilder) Cancel(ctx context.Context, component *platformv1alpha1.Component) error {
	return nil
}

func (b *TektonBuilder) GetStatus(ctx context.Context, component *platformv1alpha1.Component) (platformv1alpha1.BuildStatus, error) {
	return platformv1alpha1.BuildStatus{}, nil
}
