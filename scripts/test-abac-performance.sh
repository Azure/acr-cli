#!/bin/bash
set -uo pipefail

# ABAC Registry Performance Test Script
# Benchmarks and performance tests specifically for ABAC-enabled registries
# Focuses on testing token refresh, concurrent operations, and repository-level permissions

# Test Configuration
REGISTRY="${1:-}"
NUM_IMAGES="${2:-100}"
NUM_REPOS="${3:-5}"

# Path configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACR_CLI="${SCRIPT_DIR}/../bin/acr"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

# Performance metrics
declare -A METRICS

# Helper to measure execution time
measure_time() {
    local start_time end_time duration
    
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS: Use perl for high-resolution time
        start_time=$(perl -MTime::HiRes=time -e 'printf "%.3f\n", time')
        "$@"
        local exit_code=$?
        end_time=$(perl -MTime::HiRes=time -e 'printf "%.3f\n", time')
    else
        # Linux: Use date with nanoseconds
        start_time=$(date +%s.%N)
        "$@"
        local exit_code=$?
        end_time=$(date +%s.%N)
    fi
    
    duration=$(awk -v e="$end_time" -v s="$start_time" 'BEGIN {printf "%.3f", e-s}')
    echo "$duration"
    return $exit_code
}

# Validate prerequisites
validate_setup() {
    if [ -z "$REGISTRY" ]; then
        echo -e "${RED}Error: Registry not specified${NC}"
        echo "Usage: $0 <registry> [num_images] [num_repos]"
        echo "Example: $0 myregistry.azurecr.io 100 5"
        exit 1
    fi
    
    if ! command -v az >/dev/null 2>&1; then
        echo -e "${RED}Error: Azure CLI not found${NC}"
        exit 1
    fi
    
    if ! command -v docker >/dev/null 2>&1; then
        echo -e "${RED}Error: Docker not found${NC}"
        exit 1
    fi
    
    if [ ! -f "$ACR_CLI" ]; then
        echo "Building ACR CLI..."
        (cd "$SCRIPT_DIR/.." && make binaries)
    fi
    
    # Login to registry
    local registry_name="${REGISTRY%%.*}"
    echo "Logging in to registry..."
    az acr login --name "$registry_name" >/dev/null 2>&1
}

# Create test images efficiently
create_test_images_batch() {
    local repo="$1"
    local count="$2"
    local base_image="mcr.microsoft.com/hello-world"
    
    echo -e "${CYAN}Creating $count images in $repo...${NC}"
    
    # Pull base image once
    docker pull "$base_image" >/dev/null 2>&1
    
    # Create and push in batches
    local batch_size=10
    for ((i=1; i<=count; i+=batch_size)); do
        for ((j=i; j<i+batch_size && j<=count; j++)); do
            docker tag "$base_image" "$REGISTRY/$repo:v$(printf "%04d" $j)" &
        done
        wait
        
        for ((j=i; j<i+batch_size && j<=count; j++)); do
            docker push "$REGISTRY/$repo:v$(printf "%04d" $j)" >/dev/null 2>&1 &
        done
        wait
        
        echo "  Progress: $j/$count images"
    done
}

