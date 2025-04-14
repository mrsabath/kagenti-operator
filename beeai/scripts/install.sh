#!/bin/bash

set -euo pipefail

# --- Configuration (Adjust as needed) ---
TEKTON_VERSION="v0.66.0" # Check for the latest compatible version
OPERATOR_NAMESPACE="default" # Or your desired namespace
# --- End Configuration ---

echo "--- Installing Tekton Pipelines ---"
kubectl apply --filename "https://storage.googleapis.com/tekton-releases/pipeline/previous/${TEKTON_VERSION}/release.yaml"

#echo "--- Generating Kubernetes manifests ---"
#make generate

#echo "--- Building and installing CRDs ---"
#make manifests

echo "--- Installing the operator ---"
helm upgrade --install kagenti dist/chart
#make install

#echo "--- Running the operator ---"
#make run

echo "--- Installation complete ---"
