// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/acr-cli/cmd/worker"
	"github.com/Azure/go-autorest/autorest"
	"github.com/stretchr/testify/assert"
)

// TestPurgeTags contains all the tests regarding the purgeTags method which is called when the --dry-run flag is
// not set.
func TestPurgeTags(t *testing.T) {
	// First test if repository is not known purgeTags should only call GetAcrTags and return no error.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Once()
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "1d", "[\\s\\S]*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test, if there are no tags on a registry no error should show and no other methods should be called.
	t.Run("EmptyRepositoryTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(EmptyListTagsResult, nil).Once()
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "1d", "[\\s\\S]*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Third test there is only one tag and it should not be deleted (according to the ago flag), GetAcrTags should be called twice
	// and no other methods should be called.
	t.Run("NoDeletionAgoTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(OneTagResult, nil).Once()
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "1d", "[\\s\\S]*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fourth test there is only one tag and it should be deleted according to the ago flag but it does not match a regex filter
	// so no other method should be called
	t.Run("NoDeletionFilterTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(OneTagResult, nil).Once()
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "0m", "^hello.*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fifth test, invalid regex filter, an error should be returned.
	t.Run("InvalidRegexTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "0m", "[")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Sixth test, if a passed duration is invalid an error should be returned.
	t.Run("InvalidDurationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "0e", "^la.*")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Seventh test, if there is an error during a call to GetAcrTags (other than a 404) an error should be returned.
	t.Run("GetAcrTagsErrorSinglePageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(nil, errors.New("unauthorized")).Once()
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "1d", "[\\s\\S]*")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Eighth test, if there is an error during a call to GetAcrTags (other than a 404) an error should be returned.
	// similar to the previous test but the error occurs not on the first GetAcrTags call.
	t.Run("GetAcrTagsErrorMultiplePageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "latest").Return(nil, errors.New("unauthorized")).Once()
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "1d", "[\\s\\S]*")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Ninth test, if a tag should be deleted but the delete enabled attribute is set to true it should not be deleted
	// and no error should show on the CLI output.
	t.Run("OperationNotAllowedTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(DeleteDisabledOneTagResult, nil).Once()
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "0m", "^la.*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Tenth test, if a tag has an invalid last update time attribute an error should be returned.
	t.Run("InvalidDurationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(InvalidDateOneTagResult, nil).Once()
		deletedTags, err := purgeTags(testCtx, mockClient, testLoginURL, testRepo, "0m", "^la.*")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// The following tests involve deleting tags.
	// Eleventh test, there is only one tag and it should be deleted, the DeleteAcrTag method should be called once.
	t.Run("OneTagDeletionTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(testCtx, &wg, &mockClient, 6)
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "latest").Return(&deletedResponse, nil).Once()
		deletedTags, err := purgeTags(testCtx, &mockClient, testLoginURL, testRepo, "0m", "^la.*")
		worker.StopDispatcher()
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Twelfth test, all tags should be deleted, 5 tags in total, separated into two GetAcrTags calls, there should be
	// 5 DeleteAcrTag calls.
	t.Run("FiveTagDeletionTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(testCtx, &wg, &mockClient, 6)
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "latest").Return(FourTagsResult, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "latest").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "v1").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "v2").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "v3").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "v4").Return(&deletedResponse, nil).Once()
		deletedTags, err := purgeTags(testCtx, &mockClient, testLoginURL, testRepo, "0m", "[\\s\\S]*")
		worker.StopDispatcher()
		assert.Equal(5, deletedTags, "Number of deleted elements should be 5")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Thirteenth test, if an there is a 404 error while deleting a tag an error should not be returned.
	t.Run("DeleteNotFoundErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(testCtx, &wg, &mockClient, 6)
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "latest").Return(&notFoundResponse, errors.New("not found")).Once()
		deletedTags, err := purgeTags(testCtx, &mockClient, testLoginURL, testRepo, "0m", "^la.*")
		worker.StopDispatcher()
		// If it is not found it can be assumed deleted.
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fourteenth test, if an error (other than a 404 error) occurs during delete, an error should be returned.
	t.Run("DeleteErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(testCtx, &wg, &mockClient, 6)
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "latest").Return(nil, errors.New("error during delete")).Once()
		deletedTags, err := purgeTags(testCtx, &mockClient, testLoginURL, testRepo, "0m", "^la.*")
		worker.StopDispatcher()
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
}

// TestPurgeManifests contains the tests for the purgeDanglingManifests method, it is invoked when the --untagged flag is set
// and the --dry-run flag is not set
func TestPurgeManifests(t *testing.T) {
	// First test if repository is not known purgeDanglingManifests should only call GetAcrManifests once and return no error
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(notFoundManifestResponse, errors.New("testRepo not found")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test if there is an error (different to a 404 error) getting the first set of manifests an error should be returned.
	t.Run("GetAcrManifestsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(nil, errors.New("unauthorized")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Third test, no manifest shoud be deleted, if all the manifests have at least one tag they should not be deleted,
	// so no DeleteManifest calls should be made.
	t.Run("NoDeletionManifestTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:abc").Return(EmptyListManifestsResult, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fourth test if there is an error (different to a 404 error) getting the second set of manifests an error should be returned.
	t.Run("GetAcrManifestsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:abc").Return(nil, errors.New("error getting manifests")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// The following tests involve multiarch manifests
	// Fifth test, if there is an error while getting the multiarch manifest an error should be returned.
	t.Run("MultiArchErrorGettingManifestTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleMultiArchWithTagsResult, nil).Once()
		mockClient.On("GetManifest", testCtx, testRepo, "sha:356").Return(nil, errors.New("error getting manifest")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error not should be nil")
		mockClient.AssertExpectations(t)
	})
	// Sixth test, if a MultiArch manifest returns an invalid JSON an error should be returned.
	t.Run("MultiArchInvalidJsonTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleMultiArchWithTagsResult, nil).Once()
		mockClient.On("GetManifest", testCtx, testRepo, "sha:356").Return([]byte("invalid manifest"), nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error not should be nil")
		mockClient.AssertExpectations(t)
	})
	// The following tests involve deleting manifests.
	// Seventh test, there are three manifests split into two GetAcrManifests calls, and one is linked to a tag so there should
	// only be 2 deletions, hence the 2 DeleteManifest calls
	t.Run("DeleteTwoManifestsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(testCtx, &wg, mockClient, 6)
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:abc").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:234").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:123").Return(nil, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:234").Return(nil, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		worker.StopDispatcher()
		assert.Equal(2, deletedTags, "Number of deleted elements should be 2")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Eighth test, if there is an error while deleting the manifest but it is a 404 the manifest can be assumed deleted and there should
	// be no error.
	t.Run("ErrorManifestDeleteNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(testCtx, &wg, mockClient, 6)
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:abc").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:234").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:123").Return(nil, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:234").Return(&notFoundResponse, errors.New("manifest not found")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		worker.StopDispatcher()
		assert.Equal(2, deletedTags, "Number of deleted elements should be 2")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Ninth, if there is an error while deleting a manifest and it is different that a 404 error an error should be returned.
	t.Run("ErrorManifestDeleteTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(testCtx, &wg, mockClient, 6)
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:abc").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:234").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:123").Return(nil, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:234").Return(nil, errors.New("error deleting manifest")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		worker.StopDispatcher()
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Tenth test, if there is an error while deleting a manifest and it is different that a 404 error an error should be returned.
	// similar to the previous test but the error occurs in the second manifest that should be deleted.
	t.Run("ErrorManifestDelete2Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(testCtx, &wg, mockClient, 6)
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:abc").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:234").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:123").Return(nil, errors.New("error deleting manifest")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		worker.StopDispatcher()
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Eleventh, there are three manifests, two of them have no tags, but one belongs to a multiarch image that has tags so it
	// should not be deleted, only one call to DeleteManifest should be made because the manifest that does not belong to the
	// multiarch manifest and has no tags should be deleted.
	t.Run("MultiArchDeleteTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(testCtx, &wg, mockClient, 6)
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleMultiArchWithTagsResult, nil).Once()
		mockClient.On("GetManifest", testCtx, testRepo, "sha:356").Return(multiArchBytes, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:356").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:234").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:234").Return(nil, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, testLoginURL, testRepo)
		worker.StopDispatcher()
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

// TestDryRun contains the tests for the dryRunPurge method, it is called when the --dry-run flag is set.
func TestDryRun(t *testing.T) {
	// First test if repository is not know DryRun should not return an error, and there should not be any tags or manifest deleted.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(notFoundManifestResponse, errors.New("testRepo not found")).Once()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Twice()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "1d", "[\\s\\S]*", true)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(0, deletedManifests, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test, if an invalid duration is passed an error should be returned, and the invalid counters should be returned.
	t.Run("InvalidDurationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0e", "[\\s\\S]*", true)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be 0")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Third test, if there is an invalid regex an error should be returned as well as the invalid counters.
	t.Run("InvalidRegexTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "[", true)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be 0")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fourth test, there are 4 tags that should be deleted, note how there are no DeleteAcrTag calls because this is a dry-run.
	t.Run("FourTagDeletionDryRunTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(FourTagsResult, nil).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "[\\s\\S]*", false)
		assert.Equal(4, deletedTags, "Number of deleted elements should be 4")
		assert.Equal(0, deletedManifests, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fifth test, if there is an error on the first GetAcrTags call (different to a 404) an error should be returned.
	t.Run("GetAcrTagsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(nil, errors.New("error fetching tags")).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "[\\s\\S]*", false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Sixth test, if there is an error on the second GetAcrTags call (different to a 404) an error should be returned.
	t.Run("GetAcrTagsError2Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(nil, errors.New("error fetching tags")).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "[\\s\\S]*", false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Seventh test, if there is an error on the first GetAcrManifests call (different to a 404) an error should be returned.
	t.Run("GetAcrManifestsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(EmptyListTagsResult, nil).Twice()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(nil, errors.New("testRepo not found")).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "[\\s\\S]*", true)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Eighth test, if there is an error on the second GetAcrManifests call (different to a 404) an error should be returned.
	t.Run("GetAcrManifestsError2Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(nil, errors.New("error fetching tags")).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "[\\s\\S]*", true)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Ninth test, if there is a GetManifest error for the MultiArch scenario an error should be returned.
	t.Run("MultiArchGetManifestErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(FourTagsResult, nil).Twice()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "v4").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleMultiArchWithTagsResult, nil).Once()
		mockClient.On("GetManifest", testCtx, testRepo, "sha:356").Return(nil, errors.New("error getting manifest")).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "^lat.*", true)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Tenth test, if the returned multiarch manifest json is invalid an error should be returned.
	t.Run("MultiArchInvalidJSONTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(FourTagsResult, nil).Twice()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "v4").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleMultiArchWithTagsResult, nil).Once()
		mockClient.On("GetManifest", testCtx, testRepo, "sha:356").Return([]byte("invalid json"), nil).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "^lat.*", true)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Eleventh test, error on the fourth getAcrTags, an error should be returned
	t.Run("MultiArchGetAcrTagsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(FourTagsResult, nil).Twice()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "v4").Return(nil, errors.New("error fetching tags")).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "^lat.*", true)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Twelfth test, if there is an error during the second call of GetAcrManifests an error should be returned.
	t.Run("MultiArchGetAcrTagsError2Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(FourTagsResult, nil).Twice()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "v4").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleMultiArchWithTagsResult, nil).Once()
		mockClient.On("GetManifest", testCtx, testRepo, "sha:356").Return(multiArchBytes, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:356").Return(nil, errors.New("error fetching manifests")).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "^lat.*", true)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.Equal(-1, deletedManifests, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Thirteenth test, one image that has no tags belongs to a multiarch image that has tags so it should not be deleted, but there is one manifest
	// that should be deleted,
	t.Run("MultiArchDryRunTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(FourTagsResult, nil).Twice()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "v4").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleMultiArchWithTagsResult, nil).Once()
		mockClient.On("GetManifest", testCtx, testRepo, "sha:356").Return(multiArchBytes, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:356").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:234").Return(EmptyListManifestsResult, nil).Once()
		deletedTags, deletedManifests, err := dryRunPurge(testCtx, mockClient, testLoginURL, testRepo, "0m", "^lat.*", true)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(1, deletedManifests, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

// TestGetRepositoryAndTagRegex returns the repository and the regex from a string in the form <repository>:<regex filter>
func TestGetRepositoryAndTagRegex(t *testing.T) {
	// First test normal functionality
	t.Run("NormalFunctionalityTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo:bar"
		repository, filter, err := getRepositoryAndTagRegex(testString)
		assert.Equal("foo", repository)
		assert.Equal("bar", filter)
		assert.Equal(nil, err, "Error should be nil")
	})
	// Second test no colon
	t.Run("NoColonTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo"
		repository, filter, err := getRepositoryAndTagRegex(testString)
		assert.Equal("", repository)
		assert.Equal("", filter)
		assert.NotEqual(nil, err, "Error should not be nil")
	})
	// Third test more than one colon
	t.Run("TwoColonsTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo:bar:zzz"
		repository, filter, err := getRepositoryAndTagRegex(testString)
		assert.Equal("", repository)
		assert.Equal("", filter)
		assert.NotEqual(nil, err, "Error should not be nil")
	})
}

// TestGetLastTagFromResponse returns the last tag from response.
func TestGetLastTagFromResponse(t *testing.T) {

	t.Run("ReturnEmptyForNoHeaders", func(t *testing.T) {
		assert := assert.New(t)
		lastTag := getLastTagFromResponse(OneTagResult)
		assert.Equal("", lastTag)
	})

	t.Run("ReturnEmptyForNoLinkHeaders", func(t *testing.T) {
		assert := assert.New(t)
		ResultWithNoLinkHeader := &acr.RepositoryTagsType{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
					Header:     http.Header{"testHeader": {"Test Values"}},
				},
			},
		}
		lastTag := getLastTagFromResponse(ResultWithNoLinkHeader)
		assert.Equal("", lastTag)
	})

	t.Run("ReturnEmptyForNoQueryString", func(t *testing.T) {
		assert := assert.New(t)
		ResultWithNoQuery := &acr.RepositoryTagsType{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
					Header:     http.Header{linkHeader: {"/acr/v1/&testRepo/_tags"}}},
			},
		}
		lastTag := getLastTagFromResponse(ResultWithNoQuery)
		assert.Equal("", lastTag)
	})

	t.Run("ReturnLastTagFromHeader", func(t *testing.T) {
		assert := assert.New(t)
		lastTag := getLastTagFromResponse(OneTagResultWithNext)
		assert.Equal("latest", lastTag)
	})

	t.Run("ReturnLastWithAmpersand", func(t *testing.T) {
		assert := assert.New(t)
		lastTag := getLastTagFromResponse(OneTagResultWithAmpersand)
		assert.Equal("123&latest", lastTag)
	})

	t.Run("ReturnLastWhenQueryEndingWithLast", func(t *testing.T) {
		assert := assert.New(t)
		lastTag := getLastTagFromResponse(OneTagResultQueryEndingWithLast)
		assert.Equal("123&latest", lastTag)
	})

}

// TestParseDuration returns an extended duration from a string.
func TestParseDuration(t *testing.T) {
	tables := []struct {
		durationString string
		duration       time.Duration
		err            error
	}{
		{"15m", -15 * time.Minute, nil},
		{"1d1h3m", -25*time.Hour - 3*time.Minute, nil},
		{"3d", -3 * 24 * time.Hour, nil},
		{"", 0, io.EOF},
		{"15p", 0, errors.New("time: unknown unit p in duration 15p")},
		{"15", 0 * time.Minute, errors.New("time: missing unit in duration 15")},
	}
	assert := assert.New(t)
	for _, table := range tables {
		durationResult, errorResult := parseDuration(table.durationString)
		assert.Equal(table.duration, durationResult)
		assert.Equal(table.err, errorResult)
	}
}

// All the variables used in the tests are defined here.
var (
	testCtx          = context.Background()
	testLoginURL     = "foo.azurecr.io"
	testRepo         = "bar"
	notFoundResponse = autorest.Response{
		Response: &http.Response{
			StatusCode: 404,
		},
	}
	deletedResponse = autorest.Response{
		Response: &http.Response{
			StatusCode: 200,
		},
	}
	// Response for the GetAcrTags when the repository is not found.
	notFoundTagResponse = &acr.RepositoryTagsType{
		Response: notFoundResponse,
	}
	// Response for the GetAcrTags when there are no tags on the testRepo.
	EmptyListTagsResult = &acr.RepositoryTagsType{
		Registry:       &testLoginURL,
		ImageName:      &testRepo,
		TagsAttributes: nil,
	}
	tagName               = "latest"
	digest                = "sha:abc"
	multiArchDigest       = "sha:356"
	deleteEnabled         = true
	deleteDisabled        = false
	lastUpdateTime        = time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano) //Creation time -15minutes from current time
	invalidLastUpdateTime = "date"

	OneTagResult = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
				Digest:               &digest,
			},
		},
	}

	OneTagResultWithNext = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
				Header:     http.Header{linkHeader: {"/acr/v1/&testRepo/_tags?last=latest&n=3&orderby=timedesc rel=\"next\""}},
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
				Digest:               &digest,
			},
		},
	}

	OneTagResultWithAmpersand = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
				Header:     http.Header{linkHeader: {"/acr/v1/&testRepo/_tags?last=123%26latest&n=3&orderby=timedesc rel=\"next\""}},
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
				Digest:               &digest,
			},
		},
	}

	OneTagResultQueryEndingWithLast = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
				Header:     http.Header{linkHeader: {"/acr/v1/&testRepo/_tags?last=123%26latest&n=3&orderby=timedesc rel=\"next\""}},
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
				Digest:               &digest,
			},
		},
	}

	InvalidDateOneTagResult = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &invalidLastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
				Digest:               &digest,
			},
		},
	}

	DeleteDisabledOneTagResult = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteDisabled},
				Digest:               &digest,
			},
		},
	}
	tagName1 = "v1"
	tagName2 = "v2"
	tagName3 = "v3"
	tagName4 = "v4"

	FourTagsResult = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{{
			Name:                 &tagName1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName3,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &multiArchDigest,
		}, {
			Name:                 &tagName4,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest,
		}},
	}

	// Response for the GetAcrManifests when the repository is not found.
	notFoundManifestResponse = &acr.Manifests{
		Response: notFoundResponse,
	}
	// Response for the GetAcrManifests when there are no manifests on the testRepo.
	EmptyListManifestsResult = &acr.Manifests{
		Registry:            &testLoginURL,
		ImageName:           &testRepo,
		ManifestsAttributes: nil,
	}
	dockerV2MediaType     = "application/vnd.docker.distribution.manifest.v2+json"
	manifestListMediaType = "application/vnd.docker.distribution.manifest.list.v2+json"

	singleManifestV2WithTagsResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest,
			MediaType:            &dockerV2MediaType,
			Tags:                 &[]string{"latest"},
		}},
	}
	digest1 = "sha:123"
	digest2 = "sha:234"

	doubleManifestV2WithoutTagsResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest1,
			MediaType:            &dockerV2MediaType,
			Tags:                 nil,
		}, {
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest2,
			MediaType:            &dockerV2MediaType,
			Tags:                 nil,
		}},
	}

	singleMultiArchWithTagsResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &multiArchDigest,
			MediaType:            &manifestListMediaType,
			Tags:                 &[]string{"v3"},
		}},
	}
	multiArchBytes = []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
		"manifests": [
			{
				"mediaType": "application/vnd.docker.image.manifest.v2+json",
				"size": 7143,
				"digest": "sha:123",
				"platform": {
					"architecture": "ppc64le",
					"os": "linux"
				}
			}
		]
	}`)
)
