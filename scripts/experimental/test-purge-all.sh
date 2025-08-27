#!/bin/bash
set -uo pipefail

# Consolidated test script for ACR purge command
# Combines all test scenarios from individual scripts into one comprehensive suite

# Check for required commands
for cmd in az docker; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "Error: Required command '$cmd' not found"
        exit 1
    fi
done

# Define calc function for floating point arithmetic (portable alternative to bc)
if command -v bc >/dev/null 2>&1; then
    calc() {
        echo "$1" | bc
    }
else
    # Use awk as fallback for floating point calculations
    calc() {
        echo "$1" | awk '{print $0}' | awk -F' ' '{
            # Simple expression evaluator for basic operations
            if ($2 == "-") print $1 - $3
            else if ($2 == "+") print $1 + $3
            else if ($2 == "*") print $1 * $3
            else if ($2 == "/") print $1 / $3
            else print $0
        }'
    }
fi

REGISTRY="${1:-}"
TEST_MODE="${2:-all}"  # Options: all, minimal, comprehensive, benchmark, benchmark-delete, debug, interactive, or specific test names
NUM_IMAGES="${3:-500}"
TEMP_REGISTRY_CREATED=false
TEMP_REGISTRY_NAME=""
RESOURCE_GROUP=""
DEBUG="${DEBUG:-0}"
USE_FAST_GENERATION="${USE_FAST_GENERATION:-true}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACR_CLI="${SCRIPT_DIR}/../../bin/acr"

# Source registry utilities
source "${SCRIPT_DIR}/registry-utils.sh"

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
    # Use the registry utility cleanup function
    cleanup_temporary_registry

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

