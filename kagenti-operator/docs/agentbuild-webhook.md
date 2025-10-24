# AgentBuild Webhook

This webhook automatically injects Tekton Pipeline specifications into AgentBuild Custom Resources (CRs) at creation time.
It ensures consistent pipeline structure by merging user-provided parameters with a shared Tekton pipeline template stored in a ConfigMap.

## Overview

The AgentBuild Mutating Webhook is part of the Kagenti Operator.
It performs two key functions:

1. Defaulting webhook (AgentBuildDefaulter)

    - Mutates the incoming AgentBuild CR by injecting a Tekton Pipeline specification.

    - Pulls the pipeline definition from a ConfigMap template based on build mode (dev, preprod, prod).

    - Adds missing default values and parameters (e.g., image name).

2. Validating webhook (AgentBuildValidator)

    - Validates required fields before allowing CR creation or updates.

## Architecture

```
User creates AgentBuild CR
         ↓
   API Server
         ↓
   Mutating Webhook (Defaulting)
         ↓
   Validating Webhook (Validation)
         ↓
   Persisted to etcd
         ↓
   AgentBuild Controller reconciles
```

## Defaulting Webhook

### Functionality

The `AgentBuildDefaulter` automatically populates default values for optional fields that users don't specify.

### Defaults Applied

| Field | Default Value | Reason |
|-------|---------------|--------|
| `spec.mode` | `"dev"` | Use development pipeline template |
| `spec.cleanupAfterBuild` | `true` | Clean up build resources after completion |
| `spec.source.sourceRevision` | `"main"` | Use main branch if not specified |
| `spec.pipeline.namespace` | `"kagenti-system"` | Look for pipeline step ConfigMaps in kagenti-system |


### Webhook Logic Flow

When a new AgentBuild CR is created:

1. Defaults applied:

    - spec.mode → defaults to "dev" if not provided.

    - spec.pipeline.namespace → defaults to CR’s namespace.

    - Adds image parameter based on spec.buildOutput fields if missing.

2. Tekton pipeline injection:
    - Fetches ConfigMap named pipeline-template-<mode> (e.g., pipeline-template-dev).

    - Reads JSON payload under key template.json.

    - Validates that all required parameters in the template exist.

    - Merges user parameters with template to produce a final pipeline definition.

### Pipeline Template Example

Each template ConfigMap is named pipeline-template-<mode> and must include a template.json key.

```json

apiVersion: v1
data:
  template.json: |
    {
      "name": "Development Pipeline",
      "namespace": "kagenti-system",
      "description": "Basic pipeline for development builds",
      "requiredParameters": [
        "repo-url",
        "revision",
        "subfolder-path",
        "image"
      ],
      "steps": [
        {
          "name": "github-clone",
          "configMap": "github-clone-step",
          "enabled": true,
          "description": "Clone source code from GitHub repository",
          "requiredParameters": ["repo-url"]
        },
        {
          "name": "folder-verification",
          "configMap": "check-subfolder-step",
          "enabled": true,
          "description": "Verify that the specified subfolder exists",
          "requiredParameters": ["subfolder-path"]
        },
        {
          "name": "kaniko-build",
          "configMap": "kaniko-docker-build-step",
          "enabled": true,
          "description": "Build container image using Kaniko",
          "requiredParameters": ["image"]
        }
      ],
      "globalParameters": [
        {
          "name": "pipeline-timeout",
          "value": "20m",
          "description": "Overall pipeline timeout"
        }
      ]
    }
```