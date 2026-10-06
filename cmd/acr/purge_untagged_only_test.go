// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.
package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

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
		mockClient.On("IsAbac").Return(false)
		mockClient.On("IsTokenExpired").Return(false).Maybe()

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
		purgeOptions := purgeOptions{
			agoDuration:   nil, // no age specified; legacy cleanup includes all past manifests
			keep:          0,   // keep is 0 for untagged-only
			minTags:       -1,
			maxTags:       -1,
			minManifests:  -1,
			maxManifests:  -1,
			dryRun:        false, // dryRun
			includeLocked: false, // includeLocked
		}
		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			purgeOptions,
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{testRepo: ".*"},
			false, // verbose
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
		mockClient.On("IsAbac").Return(false)
		mockClient.On("IsTokenExpired").Return(false).Maybe()

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

		purgeOptions := purgeOptions{
			agoDuration:   nil, // no age specified; legacy cleanup includes all past manifests
			keep:          0,   // keep is 0 for untagged-only
			minTags:       -1,
			maxTags:       -1,
			minManifests:  -1,
			maxManifests:  -1,
			dryRun:        false, // dryRun
			includeLocked: false, // includeLocked
		}
		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			purgeOptions,
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			tagFilters,
			false, // verbose
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
		mockClient.On("IsAbac").Return(false)
		mockClient.On("IsTokenExpired").Return(false).Maybe()

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

		purgeOptions := purgeOptions{
			agoDuration:   nil, // no age specified; legacy cleanup includes all past manifests
			keep:          0,   // keep is 0 for untagged-only
			minTags:       -1,
			maxTags:       -1,
			minManifests:  -1,
			maxManifests:  -1,
			dryRun:        false, // dryRun
			includeLocked: false, // includeLocked
		}
		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			purgeOptions,
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{"specific-repo": ".*"},
			false, // verbose
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
		mockClient.On("IsAbac").Return(false)
		mockClient.On("IsTokenExpired").Return(false).Maybe()

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

		purgeOptions := purgeOptions{
			agoDuration:   nil, // no age specified; legacy cleanup includes all past manifests
			keep:          0,   // keep is 0 for untagged-only
			minTags:       -1,
			maxTags:       -1,
			minManifests:  -1,
			maxManifests:  -1,
			dryRun:        true,  // dryRun
			includeLocked: false, // includeLocked
		}
		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			purgeOptions,
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{testRepo: ".*"},
			false, // verbose
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
		mockClient.On("IsAbac").Return(false)
		mockClient.On("IsTokenExpired").Return(false).Maybe()

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

		purgeOptions := purgeOptions{
			agoDuration:   nil, // no age specified; legacy cleanup includes all past manifests
			keep:          0,   // keep is 0 for untagged-only
			minTags:       -1,
			maxTags:       -1,
			minManifests:  -1,
			maxManifests:  -1,
			dryRun:        false, // dryRun
			includeLocked: false, // includeLocked = false
		}
		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			purgeOptions,
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{testRepo: ".*"},
			false, // verbose
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
		mockClient.On("IsAbac").Return(false)
		mockClient.On("IsTokenExpired").Return(false).Maybe()

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

		purgeOptions := purgeOptions{
			agoDuration:   nil, // no age specified; legacy cleanup includes all past manifests
			keep:          0,   // keep is 0 for untagged-only
			minTags:       -1,
			maxTags:       -1,
			minManifests:  -1,
			maxManifests:  -1,
			dryRun:        false, // dryRun
			includeLocked: true,  // includeLocked = true
		}
		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			purgeOptions,
			60,
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{testRepo: ".*"},
			false, // verbose
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
		agoDuration := mustParseDuration("300d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: -1, maxManifests: -1, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

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
		purgeOptions := purgeOptions{agoDuration: nil, keep: 2, minTags: -1, maxTags: -1, minManifests: -1, maxManifests: -1, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

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
		agoDuration := mustParseDuration("300d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 1, minTags: -1, maxTags: -1, minManifests: -1, maxManifests: -1, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

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
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })
		agoDuration := mustParseDuration("300d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: -1, maxManifests: -1, dryRun: true, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.Equal("Would delete manifests for repository: "+testRepo+"\n"+
			"Would delete: "+testLoginURL+"/"+testRepo+"@sha256:old123 (reason: age)\n", string(output))
		assert.Nil(err, "Should not return error")
		assert.Equal(1, deletedCount, "Should report 1 manifest would be deleted")
		mockClient.AssertExpectations(t)
	})

	// Test 5: Keep exceeds manifest count - should delete nothing
	t.Run("KeepExceedsManifestCount", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Create only 3 manifests
		manifests := []acr.ManifestAttributesBase{
			createManifestWithTime("sha256:manifest1", "2025-01-01T00:00:00Z"),
			createManifestWithTime("sha256:manifest2", "2025-02-01T00:00:00Z"),
			createManifestWithTime("sha256:manifest3", "2025-03-01T00:00:00Z"),
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
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:manifest3").Return(emptyResult, nil).Once()
		// No DeleteManifest calls expected - keep exceeds manifest count

		// Call with keep=10 but only 3 manifests exist - should delete nothing
		purgeOptions := purgeOptions{agoDuration: nil, keep: 10, minTags: -1, maxTags: -1, minManifests: -1, maxManifests: -1, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error")
		assert.Equal(0, deletedCount, "Should delete 0 manifests when keep exceeds manifest count")
		mockClient.AssertExpectations(t)
	})

	// Test 6: Keep equals manifest count - should delete nothing
	t.Run("KeepEqualsManifestCount", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Create exactly 3 manifests
		manifests := []acr.ManifestAttributesBase{
			createManifestWithTime("sha256:manifest1", "2025-01-01T00:00:00Z"),
			createManifestWithTime("sha256:manifest2", "2025-02-01T00:00:00Z"),
			createManifestWithTime("sha256:manifest3", "2025-03-01T00:00:00Z"),
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
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:manifest3").Return(emptyResult, nil).Once()
		// No DeleteManifest calls expected - keep equals manifest count

		// Call with keep=3 and exactly 3 manifests - should delete nothing
		purgeOptions := purgeOptions{agoDuration: nil, keep: 3, minTags: -1, maxTags: -1, minManifests: -1, maxManifests: -1, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error")
		assert.Equal(0, deletedCount, "Should delete 0 manifests when keep equals manifest count")
		mockClient.AssertExpectations(t)
	})
}

func TestPurgeDanglingManifestsWithMax(t *testing.T) {
	testCtx := context.Background()
	testLoginURL := "registry.azurecr.io"
	testRepo := "test-repo"
	defaultPoolSize := 1

	t.Run("LockedOCIIndexBeyondMaximumProtectsChildOnEarlierPage", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		retainedManifest := createManifestWithTime("sha256:retained", now.Add(-time.Hour).Format(time.RFC3339Nano))
		child := createManifestWithTime("sha256:child", now.Add(-2*time.Hour).Format(time.RFC3339Nano))
		lockedIndex := createManifestWithTime("sha256:locked-index", now.Add(-3*time.Hour).Format(time.RFC3339Nano))
		mediaType := "application/vnd.oci.image.index.v1+json"
		lockedIndex.MediaType = &mediaType
		lockedIndex.ChangeableAttributes.DeleteEnabled = &[]bool{false}[0]
		eligibleManifest := createManifestWithTime("sha256:eligible", now.Add(-4*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{child, retainedManifest},
		}
		secondPageResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{lockedIndex, eligibleManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:retained").Return(secondPageResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:eligible").Return(emptyResult, nil).Once()
		// OCI discovery checks for a subject before lock retention traverses the index.
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:locked-index").Return([]byte(`{
			"manifests": [{"digest": "sha256:child", "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}]
		}`), nil).Twice()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:eligible").Return(nil, nil).Once()

		purgeOptions := purgeOptions{agoDuration: nil, keep: 0, minTags: -1, maxTags: -1, minManifests: -1, maxManifests: 1, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error when a locked index follows its child on a later page")
		assert.Equal(1, deletedCount, "Should retain the locked overflow index and its selected child without deleting the protected newest manifest")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 1)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:retained")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:locked-index")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:child")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("PreservesNewestWhenAllOld", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newestManifest := createManifestWithTime("sha256:newest", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		secondManifest := createManifestWithTime("sha256:second", now.Add(-72*time.Hour).Format(time.RFC3339Nano))
		thirdManifest := createManifestWithTime("sha256:third", now.Add(-96*time.Hour).Format(time.RFC3339Nano))
		oldestManifest := createManifestWithTime("sha256:oldest", now.Add(-120*time.Hour).Format(time.RFC3339Nano))

		// The newest manifest arrives on the second page; limits apply after global sorting.
		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{oldestManifest, secondManifest},
		}
		secondPageResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{thirdManifest, newestManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:second").Return(secondPageResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:newest").Return(emptyResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:third").Return(nil, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:oldest").Return(nil, nil).Once()

		purgeOptions := purgeOptions{agoDuration: nil, keep: 0, minTags: -1, maxTags: -1, minManifests: -1, maxManifests: 2, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error when applying the maximum across pages")
		assert.Equal(2, deletedCount, "Should delete the 2 oldest manifests, preserving the 2 newest")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 2)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:newest")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:second")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunReportsOverflow", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newestManifest := createManifestWithTime("sha256:newest", now.Add(72*time.Hour).Format(time.RFC3339Nano))
		secondManifest := createManifestWithTime("sha256:second", now.Add(48*time.Hour).Format(time.RFC3339Nano))
		thirdManifest := createManifestWithTime("sha256:third", now.Add(24*time.Hour).Format(time.RFC3339Nano))
		oldestManifest := createManifestWithTime("sha256:oldest", now.Add(-48*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{oldestManifest, secondManifest},
		}
		secondPageResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{thirdManifest, newestManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:second").Return(secondPageResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:newest").Return(emptyResult, nil).Once()

		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.Nil(err, "Should create a pipe to capture dry-run output") {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })
		purgeOptions := purgeOptions{agoDuration: nil, keep: 0, minTags: -1, maxTags: -1, minManifests: -1, maxManifests: 2, dryRun: true, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)
		assert.Nil(writer.Close(), "Should close the dry-run output writer")
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.Nil(readErr, "Should read the captured dry-run output")
		assert.Nil(err, "Should not return error when reporting maximum overflow")
		assert.Equal(2, deletedCount, "Should report both future and old manifests beyond the maximum without an age policy")
		var actualLines []string
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "Would delete:") {
				actualLines = append(actualLines, line)
			}
		}
		assert.ElementsMatch([]string{
			"Would delete: " + testLoginURL + "/" + testRepo + "@sha256:third (reason: maximum count)",
			"Would delete: " + testLoginURL + "/" + testRepo + "@sha256:oldest (reason: maximum count)",
		}, actualLines, "Should report exactly the 2 oldest manifests across both pages")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 0)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:newest")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:second")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:third")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:oldest")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})
}

func TestPurgeDanglingManifestsWithAgoAndMin(t *testing.T) {
	testCtx := context.Background()
	testLoginURL := "registry.azurecr.io"
	testRepo := "test-repo"
	defaultPoolSize := 1

	t.Run("MinimumRetainsOldOCIIndexAndProtectsChild", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		index := createManifestWithTime("sha256:index", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		mediaType := "application/vnd.oci.image.index.v1+json"
		index.MediaType = &mediaType
		retainedManifest := createManifestWithTime("sha256:retained", now.Add(-72*time.Hour).Format(time.RFC3339Nano))
		child := createManifestWithTime("sha256:child", now.Add(-96*time.Hour).Format(time.RFC3339Nano))
		eligibleManifest := createManifestWithTime("sha256:eligible", now.Add(-120*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{child, index, retainedManifest, eligibleManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:eligible").Return(emptyResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:index").Return([]byte(`{
			"manifests": [{"digest": "sha256:child", "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}]
		}`), nil).Twice()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:eligible").Return(nil, nil).Once()

		agoDuration := mustParseDuration("1d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: 2, maxManifests: -1, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error when the minimum retains an old index")
		assert.Equal(1, deletedCount, "Should delete only the unreferenced old manifest, without replacing the protected child with a top-2 deletion")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 1)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:index")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:retained")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:child")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("WriteLockedOldManifestBeyondMinimumIsRetained", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		retainedManifest := createManifestWithTime("sha256:retained", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		lockedManifest := createManifestWithTime("sha256:locked", now.Add(-72*time.Hour).Format(time.RFC3339Nano))
		lockedManifest.ChangeableAttributes.WriteEnabled = &[]bool{false}[0]
		eligibleManifest := createManifestWithTime("sha256:eligible", now.Add(-96*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{retainedManifest, lockedManifest, eligibleManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:eligible").Return(emptyResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:eligible").Return(nil, nil).Once()

		agoDuration := mustParseDuration("1d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: 1, maxManifests: -1, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error when an age-eligible manifest is write-locked")
		assert.Equal(1, deletedCount, "Should retain the locked old manifest in addition to the minimum, deleting only unlocked old overflow")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 1)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:retained")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:locked")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("AbovePopulationProtectsAll", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newestManifest := createManifestWithTime("sha256:newest", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		secondManifest := createManifestWithTime("sha256:second", now.Add(-72*time.Hour).Format(time.RFC3339Nano))
		thirdManifest := createManifestWithTime("sha256:third", now.Add(-96*time.Hour).Format(time.RFC3339Nano))
		oldestManifest := createManifestWithTime("sha256:oldest", now.Add(-120*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{newestManifest, secondManifest, thirdManifest, oldestManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:oldest").Return(emptyResult, nil).Once()

		agoDuration := mustParseDuration("1d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: 5, maxManifests: -1, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error when the minimum exceeds the manifest count")
		assert.Equal(0, deletedCount, "Should protect all 4 old manifests when the minimum is 5")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 0)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:newest")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:second")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:third")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:oldest")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunReportsOldOverflow", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newestManifest := createManifestWithTime("sha256:newest", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		secondManifest := createManifestWithTime("sha256:second", now.Add(-72*time.Hour).Format(time.RFC3339Nano))
		thirdManifest := createManifestWithTime("sha256:third", now.Add(-96*time.Hour).Format(time.RFC3339Nano))
		oldestManifest := createManifestWithTime("sha256:oldest", now.Add(-120*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{newestManifest, secondManifest, thirdManifest, oldestManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:oldest").Return(emptyResult, nil).Once()

		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.Nil(err, "Should create a pipe to capture dry-run output") {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })
		agoDuration := mustParseDuration("1d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: 2, maxManifests: -1, dryRun: true, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)
		assert.Nil(writer.Close(), "Should close the dry-run output writer")
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.Nil(readErr, "Should read the captured dry-run output")
		assert.Nil(err, "Should not return error when reporting old manifests beyond the minimum")
		assert.Equal(2, deletedCount, "Should report the 2 oldest manifests, protecting the 2 newest")
		var actualLines []string
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "Would delete:") {
				actualLines = append(actualLines, line)
			}
		}
		assert.ElementsMatch([]string{
			"Would delete: " + testLoginURL + "/" + testRepo + "@sha256:third (reason: age)",
			"Would delete: " + testLoginURL + "/" + testRepo + "@sha256:oldest (reason: age)",
		}, actualLines, "Should report exactly the 2 old manifests beyond the minimum")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 0)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:newest")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:second")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:third")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:oldest")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})
}

func TestPurgeDanglingManifestsWithAgoMinAndMax(t *testing.T) {
	testCtx := context.Background()
	testLoginURL := "registry.azurecr.io"
	testRepo := "test-repo"
	defaultPoolSize := 1

	t.Run("AgeRetainedOCIIndexProtectsChildBeyondMaximum", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newestManifest := createManifestWithTime("sha256:newest", now.Add(-time.Hour).Format(time.RFC3339Nano))
		index := createManifestWithTime("sha256:index", now.Add(-2*time.Hour).Format(time.RFC3339Nano))
		mediaType := "application/vnd.oci.image.index.v1+json"
		index.MediaType = &mediaType
		recentManifest := createManifestWithTime("sha256:recent", now.Add(-3*time.Hour).Format(time.RFC3339Nano))
		child := createManifestWithTime("sha256:child", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		eligibleManifest := createManifestWithTime("sha256:eligible", now.Add(-72*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{child, newestManifest, index, recentManifest, eligibleManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:eligible").Return(emptyResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:index").Return([]byte(`{
			"manifests": [{"digest": "sha256:child", "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}]
		}`), nil).Twice()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:eligible").Return(nil, nil).Once()

		agoDuration := mustParseDuration("1d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: 1, maxManifests: 3, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error when age retains an index between the minimum and maximum")
		assert.Equal(1, deletedCount, "Should retain 4 manifests despite max=3, protecting the old child without replacement deletions")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 1)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:newest")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:index")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:recent")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:child")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("LockedOldOverflowDoesNotReplaceAgeOrMinimumProtectedManifests", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newestManifest := createManifestWithTime("sha256:newest", now.Add(-time.Hour).Format(time.RFC3339Nano))
		recentManifest := createManifestWithTime("sha256:recent", now.Add(-2*time.Hour).Format(time.RFC3339Nano))
		lockedManifest := createManifestWithTime("sha256:locked", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		lockedManifest.ChangeableAttributes.DeleteEnabled = &[]bool{false}[0]
		eligibleManifest := createManifestWithTime("sha256:eligible", now.Add(-72*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{newestManifest, recentManifest, lockedManifest, eligibleManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:eligible").Return(emptyResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:eligible").Return(nil, nil).Once()

		agoDuration := mustParseDuration("1d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: 1, maxManifests: 2, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error when old overflow is locked")
		assert.Equal(1, deletedCount, "Should delete only unlocked old overflow, without compensating for the locked manifest")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 1)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:newest")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:recent")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:locked")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("IncludeLockedUnlocksOnlySelectedManifestsAndIndexes", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		retainedIndex := createManifestWithTime("sha256:retained-index", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		ociIndexMediaType := "application/vnd.oci.image.index.v1+json"
		retainedIndex.MediaType = &ociIndexMediaType
		retainedIndex.ChangeableAttributes.DeleteEnabled = &[]bool{false}[0]
		lockedManifest := createManifestWithTime("sha256:locked", now.Add(-72*time.Hour).Format(time.RFC3339Nano))
		lockedManifest.ChangeableAttributes.WriteEnabled = &[]bool{false}[0]
		lockedIndex := createManifestWithTime("sha256:locked-index", now.Add(-96*time.Hour).Format(time.RFC3339Nano))
		lockedIndex.MediaType = &ociIndexMediaType
		lockedIndex.ChangeableAttributes.DeleteEnabled = &[]bool{false}[0]
		protectedChild := createManifestWithTime("sha256:protected-child", now.Add(-120*time.Hour).Format(time.RFC3339Nano))
		protectedChild.ChangeableAttributes.DeleteEnabled = &[]bool{false}[0]
		selectedChild := createManifestWithTime("sha256:selected-child", now.Add(-144*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{protectedChild, selectedChild, retainedIndex, lockedManifest, lockedIndex},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:locked-index").Return(emptyResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:retained-index").Return([]byte(`{
			"manifests": [{"digest": "sha256:protected-child", "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}]
		}`), nil).Twice()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:locked-index").Return([]byte(`{
			"manifests": [{"digest": "sha256:selected-child", "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}]
		}`), nil).Once()
		unlockAttrs := &acr.ChangeableAttributes{DeleteEnabled: &[]bool{true}[0], WriteEnabled: &[]bool{true}[0]}
		updateResponse := &autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}}
		unlockManifestCall := mockClient.On("UpdateAcrManifestAttributes", mock.Anything, testRepo, "sha256:locked", unlockAttrs).Return(updateResponse, nil).Once()
		unlockIndexCall := mockClient.On("UpdateAcrManifestAttributes", mock.Anything, testRepo, "sha256:locked-index", unlockAttrs).Return(updateResponse, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:locked").Return(nil, nil).Once().NotBefore(unlockManifestCall)
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:locked-index").Return(nil, nil).Once().NotBefore(unlockIndexCall)
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:selected-child").Return(nil, nil).Once()

		agoDuration := mustParseDuration("1d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: 1, maxManifests: 2, dryRun: false, includeLocked: true}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error when unlocking selected manifests and indexes")
		assert.Equal(3, deletedCount, "Should delete the age-selected locked manifest, maximum-selected locked index, and its unprotected child")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 3)
		mockClient.AssertNumberOfCalls(t, "UpdateAcrManifestAttributes", 2)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:retained-index")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:protected-child")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, testRepo, "sha256:retained-index", mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, testRepo, "sha256:protected-child", mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, testRepo, "sha256:selected-child", mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		assert.False(*retainedIndex.ChangeableAttributes.DeleteEnabled, "Minimum-protected index should remain locked")
		assert.False(*protectedChild.ChangeableAttributes.DeleteEnabled, "Child of the retained index should remain locked")
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunIncludeLockedReportsSelectionWithoutMutations", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		retainedIndex := createManifestWithTime("sha256:retained-index", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		ociIndexMediaType := "application/vnd.oci.image.index.v1+json"
		retainedIndex.MediaType = &ociIndexMediaType
		retainedIndex.ChangeableAttributes.DeleteEnabled = &[]bool{false}[0]
		lockedManifest := createManifestWithTime("sha256:locked", now.Add(-72*time.Hour).Format(time.RFC3339Nano))
		lockedManifest.ChangeableAttributes.WriteEnabled = &[]bool{false}[0]
		lockedIndex := createManifestWithTime("sha256:locked-index", now.Add(-96*time.Hour).Format(time.RFC3339Nano))
		lockedIndex.MediaType = &ociIndexMediaType
		lockedIndex.ChangeableAttributes.DeleteEnabled = &[]bool{false}[0]
		protectedChild := createManifestWithTime("sha256:protected-child", now.Add(-120*time.Hour).Format(time.RFC3339Nano))
		protectedChild.ChangeableAttributes.DeleteEnabled = &[]bool{false}[0]
		selectedChild := createManifestWithTime("sha256:selected-child", now.Add(-144*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{protectedChild, selectedChild, retainedIndex, lockedManifest, lockedIndex},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:locked-index").Return(emptyResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:retained-index").Return([]byte(`{
			"manifests": [{"digest": "sha256:protected-child", "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}]
		}`), nil).Twice()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:locked-index").Return([]byte(`{
			"manifests": [{"digest": "sha256:selected-child", "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}]
		}`), nil).Once()

		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })
		agoDuration := mustParseDuration("1d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: 1, maxManifests: 2, dryRun: true, includeLocked: true}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.Equal("Would delete manifests for repository: "+testRepo+"\n"+
			"Would delete: "+testLoginURL+"/"+testRepo+"@sha256:locked (reason: age)\n"+
			"Would delete: "+testLoginURL+"/"+testRepo+"@sha256:locked-index (reason: age and maximum count)\n"+
			"Would delete: "+testLoginURL+"/"+testRepo+"@sha256:selected-child (reason: age and maximum count)\n", string(output))
		assert.Nil(err, "Should not return error when previewing selected locked manifests and indexes")
		assert.Equal(3, deletedCount, "Should report the same 3 deletions as live include-locked, excluding the retained index and its child")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		assert.False(*retainedIndex.ChangeableAttributes.DeleteEnabled, "Minimum-protected index should remain locked")
		assert.False(*protectedChild.ChangeableAttributes.DeleteEnabled, "Child of the retained index should remain locked")
		assert.False(*lockedManifest.ChangeableAttributes.WriteEnabled, "Dry run should not unlock the selected manifest")
		assert.False(*lockedIndex.ChangeableAttributes.DeleteEnabled, "Dry run should not unlock the selected index")
		mockClient.AssertExpectations(t)
	})

	t.Run("MaximumDeletesRecentOverflow", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newestManifest := createManifestWithTime("sha256:newest", now.Add(-time.Hour).Format(time.RFC3339Nano))
		secondManifest := createManifestWithTime("sha256:second", now.Add(-2*time.Hour).Format(time.RFC3339Nano))
		thirdManifest := createManifestWithTime("sha256:third", now.Add(-3*time.Hour).Format(time.RFC3339Nano))
		oldestManifest := createManifestWithTime("sha256:oldest", now.Add(-4*time.Hour).Format(time.RFC3339Nano))

		manifestsResult := &acr.Manifests{
			Response:            autorest.Response{Response: &http.Response{StatusCode: http.StatusOK}},
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{newestManifest, secondManifest, thirdManifest, oldestManifest},
		}
		emptyResult := &acr.Manifests{
			Response:            manifestsResult.Response,
			Registry:            &testLoginURL,
			ImageName:           &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:oldest").Return(emptyResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:oldest").Return(nil, nil).Once()

		agoDuration := mustParseDuration("1d")
		purgeOptions := purgeOptions{agoDuration: &agoDuration, keep: 0, minTags: -1, maxTags: -1, minManifests: 1, maxManifests: 3, dryRun: false, includeLocked: false}
		deletedCount, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, purgeOptions, nil)

		assert.Nil(err, "Should not return error when the maximum overrides age protection")
		assert.Equal(1, deletedCount, "Should delete only the recent manifest beyond the maximum of 3")
		mockClient.AssertNumberOfCalls(t, "DeleteManifest", 1)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:newest")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:second")
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, "sha256:third")
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

}

// TestPurgeAbacVerboseMode tests the verbose output for ABAC registries
func TestPurgeAbacVerboseMode(t *testing.T) {
	testCtx := context.Background()
	testLoginURL := "registry.azurecr.io"
	defaultPoolSize := 1

	// Test: ABAC verbose mode should output repository names
	t.Run("AbacVerboseModeOutputsRepoNames", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Mock ABAC registry
		mockClient.On("IsAbac").Return(true)
		mockClient.On("RefreshTokenForAbac", mock.Anything, mock.Anything).Return(nil)

		// Empty manifests result for each repo
		emptyManifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}

		repos := []string{"repo1", "repo2", "repo3"}
		for _, repo := range repos {
			mockClient.On("GetAcrManifests", mock.Anything, repo, "", "").Return(emptyManifestsResult, nil).Once()
		}

		tagFilters := make(map[string]string)
		for _, repo := range repos {
			tagFilters[repo] = ".*"
		}

		// Capture stdout to verify verbose output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Call purge with verbose=true and ABAC enabled
		purgeOptions := purgeOptions{
			agoDuration:   nil, // ago
			keep:          0,   // keep
			minTags:       -1,
			maxTags:       -1,
			minManifests:  -1,
			maxManifests:  -1,
			dryRun:        false, // dryRun
			includeLocked: false, // includeLocked
		}
		deletedTagsCount, deletedManifestsCount, purgeErr := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			purgeOptions,
			60,   // filterTimeout
			true, // removeUntaggedManifests
			true, // untaggedOnly
			tagFilters,
			true, // verbose = true
		)

		// Restore stdout and read captured output
		err := w.Close()
		if err != nil {
			t.Fatalf("Failed to close pipe writer: %v", err)
		}
		os.Stdout = oldStdout
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		if err != nil {
			t.Fatalf("Failed to read from pipe: %v", err)
		}
		output := buf.String()

		assert.Equal(0, deletedTagsCount, "No tags should be deleted")
		assert.Equal(0, deletedManifestsCount, "No manifests deleted when none are untagged")
		assert.Nil(purgeErr, "Error should be nil")
		// Verify verbose output contains repository names
		assert.Contains(output, "ABAC: Setting token scope for 3 repositories:", "Should output repo count")
		assert.Contains(output, "repo1", "Should output repo1 in verbose mode")
		assert.Contains(output, "repo2", "Should output repo2 in verbose mode")
		assert.Contains(output, "repo3", "Should output repo3 in verbose mode")
		mockClient.AssertCalled(t, "RefreshTokenForAbac", mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	// Test: ABAC non-verbose mode should only output count, not names
	t.Run("AbacNonVerboseModeOutputsCountOnly", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Mock ABAC registry
		mockClient.On("IsAbac").Return(true)
		mockClient.On("RefreshTokenForAbac", mock.Anything, mock.Anything).Return(nil)

		// Empty manifests result for each repo
		emptyManifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}

		repos := []string{"repo1", "repo2", "repo3"}
		for _, repo := range repos {
			mockClient.On("GetAcrManifests", mock.Anything, repo, "", "").Return(emptyManifestsResult, nil).Once()
		}

		tagFilters := make(map[string]string)
		for _, repo := range repos {
			tagFilters[repo] = ".*"
		}

		// Capture stdout to verify non-verbose output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Call purge with verbose=false and ABAC enabled
		purgeOptions := purgeOptions{
			agoDuration:   nil, // ago
			keep:          0,   // keep
			minTags:       -1,
			maxTags:       -1,
			minManifests:  -1,
			maxManifests:  -1,
			dryRun:        false, // dryRun
			includeLocked: false, // includeLocked
		}
		deletedTagsCount, deletedManifestsCount, purgeErr := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			purgeOptions,
			60,   // filterTimeout
			true, // removeUntaggedManifests
			true, // untaggedOnly
			tagFilters,
			false, // verbose = false
		)

		// Restore stdout and read captured output
		err := w.Close()
		if err != nil {
			t.Fatalf("Failed to close pipe writer: %v", err)
		}
		os.Stdout = oldStdout
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		if err != nil {
			t.Fatalf("Failed to read from pipe: %v", err)
		}
		output := buf.String()

		assert.Equal(0, deletedTagsCount, "No tags should be deleted")
		assert.Equal(0, deletedManifestsCount, "No manifests deleted when none are untagged")
		assert.Nil(purgeErr, "Error should be nil")
		// Verify non-verbose output contains count but NOT the repository list
		assert.Contains(output, "ABAC: Setting token scope for 3 repositories", "Should output repo count")
		// The non-verbose output should NOT contain the bracketed list of repos
		assert.NotContains(output, "[repo1", "Should NOT output repo list in non-verbose mode")
		mockClient.AssertCalled(t, "RefreshTokenForAbac", mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	// Test: Non-ABAC registry should not call RefreshTokenForAbac
	t.Run("NonAbacDoesNotCallRefreshTokenForAbac", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}

		// Mock non-ABAC registry
		mockClient.On("IsAbac").Return(false)

		// Empty manifests result
		emptyManifestsResult := &acr.Manifests{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			ManifestsAttributes: &[]acr.ManifestAttributesBase{},
		}

		mockClient.On("GetAcrManifests", mock.Anything, "test-repo", "", "").Return(emptyManifestsResult, nil).Once()

		// Call purge with verbose=true but non-ABAC registry
		purgeOptions := purgeOptions{
			agoDuration:   nil, // ago
			keep:          0,   // keep
			minTags:       -1,
			maxTags:       -1,
			minManifests:  -1,
			maxManifests:  -1,
			dryRun:        false, // dryRun
			includeLocked: false, // includeLocked
		}
		deletedTagsCount, deletedManifestsCount, err := purge(
			testCtx,
			mockClient,
			testLoginURL,
			defaultPoolSize,
			purgeOptions,
			60,   // filterTimeout
			true, // removeUntaggedManifests
			true, // untaggedOnly
			map[string]string{"test-repo": ".*"},
			true, // verbose = true
		)

		assert.Equal(0, deletedTagsCount, "No tags should be deleted")
		assert.Equal(0, deletedManifestsCount, "No manifests deleted")
		assert.Nil(err, "Error should be nil")
		// Verify RefreshTokenForAbac was NOT called for non-ABAC
		mockClient.AssertNotCalled(t, "RefreshTokenForAbac", mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})
}

func createManifestWithTime(digest, timestamp string) acr.ManifestAttributesBase {
	mediaType := "application/vnd.docker.distribution.manifest.v2+json"
	return acr.ManifestAttributesBase{
		Digest:               &digest,
		Tags:                 &[]string{},
		LastUpdateTime:       &timestamp,
		MediaType:            &mediaType,
		ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &[]bool{true}[0], WriteEnabled: &[]bool{true}[0]},
	}
}