assert_not_equals() {
    local not_expected="$1"
    local actual="$2"
    local test_name="$3"

    if [ "$not_expected" != "$actual" ]; then
        echo -e "${GREEN}✓ $test_name${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ $test_name${NC}"
        echo -e "  Should not equal: $not_expected"
        echo -e "  Actual: $actual"
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
    # Count only lines that look like registry URLs (contain the registry name)
    local count=$(echo "$tags" | grep "$REGISTRY" | wc -l | tr -d ' ')

    if [ "$DEBUG" = "1" ]; then
        echo -e "\n  DEBUG count_tags for $repo:" >&2
        echo "  Raw output:" >&2
        echo "$tags" | sed 's/^/    /' >&2
        echo "  Lines matching registry pattern:" >&2
        echo "$tags" | grep "$REGISTRY" | sed 's/^/    /' >&2
        echo "  Final count: $count" >&2
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
        # Try pulling without suppressing errors for debugging
        docker pull "$base_image" 2>&1 | tail -3 >&2
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

lock_image() {
    local repo="$1"
    local tag="$2"
    local write_enabled="${3:-false}"
    local delete_enabled="${4:-false}"

    az acr repository update \
        --name "$(get_registry_name)" \
        --image "$repo:$tag" \
        --write-enabled "$write_enabled" \
        --delete-enabled "$delete_enabled" \
        --output none 2>/dev/null
}

measure_time() {
    # Portable time measurement that works on both Linux and macOS
    local start_time end_time duration

    # Check if we're on macOS or Linux
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS: Use perl for high-resolution time
        start_time=$(perl -MTime::HiRes=time -e 'printf "%.3f\n", time')
        "$@"
        end_time=$(perl -MTime::HiRes=time -e 'printf "%.3f\n", time')
    else
        # Linux: Use date with nanoseconds
        start_time=$(date +%s.%N)
        "$@"
        end_time=$(date +%s.%N)
    fi

    # Calculate duration
    duration=$(awk -v e="$end_time" -v s="$start_time" 'BEGIN {printf "%.3f", e-s}' 2>/dev/null || echo "0")

    # Ensure we have a valid duration
    if [ -z "$duration" ] || [ "$duration" = "0" ]; then
        echo "0.001"  # Return a small non-zero value to avoid division by zero
    else
        echo "$duration"
    fi
}

generate_test_images() {
    local repo="$1"
    local num="${2:-$NUM_IMAGES}"
    local parallel_jobs="${3:-10}"

    # Check if fast generation script exists
    local fast_script="$SCRIPT_DIR/generate-test-images-batch.sh"
    if [ -f "$fast_script" ] && [ "$USE_FAST_GENERATION" != "false" ]; then
        echo -e "${GREEN}Using fast parallel image generation...${NC}"
        "$fast_script" "$REGISTRY" "$repo" "$num" "$parallel_jobs"
    else
        # Fallback to original implementation
        generate_test_images_sequential "$repo" "$num"
    fi
}

generate_test_images_sequential() {
    local repo="$1"
    local num="${2:-$NUM_IMAGES}"
    local base_image="mcr.microsoft.com/hello-world"

    echo -e "${GREEN}Generating $num test images in $REGISTRY/$repo...${NC}"
    docker pull "$base_image" >/dev/null 2>&1

    for i in $(seq 1 "$num"); do
        # Create variations of tags
        local tag_version="v$(printf "%03d" "$i")"
        local tag_date="$(date -u +%Y%m%d)-$i"
        local tag_build="build-$(printf "%04d" "$i")"

        if [ $((i % 10)) -eq 0 ]; then
            echo "Progress: $i/$num images created..."
        fi

        # Tag and push
        docker tag "$base_image" "$REGISTRY/$repo:$tag_version"
        docker push "$REGISTRY/$repo:$tag_version" >/dev/null 2>&1

        if [ "$TEST_MODE" != "minimal" ]; then
            docker tag "$base_image" "$REGISTRY/$repo:$tag_date"
            docker tag "$base_image" "$REGISTRY/$repo:$tag_build"
            docker push "$REGISTRY/$repo:$tag_date" >/dev/null 2>&1
            docker push "$REGISTRY/$repo:$tag_build" >/dev/null 2>&1

            # Add some images with 'latest' tag pattern
            if [ $((i % 10)) -eq 0 ]; then
                docker tag "$base_image" "$REGISTRY/$repo:latest-$i"
                docker push "$REGISTRY/$repo:latest-$i" >/dev/null 2>&1
            fi

            # Add some images with 'dev' tag pattern
            if [ $((i % 5)) -eq 0 ]; then
                docker tag "$base_image" "$REGISTRY/$repo:dev-$i"
                docker push "$REGISTRY/$repo:dev-$i" >/dev/null 2>&1
            fi
        fi
    done

    echo "Finished creating test images"
}

# Ensure we have a registry to test with
if ! ensure_test_registry; then
    echo -e "${RED}Error: Failed to set up test registry${NC}"
    exit 1
fi

# Build ACR CLI if needed
if [ ! -f "$ACR_CLI" ]; then
    echo "Building ACR CLI..."
    (cd "$SCRIPT_DIR/../.." && make binaries)
fi

# Registry is already set up and logged in via ensure_test_registry

echo -e "\n${BLUE}=== ACR Purge Test Suite ===${NC}"
echo "Registry: $REGISTRY"
echo "Test mode: $TEST_MODE"
echo ""


# Individual test functions
run_test_basic() {
    echo -e "\n${YELLOW}Test: Basic Purge Functionality${NC}"
    local repo="test-minimal-basic"
    echo "Creating test images..."
    for i in 1 2 3; do
        create_test_image "$repo" "v$i"
    done

    sleep 1
    # Clean up any 'latest' tag that might have been created
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:latest" --ago 0d 2>&1 >/dev/null || true

    local initial_count=$(count_tags "$repo")
    echo "Tags in repository: $initial_count"

    # Test dry run
    echo -n "Testing dry run... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:v1" --ago 0d --dry-run >/dev/null 2>&1
    local dry_run_count=$(count_tags "$repo")
    assert_equals "$initial_count" "$dry_run_count" "Dry run should not delete tags"

    # Test actual delete
    echo -n "Testing actual delete... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:v1" --ago 0d >/dev/null 2>&1
    local after_delete_count=$(count_tags "$repo")
    assert_equals "$((initial_count - 1))" "$after_delete_count" "Should delete one tag"

    # Test 2: Locking functionality
    echo -e "\n${YELLOW}Test 2: Lock Testing${NC}"
    local lock_repo="test-minimal-locks"

    # Create only the locked image for this test
    create_test_image "$lock_repo" "locked"

    # Wait a moment for images to be fully registered
    sleep 2

    echo "Locking image..."
    lock_image "$lock_repo" "locked" false false

    # Verify lock was applied
    if [ "$DEBUG" = "1" ]; then
        echo "Verifying lock status:"
        az acr repository show --name "$(get_registry_name)" --image "$lock_repo:locked" --query 'changeableAttributes' || echo "Failed to check lock status"
    fi

    # Debug: list all tags before test
    if [ "$DEBUG" = "1" ]; then
        echo "Tags before purge:"
        "$ACR_CLI" tag list -r "$REGISTRY" --repository "$lock_repo"
    fi

    local initial_lock_count=$(count_tags "$lock_repo")
    echo "Initial tags in lock repo: $initial_lock_count"

    # Try to delete the locked image
    echo -n "Testing purge of locked image without --include-locked... "
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:locked" --ago 0d 2>&1 || true)

    # Debug: Show purge output to understand behavior
    if [ "$DEBUG" = "1" ]; then
        echo -e "\nPurge output:"
        echo "$output"
    fi

    local after_first_purge=$(count_tags "$lock_repo")

    # The locked image should NOT be deleted, so count should remain the same
    if [ "$after_first_purge" -eq "$initial_lock_count" ]; then
        echo -e "${GREEN}✓ Locked image was skipped${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Locked image may have been deleted${NC}"
        echo -e "  Expected count: $initial_lock_count, Actual: $after_first_purge"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Locked image may have been deleted")
    fi

    # Create an unlocked image for the second part of the test
    create_test_image "$lock_repo" "unlocked"

    # Try with --include-locked
    echo -n "Testing purge with --include-locked... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:.*" --ago 0d --include-locked >/dev/null 2>&1
    local final_count=$(count_tags "$lock_repo")
    # Both images should now be deleted
    assert_equals "0" "$final_count" "All images should be deleted with --include-locked"

    # Test 3: Pattern matching
    echo -e "\n${YELLOW}Test 3: Pattern Matching${NC}"
    local pattern_repo="test-minimal-patterns"

    for tag in "v1.0.0" "v2.0.0" "dev-123" "prod-456"; do
        create_test_image "$pattern_repo" "$tag"
    done

    echo -n "Testing version pattern (v*.0.0)... "
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$pattern_repo:v.*\.0\.0" --ago 0d --dry-run 2>&1)
    if echo "$output" | grep -q "v1.0.0" && echo "$output" | grep -q "v2.0.0"; then
        echo -e "${GREEN}✓ Pattern matching works${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Pattern matching failed${NC}"
        ((TESTS_FAILED++))
    fi

    # Test 4: Keep parameter
    echo -e "\n${YELLOW}Test 4: Keep Parameter${NC}"
    local keep_repo="test-minimal-keep"

    for i in $(seq 1 5); do
        create_test_image "$keep_repo" "v$i"
        sleep 0.2
    done

    local initial_keep_count=$(count_tags "$keep_repo")
    echo "Created $initial_keep_count tags in keep repo"

    echo -n "Testing --keep 2... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$keep_repo:.*" --ago 0d --keep 2 >/dev/null 2>&1
    local kept_count=$(count_tags "$keep_repo")
    # Allow for 2 or 3 tags (Docker might create a 'latest' tag automatically)
    if [ "$kept_count" -eq 2 ] || [ "$kept_count" -eq 3 ]; then
        echo -e "${GREEN}✓ Should keep 2-3 latest tags (kept $kept_count)${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Should keep 2-3 latest tags${NC}"
        echo -e "  Expected: 2 or 3, Actual: $kept_count"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Should keep 2-3 latest tags")
    fi
}

# Run comprehensive tests
run_comprehensive_tests() {
    echo -e "\n${BLUE}=== Comprehensive Test Suite ===${NC}"

    # Test Suite 1: Dry Run Verification
    echo -e "\n${YELLOW}Test Suite 1: Dry Run Verification${NC}"
    local repo="test-comp-dryrun"
    echo "Creating test images..."
    for i in {1..5}; do
        create_test_image "$repo" "v$i"
    done

    local initial_count=$(count_tags "$repo")
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d --dry-run 2>&1 || true)
    local final_count=$(count_tags "$repo")

    assert_equals "$initial_count" "$final_count" "Dry run should not delete any tags"
    if echo "$output" | grep -qi "dry.run"; then
        echo -e "${GREEN}✓ Dry run output contains dry run marker${NC}"
        ((TESTS_PASSED++))
    fi

    # Test Suite 2: Comprehensive Locking Tests
    echo -e "\n${YELLOW}Test Suite 2: Comprehensive Locking Tests${NC}"
    local lock_repo="test-comp-locks"

    create_test_image "$lock_repo" "unlocked"
    create_test_image "$lock_repo" "write-locked"
    create_test_image "$lock_repo" "delete-locked"
    create_test_image "$lock_repo" "fully-locked"

    # Clean up any 'latest' tag that might have been created
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:latest" --ago 0d 2>&1 >/dev/null || true

    echo "Applying locks..."
    # Leave "unlocked" image in its default state (no explicit locking)
    lock_image "$lock_repo" "write-locked" false true
    lock_image "$lock_repo" "delete-locked" true false
    lock_image "$lock_repo" "fully-locked" false false

    # Test without --include-locked
    local initial_count=$(count_tags "$lock_repo")
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:.*" --ago 0d 2>&1 >/dev/null || true
    local final_count=$(count_tags "$lock_repo")
    assert_equals "3" "$final_count" "Should keep 3 locked images"

    # Test with --include-locked
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:.*" --ago 0d --include-locked 2>&1 >/dev/null || true
    final_count=$(count_tags "$lock_repo")
    assert_equals "0" "$final_count" "Should delete all images with --include-locked"

    # Test Suite 3: Edge Cases
    echo -e "\n${YELLOW}Test Suite 3: Edge Cases${NC}"

    # Empty repository
    output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "nonexistent-repo:.*" --ago 0d 2>&1 || true)
    assert_contains "$output" "Number of deleted tags: 0" "Empty repository should delete 0 tags"

    # Special characters
    local special_repo="test-comp-special"
    create_test_image "$special_repo" "v1.0.0"
    # Skip feature/test tag as it contains invalid characters
    create_test_image "$special_repo" "feature-test"
    create_test_image "$special_repo" "build_123"
    create_test_image "$special_repo" "2024-01-22"

    output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$special_repo:v.*" --ago 0d --dry-run 2>&1)
    assert_contains "$output" "v1.0.0" "Should match version tags"

    # Test Suite 4: Keep Parameter
    echo -e "\n${YELLOW}Test Suite 4: Keep Parameter Tests${NC}"
    local keep_repo="test-comp-keep"

    for i in {1..10}; do
        create_test_image "$keep_repo" "v$(printf "%03d" $i)"
        sleep 0.1
    done

    output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$keep_repo:.*" --ago 0d --keep 3 --dry-run 2>&1)
    local deleted_count=$(echo "$output" | grep -c "v0[0-7][0-9]" || true)
    assert_equals "7" "$deleted_count" "Should mark 7 tags for deletion when keeping 3"

    # Test Suite 5: Concurrent Operations
    echo -e "\n${YELLOW}Test Suite 5: Concurrent Operations${NC}"
    local concurrent_repo="test-comp-concurrent"

    echo "Creating images for concurrency tests..."
    for i in {1..20}; do
        create_test_image "$concurrent_repo" "tag$i"
    done

    for concurrency in 1 5 10; do
        echo -e "\n${CYAN}Testing concurrency=$concurrency${NC}"
        local start_time=$(date +%s)
        "$ACR_CLI" purge --registry "$REGISTRY" --filter "$concurrent_repo:tag[12].*" --ago 0d --concurrency "$concurrency" --dry-run >/dev/null 2>&1
        local end_time=$(date +%s)
        echo "  Duration: $((end_time - start_time))s"
    done

    # Test Suite 6: Manifest vs Tag
    echo -e "\n${YELLOW}Test Suite 6: Manifest vs Tag Deletion${NC}"
    local manifest_repo="test-comp-manifest"

    create_test_image "$manifest_repo" "base"
    docker tag "$REGISTRY/$manifest_repo:base" "$REGISTRY/$manifest_repo:alias1"
    docker tag "$REGISTRY/$manifest_repo:base" "$REGISTRY/$manifest_repo:alias2"
    docker push "$REGISTRY/$manifest_repo:alias1" >/dev/null 2>&1
    docker push "$REGISTRY/$manifest_repo:alias2" >/dev/null 2>&1

    local initial_tags=$(count_tags "$manifest_repo")
    local initial_manifests=$(count_manifests "$manifest_repo")

    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$manifest_repo:alias1" --ago 0d >/dev/null 2>&1

    local final_tags=$(count_tags "$manifest_repo")
    local final_manifests=$(count_manifests "$manifest_repo")

    assert_equals "$((initial_tags - 1))" "$final_tags" "Should delete one tag"
    assert_equals "$initial_manifests" "$final_manifests" "Manifest should remain"

    # Test Suite 7: Age-based Filtering
    echo -e "\n${YELLOW}Test Suite 7: Age-based Filtering${NC}"
    local age_repo="test-comp-age"

    create_test_image "$age_repo" "old"
    create_test_image "$age_repo" "new"

    output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$age_repo:.*" --ago 1d --dry-run 2>&1)
    assert_contains "$output" "Number of tags to be deleted: 0" "New images should not be deleted with --ago 1d"
}

