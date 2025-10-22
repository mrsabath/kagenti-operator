# kagenti-operator

This repository contains Kubernetes operator for managing AI agents.
## Platform Operator
The Platform operator, located in [platform-operator/](platform-operator/) directory, simplifies the deployment of complex applications by managing collections of components through two key Custom Resources: Platform and Component.
* **`Component`**: The Component CR represents an individual deployable unit within a platform, with each component being exactly one of three types: Agent (AI/ML applications), Tool (utilities and supporting services), or Infrastructure (databases and foundational services). The Component controller manages the complete lifecycle from build execution through deployment, while the component starts in a suspended state until activated by the Platform controller based on dependency requirements and execution order.

* **`Platform`**: The Platform CR serves as a high-level orchestrator that manages collections of related Component CRs as a cohesive application unit. It defines the execution order and dependency relationships between components, ensuring that infrastructure components are deployed before the applications that depend on them.

For detailed information about the Platform operator, including its proposal, design, CRD definitions, and implementation details, please refer to the [README.md](platform-operator/README.md) in the `platform-operator` directory.
