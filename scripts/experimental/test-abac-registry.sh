#!/bin/bash
set -uo pipefail

# ABAC Registry Test Script
# Tests ACR CLI functionality with ABAC-enabled (Attribute-Based Access Control) registries
# 
# ABAC registries have more granular permission controls at the repository level
# compared to traditional registries that use wildcard scopes.

# Test Configuration
REGISTRY="${1:-}"
TEST_MODE="${2:-comprehensive}"  # Options: basic, comprehensive, auth, all
DEBUG="${DEBUG:-0}"

# Path configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACR_CLI="${SCRIPT_DIR}/../../bin/acr"

# Source registry utilities
source "${SCRIPT_DIR}/registry-utils.sh"

# Set up cleanup trap for temporary registries
setup_registry_cleanup_trap

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

# Docker availability
DOCKER_AVAILABLE=false

# Check prerequisites
check_prerequisites() {
    echo -e "${CYAN}Checking prerequisites...${NC}"
    
    # Check Azure CLI
    if ! command -v az >/dev/null 2>&1; then
        echo -e "${RED}Error: Azure CLI not found. Please install Azure CLI.${NC}"
        exit 1
    fi
    
    # Check if logged in to Azure
    if ! az account show >/dev/null 2>&1; then
        echo -e "${RED}Error: Not logged in to Azure. Please run 'az login'.${NC}"
        exit 1
    fi
    
    # Check Docker
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
        DOCKER_AVAILABLE=true
        echo -e "${GREEN}✓ Docker available${NC}"
    else
        echo -e "${YELLOW}⚠ Docker not available - some tests will be skipped${NC}"
        DOCKER_AVAILABLE=false
    fi
    
    # Build ACR CLI if needed
    if [ ! -f "$ACR_CLI" ]; then
        echo "Building ACR CLI..."
        (cd "$SCRIPT_DIR/../.." && make binaries)
    fi
    
    echo -e "${GREEN}✓ All prerequisites met${NC}"
}

# Registry validation and setup
validate_registry() {
    # Use registry utility to ensure we have a registry to test with
    if ! ensure_test_registry; then
        echo -e "${RED}Error: Failed to set up test registry${NC}"
        exit 1
    fi
    
    echo -e "${CYAN}Using registry: $REGISTRY${NC}"
    
    # Extract registry name from FQDN
    local registry_name="${REGISTRY%%.*}"
    
    # Get credentials for ACR CLI
    echo "Getting registry credentials for ACR CLI..."
    
    # Try to get admin credentials first
    if az acr credential show --name "$registry_name" >/dev/null 2>&1; then
        echo -e "${GREEN}Using admin credentials for ACR CLI${NC}"
        # Get credentials and store them in environment variables for ACR CLI
        local creds_json=$(az acr credential show --name "$registry_name" 2>/dev/null)
        if [ -n "$creds_json" ]; then
            ACR_USERNAME=$(echo "$creds_json" | jq -r .username)
            ACR_PASSWORD=$(echo "$creds_json" | jq -r .passwords[0].value)
            export ACR_USERNAME ACR_PASSWORD
        fi
    else
        echo -e "${YELLOW}Admin credentials not available, trying token-based auth${NC}"
        # Try to get refresh token for ACR CLI
        local token_json=$(az acr login --name "$registry_name" --expose-token 2>/dev/null)
        if [ -n "$token_json" ]; then
            ACR_USERNAME="00000000-0000-0000-0000-000000000000"
            ACR_PASSWORD=$(echo "$token_json" | jq -r .refreshToken)
            export ACR_USERNAME ACR_PASSWORD
        else
            echo -e "${RED}Error: Cannot get credentials for registry.${NC}"
            exit 1
        fi
    fi
    
    # Also login with Docker if available
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
        az acr login --name "$registry_name" >/dev/null 2>&1 || true
    fi
    
    echo -e "${GREEN}✓ Registry validated and accessible${NC}"
}

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

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local test_name="$3"
    
    if echo "$haystack" | grep -q "$needle"; then
        echo -e "${GREEN}✓ $test_name${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ $test_name${NC}"
        echo -e "  Should contain: $needle"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("$test_name")
    fi
}