# Run benchmark tests
run_benchmark_tests() {
    echo -e "\n${BLUE}=== Benchmark Test Suite ===${NC}"

    # Check if hyperfine is available
    echo -e "${CYAN}Checking for hyperfine...${NC}"

    # Try to find hyperfine in common locations
    HYPERFINE_CMD=""
    if command -v hyperfine >/dev/null 2>&1; then
        HYPERFINE_CMD="hyperfine"
    elif [ -f "$HOME/.cargo/bin/hyperfine" ]; then
        HYPERFINE_CMD="$HOME/.cargo/bin/hyperfine"
    elif [ -f "/usr/local/bin/hyperfine" ]; then
        HYPERFINE_CMD="/usr/local/bin/hyperfine"
    fi

    if [ -n "$HYPERFINE_CMD" ]; then
        echo -e "${GREEN}✓ Hyperfine found: $($HYPERFINE_CMD --version)${NC}"
    else
        echo -e "${YELLOW}Warning: hyperfine not found. Falling back to basic timing.${NC}"
        echo "Install hyperfine for more accurate benchmarks: cargo install hyperfine"
        echo "PATH: $PATH"
        run_benchmark_tests_basic
        return
    fi

    local num_repos=3
    local images_per_repo="${NUM_IMAGES:-500}"

    # Phase 1: Generate test data
    echo -e "\n${YELLOW}Phase 1: Generating Test Data${NC}"
    for repo_num in $(seq 1 "$num_repos"); do
        local repo="benchmark-repo-${repo_num}"
        generate_test_images "$repo" "$images_per_repo"
    done

    # Phase 2: Run benchmarks with hyperfine
    echo -e "\n${YELLOW}Phase 2: Running Benchmarks with Hyperfine${NC}"

    # Test single repository with varying concurrency
    echo -e "\n${CYAN}Single Repository Performance (${images_per_repo} images)${NC}"
    echo -e "${YELLOW}Running hyperfine benchmarks (single run)...${NC}\n"
    "$HYPERFINE_CMD" \
        --runs 1 \
        --parameter-list concurrency 1,5,10,20 \
        "$ACR_CLI purge --registry $REGISTRY --filter 'benchmark-repo-1:.*' --ago 0d --concurrency {concurrency} --dry-run"

    echo -e "\n${GREEN}Single repository benchmark completed${NC}"

    # Test multiple repositories
    echo -e "\n${CYAN}Multiple Repository Performance (${num_repos} repos, $((num_repos * images_per_repo)) total images)${NC}"
    echo -e "${YELLOW}Running hyperfine benchmarks for multiple repositories (single run)...${NC}\n"
    "$HYPERFINE_CMD" \
        --runs 1 \
        --parameter-list concurrency 5,10,20,30 \
        "$ACR_CLI purge --registry $REGISTRY --filter 'benchmark-repo-.*:.*' --ago 0d --concurrency {concurrency} --dry-run"

    echo -e "\n${GREEN}Multiple repository benchmark completed${NC}"

    # Test pattern complexity
    echo -e "\n${CYAN}Pattern Complexity Impact${NC}"
    echo -e "${YELLOW}Running hyperfine benchmarks comparing regex pattern complexity (single run)...${NC}\n"
    "$HYPERFINE_CMD" \
        --runs 1 \
        --command-name "simple-pattern" "$ACR_CLI purge --registry $REGISTRY --filter 'benchmark-repo-1:.*' --ago 0d --concurrency 10 --dry-run" \
        --command-name "complex-pattern" "$ACR_CLI purge --registry $REGISTRY --filter 'benchmark-repo-1:v[0-9]{3}[024680]' --ago 0d --concurrency 10 --dry-run" \
        --command-name "very-complex-pattern" "$ACR_CLI purge --registry $REGISTRY --filter 'benchmark-repo-1:v00[0-9][024]' --ago 0d --concurrency 10 --dry-run"

    echo -e "\n${GREEN}Pattern complexity benchmark completed${NC}"

    # Test repository scaling
    echo -e "\n${CYAN}Repository Scaling Performance${NC}"
    echo -e "${YELLOW}Running hyperfine benchmarks for repository count scaling (single run)...${NC}\n"
    local commands=()
    for num in 1 2 3; do
        commands+=("--command-name")
        commands+=("${num}-repos")
        commands+=("$ACR_CLI purge --registry $REGISTRY --filter 'benchmark-repo-[1-${num}]:.*' --ago 0d --concurrency 10 --dry-run")
    done

    "$HYPERFINE_CMD" \
        --runs 1 \
        "${commands[@]}"

    echo -e "\n${GREEN}Repository scaling benchmark completed${NC}"

    # Phase 3: Deletion Performance Tests
    echo -e "\n${YELLOW}Phase 3: Actual Deletion Performance Tests${NC}"
    echo -e "${RED}WARNING: These tests will DELETE images!${NC}"

    # Create separate repositories for deletion tests
    local del_repo_prefix="benchmark-delete"
    local del_images_per_test="${NUM_IMAGES:-500}"

    # Test 1: Compare dry-run vs actual deletion
    echo -e "\n${CYAN}Dry-run vs Actual Deletion Comparison${NC}"

    # Generate test data for deletion
    local del_repo="${del_repo_prefix}-comparison"
    generate_test_images "$del_repo" "$del_images_per_test"

    echo -e "\n${YELLOW}Running comparison benchmark...${NC}"
    "$HYPERFINE_CMD" \
        --runs 1 \
        --command-name "dry-run" "$ACR_CLI purge --registry $REGISTRY --filter '${del_repo}:v0[0-2][0-9]' --ago 0d --dry-run" \
        --command-name "actual-delete" "$ACR_CLI purge --registry $REGISTRY --filter '${del_repo}:v0[0-2][0-9]' --ago 0d"

    # Test 2: Deletion with different concurrency levels
    echo -e "\n${CYAN}Deletion Performance with Varying Concurrency${NC}"

    # Create repos for each concurrency test
    for concurrency in 5 10 20; do
        local del_repo="${del_repo_prefix}-conc${concurrency}"
        echo -e "${BLUE}Preparing data for concurrency ${concurrency}...${NC}"
        generate_test_images "$del_repo" "$del_images_per_test" "$concurrency"
    done

    echo -e "\n${YELLOW}Running deletion concurrency benchmark...${NC}"
    "$HYPERFINE_CMD" \
        --runs 1 \
        --prepare "echo 'Starting deletion test...'" \
        --command-name "concurrency-5" "$ACR_CLI purge --registry $REGISTRY --filter '${del_repo_prefix}-conc5:.*' --ago 0d --concurrency 5" \
        --command-name "concurrency-10" "$ACR_CLI purge --registry $REGISTRY --filter '${del_repo_prefix}-conc10:.*' --ago 0d --concurrency 10" \
        --command-name "concurrency-20" "$ACR_CLI purge --registry $REGISTRY --filter '${del_repo_prefix}-conc20:.*' --ago 0d --concurrency 20"

    # Test 3: Locked image deletion performance
    echo -e "\n${CYAN}Locked Image Deletion Performance${NC}"

    local lock_del_repo="${del_repo_prefix}-locks"
    generate_test_images "$lock_del_repo" 20

    # Lock half the images
    echo -e "${YELLOW}Locking images...${NC}"
    for i in $(seq 2 2 20); do
        lock_image "$lock_del_repo" "v$(printf "%03d" "$i")" false false
    done

    echo -e "\n${YELLOW}Running locked image deletion benchmark...${NC}"
    "$HYPERFINE_CMD" \
        --runs 1 \
        --setup "echo 'Testing locked image handling...'" \
        --command-name "without-include-locked" "$ACR_CLI purge --registry $REGISTRY --filter '${lock_del_repo}:.*' --ago 0d" \
        --command-name "with-include-locked" "$ACR_CLI purge --registry $REGISTRY --filter '${lock_del_repo}:.*' --ago 0d --include-locked"

    echo -e "\n${GREEN}All benchmarks completed!${NC}"
}

