#!/bin/bash

# Registry Utility Functions
# Provides common functions for creating and managing test registries
# Source this file in test scripts to use these functions

# Generate a random registry name
generate_random_registry_name() {
    local prefix="${1:-acrtest}"
    local suffix=""
    
    # Generate random suffix using different methods based on availability
    if command -v openssl >/dev/null 2>&1; then
        suffix=$(openssl rand -hex 4)
    elif command -v sha256sum >/dev/null 2>&1; then
        suffix=$(date +%s | sha256sum | head -c 8)
    elif command -v shasum >/dev/null 2>&1; then
        suffix=$(date +%s | shasum | head -c 8)
    else
        # Fallback to using process ID and timestamp
        suffix=$(printf "%x%x" $$ $(date +%s) | head -c 8)
    fi
    
    echo "${prefix}${suffix}"
}

# Create a temporary registry with all required resources
create_temporary_registry() {
    local registry_name="${1:-$(generate_random_registry_name)}"
    local location="${2:-eastus}"
    
    # Set global variables for cleanup
    export TEMP_REGISTRY_NAME="$registry_name"
    export TEMP_RESOURCE_GROUP="rg-acr-test-$(echo $registry_name | sed 's/acrtest//')"
    export TEMP_REGISTRY_CREATED=true
    
    echo "Creating resource group: $TEMP_RESOURCE_GROUP"
    if ! az group create --name "$TEMP_RESOURCE_GROUP" --location "$location" --output none; then
        echo "Error: Failed to create resource group" >&2
        return 1
    fi
    
    echo "Creating registry: $registry_name"
    if ! az acr create \
        --resource-group "$TEMP_RESOURCE_GROUP" \
        --name "$registry_name" \
        --sku Basic \
        --admin-enabled true \
        --output none; then
        echo "Error: Failed to create registry" >&2
        return 1
    fi
    
    # Set the full registry URL
    export TEMP_REGISTRY_URL="${registry_name}.azurecr.io"
    
    echo "Registry created successfully: $TEMP_REGISTRY_URL"
    
    # Login to the registry
    echo "Logging in to registry..."
    az acr login --name "$registry_name" >/dev/null 2>&1
    
    return 0
}

# Clean up temporary registry and resource group
cleanup_temporary_registry() {
    if [ "${TEMP_REGISTRY_CREATED:-false}" = "true" ] && [ -n "${TEMP_REGISTRY_NAME:-}" ]; then
        echo "Cleaning up temporary registry: $TEMP_REGISTRY_NAME"
        echo "Resource group: $TEMP_RESOURCE_GROUP"
        
        # In non-interactive mode, auto-delete. In interactive mode, ask.
        if [ -t 0 ] && [ -t 1 ]; then
            # Interactive mode - ask user
            read -p "Delete temporary registry and resource group? (y/N) " -n 1 -r
            echo
            if [[ $REPLY =~ ^[Yy]$ ]]; then
                echo "Deleting temporary registry..."
                az group delete --name "$TEMP_RESOURCE_GROUP" --yes --no-wait
                echo "Deletion initiated."
            else
                echo "Keeping temporary registry. Delete manually with:"
                echo "  az group delete --name $TEMP_RESOURCE_GROUP --yes"
            fi
        else
            # Non-interactive mode - auto-delete
            echo "Auto-deleting temporary registry in non-interactive mode..."
            az group delete --name "$TEMP_RESOURCE_GROUP" --yes --no-wait
            echo "Deletion initiated."
        fi
        
        # Clear environment variables
        unset TEMP_REGISTRY_NAME
        unset TEMP_RESOURCE_GROUP  
        unset TEMP_REGISTRY_CREATED
        unset TEMP_REGISTRY_URL
    fi
}

# Get or create a registry for testing
# If REGISTRY is not set, creates a temporary one
ensure_test_registry() {
    local registry_var_name="${1:-REGISTRY}"
    
    # Get the current value of the registry variable
    local current_registry
    eval "current_registry=\$$registry_var_name"
    
    if [ -z "$current_registry" ]; then
        echo "No registry specified. Creating temporary registry..."
        
        if create_temporary_registry; then
            # Set the registry variable to the temporary registry URL
            eval "export $registry_var_name=\"$TEMP_REGISTRY_URL\""
            echo "Using temporary registry: $TEMP_REGISTRY_URL"
        else
            echo "Error: Failed to create temporary registry" >&2
            return 1
        fi
    else
        echo "Using specified registry: $current_registry"
        
        # Validate that the registry exists and is accessible
        local registry_name="${current_registry%%.*}"
        if ! az acr show --name "$registry_name" >/dev/null 2>&1; then
            echo "Warning: Registry '$registry_name' not found or not accessible" >&2
            echo "Make sure you're logged in and have appropriate permissions" >&2
            return 1
        fi
        
        # Login to the registry
        echo "Logging in to registry..."
        az acr login --name "$registry_name" >/dev/null 2>&1
    fi
    
    return 0
}

# Set up cleanup trap for temporary registries
setup_registry_cleanup_trap() {
    trap cleanup_temporary_registry EXIT
}

# Print usage information
print_registry_utils_usage() {
    cat << EOF
Registry Utility Functions Usage:

Source this file in your test scripts:
  source path/to/registry-utils.sh

Functions available:

1. generate_random_registry_name [prefix]
   - Generates a random registry name with optional prefix
   - Default prefix: "acrtest"
   - Example: generate_random_registry_name "mytest"

2. create_temporary_registry [name] [location]  
   - Creates a temporary registry with resource group
   - Sets global variables: TEMP_REGISTRY_NAME, TEMP_RESOURCE_GROUP, TEMP_REGISTRY_URL
   - Default location: "eastus"
   - Example: create_temporary_registry "myregistry" "westus2"

3. cleanup_temporary_registry
   - Cleans up temporary registry and resource group
   - Prompts user in interactive mode, auto-deletes in non-interactive mode
   
4. ensure_test_registry [registry_var_name]
   - Gets or creates a registry for testing
   - If registry variable is empty, creates temporary registry
   - Default variable name: "REGISTRY" 
   - Example: ensure_test_registry "MY_REGISTRY"

5. setup_registry_cleanup_trap
   - Sets up EXIT trap to automatically cleanup temporary registries

Example usage in a test script:

#!/bin/bash
source "$(dirname "\${BASH_SOURCE[0]}")/registry-utils.sh"

# Set up cleanup trap
setup_registry_cleanup_trap

# Ensure we have a registry to test with
ensure_test_registry

# Now REGISTRY variable contains a valid registry URL
echo "Using registry: \$REGISTRY"

# Run your tests...
# Cleanup will happen automatically on script exit

EOF
}

# If script is run directly (not sourced), show usage
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    print_registry_utils_usage
fi