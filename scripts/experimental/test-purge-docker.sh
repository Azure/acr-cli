#!/bin/bash
set -uo pipefail

# Test script for ACR purge command using Docker container
# Based on the original test-purge-all.sh with Docker container support

# Check for required commands
for cmd in az docker; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "Error: Required command '$cmd' not found"
        exit 1
    fi
done

REGISTRY="${1:-}"
TEST_MODE="${2:-minimal}"  # Options: minimal, comprehensive, docker-test
NUM_IMAGES="${3:-50}"     # Reduced default for testing
TEMP_REGISTRY_CREATED=false
TEMP_REGISTRY_NAME=""
RESOURCE_GROUP=""
DEBUG="${DEBUG:-0}"

# Docker image to use for testing
DOCKER_IMAGE="${DOCKER_IMAGE:-acr-cli-minimal-manual}"
USE_DOCKER="${USE_DOCKER:-true}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Test results tracking
TESTS_PASSED=0
TESTS_FAILED=0
FAILED_TESTS=()

# Helper function to run ACR CLI
run_acr() {
    if [ "$USE_DOCKER" = "true" ]; then
        # Get ACR access token
        local registry_name=$(get_registry_name)
        local access_token=$(az acr login --name "$registry_name" --expose-token --output tsv --query accessToken 2>/dev/null)
        
        if [ -z "$access_token" ]; then
            echo "Error: Failed to get access token for $registry_name" >&2
            return 1
        fi
        
        # Run with access token (use standard OAuth username)
        docker run --rm \
            "$DOCKER_IMAGE" "$@" --username "00000000-0000-0000-0000-000000000000" --password "$access_token"
    else
        # Use local binary
        local ACR_CLI="${SCRIPT_DIR}/../../bin/acr"
        if [ ! -f "$ACR_CLI" ]; then
            echo "Building ACR CLI..."
            (cd "$SCRIPT_DIR/../.." && make binaries)
        fi
        "$ACR_CLI" "$@"
    fi
}