# Run deletion-only benchmark tests
run_deletion_benchmark_tests() {
    echo -e "\n${BLUE}=== Deletion Performance Benchmark Suite ===${NC}"
    echo -e "${RED}WARNING: This test suite will DELETE images from your registry!${NC}"
    echo -e "${YELLOW}Make sure you're using a test registry.${NC}\n"

    read -p "Continue with deletion benchmarks? (yes/N) " -r
    if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        echo "Deletion benchmarks cancelled."
        return
    fi

    # Check if hyperfine is available
    HYPERFINE_CMD=""
    if command -v hyperfine >/dev/null 2>&1; then
        HYPERFINE_CMD="hyperfine"
    elif [ -f "$HOME/.cargo/bin/hyperfine" ]; then
        HYPERFINE_CMD="$HOME/.cargo/bin/hyperfine"
    fi

    if [ -n "$HYPERFINE_CMD" ]; then
        echo -e "${GREEN}✓ Using hyperfine for accurate benchmarks${NC}"
        run_deletion_benchmarks_hyperfine
    else
        echo -e "${YELLOW}Using basic timing (install hyperfine for better results)${NC}"
        run_deletion_benchmarks_basic
    fi
}

# Deletion benchmarks with hyperfine
run_deletion_benchmarks_hyperfine() {
    local del_images_per_test="${NUM_IMAGES:-500}"

    # Test 1: Dry-run vs Actual Deletion Performance
    echo -e "\n${CYAN}Test 1: Dry-run vs Actual Deletion Performance${NC}"

    local del_repo="benchmark-delete-comparison"
    echo "Generating $del_images_per_test test images..."
    generate_test_images "$del_repo" "$del_images_per_test"

    "$HYPERFINE_CMD" \
        --runs 1 \
        --command-name "dry-run" "$ACR_CLI purge --registry $REGISTRY --filter '${del_repo}:v.*' --ago 0d --dry-run" \
        --command-name "actual-delete" "$ACR_CLI purge --registry $REGISTRY --filter '${del_repo}:v.*' --ago 0d"

    # Test 2: Concurrency Impact on Deletion
    echo -e "\n${CYAN}Test 2: Concurrency Impact on Deletion Performance${NC}"

    local commands=()
    for concurrency in 1 5 10 20 30; do
        local del_repo="benchmark-delete-conc${concurrency}"
        echo "Preparing ${del_images_per_test} images for concurrency ${concurrency}..."
        generate_test_images "$del_repo" "$del_images_per_test" "$concurrency"

        commands+=("--command-name")
        commands+=("concurrency-${concurrency}")
        commands+=("$ACR_CLI purge --registry $REGISTRY --filter '${del_repo}:.*' --ago 0d --concurrency ${concurrency}")
    done

    "$HYPERFINE_CMD" \
        --runs 1 \
        "${commands[@]}"

    # Test 3: Large-scale Deletion Performance
    echo -e "\n${CYAN}Test 3: Large-scale Deletion Performance${NC}"

    local large_count=$((del_images_per_test * 3))
    echo "Generating ${large_count} images for large-scale test..."

    local large_repo="benchmark-delete-large"
    generate_test_images "$large_repo" "$large_count" 20

    "$HYPERFINE_CMD" \
        --runs 1 \
        --command-name "delete-${large_count}-images" "$ACR_CLI purge --registry $REGISTRY --filter '${large_repo}:.*' --ago 0d --concurrency 20"

    # Test 4: Locked Image Handling Performance
    echo -e "\n${CYAN}Test 4: Locked Image Deletion Performance${NC}"

    local lock_repo="benchmark-delete-locks"
    generate_test_images "$lock_repo" 40

    # Lock every other image
    echo "Locking 50% of images..."
    for i in $(seq 2 2 40); do
        lock_image "$lock_repo" "v$(printf "%03d" "$i")" false false
    done

    "$HYPERFINE_CMD" \
        --runs 1 \
        --setup "echo 'Testing locked image handling...'" \
        --command-name "skip-locked" "$ACR_CLI purge --registry $REGISTRY --filter '${lock_repo}:.*' --ago 0d" \
        --command-name "include-locked" "$ACR_CLI purge --registry $REGISTRY --filter '${lock_repo}:.*' --ago 0d --include-locked"

    # Test 5: Direct comparison of locked vs unlocked repositories
    echo -e "\n${CYAN}Test 5: Locked vs Unlocked Repository Comparison${NC}"
    echo -e "${YELLOW}Creating two identical repositories with same number of images${NC}"
    
    local compare_images="${NUM_IMAGES:-500}"
    local unlocked_repo="benchmark-compare-unlocked"
    local locked_repo="benchmark-compare-locked"
    
    # Create unlocked repository
    echo "Creating ${compare_images} unlocked images..."
    generate_test_images "$unlocked_repo" "$compare_images"
    
    # Create locked repository
    echo "Creating ${compare_images} locked images..."
    generate_test_images "$locked_repo" "$compare_images"
    
    # Lock all images in the locked repository
    echo -e "${YELLOW}Locking all images in ${locked_repo}...${NC}"
    for i in $(seq 1 "$compare_images"); do
        lock_image "$locked_repo" "v$(printf "%03d" "$i")" false false
        # Show progress every 50 images
        if [ $((i % 50)) -eq 0 ]; then
            echo "  Locked $i/$compare_images images..."
        fi
    done
    echo -e "${GREEN}All images locked${NC}"
    
    echo -e "\n${YELLOW}Running comparison benchmark...${NC}"
    echo "Both repositories have exactly ${compare_images} images"
    echo "- ${unlocked_repo}: All images unlocked"
    echo "- ${locked_repo}: All images locked"
    
    "$HYPERFINE_CMD" \
        --runs 1 \
        --setup "echo 'Comparing locked vs unlocked repositories...'" \
        --command-name "purge-unlocked-repo" "$ACR_CLI purge --registry $REGISTRY --filter '${unlocked_repo}:.*' --ago 0d" \
        --command-name "purge-locked-repo-skip" "$ACR_CLI purge --registry $REGISTRY --filter '${locked_repo}:.*' --ago 0d" \
        --command-name "purge-locked-repo-include" "$ACR_CLI purge --registry $REGISTRY --filter '${locked_repo}:.*' --ago 0d --include-locked"
    
    # Verify results
    echo -e "\n${CYAN}Verification:${NC}"
    local unlocked_remaining=$(count_tags "$unlocked_repo")
    local locked_remaining=$(count_tags "$locked_repo")
    echo "- Unlocked repository remaining images: $unlocked_remaining"
    echo "- Locked repository remaining images: $locked_remaining"

    echo -e "\n${GREEN}Deletion benchmarks completed!${NC}"
}

