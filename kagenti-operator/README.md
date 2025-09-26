# Kagenti Operator
This document presents a proposal for a Kubernetes Operator to automate the lifecycle management of AI agents within a Kubernetes cluster. This operator will manage two Custom Resources (CRs): `KagentiAgent` and `KagentiAgentBuild`.

The `KagentiAgent` CR defines the desired state of a AI agent, including its container image, environment variables, and resource requirements. The operator will reconcile `KagentiAgent` resources by ensuring a corresponding Kubernetes Deployment and Service exist with the specified configurations.

The `KagentiAgentBuild` CR defines the specifications for building and publishing a container image for a AI agent. Upon creation or update of an `KagentiAgentBuild` resource, the operator will trigger a Tekton pipeline to automate pulling source code, building a Docker image, and pushing it to a specified image registry. Secure access to private repositories is managed through a reference to a Kubernetes Secret containing a GitHub token. Once the build finishes, the controller reconcilling `KagentiAgentBuild` CR will create `KagentiAgent` CustomResource.

## 2. Goals

* Automate the creation and management of Kubernetes Deployments and Services based on `KagentiAgent` CR specifications for AI agents.
* Provide a declarative way to define and manage AI agents.
* Automate the container image building and publishing process for BeeAI agents triggered by `KagentiAgentBuild` CRs.
* Integrate with Tekton Pipelines for the image building workflow, consisting of pull, build, and push tasks.
* Securely manage GitHub repository access using a referenced Kubernetes Secret.
* Lock down agent pods with read-only filesystem

## 3. Proposed Design

```mermaid
graph TD;
    subgraph Kubernetes
        direction RL
        style Kubernetes fill:#f0f4ff,stroke:#8faad7,stroke-width:2px

        subgraph Tekton_Pipeline
            direction RL
            style Tekton_Pipeline fill:#e7f3e7,stroke:#73b473,stroke-width:1px
            
            Pull[Pull Task]
            style Pull fill:#e8eaf6,stroke:#5c6bc0

            Build[Build Task]
            style Build fill:#fff3e0,stroke:#ffa726

            Push[Push Image Task]
            style Push fill:#f3e5f5,stroke:#ab47bc

            Pull --> Build --> Push
        end
        
        Operator[Operator] 
        style Operator fill:#ffe0b2,stroke:#fb8c00

        KagentiAgentCRD["KagentiAgent CRD"] 
        style KagentiAgentCRD fill:#e1f5fe,stroke:#039be5

        KagentiAgentBuildCRD["KagentiAgentBuild CRD"]
        style KagentiAgentBuildCRD fill:#fce4ec,stroke:#e91e63

        Operator -- Reacts to --> KagentiAgentCRD
        Operator -- Reacts to --> KagentiAgentBuildCRD

        KagentiAgentBuildCRD -->|Triggers| Tekton_Pipeline
        KagentiAgentCRD --> |Creates| Service_Service[Service]
        style Service_Service fill:#dcedc8,stroke:#689f38

        KagentiAgentCRD --> |Creates| Deployment_Deployment[Deployment]
        style Deployment_Deployment fill:#d1c4e9,stroke:#7e57c2
    end
```    


## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

