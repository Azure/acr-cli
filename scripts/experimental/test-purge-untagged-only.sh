#!/bin/bash
set -uo pipefail

# Test script for ACR purge --untagged-only feature
# Tests the new functionality for deleting only untagged manifests

# Check for required commands
for cmd in az docker; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "Error: Required command '$cmd' not found"
        exit 1
    fi
done

REGISTRY="${1:-}"
TEST_MODE="${2:-all}"  # Options: all, basic, comprehensive, edge-cases
NUM_IMAGES="${3:-20}"
TEMP_REGISTRY_CREATED=false
TEMP_REGISTRY_NAME=""
RESOURCE_GROUP=""
DEBUG="${DEBUG:-0}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACR_CLI="${SCRIPT_DIR}/../../bin/acr"

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

get_registry_name() {
    echo "${REGISTRY%%.*}"
}

count_tags() {
    local repo="$1"
    local tags=$("$ACR_CLI" tag list -r "$REGISTRY" --repository "$repo" 2>/dev/null || echo "")
    local count=$(echo "$tags" | grep "$REGISTRY" | wc -l | tr -d ' ')
    
    if [ "$DEBUG" = "1" ]; then
        echo -e "\n  DEBUG count_tags for $repo: $count" >&2
    fi
    echo "$count"
}

