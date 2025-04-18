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
* **Clone [beeai examples]:** (https://github.com/i-am-bee/beeai-platform.git)


## Installation

### 1. Start Ollama

In a new terminal, run:

```shell
ollama run "your-llm-model" --keepalive 60m
```
### 2. Start Kagenti operator
In a new terminal, run:

```shell
curl -sSL https://raw.githubusercontent.com/kagenti/kagenti-operator/main/beeai/scripts/install.sh | bash
```
### 3. Create k8s secret containing your github repo token
Github token is required to push the agent docker image (built by the operator) to your ghcr.io image repo.
In a new terminal, run:
```
export PASSWORD=<your github token>
kubectl create secret generic github-token-secret --from-literal=token="$PASSWORD"
```

### 4. Usage 

### Testing the Operator with a BeeAI Agent Example

To test the `kagenti-operator` with a BeeAI agent example, we will leverage the `ollama-deep-researcher` example agent. Its source code
can be found in [https://github.com/kagenti/agent-examples](https://github.com/kagenti/agent-examples) repo under beeai folder. 

**Alternatively, if you have your own BeeAI agent code already hosted in a GitHub repository, you can directly use that repository's URL in your `AgentBuild` configuration.**

You can proceed to create an `AgentBuild` custom resource in your Kubernetes cluster, referencing the appropriate repository URL to test the `kagenti-operator`'s build and deployment capabilities.

This section explains how to create and use the `AgentBuild` custom resource, which is the primary way to interact with the `kagenti-operator`.

###   Example `AgentBuild` Custom Resource

Here's an example YAML file (call it: research-agent-build.yaml) for creating an `AgentBuild` custom resource. 

```yaml
apiVersion: beeai.beeai.dev/v1
kind: AgentBuild
metadata:
  labels:
    app.kubernetes.io/name: kagenti-operator
    app.kubernetes.io/managed-by: kustomize
  name: research-agent-build
spec:
  # Example agent source code repository
  repoUrl: "github.com/kagenti/agent-examples.git"
   # subfolder in the above repo containing ollama-deep-researcher agent source code
  sourceSubfolder: beeai/ollama-deep-researcher
  # Github user name
  repoUser: "your-github-username"
  # branch name in your source repository
  revision: "main"
  # image name to build
  image: "research-agent"
  imageTag: "v0.0.1"
  # repo url to receive the image built. The Tekton build step will push the docker image
  # to this repo.
  imageRegistry: "ghcr.io/your-github-username"
  # reference the secret created in step 3 above
  env:
    # your github repo token to push the docker image
    - name: "SOURCE_REPO_SECRET"
      valueFrom:
        secretKeyRef:
          name: "github-token-secret"
          key: "token"

  # auto deploy the agent when pipeline succeeds
  deployAfterBuild: true
  # define agent configuration parameters
  agent:
    name: "research-agent"
    description: "research-agent" 
    env:
    # Make sure the port is set. Otherwise, BeeAI will use random port in your agent
      - name: PORT
        value: 8000
      - name: LLM_API_BASE
        value: "http://host.docker.internal:11434/v1"
      - name: LLM_API_KEY
        value: "dummy"  
      - name: LLM_MODEL
        value: "your-llm-model"
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
kubectl apply -f research-agent-build.yaml
```

In a new terminal, watch the pods using command:

```shell
watch kubectl get pods
```

When a Tekton pipeline succeeds you should see the following pods:

```
NAME                                         READY   STATUS      RESTARTS   AGE
kagenti-operator-controller-manager-585c587f7f-nxctl   1/1     Running     0          85s
research-agent-183740ed-build-and-push-pod             0/1     Completed   0          47s
research-agent-183740ed-check-subfolder-pod            0/1     Completed   0          56s
research-agent-183740ed-git-clone-pod                  0/1     Completed   0          66s
research-agent-c7cc5f568-82chd                         1/1     Running     0          9s
```

`Important:` Take note of the pod name that starts with `research-agent-`. In the example above, it is research-agent-c7cc5f568-82chd. The `research-agent` is the name of your running agent and will be used in subsequent commands to interact with it.

If your configured agent pod to expose port 8000 (env var PORT value), port-forward to the agent's k8s service as follows:

```shell
kubectl port-forward svc/"your-agent-name" 8000:8000
```


Run BeeAI client, making sure to use correct agent name as noted above. In our case, the agent name is `research-agent`:

`Important:` To run the beeai client code below, you must first clone beeai source code and cd to beeai directory. `This is an inconvenience
that we are working on to resolve in a near future.`

```
git clone https://github.com/i-am-bee/beeai-platform.git
cd beeai
```

```shell
 env BEEAI__HOST=http://localhost:8000 \
    BEEAI__MCP_SSE_PATH='/sse' \
    uv run beeai agent run research-agent "llamas vs alpacas main differences"
```    