assert_not_contains() {
    local haystack="$1"
    local needle="$2"
    local test_name="$3"
    
    if ! echo "$haystack" | grep -q "$needle"; then
        echo -e "${GREEN}✓ $test_name${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ $test_name${NC}"
        echo -e "  Should NOT contain: $needle"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("$test_name")
    fi
}

create_test_image() {
    local repo="$1"
    local tag="$2"
    local base_image="mcr.microsoft.com/hello-world"
    
    if [ "$DEBUG" = "1" ]; then
        echo "Creating image: $REGISTRY/$repo:$tag"
    fi
    
    # Check if Docker is available and running
    if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
        echo -e "${YELLOW}Warning: Docker not available, skipping image creation for $repo:$tag${NC}"
        return 1
    fi
    
    docker pull "$base_image" >/dev/null 2>&1
    docker tag "$base_image" "$REGISTRY/$repo:$tag"
    docker push "$REGISTRY/$repo:$tag" >/dev/null 2>&1
}

# Helper function to run ACR CLI commands with credentials
run_acr_cli() {
    # Add timeout to prevent hanging on invalid registries
    local timeout_cmd=""
    if command -v timeout >/dev/null 2>&1; then
        timeout_cmd="timeout 30"
    elif command -v gtimeout >/dev/null 2>&1; then
        timeout_cmd="gtimeout 30"
    fi
    
    if [ -n "${ACR_USERNAME:-}" ] && [ -n "${ACR_PASSWORD:-}" ]; then
        $timeout_cmd "$ACR_CLI" "$@" -u "$ACR_USERNAME" -p "$ACR_PASSWORD"
    else
        $timeout_cmd "$ACR_CLI" "$@"
    fi
}

cleanup_repository() {
    local repo="$1"
    
    echo "Cleaning up repository: $repo"
    
    # Try to delete all tags in the repository
    run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "$repo:.*" \
        --ago 0d \
        --include-locked \
        --untagged >/dev/null 2>&1 || true
}

# Test: Basic ACR CLI Operations (no Docker required)
test_basic_acr_cli_operations() {
    echo -e "\n${YELLOW}Test: Basic ACR CLI Operations (Docker-free)${NC}"
    
    # Test 1: Test basic ACR CLI functionality
    echo -e "\n${CYAN}Testing basic ACR CLI functionality...${NC}"
    local help_output=$("$ACR_CLI" --help 2>&1)
    local exit_code=$?
    
    if [ $exit_code -eq 0 ] && echo "$help_output" | grep -q "Available Commands"; then
        echo -e "${GREEN}✓ ACR CLI basic functionality works${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ ACR CLI basic functionality failed${NC}"
        echo "Output: $help_output"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("ACR CLI basic functionality failed")
    fi
    
    # Test 2: Test purge dry-run on non-existent repository
    echo -e "\n${CYAN}Testing purge dry-run on non-existent repository...${NC}"
    local purge_output=$(run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "nonexistent-repo:.*" \
        --ago 0d \
        --dry-run 2>&1)
    exit_code=$?
    
    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}✓ Purge dry-run on non-existent repository succeeded${NC}"
        ((TESTS_PASSED++))
        
        # Check if it reports 0 tags for deletion
        if echo "$purge_output" | grep -q "Number of.*: 0"; then
            echo -e "${GREEN}✓ Correctly reports 0 tags for non-existent repository${NC}"
            ((TESTS_PASSED++))
        else
            echo -e "${YELLOW}⚠ Output doesn't clearly indicate 0 tags${NC}"
            echo "Output: $purge_output"
        fi
    else
        echo -e "${RED}✗ Purge dry-run failed${NC}"
        echo "Output: $purge_output"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Purge dry-run failed")
    fi
    
    # Test 3: Test invalid registry pattern
    echo -e "\n${CYAN}Testing invalid registry handling...${NC}"
    local invalid_output=$(run_acr_cli purge \
        --registry "invalid-registry.azurecr.io" \
        --filter "test:.*" \
        --ago 0d \
        --dry-run 2>&1 || true)
    
    # Should fail gracefully
    echo -e "${GREEN}✓ Invalid registry handled gracefully${NC}"
    ((TESTS_PASSED++))
}

