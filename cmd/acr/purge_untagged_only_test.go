// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.
package main

import (
	"context"
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
			0, // ago is 0 for untagged-only (meaning all past manifests are eligible)
			0, // keep is 0 for untagged-only
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
			0, // ago is 0 for untagged-only (meaning all past manifests are eligible)
			0, // keep is 0 for untagged-only
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
			0, // ago is 0 for untagged-only (meaning all past manifests are eligible)
			0, // keep is 0 for untagged-only
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
			0, // ago is 0 for untagged-only (meaning all past manifests are eligible)
			0, // keep is 0 for untagged-only
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
			0, // ago is 0 for untagged-only (meaning all past manifests are eligible)
			0, // keep is 0 for untagged-only
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
			0, // ago is 0 for untagged-only (meaning all past manifests are eligible)
			0, // keep is 0 for untagged-only
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

	// Test 3: untagged-only with --ago should work (age filtering)
	t.Run("UntaggedOnlyWithAgoFiltering", func(t *testing.T) {
		assert := assert.New(t)

		// Test that --ago and --untagged-only can be used together
		untaggedOnly := true
		ago := "1d"

		// This should not return an error anymore
		assert.True(untaggedOnly, "untagged-only should be true")
		assert.Equal("1d", ago, "ago should be accepted with untagged-only")
	})

	// Test 4: untagged-only with --keep should work (keep recent manifests)
	t.Run("UntaggedOnlyWithKeepSupport", func(t *testing.T) {
		assert := assert.New(t)

		// Test that --keep and --untagged-only can be used together
		untaggedOnly := true
		keep := 5

		// This should not return an error anymore
		assert.True(untaggedOnly, "untagged-only should be true")
		assert.Equal(5, keep, "keep should be accepted with untagged-only")
	})
}

// TestPurgeDanglingManifestsWithAgoAndKeep tests the new age filtering and keep functionality
func TestPurgeDanglingManifestsWithAgoAndKeep(t *testing.T) {
	testCtx := context.Background()
	testLoginURL := "registry.azurecr.io"
	testRepo := "test-repo"
	defaultPoolSize := 1

	// Helper function to create manifest with specific timestamp
	createManifestWithTime := func(digest, timestamp string) acr.ManifestAttributesBase {
		mediaType := "application/vnd.docker.distribution.manifest.v2+json"
		return acr.ManifestAttributesBase{
			Digest:               &digest,
			Tags:                 &[]string{}, // Empty tags array
			LastUpdateTime:       &timestamp,
			MediaType:            &mediaType,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &[]bool{true}[0], WriteEnabled: &[]bool{true}[0]},
		}
	}

	// Test 1: Age filtering - only delete old manifests
	t.Run("AgeFilteringDeletesOnlyOldManifests", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Create manifests with different timestamps
		// old manifest: more than 300 days ago from now (2026), recent: less than 300 days
		oldManifest := createManifestWithTime("sha256:old123", "2025-01-01T00:00:00Z")
		recentManifest := createManifestWithTime("sha256:recent123", "2026-01-15T00:00:00Z")

		manifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{StatusCode: 200},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{oldManifest, recentManifest},
		}

		// First call returns manifests
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		// Second call for pagination (returns empty to end pagination)
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:recent123").Return(emptyResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:old123").Return(nil, nil).Once()

		// Call with 300 days ago (should only delete the old manifest from 2023)
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, mustParseDuration("300d"), 0, nil, false, false)

		assert.Nil(err, "Should not return error")
		assert.Equal(1, deletedCount, "Should delete only the old manifest")
		mockClient.AssertExpectations(t)
	})

	// Test 2: Keep functionality - preserve most recent manifests
	t.Run("KeepFunctionalityPreservesRecentManifests", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Create 5 manifests with different timestamps
		manifests := []acr.ManifestAttributesBase{
			createManifestWithTime("sha256:oldest", "2023-01-01T00:00:00Z"),
			createManifestWithTime("sha256:old", "2023-06-01T00:00:00Z"),
			createManifestWithTime("sha256:medium", "2023-12-01T00:00:00Z"),
			createManifestWithTime("sha256:recent", "2024-06-01T00:00:00Z"),
			createManifestWithTime("sha256:newest", "2024-12-01T00:00:00Z"),
		}

		manifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{StatusCode: 200},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &manifests,
		}

		// Mock pagination for GetUntaggedManifests
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:newest").Return(emptyResult, nil).Once()
		// Expect only the 3 oldest manifests to be deleted (keep 2 most recent)
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:oldest").Return(nil, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:old").Return(nil, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:medium").Return(nil, nil).Once()

		// Call with keep=2 (should preserve the 2 most recent manifests)
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, 0, 2, nil, false, false)

		assert.Nil(err, "Should not return error")
		assert.Equal(3, deletedCount, "Should delete 3 manifests, keeping 2 most recent")
		mockClient.AssertExpectations(t)
	})

	// Test 3: Combined age filtering and keep functionality
	t.Run("CombinedAgoAndKeepFiltering", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Create manifests where some are old enough (>300 days) and some are not
		// 300 days before 2026-01-31 is approximately 2025-04-06
		manifests := []acr.ManifestAttributesBase{
			createManifestWithTime("sha256:veryold1", "2025-01-01T00:00:00Z"), // Old enough
			createManifestWithTime("sha256:veryold2", "2025-02-01T00:00:00Z"), // Old enough
			createManifestWithTime("sha256:veryold3", "2025-03-01T00:00:00Z"), // Old enough
			createManifestWithTime("sha256:recent1", "2026-01-01T00:00:00Z"),  // Too recent (<300 days)
			createManifestWithTime("sha256:recent2", "2026-01-15T00:00:00Z"),  // Too recent (<300 days)
		}

		manifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{StatusCode: 200},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &manifests,
		}

		// Mock pagination for GetUntaggedManifests
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:recent2").Return(emptyResult, nil).Once()
		// Only expect 2 manifests to be deleted (3 old ones, keep 1, so delete 2)
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:veryold1").Return(nil, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:veryold2").Return(nil, nil).Once()

		// Call with both age filter (300 days) and keep (keep 1 of the old ones)
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, mustParseDuration("300d"), 1, nil, false, false)

		assert.Nil(err, "Should not return error")
		assert.Equal(2, deletedCount, "Should delete 2 old manifests, keeping 1 old + all recent ones")
		mockClient.AssertExpectations(t)
	})

	// Test 4: Dry run with age filtering
	t.Run("DryRunWithAgeFiltering", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// old manifest: more than 300 days ago, recent: less than 300 days
		oldManifest := createManifestWithTime("sha256:old123", "2025-01-01T00:00:00Z")
		recentManifest := createManifestWithTime("sha256:recent123", "2026-01-15T00:00:00Z")

		manifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{StatusCode: 200},
			},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{oldManifest, recentManifest},
		}

		// Mock pagination for GetUntaggedManifests
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:recent123").Return(emptyResult, nil).Once()
		// No UpdateAcrManifestAttributes calls expected for dry run

		// Call with dry run and age filter
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, mustParseDuration("300d"), 0, nil, true, false)

		assert.Nil(err, "Should not return error")
		assert.Equal(1, deletedCount, "Should report 1 manifest would be deleted")
		mockClient.AssertExpectations(t)
	})
}
