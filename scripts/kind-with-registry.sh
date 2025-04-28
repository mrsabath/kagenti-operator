#!/bin/sh
set -o errexit

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)

:
: -------------------------------------------------------------------------
: "Create kind cluster with containerd registry set to use insecure"
:
cat <<EOF | kind create cluster --name agent-platform --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
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

namespace="cr-system"
deployment_name="registry"

desired_replicas=$(kubectl -n "$namespace" get deployment/"$deployment_name" -o jsonpath='{.spec.replicas}')

echo "Waiting for deployment '$deployment_name' in namespace '$namespace' to become ready (desired: $desired_replicas)..."

while true; do
  ready_replicas=$(kubectl -n "$namespace" get deployment/"$deployment_name" -o jsonpath='{.status.readyReplicas}')
  echo "Ready replicas: $ready_replicas"
  if [ "$ready_replicas" -eq "$desired_replicas" ]; then
    echo "Deployment '$deployment_name' is fully rolled out."
    break
  fi
  sleep 5 # Wait for 5 seconds before checking again
done

#kubectl wait --for=condition=Available=true --timeout=120s -n cr-system deployment/registry
#kubectl -n cr-system rollout status deployment/registry --watch=false


:
: -------------------------------------------------------------------------
: "Apply workaround to resolve registry DNS from the Kind kubelet"
:
REGISTRY_IP=$(kubectl get service -n cr-system registry -o jsonpath='{.spec.clusterIP}')
docker exec -it agent-platform-control-plane sh -c "echo ${REGISTRY_IP} registry.cr-system.svc.cluster.local >> /etc/hosts"
