#!/bin/bash

#set -euo pipefail
set -x # echo so that users can understand what is happening

TEKTON_VERSION="v0.66.0"
OPERATOR_NAMESPACE="kagenti-system"
LATEST_TAG=0.2.0-alpha.3

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
: "Create kind cluster with containerd registry set to use insecure"
:
cat <<EOF | kind create cluster --name agent-platform --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8080
containerdConfigPatches:
  - |
    [plugins."io.containerd.grpc.v1.cri".registry]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
        [plugins."io.containerd.grpc.v1.cri".registry.mirrors."registry.cr-system.svc.cluster.local:5000"]
          endpoint = ["http://registry.cr-system.svc.cluster.local:5000"]
      [plugins."io.containerd.grpc.v1.cri".registry.configs."registry.cr-system.svc.cluster.local:5000".tls]
        insecure_skip_verify = true
EOF

:
: -------------------------------------------------------------------------
: "Deploy a container registry"
:
kubectl apply -f https://raw.githubusercontent.com/kagenti/kagenti-operator/refs/heads/main/scripts/kind-with-registry.yaml

:
: -------------------------------------------------------------------------
: "Wait to be ready"
:

    kubectl -n cr-system rollout status deployment/registry

:
: -------------------------------------------------------------------------
: "Apply workaround to resolve registry DNS from the Kind kubelet"
:
REGISTRY_IP=$(kubectl get service -n cr-system registry -o jsonpath='{.spec.clusterIP}')
# docker exec -it agent-platform-control-plane sh -c "echo ${REGISTRY_IP} registry.cr-system.svc.cluster.local >> /etc/hosts"
docker exec agent-platform-control-plane sh -c "echo ${REGISTRY_IP} registry.cr-system.svc.cluster.local >> /etc/hosts"


:    "Kind cluster '$cluster_name' created successfully."

fi

:
: -------------------------------------------------------------------------
: "Installing Tekton Pipelines"
:
kubectl apply --filename "https://storage.googleapis.com/tekton-releases/pipeline/previous/${TEKTON_VERSION}/release.yaml"

:
: -------------------------------------------------------------------------
: "Installing Cert Manager"
:
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml

# Define a timeout for the wait operations (e.g., 5 minutes = 300 seconds)
WAIT_TIMEOUT_SECONDS=300

# Function to wait for a specific deployment
wait_for_deployment() {
    local deployment_name=$1
    echo "  Waiting for deployment/${deployment_name} in namespace cert-manager..."
    kubectl wait --for=condition=Available deployment/"${deployment_name}" -n cert-manager --timeout="${WAIT_TIMEOUT_SECONDS}s"
    if [ $? -ne 0 ]; then
        echo "Error: Deployment ${deployment_name} did not become ready within ${WAIT_TIMEOUT_SECONDS}s. Check 'kubectl describe deployment ${deployment_name} -n cert-manager' for details."
        exit 1
    fi
    echo "  Deployment ${deployment_name} is ready."
}

# Wait for each cert-manager component
wait_for_deployment cert-manager
wait_for_deployment cert-manager-cainjector
wait_for_deployment cert-manager-webhook

: "All cert-manager components are ready. Continuing with operator installation..."
