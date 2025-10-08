## Kagenti Operator ##

The `Kagenti Operator` is a Kubernetes operator that manages AI Agent lifecycle supporting deployments from existing images or from source code. 

### Architecture ###
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

        AgentCRD["Agent CRD"] 
        style AgentCRD fill:#e1f5fe,stroke:#039be5

        AgentBuildCRD["AgentBuild CRD"]
        style AgentBuildCRD fill:#fce4ec,stroke:#e91e63

        Operator -- Reacts to --> AgentCRD
        Operator -- Reacts to --> AgentBuildCRD

        AgentBuildCRD -->|Triggers| Tekton_Pipeline
        AgentCRD --> |Creates| Service_Service[Service]
        style Service_Service fill:#dcedc8,stroke:#689f38

        AgentCRD --> |Creates| Deployment_Deployment[Deployment]
        style Deployment_Deployment fill:#d1c4e9,stroke:#7e57c2
    end
```    
The operator is designed with two Custom Resources (CRs) to seperate build concerns from deployment concerns: 
 - **Agent CR** Manages the deployment and lifecycle of AI Agents using container images
 - **AgentBuild CR** Manages the build phase, orchestrating Tekton Pipelines to build container images from source 

### Documentation ###
- [Design](docs/operator.md)
- API Reference
- Installation Guide
- User Guide