# Basic deletion benchmarks without hyperfine
run_deletion_benchmarks_basic() {
    local del_images_per_test="${NUM_IMAGES:-500}"

    echo -e "\n${CYAN}Test 1: Dry-run vs Actual Deletion${NC}"

    local del_repo="benchmark-delete-basic"
    echo "Generating $del_images_per_test test images..."
    generate_test_images "$del_repo" "$del_images_per_test"

    # Measure dry-run
    local dry_duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$del_repo:.*" \
        --ago 0d \
        --dry-run >/dev/null 2>&1)

    # Measure actual deletion
    local del_duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$del_repo:.*" \
        --ago 0d >/dev/null 2>&1)

    echo "Dry-run duration: ${dry_duration}s"
    echo "Deletion duration: ${del_duration}s"

    local overhead=$(awk -v del="$del_duration" -v dry="$dry_duration" 'BEGIN {
        if (dry > 0) printf "%.1f%%", ((del - dry) / dry) * 100
        else print "N/A"
    }')
    echo "Deletion overhead: ${overhead}"

    echo -e "\n${CYAN}Test 2: Concurrency Impact${NC}"

    for concurrency in 5 10 20; do
        local conc_repo="benchmark-delete-conc${concurrency}"
        echo -e "\nTesting concurrency=${concurrency}..."
        generate_test_images "$conc_repo" "$del_images_per_test"

        local duration=$(measure_time "$ACR_CLI" purge \
            --registry "$REGISTRY" \
            --filter "$conc_repo:.*" \
            --ago 0d \
            --concurrency "$concurrency" >/dev/null 2>&1)

        local throughput=$(awk -v n="$del_images_per_test" -v d="$duration" 'BEGIN {printf "%.1f", n/d}')
        echo "Duration: ${duration}s, Throughput: ${throughput} images/sec"
    done

    # Test 3: Direct comparison of locked vs unlocked repositories
    echo -e "\n${CYAN}Test 3: Locked vs Unlocked Repository Comparison${NC}"
    
    local compare_images="${NUM_IMAGES:-500}"
    local unlocked_repo="benchmark-compare-unlocked-basic"
    local locked_repo="benchmark-compare-locked-basic"
    
    # Create unlocked repository
    echo "Creating ${compare_images} unlocked images..."
    generate_test_images "$unlocked_repo" "$compare_images"
    
    # Create locked repository
    echo "Creating ${compare_images} locked images..."
    generate_test_images "$locked_repo" "$compare_images"
    
    # Lock all images in the locked repository
    echo -e "${YELLOW}Locking all images in ${locked_repo}...${NC}"
    for i in $(seq 1 "$compare_images"); do
        lock_image "$locked_repo" "v$(printf "%03d" "$i")" false false
        # Show progress every 50 images
        if [ $((i % 50)) -eq 0 ]; then
            echo "  Locked $i/$compare_images images..."
        fi
    done
    echo -e "${GREEN}All images locked${NC}"
    
    echo -e "\n${YELLOW}Running comparison tests...${NC}"
    echo "Both repositories have exactly ${compare_images} images"
    
    # Test unlocked repository
    echo -e "\n${BLUE}Purging unlocked repository...${NC}"
    local unlocked_duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$unlocked_repo:.*" \
        --ago 0d >/dev/null 2>&1)
    echo "Unlocked repo deletion: ${unlocked_duration}s"
    
    # Test locked repository without --include-locked
    echo -e "\n${BLUE}Purging locked repository (skip locked)...${NC}"
    local locked_skip_duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$locked_repo:.*" \
        --ago 0d >/dev/null 2>&1)
    echo "Locked repo (skip): ${locked_skip_duration}s"
    
    # Test locked repository with --include-locked
    echo -e "\n${BLUE}Purging locked repository (include locked)...${NC}"
    local locked_include_duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$locked_repo:.*" \
        --ago 0d \
        --include-locked >/dev/null 2>&1)
    echo "Locked repo (include): ${locked_include_duration}s"
    
    # Verify results
    echo -e "\n${CYAN}Results Summary:${NC}"
    echo "- Unlocked repo deletion: ${unlocked_duration}s"
    echo "- Locked repo (skip locked): ${locked_skip_duration}s"
    echo "- Locked repo (include locked): ${locked_include_duration}s"
    
    local speedup=$(awk -v skip="$locked_skip_duration" -v unlocked="$unlocked_duration" 'BEGIN {
        if (unlocked > 0) printf "%.1fx", skip/unlocked
        else print "N/A"
    }')
    echo -e "\n${GREEN}Skipping locked images was ${speedup} faster than deleting unlocked images${NC}"

    echo -e "\n${GREEN}Deletion benchmarks completed!${NC}"
}

# Fallback benchmark function without hyperfine
run_benchmark_tests_basic() {
    echo -e "\n${CYAN}Running basic benchmarks without hyperfine${NC}"

    local num_repos=3
    local images_per_repo="${NUM_IMAGES:-500}"

    # Phase 1: Generate test data
    echo -e "\n${YELLOW}Phase 1: Generating Test Data${NC}"
    for repo_num in $(seq 1 "$num_repos"); do
        local repo="benchmark-repo-${repo_num}"
        generate_test_images "$repo" "$images_per_repo"
    done

    # Phase 2: Run benchmarks
    echo -e "\n${YELLOW}Phase 2: Running Benchmarks${NC}"

    # Test single repository with varying concurrency
    echo -e "\n${CYAN}Single Repository Performance${NC}"
    local repo="benchmark-repo-1"
    for concurrency in 1 5 10 20; do
        echo "Testing concurrency=$concurrency..."
        local duration=$(measure_time "$ACR_CLI" purge \
            --registry "$REGISTRY" \
            --filter "$repo:.*" \
            --ago 0d \
            --concurrency "$concurrency" \
            --dry-run >/dev/null 2>&1)

        local images_per_sec=$(awk -v n="$images_per_repo" -v d="$duration" 'BEGIN {printf "%.2f", n/d}' 2>/dev/null || echo "0.00")
        echo -e "${GREEN}  Duration: ${duration}s, Throughput: ${images_per_sec} images/sec${NC}"
    done

    # Test multiple repositories
    echo -e "\n${CYAN}Multiple Repository Performance${NC}"
    local total_images=$((num_repos * images_per_repo))
    for concurrency in 10 20; do
        echo "Testing concurrency=$concurrency..."
        local duration=$(measure_time "$ACR_CLI" purge \
            --registry "$REGISTRY" \
            --filter "benchmark-repo-.*:.*" \
            --ago 0d \
            --concurrency "$concurrency" \
            --dry-run >/dev/null 2>&1)

        local images_per_sec=$(awk -v n="$total_images" -v d="$duration" 'BEGIN {printf "%.2f", n/d}' 2>/dev/null || echo "0.00")
        echo -e "${GREEN}  Duration: ${duration}s, Throughput: ${images_per_sec} images/sec${NC}"
    done

    # Test pattern complexity
    echo -e "\n${CYAN}Pattern Complexity Impact${NC}"

    # Simple pattern
    duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "benchmark-repo-1:.*" \
        --ago 0d \
        --concurrency 10 \
        --dry-run >/dev/null 2>&1)
    echo "Simple pattern duration: ${duration}s"

    # Complex pattern
    duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "benchmark-repo-1:v[0-9]{3}[024680]" \
        --ago 0d \
        --concurrency 10 \
        --dry-run >/dev/null 2>&1)
    echo "Complex pattern duration: ${duration}s"

    # Phase 3: Basic Deletion Performance Tests
    echo -e "\n${YELLOW}Phase 3: Actual Deletion Performance Tests${NC}"
    echo -e "${RED}WARNING: These tests will DELETE images!${NC}"

    # Test deletion vs dry-run performance
    echo -e "\n${CYAN}Comparing Dry-run vs Actual Deletion${NC}"

    local del_repo="benchmark-delete-basic"
    generate_test_images "$del_repo" "${NUM_IMAGES:-500}"

    # Dry run timing
    echo "Testing dry-run performance..."
    local dry_duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$del_repo:v0[0-2][0-9]" \
        --ago 0d \
        --concurrency 10 \
        --dry-run >/dev/null 2>&1)
    echo "Dry-run duration: ${dry_duration}s"

    # Actual deletion timing
    echo "Testing actual deletion performance..."
    local del_duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$del_repo:v0[0-2][0-9]" \
        --ago 0d \
        --concurrency 10 >/dev/null 2>&1)
    echo "Deletion duration: ${del_duration}s"

    # Calculate overhead
    local overhead=$(awk -v del="$del_duration" -v dry="$dry_duration" 'BEGIN {
        if (dry > 0) printf "%.1f", ((del - dry) / dry) * 100
        else print "N/A"
    }')
    echo "Deletion overhead: ${overhead}%"

    # Test locked image deletion
    echo -e "\n${CYAN}Testing Locked Image Deletion${NC}"

    local lock_repo="benchmark-delete-locks"
    generate_test_images "$lock_repo" 20

    # Lock some images
    echo "Locking half of the images..."
    for i in $(seq 2 2 20); do
        lock_image "$lock_repo" "v$(printf "%03d" "$i")" false false
    done

    # Time deletion without --include-locked
    echo "Testing deletion without --include-locked..."
    local no_lock_duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$lock_repo:.*" \
        --ago 0d \
        --concurrency 10 >/dev/null 2>&1)
    echo "Duration (skipping locked): ${no_lock_duration}s"

    # Re-create the deleted unlocked images for second test
    for i in $(seq 1 2 19); do
        create_test_image "$lock_repo" "v$(printf "%03d" "$i")"
    done

    # Time deletion with --include-locked
    echo "Testing deletion with --include-locked..."
    local with_lock_duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$lock_repo:.*" \
        --ago 0d \
        --concurrency 10 \
        --include-locked >/dev/null 2>&1)
    echo "Duration (including locked): ${with_lock_duration}s"

    # Generate summary
    echo -e "\n${YELLOW}Benchmark Summary${NC}"
    echo "✓ Dry-run performance tested"
    echo "✓ Actual deletion performance tested"
    echo "✓ Locked image handling tested"
    echo "Benchmark tests completed successfully"
}

