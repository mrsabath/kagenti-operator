#!/bin/bash

set -euo pipefail

echo "--- Installing the llama operator ---"
helm upgrade --install kagenti-beeai-operator oci://ghcr.io/kagenti/kagenti-operator/kagenti-llama-operator-chart --version 0.0.1
# helm upgrade --install kagenti dist/chart

echo "--- Installation complete ---"
