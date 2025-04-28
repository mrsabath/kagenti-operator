#!/bin/bash

set -euo pipefail
set -x # echo so that users can understand what is happening
set -e # exit on error

:
: -------------------------------------------------------------------------
: "Create Kind with local registry"
: 

curl -sL https://raw.githubusercontent.com/kagenti/kagenti-operator/refs/heads/main/scripts/kind-with-registry.sh | bash -s v0.31.0

:
: -------------------------------------------------------------------------
: "Install Tekton Pipelines"
: 

TEKTON_VERSION="v0.66.0" 
OPERATOR_NAMESPACE="default"

kubectl apply --filename "https://storage.googleapis.com/tekton-releases/pipeline/previous/${TEKTON_VERSION}/release.yaml"

:
: -------------------------------------------------------------------------
: "Installing the BeeAI Operator"
: 

helm upgrade --install kagenti-beeai-operator oci://ghcr.io/kagenti/kagenti-operator/kagenti-beeai-operator-chart --version 0.0.1

echo "--- Installation complete ---"
