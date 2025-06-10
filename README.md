# kagenti-operator

This repository contains Kubernetes operators for managing AI agents. Currently, it includes an operator for BeeAI agents and will eventually include one for Llama-based agent deployments.
## Platform Operator
Thee Platform operator, located in [platform-operator/](platform-operator/) directory, simplifies the deployment of complex applications by managing collections of components through two key Custom Resources: Platform and Component. 
* **`Comoponent`**: The Component CR represents an individual deployable unit within a platform, with each component being exactly one of three types: Agent (AI/ML applications), Tool (utilities and supporting services), or Infrastructure (databases and foundational services). The Component controller manages the complete lifecycle from build execution through deployment, while the component starts in a suspended state until activated by the Platform controller based on dependency requirements and execution order.

* **`Platform`**: The Platform CR serves as a high-level orchestrator that manages collections of related Component CRs as a cohesive application unit. It defines the execution order and dependency relationships between components, ensuring that infrastructure components are deployed before the applications that depend on them. 

For detailed information about the Platform operator, including its proposal, design, CRD definitions, and implementation details, please refer to the [README.md](platform-operator/READEM.md) in the `platoform-operator` directory.

## BeeAI Operator

The BeeAI operator, located in the [beeai/](beeai/) directory, automates the lifecycle management of BeeAI agents within a Kubernetes cluster. It manages two Custom Resources (CRs):

* **`Agent`**: Defines the desired state of a BeeAI agent, including its container image, environment variables, and resource requirements. The operator ensures a corresponding Kubernetes Deployment and Service exist with the specified configurations.
* **`AgentBuild`**: Defines the specifications for building and publishing a container image for a BeeAI agent. Upon creation or update, the operator triggers a Tekton pipeline to pull source code, build a Docker image, and push it to a specified image registry.

For detailed information about the BeeAI operator, including its proposal, design, CRD definitions, and implementation details, please refer to the [README.md](beeai/README.md) in the `beeai` directory.

For installation and usage instructions for the BeeAI operator, please refer to the [INSTALL.md](beeai/INSTALL.md) in the `beeai` directory.

## Llama Operator

