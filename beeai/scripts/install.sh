#!/bin/bash

#set -euo pipefail
set -x # echo so that users can understand what is happening

TEKTON_VERSION="v0.66.0" 
OPERATOR_NAMESPACE="kagenti-system"

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
: "Installing the BeeAI Operator"
: 
LATEST_TAG=$(curl -s "https://api.github.com/repos/kagenti/kagenti-operator/tags" | \
  jq -r '.[].name' | \
  grep '^v' | \
  sort -V | \
  tail -n 1 |
  sed 's/^v//')
helm upgrade --install kagenti-beeai-operator --create-namespace --namespace ${OPERATOR_NAMESPACE} oci://ghcr.io/kagenti/kagenti-operator/kagenti-beeai-operator-chart --version ${LATEST_TAG}

:
: -------------------------------------------------------------------------
: "Installation Complete"
: 


