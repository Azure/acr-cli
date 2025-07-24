#!/bin/bash
set -uo pipefail

# Fast test image generation script with parallel pushing
# Usage: ./generate-test-images-fast.sh <registry> <repository> <num_images> [parallel_jobs]

REGISTRY="${1:-}"
REPO="${2:-}"
NUM_IMAGES="${3:-50}"
PARALLEL_JOBS="${4:-10}"

if [ -z "$REGISTRY" ] || [ -z "$REPO" ]; then
    echo "Usage: $0 <registry> <repository> <num_images> [parallel_jobs]"
    echo "Example: $0 myregistry.azurecr.io test-repo 100 20"
    exit 1
fi

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

BASE_IMAGE="mcr.microsoft.com/hello-world:latest"

echo -e "${GREEN}Fast test image generation${NC}"
echo "Registry: $REGISTRY"
echo "Repository: $REPO"
echo "Number of images: $NUM_IMAGES"
echo "Parallel jobs: $PARALLEL_JOBS"

# Pull base image once
echo -e "\n${YELLOW}Pulling base image...${NC}"
docker pull "$BASE_IMAGE" >/dev/null 2>&1

# Create a temporary directory for batch files
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

# Progress tracking
PROGRESS_FILE="$TEMP_DIR/progress"
echo "0" > "$PROGRESS_FILE"

# Function to push a single image
push_image() {
    local tag="$1"
    local full_tag="$REGISTRY/$REPO:$tag"

    # Tag and push
    if docker tag "$BASE_IMAGE" "$full_tag" 2>/dev/null && \
       docker push "$full_tag" >/dev/null 2>&1; then
        # Update progress counter
        local count=$(($(cat "$PROGRESS_FILE") + 1))
        echo "$count" > "$PROGRESS_FILE"

        # Show progress every 10 images
        if [ $((count % 10)) -eq 0 ]; then
            printf "\r${GREEN}Progress: %d/%d images pushed${NC}" "$count" "$TOTAL_TAGS"
        fi
    else
        echo -n "!"
    fi
}

# Export function for parallel execution
export -f push_image
export BASE_IMAGE REGISTRY REPO PROGRESS_FILE TOTAL_TAGS GREEN NC

# Generate all tag names
echo -e "\n${YELLOW}Generating tags...${NC}"
TAG_FILE="$TEMP_DIR/tags.txt"

for i in $(seq 1 "$NUM_IMAGES"); do
    # Create variations of tags
    echo "v$(printf "%03d" "$i")" >> "$TAG_FILE"

    # Add additional tag variations for non-minimal mode
    if [ "$NUM_IMAGES" -le 100 ]; then
        echo "$(date -u +%Y%m%d)-$i" >> "$TAG_FILE"
        echo "build-$(printf "%04d" "$i")" >> "$TAG_FILE"

        # Add some 'latest' and 'dev' tags
        if [ $((i % 10)) -eq 0 ]; then
            echo "latest-$i" >> "$TAG_FILE"
        fi
        if [ $((i % 5)) -eq 0 ]; then
            echo "dev-$i" >> "$TAG_FILE"
        fi
    fi
done

TOTAL_TAGS=$(wc -l < "$TAG_FILE")
echo "Total tags to push: $TOTAL_TAGS"

# Push images in parallel
echo -e "\n${YELLOW}Pushing images in parallel (${PARALLEL_JOBS} workers)...${NC}"
START_TIME=$(date +%s)

# Use xargs for parallel execution
cat "$TAG_FILE" | xargs -P "$PARALLEL_JOBS" -I {} bash -c 'push_image "$@"' _ {}

# Final progress update
FINAL_COUNT=$(cat "$PROGRESS_FILE")
printf "\r${GREEN}Progress: %d/%d images pushed${NC}\n" "$FINAL_COUNT" "$TOTAL_TAGS"

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo -e "\n${GREEN}Completed!${NC}"
echo "Time taken: ${DURATION} seconds"
if [ "$DURATION" -gt 0 ]; then
    echo "Average: $(awk -v d="$DURATION" -v t="$TOTAL_TAGS" 'BEGIN {printf "%.2f", d/t}') seconds per image"
    echo "Throughput: $(awk -v t="$TOTAL_TAGS" -v d="$DURATION" 'BEGIN {printf "%.1f", t/d}') images/second"
else
    echo "Average: N/A (duration too short)"
    echo "Throughput: N/A (duration too short)"
fi