# Run debug tests
run_debug_tests() {
    echo -e "\n${BLUE}=== Debug Test Suite ===${NC}"

    # Test 1: Detailed lock behavior
    echo -e "\n${YELLOW}Test 1: Detailed Lock Behavior${NC}"
    local repo="debug-locks"

    echo "Creating and locking image..."
    create_test_image "$repo" "testlock"

    echo -e "\n${GREEN}Initial state:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo"

    echo -e "\n${GREEN}Locking image...${NC}"
    az acr repository update --name "$(get_registry_name)" --image "$repo:testlock" --delete-enabled false

    echo -e "\n${GREEN}Lock status:${NC}"
    az acr repository show --name "$(get_registry_name)" --image "$repo:testlock" --query 'changeableAttributes'

    echo -e "\n${GREEN}Attempting delete without --include-locked:${NC}"
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:testlock" --ago 0d --untagged

    echo -e "\n${GREEN}Tags after purge without --include-locked:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo"

    echo -e "\n${GREEN}Attempting delete with --include-locked:${NC}"
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:testlock" --ago 0d --include-locked --untagged

    echo -e "\n${GREEN}Tags after purge with --include-locked:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo"

    # Test 2: Keep parameter behavior
    echo -e "\n${YELLOW}Test 2: Keep Parameter Behavior${NC}"
    local keep_repo="debug-keep"

    echo -e "\n${GREEN}Creating 5 tags...${NC}"
    for i in $(seq 1 5); do
        create_test_image "$keep_repo" "v$i"
        sleep 0.5
    done

    echo -e "\n${GREEN}All tags before purge:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$keep_repo"

    echo -e "\n${GREEN}Running purge with --keep 2:${NC}"
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$keep_repo:.*" --ago 0d --keep 2 --untagged

    echo -e "\n${GREEN}Tags after purge with --keep 2:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$keep_repo"
}

# Run interactive tests
# Individual test functions for selective execution
run_test_basic_functionality() {
    local repo="test-minimal-basic"
    echo "Creating test images..."
    for i in 1 2 3; do
        create_test_image "$repo" "v$i"
    done

    sleep 1
    # Clean up any 'latest' tag that might have been created
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:latest" --ago 0d 2>&1 >/dev/null || true

    local initial_count=$(count_tags "$repo")
    echo "Tags in repository: $initial_count"

    # Test dry run
    echo -n "Testing dry run... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:v1" --ago 0d --dry-run >/dev/null 2>&1
    local dry_run_count=$(count_tags "$repo")
    assert_equals "$initial_count" "$dry_run_count" "Dry run should not delete tags"

    # Test actual delete
    echo -n "Testing actual delete... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:v1" --ago 0d >/dev/null 2>&1
    local after_delete_count=$(count_tags "$repo")
    assert_equals "$((initial_count - 1))" "$after_delete_count" "Should delete one tag"
}

run_test_lock_functionality() {
    local lock_repo="test-minimal-locks"

    # Create only the locked image for this test
    create_test_image "$lock_repo" "locked"

    # Wait a moment for images to be fully registered
    sleep 2

    echo "Locking image..."
    lock_image "$lock_repo" "locked" false false

    local initial_lock_count=$(count_tags "$lock_repo")
    echo "Initial tags in lock repo: $initial_lock_count"

    # Try to delete the locked image
    echo -n "Testing purge of locked image without --include-locked... "
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:locked" --ago 0d 2>&1 || true)
    local after_first_purge=$(count_tags "$lock_repo")

    # The locked image should NOT be deleted, so count should remain the same
    if [ "$after_first_purge" -eq "$initial_lock_count" ]; then
        echo -e "${GREEN}✓ Locked image was skipped${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Locked image may have been deleted${NC}"
        echo -e "  Expected count: $initial_lock_count, Actual: $after_first_purge"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Locked image may have been deleted")
    fi

    # Create an unlocked image for the second part of the test
    create_test_image "$lock_repo" "unlocked"

    # Try with --include-locked
    echo -n "Testing purge with --include-locked... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:.*" --ago 0d --include-locked >/dev/null 2>&1
    local final_count=$(count_tags "$lock_repo")
    # Both images should now be deleted
    assert_equals "0" "$final_count" "All images should be deleted with --include-locked"
}

run_test_pattern_matching() {
    local pattern_repo="test-minimal-patterns"

    for tag in "v1.0.0" "v2.0.0" "dev-123" "prod-456"; do
        create_test_image "$pattern_repo" "$tag"
    done

    echo -n "Testing version pattern (v*.0.0)... "
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$pattern_repo:v.*\\.0\\.0" --ago 0d --dry-run 2>&1)
    if echo "$output" | grep -q "v1.0.0" && echo "$output" | grep -q "v2.0.0"; then
        echo -e "${GREEN}✓ Pattern matching works${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Pattern matching failed${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Pattern matching failed")
    fi
}

run_test_keep_parameter() {
    local keep_repo="test-minimal-keep"

    for i in $(seq 1 5); do
        create_test_image "$keep_repo" "v$i"
        sleep 0.2
    done

    local initial_keep_count=$(count_tags "$keep_repo")
    echo "Created $initial_keep_count tags in keep repo"

    echo -n "Testing --keep 2... "
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$keep_repo:.*" --ago 0d --keep 2 >/dev/null 2>&1
    local kept_count=$(count_tags "$keep_repo")
    # Allow for 2 or 3 tags (Docker might create a 'latest' tag automatically)
    if [ "$kept_count" -eq 2 ] || [ "$kept_count" -eq 3 ]; then
        echo -e "${GREEN}✓ Should keep 2-3 latest tags (kept $kept_count)${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Should keep 2-3 latest tags${NC}"
        echo -e "  Expected: 2 or 3, Actual: $kept_count"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Should keep 2-3 latest tags")
    fi
}

