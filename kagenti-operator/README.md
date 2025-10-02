## Kagenti Operator ##

The `Kagenti Operator` is a Kubernetes operator that manages AI Agent lifecycle supporting deployments from existing images or from source code. 

### Architecture ###
```mermaid
graph TD;
    subgraph Kubernetes
        direction TB
        style Kubernetes fill:#f0f4ff,stroke:#8faad7,stroke-width:2px
        
        User[User/App]
        style User fill:#ffecb3,stroke:#ffa000
        
        AgentCRD["Agent CR"] 
        style AgentCRD fill:#e1f5fe,stroke:#039be5
        
        AgentBuildCRD["AgentBuild CR"]
        style AgentBuildCRD fill:#e1f5fe,stroke:#039be5
        
        User -->|Creates| AgentCRD
        User -->|Creates| AgentBuildCRD
        
        AgentController[Agent Controller] 
        style AgentController fill:#ffe0b2,stroke:#fb8c00
        
        AgentBuildController[AgentBuild Controller]
        style AgentBuildController fill:#ffe0b2,stroke:#fb8c00
        
        Service_Service[Service]
        style Service_Service fill:#dcedc8,stroke:#689f38
        
        Deployment_Deployment[Deployment]
        style Deployment_Deployment fill:#d1c4e9,stroke:#7e57c2
        
        AgentPod[Agent Pod]
        style AgentPod fill:#c8e6c9,stroke:#66bb6a
        
        AgentCRD -->|Reconciles| AgentController
        AgentBuildCRD -->|Reconciles| AgentBuildController
        
        AgentController --> |Creates| Service_Service
        AgentController --> |Creates| Deployment_Deployment
        
        Deployment_Deployment --> |Deploys| AgentPod
        
        subgraph Tekton_Pipeline
            direction LR
            style Tekton_Pipeline fill:#e7f3e7,stroke:#73b473,stroke-width:1px
            
            Pull[1. Pull Task]
            style Pull fill:#e8eaf6,stroke:#5c6bc0
            Build[2. Build Task]
            style Build fill:#fff3e0,stroke:#ffa726
            Push[3. Push Image Task]
            style Push fill:#f3e5f5,stroke:#ab47bc
            Pull --> Build --> Push
        end
        
        AgentBuildController -->|Triggers| Tekton_Pipeline
        AgentBuildController -->|Creates| AgentCRD
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