# kagenti-operator Documentation

This document provides information on how to install and use the `kagenti-operator`. This is still work in progress which requires a few manual steps to run the operator. The intention is to simplify operator deployment in a near future. The operator has been tested with ghcr.io image repository only.  

## Overview

The `kagenti-operator` automates the management of BeeAI Agent resources within your Kubernetes cluster. It simplifies the process of building, deploying, and managing intelligent agents.

## Prerequisites

Before installing the `kagenti-operator`, ensure you have the following prerequisites in place:

* **Kubernetes Cluster:** A working Kubernetes Kind cluster.
* **kubectl:** The Kubernetes command-line tool.
* **Agent Source:** Your agent source code in GitHub repository including working Dockerfile.
* **GitHub Token:** Your GitHub token to allow fetching source and then to push docker image to ghcr.io repository
* **Install [ollama]:**(https://ollama.com/download)


## Installation

###   1. Install the operator
Clone this project:

```shell
git clone https://github.ibm.com/aiplatform/kagenti-operator.git
```

### 2. Create a k8s secret for GitHub repository containing your agent code

```shell
PASSWORD="your-github-token"
kubectl create secret generic github-token-secret --from-literal=token="$PASSWORD"
```

### 3. Start Ollama

In a new terminal, run:

```shell
ollama run llama3.2:1b-instruct-fp16 --keepalive 60m
```
### 4. Start Kagenti operator
In a new terminal, run:

```shell
/scripts/install.sh
```

### 5. Usage 

This section explains how to create and use the `AgentBuild` custom resource, which is the primary way to interact with the `kagenti-operator`.

###   Example `AgentBuild` Custom Resource

Here's an example YAML file for creating an `AgentBuild` custom resource:

```yaml
apiVersion: beeai.beeai.dev/v1
kind: AgentBuild
metadata:
  labels:
    app.kubernetes.io/name: kagenti-operator
    app.kubernetes.io/managed-by: kustomize
  name: agentbuild-beeai
spec:
  # Your agent source code repository
  repoUrl: "github.com/"your-github-username/test-agent.git"
  # Github user name
  repoUser: "your-github-username"
  # branch name in your source repository
  revision: "main"
  # image name to build
  image: "test-agent"
  imageTag: "v0.0.1"
  # repo url to receive the image built
  imageRegistry: "ghcr.io/your-github-username"
  # reference the secret created in step 3 above
  env:
    - name: "SOURCE_REPO_SECRET"
      valueFrom:
        secretKeyRef:
          name: "github-token-secret"
          key: "token"

  # auto deploy the agent when pipeline succeeds
  deployAfterBuild: true
  # define agent configuration parameters
  agent:
    name: "beeai-agent"
    description: "Test agent"
    env:
    # Make sure the port is set. Otherwise, BeeAI will use random port in your agent
      - name: PORT
        value: 8000
      - name: LLM_API_BASE
        value: "http://host.docker.internal:11434/v1"
      - name: LLM_API_KEY
        value: "dummy"  
      - name: LLM_MODEL
        value: "llama3.2:3b-instruct-fp16"
    resources:
      limits:
        cpu: "500m"
        memory: "1Gi"
      requests:
        cpu: "100m"
        memory: "256Mi"

```

In a new terminal, deploy the AgentBuild CR:

```
kubectl apply -f "your-agentbuild.yaml"
```

In a new terminal, watch the pods using command:

```shell
watch kubectl get pods
```

When a Tekton pipeline succeeds you should see the following pods:

```
NAME                                         READY   STATUS      RESTARTS   AGE
agentbuild-beeai-18345b6e-docker-build-pod   0/1     Completed   0          88s
agentbuild-beeai-18345b6e-docker-push-pod    0/1     Completed   0          58s
agentbuild-beeai-18345b6e-git-clone-pod      0/1     Completed   0          101s
beeai-agent-f4877984b-9prhs                  1/1     Running     0          10s
```

If your configured agent pod to expose port 8000 (env var PORT value), port-forward to the agent's k8s service as follows:

```shell
kubectl port-forward svc/"your-agent-svc-name" 8000:8000
```

Run BeeAI client:

```shell
 env BEEAI__HOST=http://localhost:8000 \
    BEEAI__MCP_SSE_PATH='/sse' \
    uv run beeai agent run "your-agent-name" 'hello'
```    