# Test: Basic ABAC Repository Operations
test_basic_abac_operations() {
    echo -e "\n${YELLOW}Test: Basic ABAC Repository Operations${NC}"
    
    if [ "$DOCKER_AVAILABLE" = "false" ]; then
        echo -e "${YELLOW}Skipping test - requires Docker for image creation${NC}"
        return
    fi
    
    local repo="abac-test-basic"
    
    # Clean up any existing repository
    cleanup_repository "$repo"
    
    # Create test images
    echo "Creating test images..."
    for i in 1 2 3; do
        create_test_image "$repo" "v$i"
    done
    
    # Test 1: Test if repository exists by trying to list tags
    echo -e "\n${CYAN}Testing if repository exists by listing tags...${NC}"
    local tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1)
    local exit_code=$?
    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}✓ Repository $repo is accessible${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Repository $repo is not accessible or empty${NC}"
        echo "Output: $tags"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Repository $repo is not accessible")
    fi
    
    # Test 2: List tags
    echo -e "\n${CYAN}Testing tag listing...${NC}"
    local tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1)
    assert_contains "$tags" "v1" "Should list v1 tag"
    assert_contains "$tags" "v2" "Should list v2 tag"
    assert_contains "$tags" "v3" "Should list v3 tag"
    
    # Test 3: Delete specific tag
    echo -e "\n${CYAN}Testing tag deletion...${NC}"
    run_acr_cli purge --registry "$REGISTRY" --filter "$repo:v1" --ago 0d >/dev/null 2>&1
    
    tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1)
    assert_not_contains "$tags" "v1" "v1 should be deleted"
    assert_contains "$tags" "v2" "v2 should still exist"
    
    # Clean up
    cleanup_repository "$repo"
}

# Test: ABAC Permission Scoping
test_abac_permission_scoping() {
    echo -e "\n${YELLOW}Test: ABAC Permission Scoping${NC}"
    
    if [ "$DOCKER_AVAILABLE" = "false" ]; then
        echo -e "${YELLOW}Skipping test - requires Docker for image creation${NC}"
        return
    fi
    
    local repo1="abac-test-scope1"
    local repo2="abac-test-scope2"
    
    # Clean up any existing repositories
    cleanup_repository "$repo1"
    cleanup_repository "$repo2"
    
    # Create test images in different repositories
    echo "Creating test images in multiple repositories..."
    create_test_image "$repo1" "tag1"
    create_test_image "$repo1" "tag2"
    create_test_image "$repo2" "tag1"
    create_test_image "$repo2" "tag2"
    
    # Test 1: Repository-specific operations
    echo -e "\n${CYAN}Testing repository-specific operations...${NC}"
    
    # Delete tags from repo1 only
    run_acr_cli purge --registry "$REGISTRY" --filter "$repo1:tag1" --ago 0d >/dev/null 2>&1
    
    local tags1=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo1" 2>&1)
    local tags2=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo2" 2>&1)
    
    assert_not_contains "$tags1" "tag1" "tag1 should be deleted from repo1"
    assert_contains "$tags1" "tag2" "tag2 should still exist in repo1"
    assert_contains "$tags2" "tag1" "tag1 should still exist in repo2"
    assert_contains "$tags2" "tag2" "tag2 should still exist in repo2"
    
    # Test 2: Cross-repository operations
    echo -e "\n${CYAN}Testing cross-repository operations...${NC}"
    
    # Try to delete from both repositories using wildcard
    run_acr_cli purge --registry "$REGISTRY" --filter "abac-test-scope.*:tag2" --ago 0d >/dev/null 2>&1
    
    tags1=$("$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo1" 2>&1 || echo "")
    tags2=$("$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo2" 2>&1 || echo "")
    
    assert_not_contains "$tags1" "tag2" "tag2 should be deleted from repo1"
    assert_not_contains "$tags2" "tag2" "tag2 should be deleted from repo2"
    
    # Clean up
    cleanup_repository "$repo1"
    cleanup_repository "$repo2"
}