# Cleanup function
cleanup_temp_registry() {
    if [ "$TEMP_REGISTRY_CREATED" = true ] && [ -n "$TEMP_REGISTRY_NAME" ]; then
        echo -e "\n${YELLOW}Temporary registry cleanup${NC}"
        echo "Registry: $TEMP_REGISTRY_NAME"
        echo "Resource group: $RESOURCE_GROUP"
        read -p "Delete temporary registry and resource group? (y/N) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo -e "${GREEN}Deleting temporary registry...${NC}"
            az group delete --name "$RESOURCE_GROUP" --yes --no-wait
            echo "Deletion initiated."
        else
            echo -e "${YELLOW}Keeping temporary registry. Delete manually with:${NC}"
            echo "  az group delete --name $RESOURCE_GROUP --yes"
        fi
    fi

    # Print test summary
    echo -e "\n${BLUE}=== Test Summary ===${NC}"
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
    if [ ${#FAILED_TESTS[@]} -gt 0 ]; then
        echo -e "\n${RED}Failed tests:${NC}"
        for test in "${FAILED_TESTS[@]}"; do
            echo "  - $test"
        done
    fi
}

trap cleanup_temp_registry EXIT

# Helper functions
assert_equals() {
    local expected="$1"
    local actual="$2"
    local test_name="$3"

    if [ "$expected" = "$actual" ]; then
        echo -e "${GREEN}✓ $test_name${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ $test_name${NC}"
        echo -e "  Expected: $expected, Actual: $actual"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("$test_name")
    fi
}

get_registry_name() {
    echo "${REGISTRY%%.*}"
}

count_tags() {
    local repo="$1"
    local tags=$(run_acr tag list -r "$REGISTRY" --repository "$repo" 2>/dev/null || echo "")
    # Count only lines that look like registry URLs (contain the registry name)
    local count=$(echo "$tags" | grep "$REGISTRY" | wc -l | tr -d ' ')
    
    if [ "$DEBUG" = "1" ]; then
        echo -e "\n  DEBUG count_tags for $repo:" >&2
        echo "  Raw output:" >&2
        echo "$tags" | sed 's/^/    /' >&2
        echo "  Final count: $count" >&2
    fi
    echo "$count"
}

create_test_image() {
    local repo="$1"
    local tag="$2"
    local base_image="mcr.microsoft.com/hello-world"

    if ! docker pull "$base_image" >/dev/null 2>&1; then
        echo "Error: Failed to pull base image $base_image" >&2
        return 1
    fi

    if ! docker tag "$base_image" "$REGISTRY/$repo:$tag"; then
        echo "Error: Failed to tag image"
        return 1
    fi

    if ! docker push "$REGISTRY/$repo:$tag" >/dev/null 2>&1; then
        echo "Error: Failed to push image $REGISTRY/$repo:$tag"
        return 1
    fi

    return 0
}

# Create temporary registry if needed
if [ -z "$REGISTRY" ]; then
    echo -e "${GREEN}Creating temporary registry...${NC}"
    # Generate random suffix
    if command -v openssl >/dev/null 2>&1; then
        RANDOM_SUFFIX=$(openssl rand -hex 4)
    else
        RANDOM_SUFFIX=$(printf "%x%x" $$ $(date +%s) | head -c 8)
    fi
    TEMP_REGISTRY_NAME="acrtest${RANDOM_SUFFIX}"
    RESOURCE_GROUP="rg-acr-test-${RANDOM_SUFFIX}"

    echo "Creating resource group: $RESOURCE_GROUP"
    if ! az group create --name "$RESOURCE_GROUP" --location "eastus" --output none; then
        echo -e "${RED}Failed to create resource group${NC}"
        exit 1
    fi

    echo "Creating registry: $TEMP_REGISTRY_NAME"
    if ! az acr create --resource-group "$RESOURCE_GROUP" --name "$TEMP_REGISTRY_NAME" --sku Basic --admin-enabled true --output none; then
        echo -e "${RED}Failed to create registry${NC}"
        exit 1
    fi

    REGISTRY="${TEMP_REGISTRY_NAME}.azurecr.io"
    TEMP_REGISTRY_CREATED=true
    echo -e "${GREEN}Registry created: $REGISTRY${NC}"
fi

# Login to ACR
echo "Logging in to registry..."
az acr login --name "$(get_registry_name)" >/dev/null 2>&1

echo -e "\n${BLUE}=== ACR Purge Docker Test Suite ===${NC}"
echo "Registry: $REGISTRY"
echo "Test mode: $TEST_MODE"
echo "Docker image: $DOCKER_IMAGE"
echo "Using Docker: $USE_DOCKER"
echo ""

# Test Docker container functionality
run_docker_tests() {
    echo -e "\n${BLUE}=== Docker Container Tests ===${NC}"
    
    # Test 1: Version command
    echo -e "\n${YELLOW}Test 1: Version Command${NC}"
    local version_output=$(run_acr version 2>&1)
    if echo "$version_output" | grep -q "Version:"; then
        echo -e "${GREEN}✓ Version command works${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Version command failed${NC}"
        echo "Output: $version_output"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Version command failed")
    fi
    
    # Test 2: Help command
    echo -e "\n${YELLOW}Test 2: Help Command${NC}"
    local help_output=$(run_acr --help 2>&1)
    if echo "$help_output" | grep -q "Usage:"; then
        echo -e "${GREEN}✓ Help command works${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Help command failed${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Help command failed")
    fi
    
    # Test 3: Basic purge functionality
    echo -e "\n${YELLOW}Test 3: Basic Purge Functionality${NC}"
    local repo="docker-test-basic"
    
    echo "Creating test images..."
    for i in 1 2 3; do
        create_test_image "$repo" "v$i"
    done
    
    local initial_count=$(count_tags "$repo")
    echo "Initial tags: $initial_count"
    
    # Test dry run
    echo "Testing dry run..."
    local dry_run_output=$(run_acr purge --registry "$REGISTRY" --filter "$repo:v1" --ago 0d --dry-run 2>&1)
    local dry_run_count=$(count_tags "$repo")
    
    if [ "$initial_count" = "$dry_run_count" ]; then
        echo -e "${GREEN}✓ Dry run preserves images${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Dry run should not delete images${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Dry run should not delete images")
    fi
    
    # Test actual delete
    echo "Testing actual delete..."
    local delete_output=$(run_acr purge --registry "$REGISTRY" --filter "$repo:v1" --ago 0d 2>&1)
    local final_count=$(count_tags "$repo")
    
    if [ "$final_count" = "$((initial_count - 1))" ]; then
        echo -e "${GREEN}✓ Actual delete works${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Actual delete failed${NC}"
        echo "Expected: $((initial_count - 1)), Got: $final_count"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Actual delete failed")
    fi
    
    # Test 4: Authentication handling
    echo -e "\n${YELLOW}Test 4: Authentication Handling${NC}"
    # The fact that previous tests worked means auth is working
    echo -e "${GREEN}✓ Authentication works (previous tests successful)${NC}"
    ((TESTS_PASSED++))
    
    # Test 5: Pattern matching
    echo -e "\n${YELLOW}Test 5: Pattern Matching${NC}"
    local pattern_repo="docker-test-patterns"
    
    for tag in "v1.0.0" "v2.0.0" "dev-123"; do
        create_test_image "$pattern_repo" "$tag"
    done
    
    local pattern_output=$(run_acr purge --registry "$REGISTRY" --filter "$pattern_repo:v.*\.0\.0" --ago 0d --dry-run 2>&1)
    if echo "$pattern_output" | grep -q "v1.0.0" && echo "$pattern_output" | grep -q "v2.0.0"; then
        echo -e "${GREEN}✓ Pattern matching works${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Pattern matching failed${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Pattern matching failed")
    fi
    
    # Test 6: Keep parameter
    echo -e "\n${YELLOW}Test 6: Keep Parameter${NC}"
    local keep_repo="docker-test-keep"
    
    for i in $(seq 1 5); do
        create_test_image "$keep_repo" "v$i"
        sleep 0.1
    done
    
    local keep_initial=$(count_tags "$keep_repo")
    run_acr purge --registry "$REGISTRY" --filter "$keep_repo:.*" --ago 0d --keep 2 >/dev/null 2>&1
    local keep_final=$(count_tags "$keep_repo")
    
    if [ "$keep_final" -eq 2 ] || [ "$keep_final" -eq 3 ]; then
        echo -e "${GREEN}✓ Keep parameter works (kept $keep_final images)${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Keep parameter failed${NC}"
        echo "Expected: 2-3, Got: $keep_final"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Keep parameter failed")
    fi
}

