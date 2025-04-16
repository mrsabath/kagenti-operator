# kagenti-operator

This repository contains Kubernetes operators for managing AI agents. Currently, it includes an operator for BeeAI agents and will eventually include one for Llama-based agent deployments.

## BeeAI Operator

The BeeAI operator, located in the [beeai/](beeai/) directory, automates the lifecycle management of BeeAI agents within a Kubernetes cluster. It manages two Custom Resources (CRs):

* **`Agent`**: Defines the desired state of a BeeAI agent, including its container image, environment variables, and resource requirements. The operator ensures a corresponding Kubernetes Deployment and Service exist with the specified configurations.
* **`AgentBuild`**: Defines the specifications for building and publishing a container image for a BeeAI agent. Upon creation or update, the operator triggers a Tekton pipeline to pull source code, build a Docker image, and push it to a specified image registry.

For detailed information about the BeeAI operator, including its proposal, design, CRD definitions, and implementation details, please refer to the [README.md](beeai/README.md) in the `beeai` directory.

For installation and usage instructions for the BeeAI operator, please refer to the [INSTALL.md](beeai/INSTALL.md) in the `beeai` directory.

## Llama Operator