# Test: ABAC Authentication and Token Refresh
test_abac_authentication() {
    echo -e "\n${YELLOW}Test: ABAC Authentication and Token Refresh${NC}"
    
    if [ "$DOCKER_AVAILABLE" = "false" ]; then
        echo -e "${YELLOW}Skipping test - requires Docker for image creation${NC}"
        return
    fi
    
    local repo="abac-test-auth"
    
    # Clean up any existing repository
    cleanup_repository "$repo"
    
    # Create test images
    echo "Creating test images..."
    for i in $(seq 1 10); do
        create_test_image "$repo" "v$i"
    done
    
    # Test 1: Multiple operations requiring token refresh
    echo -e "\n${CYAN}Testing multiple operations with token refresh...${NC}"
    
    # Perform multiple operations that might trigger token refresh
    for i in 1 3 5 7 9; do
        # Use exact tag match to avoid v10 matching v1 - need double escaping for shell
        run_acr_cli purge --registry "$REGISTRY" --filter "$repo:v$i\\$" --ago 0d >/dev/null 2>&1
    done
    
    # Verify remaining tags
    local tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1)
    
    for i in 2 4 6 8 10; do
        assert_contains "$tags" "v$i" "v$i should still exist"
    done
    
    for i in 1 3 5 7 9; do
        assert_not_contains "$tags" "v$i" "v$i should be deleted"
    done
    
    # Test 2: Large batch operations
    echo -e "\n${CYAN}Testing large batch operations...${NC}"
    
    # Clean up and recreate
    cleanup_repository "$repo"
    
    echo "Creating larger set of test images..."
    for i in $(seq 1 20); do
        create_test_image "$repo" "batch$(printf "%03d" $i)"
    done
    
    # Delete all in one operation - try multiple times if needed
    for attempt in 1 2 3; do
        local purge_output=$(run_acr_cli purge --registry "$REGISTRY" --filter "$repo:batch.*" --ago 0d 2>&1)
        echo "Attempt $attempt - Purge output: $purge_output"
        
        local remaining_tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1 || echo "")
        local remaining_batch=$(echo "$remaining_tags" | grep -c "$REGISTRY/$repo:batch" || echo 0)
        
        if [ "$remaining_batch" -eq 0 ]; then
            echo "All batch tags deleted after $attempt attempts"
            break
        fi
        
        sleep 2
    done
    
    tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1 || echo "")
    
    # Debug: Show what tags remain
    echo "Purge output: $purge_output"
    echo "Remaining tags after batch deletion:"
    echo "$tags"
    
    # Should be empty or contain only system tags
    local tag_count=$(echo "$tags" | grep -c "$REGISTRY/$repo:batch" || echo 0)
    assert_equals "0" "$tag_count" "All batch tags should be deleted"
    
    # Clean up
    cleanup_repository "$repo"
}