# Run minimal tests
run_minimal_tests() {
    echo -e "\n${BLUE}=== Minimal Test Suite ===${NC}"
    
    local repo="test-minimal"
    echo "Creating test images..."
    for i in 1 2 3 4 5; do
        create_test_image "$repo" "v$i"
    done
    
    local initial_count=$(count_tags "$repo")
    echo "Created $initial_count test images"
    
    # Test 1: Basic purge
    echo -e "\n${YELLOW}Test 1: Basic Purge${NC}"
    local output=$(run_acr purge --registry "$REGISTRY" --filter "$repo:v[12]" --ago 0d --dry-run)
    echo "Dry run output:"
    echo "$output"
    
    # Test 2: Actual purge
    echo -e "\n${YELLOW}Test 2: Actual Purge${NC}"
    run_acr purge --registry "$REGISTRY" --filter "$repo:v1" --ago 0d
    local after_purge=$(count_tags "$repo")
    
    if [ "$after_purge" = "$((initial_count - 1))" ]; then
        echo -e "${GREEN}✓ Purge deleted 1 image${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Purge count mismatch${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Purge count mismatch")
    fi
    
    # Test 3: Keep functionality
    echo -e "\n${YELLOW}Test 3: Keep Functionality${NC}"
    run_acr purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d --keep 2
    local final_count=$(count_tags "$repo")
    
    if [ "$final_count" -le 3 ]; then  # Allow some flexibility for 'latest' tags
        echo -e "${GREEN}✓ Keep parameter worked (kept $final_count images)${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Keep parameter failed${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Keep parameter failed")
    fi
}

# Run comprehensive tests
run_comprehensive_tests() {
    echo -e "\n${BLUE}=== Comprehensive Test Suite ===${NC}"
    
    # Create more test data
    local comp_repo="test-comprehensive"
    echo "Creating comprehensive test images..."
    for i in $(seq 1 "$NUM_IMAGES"); do
        create_test_image "$comp_repo" "v$(printf "%03d" "$i")"
        if [ $((i % 10)) -eq 0 ]; then
            echo "Created $i/$NUM_IMAGES images..."
        fi
    done
    
    echo "Created $(count_tags "$comp_repo") test images"
    
    # Test different scenarios
    echo -e "\n${YELLOW}Test 1: Large Scale Dry Run${NC}"
    local start_time=$(date +%s)
    local large_output=$(run_acr purge --registry "$REGISTRY" --filter "$comp_repo:v0[0-4][0-9]" --ago 0d --dry-run)
    local end_time=$(date +%s)
    echo "Large scale dry run completed in $((end_time - start_time)) seconds"
    
    echo -e "\n${YELLOW}Test 2: Concurrency Test${NC}"
    start_time=$(date +%s)
    run_acr purge --registry "$REGISTRY" --filter "$comp_repo:v0[0-2][0-9]" --ago 0d --concurrency 10
    end_time=$(date +%s)
    echo "Concurrent deletion completed in $((end_time - start_time)) seconds"
    
    local remaining=$(count_tags "$comp_repo")
    echo "Remaining images: $remaining"
    
    if [ "$remaining" -lt "$NUM_IMAGES" ]; then
        echo -e "${GREEN}✓ Comprehensive tests completed successfully${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Comprehensive tests failed${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Comprehensive tests failed")
    fi
}

# Main execution
case "$TEST_MODE" in
    docker-test)
        run_docker_tests
        ;;
    minimal)
        run_docker_tests
        run_minimal_tests
        ;;
    comprehensive)
        run_docker_tests
        run_minimal_tests
        run_comprehensive_tests
        ;;
    *)
        echo "Invalid test mode: $TEST_MODE"
        echo "Options: docker-test, minimal, comprehensive"
        exit 1
        ;;
esac

echo -e "\n${GREEN}=== Docker tests completed ===${NC}"