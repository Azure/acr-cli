# Testing ACR Purge Command with Real Registry

This directory contains scripts to help test the `acr purge` command with a real Azure Container Registry.

## Prerequisites

- Azure CLI installed and authenticated (`az login`)
- Docker installed and running
- The ACR CLI tool built (`make binaries`)
- (Optional) Access to an existing Azure Container Registry

## Scripts

### 1. test-purge-all.sh (Consolidated Test Suite)

A unified test script that combines all test scenarios into one comprehensive suite. This is the recommended script for testing ACR purge functionality.

```bash
# Basic usage
./scripts/experimental/test-purge-all.sh [registry] [test_mode] [num_images]

# Test modes:
# - all: Run all test suites (default)
# - minimal: Quick basic functionality tests
# - comprehensive: Full test suite with assertions
# - benchmark: Performance benchmarking
# - debug: Detailed debugging output
# - interactive: Interactive testing with confirmations

# Examples:
# Use temporary registry for all tests
./scripts/experimental/test-purge-all.sh

# Use existing registry for minimal tests
./scripts/experimental/test-purge-all.sh myregistry.azurecr.io minimal

# Run comprehensive tests with custom image count
./scripts/experimental/test-purge-all.sh myregistry.azurecr.io comprehensive 100

# Run performance benchmarks
./scripts/experimental/test-purge-all.sh myregistry.azurecr.io benchmark

# Debug mode for troubleshooting
DEBUG=1 ./scripts/experimental/test-purge-all.sh myregistry.azurecr.io debug
```

Features:
- **Automatic registry creation**: Creates temporary registry if none provided
- **Multiple test modes**: Choose between quick, comprehensive, or focused testing
- **Comprehensive coverage**: Tests all purge functionality including locks, patterns, concurrency
- **Performance benchmarking**: Measures throughput with different configurations
- **Debug support**: Set DEBUG=1 for verbose output
- **Result tracking**: Outputs test summary and saves benchmark results to CSV

Test coverage includes:
- Basic purge functionality and dry-run verification
- Lock testing (all combinations of writeEnabled/deleteEnabled)
- Pattern matching and regex support
- Keep parameter functionality
- Concurrent operations with varying worker counts
- Manifest vs tag deletion scenarios
- Age-based filtering
- Edge cases and error handling

## Test Scenarios

The test harness covers:

1. **Basic Purge**: Delete all images older than 0 days
2. **Pattern Filtering**: Delete only tags matching specific patterns (e.g., `v*`, `dev-*`)
3. **Keep Latest**: Delete old images but keep the N most recent
4. **Concurrent Workers**: Test performance with different concurrency levels
5. **Locked Images**: Test behavior with and without `--include-locked` flag

## Manual Testing Examples

After generating test images, you can manually test various scenarios:

```bash
# Dry run - see what would be deleted
acr purge --registry myregistry.azurecr.io --filter 'test-repo:.*' --ago 0d --dry-run

# Delete all images older than 7 days
acr purge --registry myregistry.azurecr.io --filter 'test-repo:.*' --ago 7d

# Delete v* tags but keep latest 5
acr purge --registry myregistry.azurecr.io --filter 'test-repo:v.*' --ago 0d --keep 5

# Delete with high concurrency
acr purge --registry myregistry.azurecr.io --filter 'test-repo:build-.*' --ago 0d --concurrency 20

# Test with locked images
# First lock an image:
az acr repository update --name myregistry --image test-repo:v001 --write-enabled false

# Try to delete (will skip locked)
acr purge --registry myregistry.azurecr.io --filter 'test-repo:v001' --ago 0d

# Delete including locked images
acr purge --registry myregistry.azurecr.io --filter 'test-repo:v001' --ago 0d --include-locked
```

## Performance Testing

### Benchmark Mode with Hyperfine

The `test-purge-all.sh` script includes a comprehensive benchmark mode that automatically uses Hyperfine when available for accurate performance measurements:

```bash
# Run complete benchmark suite using Makefile
make benchmark

# Or run directly with a registry
./scripts/experimental/test-purge-all.sh myregistry.azurecr.io benchmark

# Use temporary registry for benchmarks
./scripts/experimental/test-purge-all.sh "" benchmark

# Configure benchmark parameters
WARMUP_RUNS=5 MIN_RUNS=20 ./scripts/experimental/test-purge-all.sh "" benchmark
```

The benchmark mode will:
- Check for Hyperfine and use it if available (falls back to basic timing if not)
- Automatically set up test data with multiple repositories
- Test single repository performance with varying concurrency (1, 5, 10, 20 workers)
- Test multiple repository performance
- Measure pattern complexity impact (simple vs complex regex)
- Test repository scaling performance
- Generate detailed reports in JSON and Markdown formats
- Create a comprehensive summary report

#### Benchmark Output Files
When using Hyperfine, the following files are generated:
- `benchmark-summary.md` - Overall summary of all benchmark results
- `benchmark-single-repo.{json,md}` - Single repository performance results
- `benchmark-multi-repo.{json,md}` - Multiple repository performance results
- `benchmark-patterns.{json,md}` - Pattern complexity comparison
- `benchmark-repo-scaling.{json,md}` - Repository count scaling results

#### Prerequisites for Hyperfine Benchmarks
- Install hyperfine: `cargo install hyperfine` (or via package manager)
- Azure CLI authenticated
- Docker running

### Quick Performance Tests

For quick performance validation:

```bash
# Run minimal test suite (includes basic performance metrics)
make benchmark-quick

# Or directly
./scripts/experimental/test-purge-all.sh myregistry.azurecr.io minimal
```

### Manual Performance Testing

To test performance manually with custom scenarios:

1. Generate test images:
   ```bash
   # Function is available after sourcing the script
   source ./scripts/experimental/test-purge-all.sh
   generate_test_images "perf-test" 1000
   ```

2. Test with different concurrency levels:
   ```bash
   time acr purge --registry myregistry.azurecr.io --filter 'perf-test:.*' --ago 0d --concurrency 1 --dry-run
   time acr purge --registry myregistry.azurecr.io --filter 'perf-test:.*' --ago 0d --concurrency 10 --dry-run
   time acr purge --registry myregistry.azurecr.io --filter 'perf-test:.*' --ago 0d --concurrency 50 --dry-run
   ```

## Safety Notes

- Always use `--dry-run` first to preview what will be deleted
- Use a dedicated test repository to avoid accidentally deleting production images
- The test scripts are destructive - they will delete images
- Make sure you have appropriate permissions in the ACR

## Temporary Registry Usage

When no registry is provided, the scripts will:
1. Create a temporary resource group and ACR with random names
2. Run all tests against the temporary registry
3. At the end, prompt you to delete the temporary resources
4. If you choose not to delete, the script will show the command to delete manually

Example temporary registry names:
- Registry: `acrtestab12cd34.azurecr.io`
- Resource Group: `rg-acr-test-ab12cd34`

## Cleanup

### For Temporary Registries
The scripts will automatically prompt for cleanup at the end. You can also manually delete:
```bash
az group delete --name rg-acr-test-<suffix> --yes
```

### For Existing Registries
To clean up test images from your registry:
```bash
# Delete entire test repository
az acr repository delete --name myregistry --repository test-repo --yes
```