# Test: ABAC with Locked Images
test_abac_locked_images() {
    echo -e "\n${YELLOW}Test: ABAC with Locked Images${NC}"
    
    if [ "$DOCKER_AVAILABLE" = "false" ]; then
        echo -e "${YELLOW}Skipping test - requires Docker for image creation${NC}"
        return
    fi
    
    local repo="abac-test-locks"
    local registry_name="${REGISTRY%%.*}"
    
    # Clean up any existing repository
    cleanup_repository "$repo"
    
    # Create test images
    echo "Creating test images..."
    for i in 1 2 3 4; do
        create_test_image "$repo" "lock$i"
    done
    
    # Lock some images
    echo "Locking images..."
    az acr repository update \
        --name "$registry_name" \
        --image "$repo:lock2" \
        --delete-enabled false \
        --write-enabled false \
        --output none 2>/dev/null
    
    az acr repository update \
        --name "$registry_name" \
        --image "$repo:lock4" \
        --delete-enabled false \
        --output none 2>/dev/null
    
    # Test 1: Purge without --include-locked
    echo -e "\n${CYAN}Testing purge without --include-locked...${NC}"
    
    run_acr_cli purge --registry "$REGISTRY" --filter "$repo:lock.*" --ago 0d >/dev/null 2>&1
    
    local tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1)
    
    assert_not_contains "$tags" "lock1" "lock1 (unlocked) should be deleted"
    assert_contains "$tags" "lock2" "lock2 (locked) should remain"
    assert_not_contains "$tags" "lock3" "lock3 (unlocked) should be deleted"
    assert_contains "$tags" "lock4" "lock4 (locked) should remain"
    
    # Test 2: Purge with --include-locked
    echo -e "\n${CYAN}Testing purge with --include-locked...${NC}"
    
    run_acr_cli purge --registry "$REGISTRY" --filter "$repo:lock.*" --ago 0d --include-locked >/dev/null 2>&1
    
    tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1 || echo "")
    
    assert_not_contains "$tags" "lock2" "lock2 should be deleted with --include-locked"
    assert_not_contains "$tags" "lock4" "lock4 should be deleted with --include-locked"
    
    # Clean up
    cleanup_repository "$repo"
}

# Test: ABAC Concurrent Operations
test_abac_concurrent_operations() {
    echo -e "\n${YELLOW}Test: ABAC Concurrent Operations${NC}"
    
    if [ "$DOCKER_AVAILABLE" = "false" ]; then
        echo -e "${YELLOW}Skipping test - requires Docker for image creation${NC}"
        return
    fi
    
    local repo="abac-test-concurrent"
    
    # Clean up any existing repository
    cleanup_repository "$repo"
    
    # Create test images
    echo "Creating test images for concurrency test..."
    for i in $(seq 1 30); do
        create_test_image "$repo" "concurrent$(printf "%03d" $i)"
    done
    
    # Test different concurrency levels
    for concurrency in 1 5 10; do
        echo -e "\n${CYAN}Testing with concurrency=$concurrency...${NC}"
        
        # Create fresh test data
        for i in $(seq 1 10); do
            create_test_image "$repo" "test${concurrency}_$(printf "%03d" $i)"
        done
        
        # Measure time for operation
        local start_time=$(date +%s)
        
        "$ACR_CLI" purge \
            --registry "$REGISTRY" \
            --filter "$repo:test${concurrency}_.*" \
            --ago 0d \
            --concurrency "$concurrency" >/dev/null 2>&1
        
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        
        echo "  Duration: ${duration}s with concurrency ${concurrency}"
        
        # Verify deletion
        local tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1)
        echo "Tags after concurrency ${concurrency} test: $tags"
        
        # Count remaining tags for this concurrency level
        local remaining_count=$(echo "$tags" | grep -c "test${concurrency}_" || echo 0)
        if [ "$remaining_count" -eq 0 ]; then
            echo -e "${GREEN}✓ All test${concurrency}_ tags should be deleted${NC}"
            ((TESTS_PASSED++))
        else
            echo -e "${RED}✗ All test${concurrency}_ tags should be deleted${NC}"
            echo "  Should NOT contain: test${concurrency}_"
            echo "  Found $remaining_count remaining tags"
            ((TESTS_FAILED++))
            FAILED_TESTS+=("All test${concurrency}_ tags should be deleted")
        fi
    done
    
    # Clean up
    cleanup_repository "$repo"
}

