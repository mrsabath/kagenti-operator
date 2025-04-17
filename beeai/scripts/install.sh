#!/bin/bash

set -euo pipefail

TEKTON_VERSION="v0.66.0" 
OPERATOR_NAMESPACE="default"

echo "--- Installing Tekton Pipelines ---"
kubectl apply --filename "https://storage.googleapis.com/tekton-releases/pipeline/previous/${TEKTON_VERSION}/release.yaml"

echo "--- Installing the beeai operator ---"
helm upgrade --install kagenti-beeai-operator oci://ghcr.io/kagenti/kagenti-operator/kagenti-beeai-operator-chart --version 0.0.1
# helm upgrade --install kagenti dist/chart

echo "--- Installation complete ---"