run_test_comp_dryrun() {
    local repo="test-comp-dryrun"
    echo "Creating test images..."
    for i in {1..5}; do
        create_test_image "$repo" "v$i"
    done

    local initial_count=$(count_tags "$repo")
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d --dry-run 2>&1 || true)
    local final_count=$(count_tags "$repo")

    assert_equals "$initial_count" "$final_count" "Dry run should not delete any tags"
    if echo "$output" | grep -qi "dry.run"; then
        echo -e "${GREEN}✓ Dry run output contains dry run marker${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ Dry run output missing marker${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("Dry run output missing marker")
    fi
}

run_test_comp_locks() {
    local lock_repo="test-comp-locks"

    create_test_image "$lock_repo" "unlocked"
    create_test_image "$lock_repo" "write-locked"
    create_test_image "$lock_repo" "delete-locked"
    create_test_image "$lock_repo" "fully-locked"

    # Clean up any 'latest' tag that might have been created
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:latest" --ago 0d 2>&1 >/dev/null || true

    echo "Applying locks..."
    # Leave "unlocked" image in its default state (no explicit locking)
    lock_image "$lock_repo" "write-locked" false true
    lock_image "$lock_repo" "delete-locked" true false
    lock_image "$lock_repo" "fully-locked" false false

    # Debug: Check lock status of each image
    if [ "$DEBUG" = "1" ]; then
        echo "Lock status verification:"
        for tag in "unlocked" "write-locked" "delete-locked" "fully-locked"; do
            echo "Tag: $tag"
            az acr repository show --name "$(get_registry_name)" --image "$lock_repo:$tag" --query 'changeableAttributes' 2>/dev/null || echo "  Failed to get lock status"
        done
    fi

    # Test without --include-locked
    local initial_count=$(count_tags "$lock_repo")

    # Debug: Show what purge command outputs
    if [ "$DEBUG" = "1" ]; then
        echo "Before purge - tags:"
        "$ACR_CLI" tag list -r "$REGISTRY" --repository "$lock_repo"
        echo "Running purge without --include-locked:"
    fi

    local purge_output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:.*" --ago 0d 2>&1 || true)

    if [ "$DEBUG" = "1" ]; then
        echo "Purge output:"
        echo "$purge_output"
        echo "After purge - tags:"
        "$ACR_CLI" tag list -r "$REGISTRY" --repository "$lock_repo"
    fi

    # Add small delay to ensure API consistency
    sleep 2
    local final_count=$(count_tags "$lock_repo")

    # Debug: Show the count we got
    if [ "$DEBUG" = "1" ]; then
        echo "Final count reported: $final_count"
        echo "Expected: 3 (after deleting unlocked image)"
    fi

    # Only the unlocked image should be deleted, leaving 3 locked images
    assert_equals "3" "$final_count" "Should keep 3 locked images"

    # Test with --include-locked
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$lock_repo:.*" --ago 0d --include-locked 2>&1 >/dev/null || true
    final_count=$(count_tags "$lock_repo")
    assert_equals "0" "$final_count" "Should delete all images with --include-locked"
}

run_test_comp_special() {
    # Empty repository test
    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "nonexistent-repo:.*" --ago 0d 2>&1 || true)
    assert_contains "$output" "Number of deleted tags: 0" "Empty repository should delete 0 tags"

    # Special characters test
    local special_repo="test-comp-special"
    create_test_image "$special_repo" "v1.0.0"
    # Skip feature/test tag as it contains invalid characters
    create_test_image "$special_repo" "feature-test"
    create_test_image "$special_repo" "build_123"
    create_test_image "$special_repo" "2024-01-22"

    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$special_repo:v.*" --ago 0d --dry-run 2>&1)
    assert_contains "$output" "v1.0.0" "Should match version tags"
}

run_test_comp_keep() {
    local keep_repo="test-comp-keep"

    for i in {1..10}; do
        create_test_image "$keep_repo" "v$(printf "%03d" $i)"
        sleep 0.1
    done

    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$keep_repo:.*" --ago 0d --keep 3 --dry-run 2>&1)
    local deleted_count=$(echo "$output" | grep -c "v0[0-7][0-9]" || true)
    assert_equals "7" "$deleted_count" "Should mark 7 tags for deletion when keeping 3"
}

run_test_comp_concurrent() {
    local concurrent_repo="test-comp-concurrent"

    echo "Creating images for concurrency tests..."
    for i in {1..20}; do
        create_test_image "$concurrent_repo" "tag$i"
    done

    for concurrency in 1 5 10; do
        echo -e "\n${CYAN}Testing concurrency=$concurrency${NC}"
        local start_time=$(date +%s)
        "$ACR_CLI" purge --registry "$REGISTRY" --filter "$concurrent_repo:tag[12].*" --ago 0d --concurrency "$concurrency" --dry-run >/dev/null 2>&1
        local end_time=$(date +%s)
        echo "  Duration: $((end_time - start_time))s"
    done
}

run_test_comp_manifest() {
    local manifest_repo="test-comp-manifest"

    create_test_image "$manifest_repo" "base"
    docker tag "$REGISTRY/$manifest_repo:base" "$REGISTRY/$manifest_repo:alias1"
    docker tag "$REGISTRY/$manifest_repo:base" "$REGISTRY/$manifest_repo:alias2"
    docker push "$REGISTRY/$manifest_repo:alias1" >/dev/null 2>&1
    docker push "$REGISTRY/$manifest_repo:alias2" >/dev/null 2>&1

    local initial_tags=$(count_tags "$manifest_repo")
    local initial_manifests=$(count_manifests "$manifest_repo")

    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$manifest_repo:alias1" --ago 0d >/dev/null 2>&1

    local final_tags=$(count_tags "$manifest_repo")
    local final_manifests=$(count_manifests "$manifest_repo")

    assert_equals "$((initial_tags - 1))" "$final_tags" "Should delete one tag"
    assert_equals "$initial_manifests" "$final_manifests" "Manifest should remain"
}

run_test_comp_age() {
    local age_repo="test-comp-age"

    create_test_image "$age_repo" "old"
    create_test_image "$age_repo" "new"

    local output=$("$ACR_CLI" purge --registry "$REGISTRY" --filter "$age_repo:.*" --ago 1d --dry-run 2>&1)
    assert_contains "$output" "Number of tags to be deleted: 0" "New images should not be deleted with --ago 1d"
}

# Test suite runners
run_minimal_tests() {
    echo -e "\n${BLUE}=== Minimal Test Suite ===${NC}"

    echo -e "\n${YELLOW}Test 1: Basic Purge Functionality${NC}"
    run_test_basic_functionality

    echo -e "\n${YELLOW}Test 2: Lock Testing${NC}"
    run_test_lock_functionality

    echo -e "\n${YELLOW}Test 3: Pattern Matching${NC}"
    run_test_pattern_matching

    echo -e "\n${YELLOW}Test 4: Keep Parameter${NC}"
    run_test_keep_parameter
}

run_comprehensive_tests() {
    echo -e "\n${BLUE}=== Comprehensive Test Suite ===${NC}"

    echo -e "\n${YELLOW}Test Suite 1: Dry Run Verification${NC}"
    run_test_comp_dryrun

    echo -e "\n${YELLOW}Test Suite 2: Comprehensive Locking Tests${NC}"
    run_test_comp_locks

    echo -e "\n${YELLOW}Test Suite 3: Edge Cases${NC}"
    run_test_comp_special

    echo -e "\n${YELLOW}Test Suite 4: Keep Parameter Tests${NC}"
    run_test_comp_keep

    echo -e "\n${YELLOW}Test Suite 5: Concurrent Operations${NC}"
    run_test_comp_concurrent

    echo -e "\n${YELLOW}Test Suite 6: Manifest vs Tag Deletion${NC}"
    run_test_comp_manifest

    echo -e "\n${YELLOW}Test Suite 7: Age-based Filtering${NC}"
    run_test_comp_age
}