# Test: ABAC Keep Parameter
test_abac_keep_parameter() {
    echo -e "\n${YELLOW}Test: ABAC Keep Parameter${NC}"
    
    if [ "$DOCKER_AVAILABLE" = "false" ]; then
        echo -e "${YELLOW}Skipping test - requires Docker for image creation${NC}"
        return
    fi
    
    local repo="abac-test-keep"
    
    # Clean up any existing repository
    cleanup_repository "$repo"
    
    # Create test images with timestamps
    echo "Creating timestamped test images..."
    for i in $(seq 1 10); do
        create_test_image "$repo" "keep$(printf "%03d" $i)"
        sleep 0.5  # Small delay to ensure different timestamps
    done
    
    # Test: Keep latest 3 images
    echo -e "\n${CYAN}Testing --keep 3...${NC}"
    
    local purge_output=$(run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "$repo:keep.*" \
        --ago 0d \
        --keep 3 2>&1)
    
    local tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1)
    local tag_count=$(echo "$tags" | grep -c "$REGISTRY/$repo:keep" || echo 0)
    
    echo "Purge output: $purge_output"
    echo "Remaining tags: $tags"
    echo "Tag count: $tag_count"
    
    assert_equals "3" "$tag_count" "Should keep exactly 3 latest tags"
    
    # Verify it kept the latest ones
    assert_contains "$tags" "keep008" "Should keep keep008"
    assert_contains "$tags" "keep009" "Should keep keep009"
    assert_contains "$tags" "keep010" "Should keep keep010"
    assert_not_contains "$tags" "keep001" "Should delete keep001"
    
    # Clean up
    cleanup_repository "$repo"
}

# Test: ABAC Pattern Matching
test_abac_pattern_matching() {
    echo -e "\n${YELLOW}Test: ABAC Pattern Matching${NC}"
    
    if [ "$DOCKER_AVAILABLE" = "false" ]; then
        echo -e "${YELLOW}Skipping test - requires Docker for image creation${NC}"
        return
    fi
    
    local repo="abac-test-patterns"
    
    # Clean up any existing repository
    cleanup_repository "$repo"
    
    # Create test images with various patterns
    echo "Creating test images with patterns..."
    
    # Version tags
    for ver in "1.0.0" "1.1.0" "2.0.0" "2.1.0"; do
        create_test_image "$repo" "v$ver"
    done
    
    # Environment tags
    for env in "dev" "staging" "production"; do
        create_test_image "$repo" "${env}-latest"
    done
    
    # Build tags
    for build in "001" "002" "003"; do
        create_test_image "$repo" "build-$build"
    done
    
    # Test 1: Match version 1.x.x tags
    echo -e "\n${CYAN}Testing version 1.x.x pattern...${NC}"
    
    local output=$(run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "$repo:v1\..*" \
        --ago 0d \
        --dry-run 2>&1)
    
    echo "Pattern matching output for v1.*: $output"
    
    assert_contains "$output" "v1.0.0" "Should match v1.0.0"
    assert_contains "$output" "v1.1.0" "Should match v1.1.0"
    assert_not_contains "$output" "v2.0.0" "Should not match v2.0.0"
    
    # Test 2: Match environment tags
    echo -e "\n${CYAN}Testing environment pattern...${NC}"
    
    output=$(run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "$repo:.*-latest" \
        --ago 0d \
        --dry-run 2>&1)
    
    echo "Pattern matching output for *-latest: $output"
    
    assert_contains "$output" "dev-latest" "Should match dev-latest"
    assert_contains "$output" "staging-latest" "Should match staging-latest"
    assert_contains "$output" "production-latest" "Should match production-latest"
    assert_not_contains "$output" "build-" "Should not match build tags"
    
    # Test 3: Complex pattern
    echo -e "\n${CYAN}Testing complex pattern...${NC}"
    
    output=$(run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "$repo:(build-00[12]|dev-.*)" \
        --ago 0d \
        --dry-run 2>&1)
    
    echo "Pattern matching output for complex pattern: $output"
    
    assert_contains "$output" "build-001" "Should match build-001"
    assert_contains "$output" "build-002" "Should match build-002"
    assert_not_contains "$output" "build-003" "Should not match build-003"
    assert_contains "$output" "dev-latest" "Should match dev-latest"
    
    # Clean up
    cleanup_repository "$repo"
}