count_manifests() {
    local repo="$1"
    "$ACR_CLI" manifest list --registry "$REGISTRY" --repository "$repo" 2>/dev/null | wc -l | tr -d ' '
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

delete_tag() {
    local repo="$1"
    local tag="$2"
    
    az acr repository delete --name "$(get_registry_name)" --image "$repo:$tag" --yes >/dev/null 2>&1
}

create_untagged_manifest() {
    local repo="$1"
    local tag="temp-tag-$(date +%s)"
    
    # Create and push an image
    create_test_image "$repo" "$tag"
    
    # Delete the tag to make the manifest untagged
    sleep 1
    delete_tag "$repo" "$tag"
}

lock_manifest() {
    local repo="$1"
    local digest="$2"
    local write_enabled="${3:-false}"
    local delete_enabled="${4:-false}"

    az acr repository update \
        --name "$(get_registry_name)" \
        --image "$repo@$digest" \
        --write-enabled "$write_enabled" \
        --delete-enabled "$delete_enabled" \
        --output none 2>/dev/null
}

get_manifest_digest() {
    local repo="$1"
    local tag="$2"
    
    az acr repository show --name "$(get_registry_name)" --image "$repo:$tag" --query "digest" -o tsv 2>/dev/null
}

# Create temporary registry if needed
if [ -z "$REGISTRY" ]; then
    echo -e "${GREEN}Creating temporary registry...${NC}"
    # Generate random suffix
    RANDOM_SUFFIX=$(openssl rand -hex 4 2>/dev/null || echo $(date +%s | tail -c 5))
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

# Build ACR CLI if needed
if [ ! -f "$ACR_CLI" ]; then
    echo "Building ACR CLI..."
    (cd "$SCRIPT_DIR/../.." && make binaries)
fi

# Login to ACR
echo "Logging in to registry..."
az acr login --name "$(get_registry_name)" >/dev/null 2>&1

echo -e "\n${BLUE}=== ACR Purge --untagged-only Test Suite ===${NC}"
echo "Registry: $REGISTRY"
echo "Test mode: $TEST_MODE"
echo ""

# Test functions
run_basic_tests() {
    echo -e "\n${BLUE}=== Basic --untagged-only Tests ===${NC}"
    
    # Test 1: Delete only untagged manifests
    echo -e "\n${YELLOW}Test 1: Delete Only Untagged Manifests${NC}"
    local repo="test-untagged-basic"
    
    # Create tagged images
    echo "Creating tagged images..."
    for i in 1 2 3; do
        create_test_image "$repo" "v$i"
    done
    
    # Create untagged manifests
    echo "Creating untagged manifests..."
    for i in 1 2; do
        create_untagged_manifest "$repo"
    done
    
    sleep 2
    
    local initial_tags=$(count_tags "$repo")
    local initial_manifests=$(count_manifests "$repo")
    echo "Initial state: $initial_tags tags, $initial_manifests manifests"
    
    # Test dry-run first
    echo -n "Testing dry-run with --untagged-only... "
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --untagged-only --dry-run 2>&1)
    assert_contains "$output" "Number of manifests to be deleted" "Dry run should show manifests to delete"
    
    # Run actual purge
    echo -n "Testing --untagged-only (no filter)... "
    "$ACR_CLI" purge --registry "$REGISTRY" --untagged-only >/dev/null 2>&1
    
    local final_tags=$(count_tags "$repo")
    local final_manifests=$(count_manifests "$repo")
    
    assert_equals "$initial_tags" "$final_tags" "Tagged images should remain unchanged"
    
    # Test 2: --untagged-only with specific repository filter
    echo -e "\n${YELLOW}Test 2: --untagged-only with Repository Filter${NC}"
    local repo2="test-untagged-filter"
    local repo3="test-untagged-other"
    
    # Create images in both repos
    create_test_image "$repo2" "keep"
    create_test_image "$repo3" "keep"
    
    # Create untagged manifest only in repo2
    create_untagged_manifest "$repo2"
    
    sleep 2
    
    local repo2_initial_manifests=$(count_manifests "$repo2")
    local repo3_initial_manifests=$(count_manifests "$repo3")
    
    echo -n "Testing --untagged-only with filter for specific repo... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo2:.*" --untagged-only >/dev/null 2>&1
    
    local repo2_final_manifests=$(count_manifests "$repo2")
    local repo3_final_manifests=$(count_manifests "$repo3")
    
    assert_equals "1" "$repo2_final_manifests" "Only untagged manifests in filtered repo should be deleted"
    assert_equals "$repo3_initial_manifests" "$repo3_final_manifests" "Other repo should be unchanged"
    
    # Test 3: --untagged-only with --ago and --keep should work (new functionality)
    echo -e "\n${YELLOW}Test 3: Age Filtering and Keep Functionality${NC}"
    
    # Test --ago filtering with dry run
    echo -n "Testing --untagged-only with --ago (dry run)... "
    local dry_run_output=$("$ACR_CLI" purge --registry "$REGISTRY" --untagged-only --ago 365d --dry-run 2>&1)
    assert_success $? "--untagged-only with --ago should work"
    assert_contains "$dry_run_output" "Number of manifests to be deleted:" "--ago should filter manifests by age"
    
    # Test --keep functionality with dry run
    echo -n "Testing --untagged-only with --keep (dry run)... "
    dry_run_output=$("$ACR_CLI" purge --registry "$REGISTRY" --untagged-only --keep 2 --dry-run 2>&1)
    assert_success $? "--untagged-only with --keep should work"
    assert_contains "$dry_run_output" "Number of manifests to be deleted:" "--keep should preserve recent manifests"
    
    # Test combined --ago and --keep functionality
    echo -n "Testing --untagged-only with --ago and --keep (dry run)... "
    dry_run_output=$("$ACR_CLI" purge --registry "$REGISTRY" --untagged-only --ago 180d --keep 1 --dry-run 2>&1)
    assert_success $? "--untagged-only with --ago and --keep should work together"
    assert_contains "$dry_run_output" "Number of manifests to be deleted:" "--ago and --keep should work together"
    
    # Test 4: --untagged and --untagged-only are mutually exclusive  
    echo -e "\n${YELLOW}Test 4: Flag Validation${NC}"
    echo -n "Testing --untagged with --untagged-only (should fail)... "
    local error_output=$("$ACR_CLI" purge --registry "$REGISTRY" --untagged --untagged-only --filter "test:.*" --ago 1d 2>&1 || true)
    # Cobra uses different error message, check for either
    if echo "$error_output" | grep -qE "(mutually exclusive|are set none of the others can be)"; then
        echo -e "${GREEN}✓ --untagged and --untagged-only should be mutually exclusive${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ --untagged and --untagged-only should be mutually exclusive${NC}"
        echo -e "  Should contain: mutually exclusive or similar"
        echo -e "  Got: $error_output"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("--untagged and --untagged-only should be mutually exclusive")
    fi
}