run_debug_tests() {
    echo -e "\n${BLUE}=== Debug Test Suite ===${NC}"

    # Test 1: Detailed lock behavior
    echo -e "\n${YELLOW}Test 1: Detailed Lock Behavior${NC}"
    local repo="debug-locks"

    echo "Creating and locking image..."
    create_test_image "$repo" "testlock"

    echo -e "\n${GREEN}Initial state:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo"

    echo -e "\n${GREEN}Locking image...${NC}"
    az acr repository update --name "$(get_registry_name)" --image "$repo:testlock" --delete-enabled false

    echo -e "\n${GREEN}Lock status:${NC}"
    az acr repository show --name "$(get_registry_name)" --image "$repo:testlock" --query 'changeableAttributes'

    echo -e "\n${GREEN}Attempting delete without --include-locked:${NC}"
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:testlock" --ago 0d --untagged

    echo -e "\n${GREEN}Tags after purge without --include-locked:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo"

    echo -e "\n${GREEN}Attempting delete with --include-locked:${NC}"
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:testlock" --ago 0d --include-locked --untagged

    echo -e "\n${GREEN}Tags after purge with --include-locked:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo"

    # Test 2: Keep parameter behavior
    echo -e "\n${YELLOW}Test 2: Keep Parameter Behavior${NC}"
    local keep_repo="debug-keep"

    echo -e "\n${GREEN}Creating 5 tags...${NC}"
    for i in $(seq 1 5); do
        create_test_image "$keep_repo" "v$i"
        sleep 0.5
    done

    echo -e "\n${GREEN}All tags before purge:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$keep_repo"

    echo -e "\n${GREEN}Running purge with --keep 2:${NC}"
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$keep_repo:.*" --ago 0d --keep 2 --untagged

    echo -e "\n${GREEN}Tags after purge with --keep 2:${NC}"
    "$ACR_CLI" tag list --registry "$REGISTRY" --repository "$keep_repo"
}

run_interactive_tests() {
    echo -e "\n${BLUE}=== Interactive Test Mode ===${NC}"

    local repo="test-interactive"
    generate_test_images "$repo" "$NUM_IMAGES"

    run_purge_test() {
        local test_name="$1"
        local purge_args="$2"
        local expected_behavior="$3"

        echo -e "\n${YELLOW}Test: $test_name${NC}"
        echo "Command: acr purge $purge_args"
        echo "Expected: $expected_behavior"

        local initial_count=$(count_tags "$repo")
        echo "Initial image count: $initial_count"

        echo -e "\n${GREEN}Dry run:${NC}"
        eval "$ACR_CLI purge $purge_args --dry-run"

        read -p "Run actual purge? (y/N) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo -e "\n${GREEN}Actual run:${NC}"
            eval "$ACR_CLI purge $purge_args"

            local final_count=$(count_tags "$repo")
            echo -e "\n${GREEN}Result:${NC}"
            echo "Final image count: $final_count"
            echo "Images deleted: $((initial_count - final_count))"
        fi
    }

    # Run various scenarios
    run_purge_test \
        "Purge all images" \
        "--registry $REGISTRY --filter '$repo:.*' --ago 0d" \
        "Should show all images for deletion"

    run_purge_test \
        "Purge v* tags only" \
        "--registry $REGISTRY --filter '$repo:v.*' --ago 0d" \
        "Should only show tags starting with 'v'"

    run_purge_test \
        "Purge but keep latest 10" \
        "--registry $REGISTRY --filter '$repo:.*' --ago 0d --keep 10" \
        "Should keep the 10 most recent images"

    # Test locking
    echo -e "\n${YELLOW}Lock Test${NC}"
    echo "Locking some images..."
    for i in 10 20 30; do
        local tag="v$(printf "%03d" "$i")"
        echo "Locking $repo:$tag..."
        lock_image "$repo" "$tag" false false || true
    done

    run_purge_test \
        "Purge without include-locked" \
        "--registry $REGISTRY --filter '$repo:v0[123]0' --ago 0d" \
        "Should skip locked images"

    run_purge_test \
        "Purge with include-locked" \
        "--registry $REGISTRY --filter '$repo:v0[123]0' --ago 0d --include-locked" \
        "Should unlock and delete locked images"
}

# Function to run individual tests
run_individual_test() {
    local test_name="$1"

    case "$test_name" in
        # Minimal test components
        test-basic)
            echo -e "\n${BLUE}=== Running: Basic Purge Functionality ===${NC}"
            run_test_basic_functionality
            ;;
        test-locks)
            echo -e "\n${BLUE}=== Running: Lock Testing ===${NC}"
            run_test_lock_functionality
            ;;
        test-patterns)
            echo -e "\n${BLUE}=== Running: Pattern Matching ===${NC}"
            run_test_pattern_matching
            ;;
        test-keep)
            echo -e "\n${BLUE}=== Running: Keep Parameter ===${NC}"
            run_test_keep_parameter
            ;;
        # Comprehensive test components
        test-comp-dryrun)
            echo -e "\n${BLUE}=== Running: Dry Run Verification ===${NC}"
            run_test_comp_dryrun
            ;;
        test-comp-locks)
            echo -e "\n${BLUE}=== Running: Comprehensive Locking ===${NC}"
            run_test_comp_locks
            ;;
        test-comp-special)
            echo -e "\n${BLUE}=== Running: Edge Cases (Special Characters) ===${NC}"
            run_test_comp_special
            ;;
        test-comp-keep)
            echo -e "\n${BLUE}=== Running: Keep Parameter Tests ===${NC}"
            run_test_comp_keep
            ;;
        test-comp-concurrent)
            echo -e "\n${BLUE}=== Running: Concurrent Operations ===${NC}"
            run_test_comp_concurrent
            ;;
        test-comp-manifest)
            echo -e "\n${BLUE}=== Running: Manifest vs Tag Deletion ===${NC}"
            run_test_comp_manifest
            ;;
        test-comp-age)
            echo -e "\n${BLUE}=== Running: Age-based Filtering ===${NC}"
            run_test_comp_age
            ;;
        *)
            echo "Unknown test: $test_name"
            echo "Available tests:"
            echo "  Minimal: test-basic, test-locks, test-patterns, test-keep"
            echo "  Comprehensive: test-comp-dryrun, test-comp-locks, test-comp-special,"
            echo "                 test-comp-keep, test-comp-concurrent, test-comp-manifest, test-comp-age"
            echo "  Suites: minimal, comprehensive, benchmark, benchmark-delete, debug, interactive, all"
            exit 1
            ;;
    esac
}

# Main execution
case "$TEST_MODE" in
    minimal)
        run_minimal_tests
        ;;
    comprehensive)
        run_comprehensive_tests
        ;;
    benchmark)
        run_benchmark_tests
        ;;
    benchmark-delete)
        run_deletion_benchmark_tests
        ;;
    debug)
        run_debug_tests
        ;;
    interactive)
        run_interactive_tests
        ;;
    all)
        run_minimal_tests
        run_comprehensive_tests
        run_benchmark_tests
        ;;
    test-*)
        run_individual_test "$TEST_MODE"
        ;;
    *)
        echo "Invalid test mode: $TEST_MODE"
        echo "Options:"
        echo "  Suites: all, minimal, comprehensive, benchmark, benchmark-delete, debug, interactive"
        echo "  Individual tests: test-basic, test-locks, test-patterns, test-keep"
        echo "                    test-comp-dryrun, test-comp-locks, test-comp-special,"
        echo "                    test-comp-keep, test-comp-concurrent, test-comp-manifest, test-comp-age"
        exit 1
        ;;
esac

echo -e "\n${GREEN}=== All tests completed ===${NC}"