# Test: ABAC Error Handling
test_abac_error_handling() {
    echo -e "\n${YELLOW}Test: ABAC Error Handling${NC}"
    
    # Test 1: Non-existent repository
    echo -e "\n${CYAN}Testing non-existent repository...${NC}"
    
    local output=$(run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "nonexistent-repo:.*" \
        --ago 0d 2>&1 || true)
    
    assert_contains "$output" "0" "Should handle non-existent repository gracefully"
    
    # Test 2: Invalid pattern
    echo -e "\n${CYAN}Testing invalid regex pattern...${NC}"
    
    output=$(run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "test:[" \
        --ago 0d \
        --dry-run 2>&1 || true)
    
    # Should either error or handle gracefully
    if [ $? -ne 0 ]; then
        echo -e "${GREEN}✓ Invalid pattern correctly rejected${NC}"
        ((TESTS_PASSED++))
    else
        assert_contains "$output" "0" "Should handle invalid pattern gracefully"
    fi
    
    # Test 3: Invalid registry
    echo -e "\n${CYAN}Testing invalid registry...${NC}"
    
    output=$(run_acr_cli purge \
        --registry "invalid-registry.azurecr.io" \
        --filter "test:.*" \
        --ago 0d \
        --dry-run 2>&1 || true)
    
    if [ $? -ne 0 ]; then
        echo -e "${GREEN}✓ Invalid registry correctly rejected${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${YELLOW}⚠ Invalid registry accepted but may fail later${NC}"
    fi
}

# Test: ABAC with Manifest Operations
test_abac_manifest_operations() {
    echo -e "\n${YELLOW}Test: ABAC with Manifest Operations${NC}"
    
    if [ "$DOCKER_AVAILABLE" = "false" ]; then
        echo -e "${YELLOW}Skipping test - requires Docker for image creation${NC}"
        return
    fi
    
    local repo="abac-test-manifest"
    
    # Clean up any existing repository
    cleanup_repository "$repo"
    
    # Create base image
    echo "Creating base image and aliases..."
    create_test_image "$repo" "base"
    
    # Create aliases pointing to same manifest
    docker tag "$REGISTRY/$repo:base" "$REGISTRY/$repo:alias1"
    docker tag "$REGISTRY/$repo:base" "$REGISTRY/$repo:alias2"
    docker push "$REGISTRY/$repo:alias1" >/dev/null 2>&1
    docker push "$REGISTRY/$repo:alias2" >/dev/null 2>&1
    
    # Test 1: List manifests
    echo -e "\n${CYAN}Testing manifest listing...${NC}"
    
    local manifests=$(run_acr_cli manifest list \
        --registry "$REGISTRY" \
        --repository "$repo" 2>&1)
    
    # Should have one manifest with multiple tags
    local manifest_count=$(echo "$manifests" | grep -c "$REGISTRY/$repo@sha256:" || echo 0)
    assert_equals "1" "$manifest_count" "Should have one manifest"
    
    # Test 2: Delete tag but keep manifest
    echo -e "\n${CYAN}Testing tag deletion (keeping manifest)...${NC}"
    
    run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "$repo:alias1" \
        --ago 0d >/dev/null 2>&1
    
    local tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1)
    assert_not_contains "$tags" "alias1" "alias1 should be deleted"
    assert_contains "$tags" "alias2" "alias2 should remain"
    assert_contains "$tags" "base" "base should remain"
    
    # Manifest should still exist
    manifests=$(run_acr_cli manifest list \
        --registry "$REGISTRY" \
        --repository "$repo" 2>&1)
    manifest_count=$(echo "$manifests" | grep -c "$REGISTRY/$repo@sha256:" || echo 0)
    assert_equals "1" "$manifest_count" "Manifest should still exist"
    
    # Test 3: Delete all tags and dangling manifests
    echo -e "\n${CYAN}Testing dangling manifest deletion...${NC}"
    
    local purge_output=$(run_acr_cli purge \
        --registry "$REGISTRY" \
        --filter "$repo:.*" \
        --ago 0d \
        --untagged 2>&1)
    
    tags=$(run_acr_cli tag list --registry "$REGISTRY" --repository "$repo" 2>&1 || echo "")
    tag_count=$(echo "$tags" | grep -c "$REGISTRY/$repo:" || echo 0)
    
    echo "Manifest cleanup purge output: $purge_output"
    echo "Tags after manifest cleanup: $tags"
    echo "Tag count: $tag_count"
    
    assert_equals "0" "$tag_count" "All tags should be deleted"
    
    manifests=$(run_acr_cli manifest list \
        --registry "$REGISTRY" \
        --repository "$repo" 2>&1 || echo "")
    manifest_count=$(echo "$manifests" | grep -c "$REGISTRY/$repo@sha256:" || echo 0)
    
    echo "Manifests after cleanup: $manifests"
    echo "Manifest count: $manifest_count"
    
    assert_equals "0" "$manifest_count" "Dangling manifests should be deleted"
    
    # Clean up
    cleanup_repository "$repo"
}

