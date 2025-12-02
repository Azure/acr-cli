#!/bin/bash
set -e

# Example script demonstrating how to use registry-utils.sh
# This script shows how to create a test registry, use it, and clean up

# Source the registry utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/registry-utils.sh"

# Set up cleanup trap - this ensures temporary registries are cleaned up on exit
setup_registry_cleanup_trap

echo "=== Registry Utilities Demo ==="
echo ""

# Example 1: Generate random registry names
echo "1. Generating random registry names:"
echo "   Default prefix: $(generate_random_registry_name)"
echo "   Custom prefix:  $(generate_random_registry_name "demo")"
echo ""

# Example 2: Ensure we have a registry (will create one if REGISTRY is not set)
echo "2. Setting up test registry:"
if [ -z "${REGISTRY:-}" ]; then
    echo "   No REGISTRY environment variable set"
    echo "   Creating temporary registry..."
else
    echo "   Using existing registry: $REGISTRY"
fi

# This will create a temporary registry if REGISTRY is not set, or validate the existing one
if ensure_test_registry; then
    echo "   ✓ Registry is ready: $REGISTRY"
    
    # Example 3: Basic registry operations
    echo ""
    echo "3. Testing basic registry operations:"
    
    # Get the registry name (without .azurecr.io)
    REGISTRY_NAME="${REGISTRY%%.*}"
    echo "   Registry name: $REGISTRY_NAME"
    
    # Check if we can access the registry
    if az acr show --name "$REGISTRY_NAME" >/dev/null 2>&1; then
        echo "   ✓ Registry is accessible"
        
        # List repositories (should be empty for new registries)
        REPO_COUNT=$(az acr repository list --name "$REGISTRY_NAME" --query "length(@)" --output tsv 2>/dev/null || echo "0")
        echo "   Current repositories: $REPO_COUNT"
    else
        echo "   ✗ Registry is not accessible"
    fi
else
    echo "   ✗ Failed to set up registry"
    exit 1
fi

echo ""
echo "4. Registry information:"
if [ "${TEMP_REGISTRY_CREATED:-false}" = "true" ]; then
    echo "   This is a temporary registry that will be cleaned up on exit"
    echo "   Registry: $TEMP_REGISTRY_NAME"
    echo "   Resource Group: $TEMP_RESOURCE_GROUP"
    echo "   Full URL: $TEMP_REGISTRY_URL"
else
    echo "   Using provided registry: $REGISTRY"
fi

echo ""
echo "Demo completed successfully!"
echo "Note: If a temporary registry was created, it will be cleaned up when this script exits."