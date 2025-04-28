#!/bin/bash

#set -euo pipefail
#set -x # echo so that users can understand what is happening


cluster_name="agent-platform" 

# Function to check if a Kind cluster exists
kind_exists() {
  if kind get clusters | grep -q "^${1}$"; then
    return 0 # Cluster exists (exit code 0)
  else
    return 1 # Cluster does not exist (exit code non-zero)
  fi
}

# Check if the Kind cluster already exists
if kind_exists "$cluster_name"; then
:   "Kind cluster '$cluster_name' already exists. Skipping creation."
else
:
: -------------------------------------------------------------------------
: "Create Kind with local registry"
: 
  curl -sL https://raw.githubusercontent.com/kagenti/kagenti-operator/refs/heads/main/scripts/kind-with-registry.sh | bash -s v0.31.0 "$cluster_name"
  if [ $? -ne 0 ]; then
:   "Error creating Kind cluster '$cluster_name'."
    exit 1
  fi
:    "Kind cluster '$cluster_name' created successfully."
  
fi

:
: -------------------------------------------------------------------------
: "Installing Tekton Pipelines"
: 

TEKTON_VERSION="v0.66.0" 
OPERATOR_NAMESPACE="default"

kubectl apply --filename "https://storage.googleapis.com/tekton-releases/pipeline/previous/${TEKTON_VERSION}/release.yaml"

:
: -------------------------------------------------------------------------
: "Installing the BeeAI Operator"
: 

helm upgrade --install kagenti-beeai-operator oci://ghcr.io/kagenti/kagenti-operator/kagenti-beeai-operator-chart --version 0.0.1

:
: -------------------------------------------------------------------------
: "Installation Complete"
: 