# Test 1: Token Refresh Performance
test_token_refresh_performance() {
    echo -e "\n${YELLOW}=== Test: Token Refresh Performance ===${NC}"
    echo "Testing how ABAC handles token refresh across multiple repositories"
    
    # Create test repositories
    local repos=()
    for i in $(seq 1 3); do
        repos+=("abac-perf-token-$i")
        create_test_images_batch "abac-perf-token-$i" 10
    done
    
    # Test sequential access to different repositories
    echo -e "\n${CYAN}Sequential repository access (forces token refresh):${NC}"
    
    local total_time=0
    for repo in "${repos[@]}"; do
        local duration=$(measure_time "$ACR_CLI" tag list \
            --registry "$REGISTRY" \
            --repository "$repo" >/dev/null 2>&1)
        echo "  $repo: ${duration}s"
        total_time=$(awk -v t="$total_time" -v d="$duration" 'BEGIN {printf "%.3f", t+d}')
    done
    
    METRICS["token_refresh_sequential"]="$total_time"
    echo -e "${GREEN}Total sequential time: ${total_time}s${NC}"
    
    # Test rapid switching between repositories
    echo -e "\n${CYAN}Rapid repository switching (stress test token management):${NC}"
    
    local switch_time=$(measure_time bash -c "
        for i in {1..10}; do
            for repo in ${repos[*]}; do
                '$ACR_CLI' tag list --registry '$REGISTRY' --repository \"\$repo\" >/dev/null 2>&1
            done
        done
    ")
    
    METRICS["token_refresh_rapid"]="$switch_time"
    echo -e "${GREEN}Rapid switching time (30 operations): ${switch_time}s${NC}"
    
    # Clean up
    for repo in "${repos[@]}"; do
        "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d >/dev/null 2>&1
    done
}

# Test 2: Repository-Level Permission Performance
test_repository_permission_performance() {
    echo -e "\n${YELLOW}=== Test: Repository-Level Permission Performance ===${NC}"
    echo "Testing performance with repository-specific permissions"
    
    # Create repositories with different numbers of images
    local small_repo="abac-perf-small"
    local medium_repo="abac-perf-medium"
    local large_repo="abac-perf-large"
    
    create_test_images_batch "$small_repo" 10
    create_test_images_batch "$medium_repo" 50
    create_test_images_batch "$large_repo" "$NUM_IMAGES"
    
    # Test listing performance
    echo -e "\n${CYAN}Repository listing performance:${NC}"
    
    for repo in "$small_repo" "$medium_repo" "$large_repo"; do
        local tag_count=$("$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo" 2>/dev/null | wc -l)
        local duration=$(measure_time "$ACR_CLI" tag list \
            --registry "$REGISTRY" \
            --repository "$repo" >/dev/null 2>&1)
        
        local throughput=$(awk -v c="$tag_count" -v d="$duration" 'BEGIN {
            if (d > 0) printf "%.1f", c/d
            else print "N/A"
        }')
        
        echo "  $repo ($tag_count tags): ${duration}s (${throughput} tags/sec)"
        METRICS["list_${repo}"]="$duration"
    done
    
    # Test deletion performance
    echo -e "\n${CYAN}Repository deletion performance:${NC}"
    
    for repo in "$small_repo" "$medium_repo" "$large_repo"; do
        local tag_count=$("$ACR_CLI" tag list --registry "$REGISTRY" --repository "$repo" 2>/dev/null | wc -l)
        local duration=$(measure_time "$ACR_CLI" purge \
            --registry "$REGISTRY" \
            --filter "$repo:.*" \
            --ago 0d \
            --dry-run >/dev/null 2>&1)
        
        local throughput=$(awk -v c="$tag_count" -v d="$duration" 'BEGIN {
            if (d > 0) printf "%.1f", c/d
            else print "N/A"
        }')
        
        echo "  $repo ($tag_count tags): ${duration}s (${throughput} tags/sec)"
        METRICS["purge_dryrun_${repo}"]="$duration"
    done
    
    # Clean up
    for repo in "$small_repo" "$medium_repo" "$large_repo"; do
        "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d >/dev/null 2>&1
    done
}

# Test 3: Concurrent Operations Across Repositories
test_concurrent_cross_repository() {
    echo -e "\n${YELLOW}=== Test: Concurrent Cross-Repository Operations ===${NC}"
    echo "Testing concurrent operations across multiple ABAC-protected repositories"
    
    # Create test repositories
    local repos=()
    for i in $(seq 1 "$NUM_REPOS"); do
        repos+=("abac-perf-concurrent-$i")
        create_test_images_batch "abac-perf-concurrent-$i" 20
    done
    
    # Test different concurrency levels
    echo -e "\n${CYAN}Testing various concurrency levels:${NC}"
    
    for concurrency in 1 5 10 20; do
        echo -e "\n${BLUE}Concurrency: $concurrency${NC}"
        
        # Purge across all repositories
        local duration=$(measure_time "$ACR_CLI" purge \
            --registry "$REGISTRY" \
            --filter "abac-perf-concurrent-.*:v000[1-5]" \
            --ago 0d \
            --concurrency "$concurrency" >/dev/null 2>&1)
        
        local total_deleted=$((NUM_REPOS * 5))
        local throughput=$(awk -v n="$total_deleted" -v d="$duration" 'BEGIN {
            if (d > 0) printf "%.1f", n/d
            else print "N/A"
        }')
        
        echo "  Time: ${duration}s"
        echo "  Throughput: ${throughput} deletions/sec"
        echo "  Repositories affected: $NUM_REPOS"
        
        METRICS["concurrent_${concurrency}"]="$duration"
        
        # Recreate deleted images for next test
        if [ "$concurrency" -lt 20 ]; then
            for repo in "${repos[@]}"; do
                for i in {1..5}; do
                    docker tag "mcr.microsoft.com/hello-world" "$REGISTRY/$repo:v$(printf "%04d" $i)"
                    docker push "$REGISTRY/$repo:v$(printf "%04d" $i)" >/dev/null 2>&1
                done
            done
        fi
    done
    
    # Clean up
    for repo in "${repos[@]}"; do
        "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d >/dev/null 2>&1
    done
}

# Test 4: Pattern Matching Performance
test_pattern_matching_performance() {
    echo -e "\n${YELLOW}=== Test: Pattern Matching Performance ===${NC}"
    echo "Testing regex pattern matching performance in ABAC context"
    
    local repo="abac-perf-patterns"
    
    # Create images with various naming patterns
    echo -e "${CYAN}Creating images with diverse naming patterns...${NC}"
    
    local base_image="mcr.microsoft.com/hello-world"
    docker pull "$base_image" >/dev/null 2>&1
    
    # Version tags
    for i in {1..30}; do
        docker tag "$base_image" "$REGISTRY/$repo:v1.$(printf "%d" $i).0"
        docker push "$REGISTRY/$repo:v1.$(printf "%d" $i).0" >/dev/null 2>&1
    done
    
    # Environment tags
    for env in dev staging prod; do
        for i in {1..10}; do
            docker tag "$base_image" "$REGISTRY/$repo:${env}-$(printf "%03d" $i)"
            docker push "$REGISTRY/$repo:${env}-$(printf "%03d" $i)" >/dev/null 2>&1
        done
    done
    
    # Build tags
    for i in {1..20}; do
        docker tag "$base_image" "$REGISTRY/$repo:build-$(date +%Y%m%d)-$(printf "%03d" $i)"
        docker push "$REGISTRY/$repo:build-$(date +%Y%m%d)-$(printf "%03d" $i)" >/dev/null 2>&1
    done
    
    echo -e "\n${CYAN}Testing pattern matching performance:${NC}"
    
    # Simple pattern
    echo -e "\n${BLUE}Simple pattern (.*):${NC}"
    local duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$repo:.*" \
        --ago 0d \
        --dry-run >/dev/null 2>&1)
    echo "  Time: ${duration}s"
    METRICS["pattern_simple"]="$duration"
    
    # Medium complexity pattern
    echo -e "\n${BLUE}Medium pattern (v1\.[0-9]+\.0):${NC}"
    duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$repo:v1\.[0-9]+\.0" \
        --ago 0d \
        --dry-run >/dev/null 2>&1)
    echo "  Time: ${duration}s"
    METRICS["pattern_medium"]="$duration"
    
    # Complex pattern
    echo -e "\n${BLUE}Complex pattern ((dev|staging)-[0-9]{3}):${NC}"
    duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$repo:(dev|staging)-[0-9]{3}" \
        --ago 0d \
        --dry-run >/dev/null 2>&1)
    echo "  Time: ${duration}s"
    METRICS["pattern_complex"]="$duration"
    
    # Very complex pattern
    echo -e "\n${BLUE}Very complex pattern (build-2024[0-9]{4}-0[0-1][0-9]):${NC}"
    duration=$(measure_time "$ACR_CLI" purge \
        --registry "$REGISTRY" \
        --filter "$repo:build-2024[0-9]{4}-0[0-1][0-9]" \
        --ago 0d \
        --dry-run >/dev/null 2>&1)
    echo "  Time: ${duration}s"
    METRICS["pattern_very_complex"]="$duration"
    
    # Clean up
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d >/dev/null 2>&1
}

# Test 5: Scale Testing
test_scale_performance() {
    echo -e "\n${YELLOW}=== Test: Scale Performance ===${NC}"
    echo "Testing ABAC performance at different scales"
    
    local scales=(10 50 100 200)
    
    echo -e "\n${CYAN}Testing at different scales:${NC}"
    
    for scale in "${scales[@]}"; do
        if [ "$scale" -gt "$NUM_IMAGES" ]; then
            echo -e "${YELLOW}Skipping scale $scale (exceeds NUM_IMAGES=$NUM_IMAGES)${NC}"
            continue
        fi
        
        echo -e "\n${BLUE}Scale: $scale images${NC}"
        
        local repo="abac-perf-scale-$scale"
        
        # Create images
        local create_time=$(measure_time create_test_images_batch "$repo" "$scale")
        echo "  Creation time: ${create_time}s"
        METRICS["scale_${scale}_create"]="$create_time"
        
        # List performance
        local list_time=$(measure_time "$ACR_CLI" tag list \
            --registry "$REGISTRY" \
            --repository "$repo" >/dev/null 2>&1)
        echo "  List time: ${list_time}s"
        METRICS["scale_${scale}_list"]="$list_time"
        
        # Purge dry-run performance
        local purge_time=$(measure_time "$ACR_CLI" purge \
            --registry "$REGISTRY" \
            --filter "$repo:.*" \
            --ago 0d \
            --dry-run >/dev/null 2>&1)
        echo "  Purge (dry-run) time: ${purge_time}s"
        METRICS["scale_${scale}_purge_dry"]="$purge_time"
        
        # Actual purge performance
        local delete_time=$(measure_time "$ACR_CLI" purge \
            --registry "$REGISTRY" \
            --filter "$repo:.*" \
            --ago 0d >/dev/null 2>&1)
        echo "  Purge (actual) time: ${delete_time}s"
        METRICS["scale_${scale}_purge_actual"]="$delete_time"
        
        # Calculate throughput
        local create_throughput=$(awk -v n="$scale" -v d="$create_time" 'BEGIN {
            if (d > 0) printf "%.1f", n/d
            else print "N/A"
        }')
        local delete_throughput=$(awk -v n="$scale" -v d="$delete_time" 'BEGIN {
            if (d > 0) printf "%.1f", n/d
            else print "N/A"
        }')
        
        echo "  Create throughput: ${create_throughput} images/sec"
        echo "  Delete throughput: ${delete_throughput} images/sec"
    done
}

# Test 6: Keep Parameter Performance
test_keep_parameter_performance() {
    echo -e "\n${YELLOW}=== Test: Keep Parameter Performance ===${NC}"
    echo "Testing performance impact of --keep parameter with ABAC"
    
    local repo="abac-perf-keep"
    
    # Create test images
    create_test_images_batch "$repo" "$NUM_IMAGES"
    
    echo -e "\n${CYAN}Testing different keep values:${NC}"
    
    for keep in 0 10 25 50; do
        echo -e "\n${BLUE}Keep: $keep images${NC}"
        
        local duration=$(measure_time "$ACR_CLI" purge \
            --registry "$REGISTRY" \
            --filter "$repo:.*" \
            --ago 0d \
            --keep "$keep" \
            --dry-run >/dev/null 2>&1)
        
        local to_delete=$((NUM_IMAGES - keep))
        if [ "$to_delete" -lt 0 ]; then
            to_delete=0
        fi
        
        echo "  Time: ${duration}s"
        echo "  Images to delete: $to_delete"
        echo "  Images to keep: $keep"
        
        METRICS["keep_${keep}"]="$duration"
    done
    
    # Clean up
    "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d >/dev/null 2>&1
}

# Print performance summary
print_performance_summary() {
    echo -e "\n${MAGENTA}=== Performance Test Summary ===${NC}"
    echo -e "${CYAN}Registry: $REGISTRY${NC}"
    echo -e "${CYAN}Test Configuration:${NC}"
    echo "  Images per test: $NUM_IMAGES"
    echo "  Number of repositories: $NUM_REPOS"
    echo ""
    
    echo -e "${YELLOW}Key Performance Metrics:${NC}"
    
    # Token Refresh
    if [ -n "${METRICS[token_refresh_sequential]:-}" ]; then
        echo -e "\n${BLUE}Token Refresh:${NC}"
        echo "  Sequential access: ${METRICS[token_refresh_sequential]}s"
        echo "  Rapid switching (30 ops): ${METRICS[token_refresh_rapid]}s"
    fi
    
    # Concurrent Operations
    if [ -n "${METRICS[concurrent_1]:-}" ]; then
        echo -e "\n${BLUE}Concurrent Operations:${NC}"
        for c in 1 5 10 20; do
            if [ -n "${METRICS[concurrent_${c}]:-}" ]; then
                echo "  Concurrency $c: ${METRICS[concurrent_${c}]}s"
            fi
        done
    fi
    
    # Pattern Matching
    if [ -n "${METRICS[pattern_simple]:-}" ]; then
        echo -e "\n${BLUE}Pattern Matching:${NC}"
        echo "  Simple pattern: ${METRICS[pattern_simple]}s"
        echo "  Medium pattern: ${METRICS[pattern_medium]}s"
        echo "  Complex pattern: ${METRICS[pattern_complex]}s"
        echo "  Very complex: ${METRICS[pattern_very_complex]}s"
    fi
    
    # Scale Testing
    echo -e "\n${BLUE}Scale Performance:${NC}"
    for scale in 10 50 100 200; do
        if [ -n "${METRICS[scale_${scale}_purge_actual]:-}" ]; then
            echo "  $scale images deletion: ${METRICS[scale_${scale}_purge_actual]}s"
        fi
    done
    
    # Generate CSV output for further analysis
    echo -e "\n${YELLOW}CSV Output (for further analysis):${NC}"
    echo "metric,value"
    for metric in "${!METRICS[@]}"; do
        echo "$metric,${METRICS[$metric]}"
    done | sort
}

# Main execution
main() {
    echo -e "${MAGENTA}=== ABAC Registry Performance Test Suite ===${NC}"
    echo "Starting performance tests..."
    echo ""
    
    # Validate setup
    validate_setup
    
    # Run performance tests
    test_token_refresh_performance
    test_repository_permission_performance
    test_concurrent_cross_repository
    test_pattern_matching_performance
    test_scale_performance
    test_keep_parameter_performance
    
    # Print summary
    print_performance_summary
    
    echo -e "\n${GREEN}Performance tests completed successfully!${NC}"
}

# Cleanup trap
cleanup() {
    echo -e "\n${YELLOW}Cleaning up test repositories...${NC}"
    
    # Clean up any remaining test repositories
    for pattern in "abac-perf-*"; do
        local repos=$("$ACR_CLI" repository list --registry "$REGISTRY" 2>/dev/null | grep "$pattern" || true)
        for repo in $repos; do
            "$ACR_CLI" purge --registry "$REGISTRY" --filter "$repo:.*" --ago 0d --include-locked >/dev/null 2>&1 || true
        done
    done
    
    echo -e "${GREEN}Cleanup completed${NC}"
}

# Set up cleanup trap
trap cleanup EXIT

# Run main function
main "$@"