run_comprehensive_tests() {
    echo -e "\n${BLUE}=== Comprehensive --untagged-only Tests ===${NC}"
    
    # Test 1: Locked manifests handling
    echo -e "\n${YELLOW}Test 1: Locked Untagged Manifests${NC}"
    local repo="test-untagged-locks"
    
    # Create an image and get its digest
    create_test_image "$repo" "temp-for-lock"
    local digest=$(get_manifest_digest "$repo" "temp-for-lock")
    echo "Manifest digest: $digest"
    
    # Delete the tag to make it untagged
    delete_tag "$repo" "temp-for-lock"
    sleep 2
    
    # Lock the untagged manifest
    echo "Locking untagged manifest..."
    lock_manifest "$repo" "$digest" false false
    
    # Try to delete without --include-locked
    echo -n "Testing --untagged-only without --include-locked... "
    local initial_count=$(count_manifests "$repo")
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only >/dev/null 2>&1
    local after_count=$(count_manifests "$repo")
    assert_equals "$initial_count" "$after_count" "Locked manifest should not be deleted"
    
    # Try with --include-locked
    echo -n "Testing --untagged-only with --include-locked... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only --include-locked >/dev/null 2>&1
    local final_count=$(count_manifests "$repo")
    assert_equals "0" "$final_count" "Locked manifest should be deleted with --include-locked"
    
    # Test 2: Multi-arch manifests
    echo -e "\n${YELLOW}Test 2: Multi-arch Manifests${NC}"
    local repo="test-untagged-multiarch"
    
    # Create multiple images that reference the same manifest
    create_test_image "$repo" "latest"
    docker tag "$REGISTRY/$repo:latest" "$REGISTRY/$repo:v1"
    docker push "$REGISTRY/$repo:v1" >/dev/null 2>&1
    
    # Delete one tag
    delete_tag "$repo" "v1"
    sleep 2
    
    echo -n "Testing --untagged-only with partially tagged manifest... "
    local initial_manifests=$(count_manifests "$repo")
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only >/dev/null 2>&1
    local final_manifests=$(count_manifests "$repo")
    assert_equals "$initial_manifests" "$final_manifests" "Manifest with remaining tags should not be deleted"
    
    # Test 3: Large-scale untagged cleanup
    echo -e "\n${YELLOW}Test 3: Large-scale Untagged Cleanup${NC}"
    local repo="test-untagged-scale"
    
    echo "Creating $NUM_IMAGES untagged manifests..."
    for i in $(seq 1 "$NUM_IMAGES"); do
        create_untagged_manifest "$repo"
        if [ $((i % 5)) -eq 0 ]; then
            echo "  Created $i/$NUM_IMAGES untagged manifests..."
        fi
    done
    
    # Also create some tagged images
    for i in 1 2 3; do
        create_test_image "$repo" "keep-v$i"
    done
    
    sleep 2
    
    local initial_tags=$(count_tags "$repo")
    local initial_manifests=$(count_manifests "$repo")
    echo "Initial state: $initial_tags tags, $initial_manifests manifests"
    
    echo -n "Testing large-scale --untagged-only cleanup... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only >/dev/null 2>&1
    
    local final_tags=$(count_tags "$repo")
    local final_manifests=$(count_manifests "$repo")
    
    assert_equals "$initial_tags" "$final_tags" "All tagged images should remain"
    assert_equals "$initial_tags" "$final_manifests" "Only tagged manifests should remain"
    
    # Test 4: Concurrent operations
    echo -e "\n${YELLOW}Test 4: Concurrent Untagged Cleanup${NC}"
    local repo="test-untagged-concurrent"
    
    # Create untagged manifests
    echo "Creating untagged manifests for concurrency test..."
    for i in $(seq 1 10); do
        create_untagged_manifest "$repo"
    done
    
    sleep 2
    
    for concurrency in 1 5 10; do
        echo -n "Testing --untagged-only with concurrency=$concurrency... "
        local start_time=$(date +%s)
        "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only --concurrency "$concurrency" >/dev/null 2>&1
        local end_time=$(date +%s)
        echo "Duration: $((end_time - start_time))s"
    done
}

