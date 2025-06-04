# Kagenti Platform Operator Documentation

This document provides information on how to install and use the `kagenti-platform-operator` which automates the lifecycle management of agentic platform components within a Kubernetes cluster. 
## Overview
The operator will manage two primary Custom Resources: `Component` CRs and `Platform` CR.

The `Component` CR represents various types of deployable entities including Agents, Tools, and Infrastructure.

The `Platform` CR models the entire platform as a collection of components, managing cross-component dependencies and providing a unified view of the platform's status.

The operator implements controllers for these CRs that handle automated creation, updating, and deletion of underlying Kubernetes resources. It will facilitate building components from source code using Tekton pipelines and deploying these components using various methods such as direct Kubernetes manifests, Helm charts, and the Operator Lifecycle Manager (OLM).


## Prerequisites

Before installing the `kagenti-operator`, ensure you have:

* **Kubernetes Cluster:** A working Kubernetes cluster (Kind recommended for development)
* **kubectl:** The Kubernetes command-line tool
* **For building agents:**
  * Agent source code in a GitHub repository with a working Dockerfile
  * GitHub token for accessing repositories and pushing images to ghcr.io
* **For deploying agents:**
  * Existing agent image in a container registry (ghcr.io, Docker Hub, etc.)
  * (Optional) Registry credentials if using private images
* **Install [ollama]:**(https://ollama.com/download)


## Installation

### 1. Start Ollama

In a new terminal, run:

```shell
ollama run "your-llm-model" --keepalive 60m
```
### 2. Start Kagenti platform operator
In a new terminal, run:

```shell
curl -sSL https://raw.githubusercontent.com/kagenti/kagenti-operator/main/platform-operator/scripts/install.sh | bash
```

## Usage

### Option 1: Building and Deploying New Agents From Source
By default the operator install script creates Local Registry within Kind and a token is not needed to
push and pull images.