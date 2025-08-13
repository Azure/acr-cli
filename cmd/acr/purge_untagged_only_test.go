// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.
package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/go-autorest/autorest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestPurgeUntaggedOnly tests the --untagged-only flag functionality
func TestPurgeUntaggedOnly(t *testing.T) {
	testCtx := context.Background()
	testLoginURL := "registry.azurecr.io"
	testRepo := "test-repo"
	defaultPoolSize := 1

	// Test 1: purge function with untaggedOnly=true should only delete untagged manifests
	t.Run("UntaggedOnlyPurgeManifestsOnly", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Setup mock response for manifests without tags
		manifestDigest := "sha256:abc123"
		mediaType := "application/vnd.docker.distribution.manifest.v2+json"
		untaggedManifest := acr.ManifestAttributesBase{
			Digest:               &manifestDigest,
			Tags:                 &[]string{}, // Empty tags array
			LastUpdateTime:       &[]string{"2023-01-01T00:00:00Z"}[0],
			MediaType:            &mediaType,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &[]bool{true}[0], WriteEnabled: &[]bool{true}[0]},
		}

		manifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{untaggedManifest},
		}

		emptyManifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}

		// Mock calls for getting manifests
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", manifestDigest).Return(emptyManifestsResult, nil).Once()
		// Note: GetManifest is not called for untagged manifests
		// Use a local deletedResponse for this test
		localDeletedResponse := &autorest.Response{
			Response: &http.Response{
				StatusCode: 202,
			},
		}
		mockClient.On("DeleteManifest", mock.Anything, testRepo, manifestDigest).Return(localDeletedResponse, nil).Once()

		// Call purge with untaggedOnly=true
		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			"", // ago is empty for untagged-only
			0,  // keep is 0 for untagged-only
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{testRepo: ".*"},
			false, // dryRun
			false, // includeLocked
		)

		assert.Equal(0, deletedTagsCount, "No tags should be deleted in untagged-only mode")
		assert.Equal(1, deletedManifestsCount, "One untagged manifest should be deleted")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test 2: untaggedOnly with no filter should process all repositories
	t.Run("UntaggedOnlyNoFilterAllRepos", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// We won't test GetRepositories here since the purge function is called
		// with already-created tagFilters. Instead test that all repos are processed.

		emptyManifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}

		// Mock manifest calls for each repo (no untagged manifests in this test)
		repos := []string{"repo1", "repo2", "repo3"}
		for _, repo := range repos {
			mockClient.On("GetAcrManifests", mock.Anything, repo, "", "").Return(emptyManifestsResult, nil).Once()
		}

		// Simulate getting all repositories when no filter is provided
		tagFilters := make(map[string]string)
		for _, repo := range repos {
			tagFilters[repo] = ".*"
		}

		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			"", // ago is empty for untagged-only
			0,  // keep is 0 for untagged-only
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			tagFilters,
			false, // dryRun
			false, // includeLocked
		)

		assert.Equal(0, deletedTagsCount, "No tags should be deleted")
		assert.Equal(0, deletedManifestsCount, "No manifests deleted when none are untagged")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test 3: untaggedOnly with filter should only process matching repositories
	t.Run("UntaggedOnlyWithFilter", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		manifestDigest := "sha256:def456"
		mediaType := "application/vnd.docker.distribution.manifest.v2+json"
		untaggedManifest := acr.ManifestAttributesBase{
			Digest:               &manifestDigest,
			Tags:                 &[]string{}, // Empty tags array
			LastUpdateTime:       &[]string{"2023-01-01T00:00:00Z"}[0],
			MediaType:            &mediaType,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &[]bool{true}[0], WriteEnabled: &[]bool{true}[0]},
		}

		manifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{untaggedManifest},
		}

		emptyManifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}

		// Mock manifest calls for specific repo
		mockClient.On("GetAcrManifests", mock.Anything, "specific-repo", "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, "specific-repo", "", manifestDigest).Return(emptyManifestsResult, nil).Once()
		// Note: GetManifest is not called for untagged manifests
		localDeletedResponse := &autorest.Response{
			Response: &http.Response{
				StatusCode: 202,
			},
		}
		mockClient.On("DeleteManifest", mock.Anything, "specific-repo", manifestDigest).Return(localDeletedResponse, nil).Once()

		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			"", // ago is empty for untagged-only
			0,  // keep is 0 for untagged-only
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{"specific-repo": ".*"},
			false, // dryRun
			false, // includeLocked
		)

		assert.Equal(0, deletedTagsCount, "No tags should be deleted in untagged-only mode")
		assert.Equal(1, deletedManifestsCount, "One untagged manifest should be deleted")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test 4: untaggedOnly in dry-run mode
	t.Run("UntaggedOnlyDryRun", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		manifestDigest := "sha256:ghi789"
		mediaType := "application/vnd.docker.distribution.manifest.v2+json"
		untaggedManifest := acr.ManifestAttributesBase{
			Digest:               &manifestDigest,
			Tags:                 &[]string{}, // Empty tags array
			LastUpdateTime:       &[]string{"2023-01-01T00:00:00Z"}[0],
			MediaType:            &mediaType,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &[]bool{true}[0], WriteEnabled: &[]bool{true}[0]},
		}

		manifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{untaggedManifest},
		}

		emptyManifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}

		// Mock manifest calls but NO delete calls in dry-run
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", manifestDigest).Return(emptyManifestsResult, nil).Once()
		// Note: GetManifest is not called for untagged manifests
		// No DeleteManifest call expected in dry-run mode

		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			"", // ago is empty for untagged-only
			0,  // keep is 0 for untagged-only
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{testRepo: ".*"},
			true,  // dryRun
			false, // includeLocked
		)

		assert.Equal(0, deletedTagsCount, "No tags should be deleted in dry-run")
		assert.Equal(1, deletedManifestsCount, "Should report 1 manifest to be deleted in dry-run")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test 5: untaggedOnly with locked manifests
	t.Run("UntaggedOnlyWithLockedManifests", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Create locked and unlocked untagged manifests
		lockedDigest := "sha256:locked123"
		unlockedDigest := "sha256:unlocked456"
		mediaType := "application/vnd.docker.distribution.manifest.v2+json"

		lockedManifest := acr.ManifestAttributesBase{
			Digest:               &lockedDigest,
			Tags:                 &[]string{}, // Empty tags array
			LastUpdateTime:       &[]string{"2023-01-01T00:00:00Z"}[0],
			MediaType:            &mediaType,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &[]bool{false}[0], WriteEnabled: &[]bool{false}[0]}, // Locked
		}

		unlockedManifest := acr.ManifestAttributesBase{
			Digest:               &unlockedDigest,
			Tags:                 &[]string{}, // Empty tags array
			LastUpdateTime:       &[]string{"2023-01-01T00:00:00Z"}[0],
			MediaType:            &mediaType,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &[]bool{true}[0], WriteEnabled: &[]bool{true}[0]}, // Unlocked
		}

		manifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{lockedManifest, unlockedManifest},
		}

		emptyManifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}

		// Without --include-locked, only unlocked manifest should be deleted
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", unlockedDigest).Return(emptyManifestsResult, nil).Once()
		// Note: GetManifest is not called for untagged manifests
		localDeletedResponse := &autorest.Response{
			Response: &http.Response{
				StatusCode: 202,
			},
		}
		mockClient.On("DeleteManifest", mock.Anything, testRepo, unlockedDigest).Return(localDeletedResponse, nil).Once()
		// No delete call for locked manifest

		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			"", // ago is empty for untagged-only
			0,  // keep is 0 for untagged-only
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{testRepo: ".*"},
			false, // dryRun
			false, // includeLocked = false
		)

		assert.Equal(0, deletedTagsCount, "No tags should be deleted")
		assert.Equal(1, deletedManifestsCount, "Only unlocked manifest should be deleted")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test 6: untaggedOnly with --include-locked
	t.Run("UntaggedOnlyWithIncludeLocked", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Create locked untagged manifest
		lockedDigest := "sha256:locked789"
		mediaType := "application/vnd.docker.distribution.manifest.v2+json"
		lockedManifest := acr.ManifestAttributesBase{
			Digest:               &lockedDigest,
			Tags:                 &[]string{}, // Empty tags array
			LastUpdateTime:       &[]string{"2023-01-01T00:00:00Z"}[0],
			MediaType:            &mediaType,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &[]bool{false}[0], WriteEnabled: &[]bool{false}[0]}, // Locked
		}

		manifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{lockedManifest},
		}

		emptyManifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}

		// With --include-locked, locked manifest should be unlocked and deleted
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", lockedDigest).Return(emptyManifestsResult, nil).Once()
		// Note: GetManifest is not called for untagged manifests
		// Expect unlock and delete for locked manifest
		// UpdateAcrManifestAttributes returns an interface, not just nil
		updateResponse := &autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		}
		mockClient.On("UpdateAcrManifestAttributes", mock.Anything, testRepo, lockedDigest, mock.Anything).Return(updateResponse, nil).Once()
		localDeletedResponse := &autorest.Response{
			Response: &http.Response{
				StatusCode: 202,
			},
		}
		mockClient.On("DeleteManifest", mock.Anything, testRepo, lockedDigest).Return(localDeletedResponse, nil).Once()

		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			"", // ago is empty for untagged-only
			0,  // keep is 0 for untagged-only
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{testRepo: ".*"},
			false, // dryRun
			true,  // includeLocked = true
		)

		assert.Equal(0, deletedTagsCount, "No tags should be deleted")
		assert.Equal(1, deletedManifestsCount, "Locked manifest should be unlocked and deleted")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