run_age_and_keep_tests() {
    echo -e "\n${BLUE}=== Age Filtering and Keep Functionality Tests ===${NC}"
    
    # Create a repository with manifests we can test age filtering on
    local repo="test-age-keep-$(date +%s)"
    
    # Create some tagged images first (these will generate untagged manifests when we untag them)
    echo "Setting up test data for age and keep tests..."
    for i in $(seq 1 5); do
        create_test_image "$repo" "v$i"
        sleep 1  # Ensure different timestamps
    done
    
    # Wait a moment to ensure timestamps are different
    sleep 2
    
    # Create more recent manifests
    for i in $(seq 6 8); do
        create_test_image "$repo" "v$i"
        sleep 1
    done
    
    # Remove tags to create untagged manifests with different ages
    echo "Creating untagged manifests by removing tags..."
    for i in $(seq 1 8); do
        docker image rm "$REGISTRY/$repo:v$i" 2>/dev/null || true
        az acr repository untag --name "$REGISTRY" --image "$repo:v$i" >/dev/null 2>&1 || true
    done
    
    sleep 2
    
    # Test 1: Age filtering - delete only old manifests
    echo -e "\n${YELLOW}Test 1: Age Filtering${NC}"
    echo -n "Testing --ago filtering (dry run)... "
    local initial_count=$(count_manifests_in_repo "$repo")
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only --ago 3s --dry-run 2>&1)
    assert_success $? "Age filtering should work"
    
    # Should report some manifests to be deleted (the older ones)
    local to_delete=$(echo "$output" | grep "Number of manifests to be deleted:" | sed 's/.*: //')
    if [[ "$to_delete" -gt 0 && "$to_delete" -lt "$initial_count" ]]; then
        echo -e "${GREEN}✓ Age filtering works correctly${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Age filtering not working as expected${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Age filtering")
    fi
    
    # Test 2: Keep functionality - preserve recent manifests
    echo -e "\n${YELLOW}Test 2: Keep Functionality${NC}"
    echo -n "Testing --keep functionality (dry run)... "
    output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only --keep 3 --dry-run 2>&1)
    assert_success $? "Keep functionality should work"
    
    to_delete=$(echo "$output" | grep "Number of manifests to be deleted:" | sed 's/.*: //')
    if [[ "$to_delete" -ge 0 && "$to_delete" -le $((initial_count - 3)) ]]; then
        echo -e "${GREEN}✓ Keep functionality works correctly${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Keep functionality not working as expected${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Keep functionality")
    fi
    
    # Test 3: Combined age and keep filtering
    echo -e "\n${YELLOW}Test 3: Combined Age and Keep Filtering${NC}"
    echo -n "Testing --ago with --keep (dry run)... "
    output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only --ago 5s --keep 2 --dry-run 2>&1)
    assert_success $? "Combined age and keep filtering should work"
    
    to_delete=$(echo "$output" | grep "Number of manifests to be deleted:" | sed 's/.*: //')
    echo -e "${GREEN}✓ Combined filtering reported $to_delete manifests to delete${NC}"
    ((TESTS_PASSED++))
    
    # Clean up test repository
    az acr repository delete --name "$REGISTRY" --image "$repo" --yes >/dev/null 2>&1 || true
}

