#!/bin/bash
set -uo pipefail

# Ultra-fast test image generation using Docker buildx and parallel pushing
# Usage: ./generate-test-images-batch.sh <registry> <repository> <num_images> [parallel_jobs]

REGISTRY="${1:-}"
REPO="${2:-}"
NUM_IMAGES="${3:-50}"
PARALLEL_JOBS="${4:-20}"

if [ -z "$REGISTRY" ] || [ -z "$REPO" ]; then
    echo "Usage: $0 <registry> <repository> <num_images> [parallel_jobs]"
    echo "Example: $0 myregistry.azurecr.io test-repo 100 20"
    exit 1
fi

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}Ultra-fast batch test image generation${NC}"
echo "Registry: $REGISTRY"
echo "Repository: $REPO" 
echo "Number of images: $NUM_IMAGES"
echo "Parallel jobs: $PARALLEL_JOBS"

# Create a temporary directory
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

# Create a minimal Dockerfile that produces tiny images
cat > "$TEMP_DIR/Dockerfile" << 'EOF'
FROM scratch
COPY timestamp /
EOF

# Method 1: Pre-create all images locally then push in parallel
echo -e "\n${YELLOW}Method 1: Batch creation with parallel push${NC}"

START_TIME=$(date +%s)

# Create a base timestamp file
echo "$(date)" > "$TEMP_DIR/timestamp"

# Build base image once
echo -e "${BLUE}Building base image...${NC}"
docker build -t "test-base:latest" "$TEMP_DIR" >/dev/null 2>&1

# Function to tag and queue for push
queue_image() {
    local i="$1"
    local tag="v$(printf "%03d" "$i")"
    docker tag "test-base:latest" "$REGISTRY/$REPO:$tag" 2>/dev/null
    echo "$REGISTRY/$REPO:$tag"
}

# Generate all tags
echo -e "${BLUE}Creating tags...${NC}"
for i in $(seq 1 "$NUM_IMAGES"); do
    queue_image "$i"
done > "$TEMP_DIR/images.txt"

# Push all images in parallel
echo -e "${BLUE}Pushing $NUM_IMAGES images with $PARALLEL_JOBS parallel workers...${NC}"
cat "$TEMP_DIR/images.txt" | xargs -P "$PARALLEL_JOBS" -I {} docker push {} >/dev/null 2>&1

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo -e "${GREEN}Method 1 completed!${NC}"
echo "Time taken: ${DURATION} seconds"
echo "Throughput: $(awk -v n="$NUM_IMAGES" -v d="$DURATION" 'BEGIN {printf "%.1f", n/d}') images/second"

# Method 2: Using docker manifest for even faster creation (if registry supports it)
echo -e "\n${YELLOW}Method 2: Manifest-based batch creation (experimental)${NC}"

# This method creates images by directly pushing manifests
# Only works with registries that support manifest manipulation

# Check if we can use experimental features
if docker manifest --help >/dev/null 2>&1; then
    echo -e "${BLUE}Docker manifest command available${NC}"
    
    # Create a single layer that we'll reuse
    BLOB_FILE="$TEMP_DIR/layer.tar"
    echo "test" | tar -cf "$BLOB_FILE" -
    
    # Note: Full manifest manipulation would require direct registry API access
    # For now, we'll stick with the parallel push method
    
    echo -e "${YELLOW}Note: Direct manifest manipulation requires registry API access${NC}"
else
    echo -e "${RED}Docker manifest command not available${NC}"
fi

# Cleanup local images to save space
echo -e "\n${BLUE}Cleaning up local images...${NC}"
docker rmi "test-base:latest" >/dev/null 2>&1
for i in $(seq 1 "$NUM_IMAGES"); do
    docker rmi "$REGISTRY/$REPO:v$(printf "%03d" "$i")" >/dev/null 2>&1 || true
done

echo -e "\n${GREEN}All done!${NC}"