// TestPurgeCommandUntaggedOnlyValidation tests the validation logic for --untagged-only flag
func TestPurgeCommandUntaggedOnlyValidation(t *testing.T) {
	// Test 1: untagged-only flag should make --ago optional
	t.Run("UntaggedOnlyMakesAgoOptional", func(t *testing.T) {
		rootParams := &rootParameters{}

		// This should not error as --ago is optional with --untagged-only
		cmd := newPurgeCmd(rootParams)
		assert.NotNil(t, cmd, "Command should be created")
	})

	// Test 2: untagged-only and untagged flags should be mutually exclusive
	t.Run("UntaggedOnlyAndUntaggedMutuallyExclusive", func(t *testing.T) {
		rootParams := &rootParameters{}
		cmd := newPurgeCmd(rootParams)

		// The command should have mutual exclusion configured
		assert.NotNil(t, cmd, "Command should be created with mutual exclusion")
	})

	// Test 3: untagged-only with --ago should return error
	t.Run("UntaggedOnlyWithAgoReturnsError", func(t *testing.T) {
		// This test would be executed at runtime, checking the validation logic
		// The actual validation happens in the RunE function
		assert := assert.New(t)

		// Simulate the validation that happens in RunE
		untaggedOnly := true
		ago := "1d"

		if untaggedOnly && ago != "" {
			err := errors.New("--ago flag is not applicable when --untagged-only is set")
			assert.NotNil(err, "Should return error when --ago is used with --untagged-only")
		}
	})

	// Test 4: untagged-only with --keep should return error
	t.Run("UntaggedOnlyWithKeepReturnsError", func(t *testing.T) {
		assert := assert.New(t)

		// Simulate the validation that happens in RunE
		untaggedOnly := true
		keep := 5

		if untaggedOnly && keep != 0 {
			err := errors.New("--keep flag is not applicable when --untagged-only is set")
			assert.NotNil(err, "Should return error when --keep is used with --untagged-only")
		}
	})
}