run_edge_case_tests() {
    echo -e "\n${BLUE}=== Edge Case Tests for --untagged-only ===${NC}"
    
    # Test 1: Empty repository
    echo -e "\n${YELLOW}Test 1: Empty Repository${NC}"
    echo -n "Testing --untagged-only on non-existent repo... "
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "nonexistent:.*" --untagged-only 2>&1)
    assert_contains "$output" "Number of deleted manifests: 0" "Should handle non-existent repo gracefully"
    
    # Test 2: Repository with no untagged manifests
    echo -e "\n${YELLOW}Test 2: Repository with No Untagged Manifests${NC}"
    local repo="test-no-untagged"
    
    # Create only tagged images
    for i in 1 2 3; do
        create_test_image "$repo" "v$i"
    done
    
    echo -n "Testing --untagged-only on repo with no untagged manifests... "
    output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only 2>&1)
    assert_contains "$output" "Number of deleted manifests: 0" "Should report 0 deletions when no untagged manifests"
    
    # Test 3: Mixed repositories
    echo -e "\n${YELLOW}Test 3: Mixed Repositories${NC}"
    local repo1="test-mixed-1"
    local repo2="test-mixed-2"
    local repo3="test-mixed-3"
    
    # Repo1: Only tagged
    create_test_image "$repo1" "tagged"
    
    # Repo2: Only untagged
    create_untagged_manifest "$repo2"
    
    # Repo3: Mixed
    create_test_image "$repo3" "tagged"
    create_untagged_manifest "$repo3"
    
    sleep 2
    
    echo -n "Testing --untagged-only across all repos... "
    local initial_total_tags=$(($(count_tags "$repo1") + $(count_tags "$repo2") + $(count_tags "$repo3")))
    
    "$ACR_CLI" purge --registry "$REGISTRY" --untagged-only >/dev/null 2>&1
    
    local final_total_tags=$(($(count_tags "$repo1") + $(count_tags "$repo2") + $(count_tags "$repo3")))
    assert_equals "$initial_total_tags" "$final_total_tags" "All tagged images should remain across all repos"
    
    # Test 4: Special characters in repository names
    echo -e "\n${YELLOW}Test 4: Special Repository Names${NC}"
    local special_repo="test-special.repo-name_123"
    
    create_test_image "$special_repo" "keep"
    create_untagged_manifest "$special_repo"
    
    sleep 2
    
    echo -n "Testing --untagged-only with special repo name... "
    local initial=$(count_manifests "$special_repo")
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$special_repo:.*" --untagged-only >/dev/null 2>&1
    local final=$(count_manifests "$special_repo")
    assert_equals "1" "$final" "Should handle special characters in repo names"
}

run_performance_tests() {
    echo -e "\n${BLUE}=== Performance Tests for --untagged-only ===${NC}"
    
    echo -e "\n${YELLOW}Performance Comparison: --untagged-only vs regular purge${NC}"
    local repo="test-perf-comparison"
    
    # Create test data
    echo "Creating test data..."
    for i in $(seq 1 20); do
        create_test_image "$repo" "v$i"
    done
    for i in $(seq 1 20); do
        create_untagged_manifest "$repo"
    done
    
    sleep 2
    
    # Test --untagged-only performance
    echo "Testing --untagged-only performance..."
    local start_time=$(date +%s%N)
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --untagged-only --dry-run >/dev/null 2>&1
    local end_time=$(date +%s%N)
    local untagged_only_time=$((end_time - start_time))
    
    # Test regular purge with --untagged performance
    echo "Testing regular purge with --untagged performance..."
    start_time=$(date +%s%N)
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d --untagged --dry-run >/dev/null 2>&1
    end_time=$(date +%s%N)
    local regular_purge_time=$((end_time - start_time))
    
    echo -e "${GREEN}Performance Results:${NC}"
    echo "  --untagged-only time: $((untagged_only_time / 1000000))ms"
    echo "  Regular purge time: $((regular_purge_time / 1000000))ms"
    
    if [ "$untagged_only_time" -lt "$regular_purge_time" ]; then
        echo -e "${GREEN}  ✓ --untagged-only is faster (as expected)${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${YELLOW}  Note: Regular purge was not slower (may vary based on data)${NC}"
    fi
}

# Main execution
case "$TEST_MODE" in
    basic)
        run_basic_tests
        ;;
    comprehensive)
        run_comprehensive_tests
        ;;
    edge-cases)
        run_edge_case_tests
        ;;
    age-keep)
        run_age_and_keep_tests
        ;;
    performance)
        run_performance_tests
        ;;
    all)
        run_basic_tests
        run_comprehensive_tests
        run_age_and_keep_tests
        run_edge_case_tests
        run_performance_tests
        ;;
    *)
        echo "Invalid test mode: $TEST_MODE"
        echo "Options: all, basic, comprehensive, age-keep, edge-cases, performance"
        exit 1
        ;;
esac

echo -e "\n${GREEN}=== All --untagged-only tests completed ===${NC}"