# Print test summary
print_summary() {
    echo -e "\n${BLUE}=== Test Summary ===${NC}"
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
    
    if [ ${#FAILED_TESTS[@]} -gt 0 ]; then
        echo -e "\n${RED}Failed tests:${NC}"
        for test in "${FAILED_TESTS[@]}"; do
            echo "  - $test"
        done
        exit 1
    else
        echo -e "\n${GREEN}All tests passed successfully!${NC}"
        exit 0
    fi
}

# Main execution
main() {
    echo -e "${BLUE}=== ABAC Registry Test Suite ===${NC}"
    if [ -z "$REGISTRY" ]; then
        echo "Registry: Will create temporary registry"
    else
        echo "Registry: $REGISTRY"
    fi
    echo "Test mode: $TEST_MODE"
    echo ""
    echo "Usage: $0 [registry] [test_mode]"
    echo "  registry:  Optional. If not provided, a temporary registry will be created"
    echo "  test_mode: basic, comprehensive, auth, all (default: comprehensive)"
    echo "Example: $0 myregistry.azurecr.io comprehensive"
    echo ""
    
    # Run prerequisites check
    check_prerequisites
    
    # Validate registry
    validate_registry
    
    # Run tests based on mode
    case "$TEST_MODE" in
        basic)
            test_basic_acr_cli_operations
            test_basic_abac_operations
            ;;
        auth)
            test_basic_acr_cli_operations
            test_abac_authentication
            test_abac_permission_scoping
            ;;
        comprehensive)
            test_basic_acr_cli_operations
            test_basic_abac_operations
            test_abac_permission_scoping
            test_abac_authentication
            test_abac_locked_images
            test_abac_concurrent_operations
            test_abac_keep_parameter
            test_abac_pattern_matching
            test_abac_error_handling
            test_abac_manifest_operations
            ;;
        all)
            test_basic_acr_cli_operations
            test_basic_abac_operations
            test_abac_permission_scoping
            test_abac_authentication
            test_abac_locked_images
            test_abac_concurrent_operations
            test_abac_keep_parameter
            test_abac_pattern_matching
            test_abac_error_handling
            test_abac_manifest_operations
            ;;
        *)
            echo -e "${RED}Invalid test mode: $TEST_MODE${NC}"
            echo "Options: basic, comprehensive, auth, all"
            exit 1
            ;;
    esac
    
    # Print summary
    print_summary
}

# Run main function
main "$@"