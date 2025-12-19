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
	"github.com/Azure/acr-cli/cmd/repository"
	"github.com/Azure/go-autorest/autorest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const defaultAgo = "0m"

// TestPurgeTags contains all the tests regarding the purgeTags method which is called when the --dry-run flag is
// not set.
func TestPurgeTags(t *testing.T) {
	t.Run("Delete tag with local in it", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(TagWithLocal, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-c-local.test").Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", ".*-?local[.].+", 0, 60, false, false)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Advanced filter tests
	t.Run("Delete 2 with negative lookahead", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsWithRepoFilterMatch, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-c").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-b").Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "v1(?!-a)", 0, 60, false, false)
		assert.Equal(2, deletedTags, "Number of deleted elements should be 2")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("Delete 2 with negative lookbehind", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsWithRepoFilterMatch, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-c").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-b").Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "v1-*[abc]+(?<!-[a])", 0, 60, false, false)
		assert.Equal(2, deletedTags, "Number of deleted elements should be 2")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Basic tests
	// If repository is not known purgeTags should only call GetAcrTags and return no error.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "1d", "[\\s\\S]*", 0, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there are no tags on a registry no error should show and no other methods should be called.
	t.Run("EmptyRepositoryTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(EmptyListTagsResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "1d", "[\\s\\S]*", 0, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// There is only one tag and it should not be deleted (according to the ago flag), GetAcrTags should be called twice
	// and no other methods should be called.
	t.Run("NoDeletionAgoTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "1d", "[\\s\\S]*", 0, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// There is only one tag and it should be deleted according to the ago flag but it does not match a regex filter
	// so no other method should be called
	t.Run("NoDeletionFilterTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "^hello.*", 0, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Invalid regex filter, an error should be returned.
	t.Run("InvalidRegexTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "[", 0, 60, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If a passed duration is invalid an error should be returned.
	t.Run("InvalidDurationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0e", "^la.*", 0, 60, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error during a call to GetAcrTags (other than a 404) an error should be returned.
	t.Run("GetAcrTagsErrorSinglePageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(nil, errors.New("unauthorized")).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "1d", "[\\s\\S]*", 0, 60, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error during a call to GetAcrTags (other than a 404) an error should be returned.
	// similar to the previous test but the error occurs not on the first GetAcrTags call.
	t.Run("GetAcrTagsErrorMultiplePageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "latest").Return(nil, errors.New("unauthorized")).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "1d", "[\\s\\S]*", 0, 60, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If a tag should be deleted but the delete or write enabled attribute is set to false it should not be deleted
	// and no error should show on the CLI output.
	t.Run("OperationNotAllowedTagDeleteDisabledTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(DeleteDisabledOneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "^la.*", 0, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("OperationNotAllowedTagWriteDisabledTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(WriteDisabledOneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "^la.*", 0, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If a tag has an invalid last update time attribute an error should be returned.
	t.Run("InvalidDurationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(InvalidDateOneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "^la.*", 0, 60, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// The following tests involve deleting tags.
	// There is only one tag and it should be deleted, the DeleteAcrTag method should be called once.
	t.Run("OneTagDeletionTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "latest").Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "^la.*", 0, 60, false, false)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// All tags should be deleted, 5 tags in total, separated into two GetAcrTags calls, there should be
	// 5 DeleteAcrTag calls.
	t.Run("FiveTagDeletionTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "latest").Return(FourTagsResult, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "latest").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v2").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v3").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v4").Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "[\\s\\S]*", 0, 60, false, false)
		assert.Equal(5, deletedTags, "Number of deleted elements should be 5")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If an there is a 404 error while deleting a tag an error should not be returned.
	t.Run("DeleteNotFoundErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "latest").Return(&notFoundResponse, errors.New("not found")).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "^la.*", 0, 60, false, false)
		// If it is not found it can be assumed deleted.
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If an error (other than a 404 error) occurs during delete, an error should be returned.
	t.Run("DeleteErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "latest").Return(nil, errors.New("error during delete")).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "^la.*", 0, 60, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("Keep 1 tag", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsResult, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v2").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v3").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v4").Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "[\\s\\S]*", 1, 60, false, false)
		assert.Equal(3, deletedTags, "Number of deleted elements should be 3")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("Keep 1 tag when repo filter doesn't match all results", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsWithRepoFilterMatch, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-c").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-b").Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", "v1-.*", 1, 60, false, false)
		assert.Equal(2, deletedTags, "Number of deleted elements should be 2")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("Keep 1 tag when repo filter doesn't match all results and not all results match due to ago filter", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsWithRepoFilterMatch, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-c").Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "30m", "v1-.*", 1, 60, false, false)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

// TestPurgeManifests contains the tests for the purgeDanglingManifests method, it is invoked when the --untagged flag is set
// and the --dry-run flag is not set
func TestPurgeManifests(t *testing.T) {
	// If repository is not known purgeDanglingManifests should only call GetAcrManifests once and return no error
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(notFoundManifestResponse, errors.New("testRepo not found")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error (different to a 404 error) getting the first set of manifests an error should be returned.
	t.Run("GetAcrManifestsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(nil, errors.New("unauthorized")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// No manifest should be deleted, if all the manifests have at least one tag they should not be deleted,
	// so no DeleteManifest calls should be made.
	t.Run("NoDeletionManifestTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(EmptyListManifestsResult, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("SkipsManifestsNewerThanAgo", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		manifestList := &acr.Manifests{
			Registry:  &testLoginURL,
			ImageName: &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{{
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
				Digest:               &digest1,
				MediaType:            &dockerV2MediaType,
				Tags:                 nil,
			}},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestList, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest1).Return(EmptyListManifestsResult, nil).Once()

		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "1h", nil, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.NoError(err)
		mockClient.AssertExpectations(t)
	})

	t.Run("DeletesManifestsOlderThanAgo", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		manifestList := &acr.Manifests{
			Registry:  &testLoginURL,
			ImageName: &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{{
				LastUpdateTime:       &lastUpdateTime2DaysAgo,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
				Digest:               &digest2,
				MediaType:            &dockerV2MediaType,
				Tags:                 nil,
			}},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(manifestList, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest2).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, digest2).Return(nil, nil).Once()

		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "24h", nil, false, false)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.NoError(err)
		mockClient.AssertExpectations(t)
	})

	// If there is an error (different to a 404 error) getting the second set of manifests an error should be returned.
	t.Run("GetAcrManifestsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(nil, errors.New("error getting manifests")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// The following tests involve multiarch manifests
	// If there is an error while getting the multiarch manifest an error should be returned.
	t.Run("MultiArchErrorGettingManifestTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(nil, errors.New("error getting manifest")).Once()
		// Despite the failure, the GetAcrManifests method may be called again before the failure happens
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(nil, nil).Maybe()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error not should be nil")
		mockClient.AssertExpectations(t)
	})

	// If a MultiArch manifest returns an invalid JSON an error should be returned.
	t.Run("MultiArchInvalidJsonTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return([]byte("invalid manifest"), nil).Once()
		// Despite the failure, the GetAcrManifests method may be called again before the failure happens
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(nil, nil).Maybe()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error not should be nil")
		mockClient.AssertExpectations(t)
	})

	// The following tests involve deleting manifests.
	// There are three manifests split into two GetAcrManifests calls, and one is linked to a tag so there should
	// only be 2 deletions, hence the 2 DeleteManifest calls
	t.Run("DeleteTwoManifestsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683").Return(nil, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(nil, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(2, deletedTags, "Number of deleted elements should be 2")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error while deleting the manifest but it is a 404 the manifest can be assumed deleted and there should
	// be no error.
	t.Run("ErrorManifestDeleteNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683").Return(nil, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(&notFoundResponse, errors.New("manifest not found")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(2, deletedTags, "Number of deleted elements should be 2")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error while deleting a manifest and it is different that a 404 error an error should be returned.
	t.Run("ErrorManifestDeleteTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683").Return(nil, errors.New("error deleting manifest")).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(nil, nil).Maybe()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error while deleting a manifest and it is different that a 404 error an error should be returned.
	// similar to the previous test but the error occurs in the second manifest that should be deleted.
	t.Run("ErrorManifestDelete2Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683").Return(nil, nil).Maybe()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(nil, errors.New("error deleting manifest")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// There are three manifests, two of them have no tags, but one belongs to a multiarch image that has tags so it
	// should not be deleted, only one call to DeleteManifest should be made because the manifest that does not belong to the
	// multiarch manifest and has no tags should be deleted.
	t.Run("MultiArchDeleteTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(multiArchManifestV2Bytes, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(nil, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Same as above, but the multiarch image manifest is an OCI index,
	// instead of a Docker Schema v2 manifest list.
	t.Run("OCIMultiArchDeleteTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchOCIWithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(multiArchOCIBytes, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683").Return(emptyManifestBytes, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(emptyManifestBytes, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(doubleOCIWithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(nil, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If a manifest should be deleted but the delete enabled attribute is set to false it should not be deleted
	// and no error should show on the CLI output.
	t.Run("OperationNotAllowedManifestDeleteDisabledTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(deleteDisabledOneManifestResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If a manifest should be deleted but the write enabled attribute is set to false it should not be deleted
	// and no error should show on the CLI output.
	t.Run("OperationNotAllowedManifestWriteDisabledTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(writeDisabledOneManifestResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If an OCI artifact manifest is untagged but it has subject manifests, the manifest should not be purged
	t.Run("OCIArtificateManifestWithSubjectDeleteTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestWithSubjectWithoutTagResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:118811b833e6ca4f3c65559654ca6359410730e97c719f5090d0bfe4db0ab588").Return(manifestWithSubjectOCIArtificate, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:118811b833e6ca4f3c65559654ca6359410730e97c719f5090d0bfe4db0ab588").Return(EmptyListManifestsResult, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

// TestDryRun contains the tests for the dryRunPurge method, it is called when the --dry-run flag is set.
func TestDryRun(t *testing.T) {
	// If repository is not know DryRun should not return an error, and there should not be any tags or manifest deleted.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(notFoundManifestResponse, errors.New("testRepo not found")).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Once()
		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 60, "1d", 0, 1, true, map[string]string{testRepo: "[\\s\\S]*"}, true, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(0, deletedManifests, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

// TestCollectTagFilters contains all the tests regarding the collectTagFilters with retrieves matching repo names
// and aggregates the associated tag filters
func TestCollectTagFilters(t *testing.T) {
	t.Run("AllReposWildcardWithTagLocal", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{".+:.*-?local[.].+"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(4, len(filters), "Number of found should be 4")
		assert.Equal(".*-?local[.].+", filters[testRepo], "Filter for test repo should be .*-?local[.].+")
		assert.Equal(".*-?local[.].+", filters["bar"], "Filter for bar repo should be .*-?local[.].+")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("AllReposWildcardWithTagLocal2", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{".+:.*-?local\\..+"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(4, len(filters), "Number of found should be 4")
		assert.Equal(".*-?local\\..+", filters[testRepo], "Filter for test repo should be .*-?local\\..+")
		assert.Equal(".*-?local\\..+", filters["bar"], "Filter for bar repo should be .*-?local\\..+")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("SingleRepo", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{testRepo + ":.*"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Number of found should be one")
		assert.Equal(".*", filters[testRepo], "Filter for test repo should be .*")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("AllReposWildcard", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{".*:.*"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(4, len(filters), "Number of found should be 4")
		assert.Equal(".*", filters[testRepo], "Filter for test repo should be .*")
		assert.Equal(".*", filters["bar"], "Filter for bar repo should be .*")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NoPartialMatch", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{"ba:.*"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(0, len(filters), "Number of found repos should be zero")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NameWithSlash", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{"foo/bar:.*"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Number of found repos should be one")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NameWithSlashAndNonCaptureGroupInTag", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{"foo/bar:(?:.*)"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Number of found repos should be one")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NameWithSlashAndTwoNonCaptureGroup", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{"foo/bar(?:.*):(?:.*)"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Number of found repos should be one")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NameWithSlashAndTwoNonCaptureGroupAndQuantifier", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{"foo/bar(?:.*)?:(?:.*)"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Number of found repos should be one")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NameWithSlashAndTwoNonCaptureGroupInRepo", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{"foo/bar(?:.*):.(?:.*)"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Number of found repos should be one")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NameWithSlashAndTwoNonCaptureGroupInRepoAndCharacterClasses", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{"foo/b[[:alpha:]]r(?:.*):.(?:.*)"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Number of found repos should be one")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NoRepos", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(NoRepositoriesResult, nil).Once()
		filters, err := repository.CollectTagFilters(testCtx, []string{testRepo + ":.*"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(0, len(filters), "Number of found repos should be zero")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("EmptyRepoRegex", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		_, err := repository.CollectTagFilters(testCtx, []string{":.*"}, mockClient, 60, defaultRepoPageSize)
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("EmptyTagRegex", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		_, err := repository.CollectTagFilters(testCtx, []string{testRepo + ".*:"}, mockClient, 60, defaultRepoPageSize)
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
}

func TestGetAllRepositoryNames(t *testing.T) {
	t.Run("OneSlice", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		allRepoNames, err := repository.GetAllRepositoryNames(testCtx, mockClient, defaultRepoPageSize)
		assert.Equal(4, len(allRepoNames), "Number of all repo names should be 4")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("MoreSlice", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(ManyRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(MoreRepositoriesResult, nil).Once()
		mockClient.On("GetRepositories", mock.Anything, mock.Anything, mock.Anything).Return(NoRepositoriesResult, nil).Once()
		allRepoNames, err := repository.GetAllRepositoryNames(testCtx, mockClient, defaultRepoPageSize)
		assert.Equal(7, len(allRepoNames), "Number of all repo names should be 7")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NoSlice", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		mockClient.On("GetRepositories", mock.Anything, "", mock.Anything).Return(NoRepositoriesResult, nil).Once()
		allRepoNames, err := repository.GetAllRepositoryNames(testCtx, mockClient, defaultRepoPageSize)
		assert.Equal(0, len(allRepoNames), "Number of all repo names should be 7")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

// TestGetRepositoryAndTagRegex returns the repository and the regex from a string in the form <repository>:<regex filter>
func TestGetRepositoryAndTagRegex(t *testing.T) {
	// Test normal functionality
	t.Run("NormalFunctionalityTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo:bar"
		repository, filter, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("foo", repository)
		assert.Equal("bar", filter)
		assert.Equal(nil, err, "Error should be nil")
	})

	// Test no colon
	t.Run("NoColonTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo"
		repository, filter, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("", repository)
		assert.Equal("", filter)
		assert.NotEqual(nil, err, "Error should not be nil")
	})

	// Test more than one colon
	t.Run("TwoColonsTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo:bar:zzz"
		repository, filter, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("", repository)
		assert.Equal("", filter)
		assert.NotEqual(nil, err, "Error should not be nil")
	})

	// Test non capture group in repo name
	t.Run("NonCaptureGroupInRepoName", func(t *testing.T) {
		assert := assert.New(t)
		testString := "hello-(?:abc):zzz"
		repository, filter, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("hello-(?:abc)", repository)
		assert.Equal("zzz", filter)
		assert.Equal(nil, err, "Error should be nil")
	})

	// Test non capture group in tag
	t.Run("NonCaptureGroupInTag", func(t *testing.T) {
		assert := assert.New(t)
		testString := "hello-:z-(?:abc)zz"
		repository, filter, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("hello-", repository)
		assert.Equal("z-(?:abc)zz", filter)
		assert.Equal(nil, err, "Error should be nil")
	})

	// Test non capture group in both and quantifier
	t.Run("NonCaptureGroupAndQuantifier", func(t *testing.T) {
		assert := assert.New(t)
		testString := "hello-(?:abc)?:z-(?:abc)zz"
		repository, filter, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("hello-(?:abc)?", repository)
		assert.Equal("z-(?:abc)zz", filter)
		assert.Equal(nil, err, "Error should be nil")
	})

	// Test colon character class inside capture group
	t.Run("ColonInsideNonCaptureGroup", func(t *testing.T) {
		assert := assert.New(t)
		testString := "hello-(?:abc)?:z-(?:[:])zz"
		repository, filter, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("hello-(?:abc)?", repository)
		assert.Equal("z-(?:[:])zz", filter)
		assert.Equal(nil, err, "Error should be nil")
	})

	// Test with character classes
	t.Run("NonCaptureGroupQuantifierAndCharacterClasses", func(t *testing.T) {
		assert := assert.New(t)
		testString := "[[:alpha:]](?:abc)(?:.*)?:test123[[:digit:]](?:.*)"
		repository, tag, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("[[:alpha:]](?:abc)(?:.*)?", repository)
		assert.Equal("test123[[:digit:]](?:.*)", tag)
		assert.Equal(nil, err, "Error should be nil")
	})

	// Test with character classes, negated character classes, non-capture group flags, negated character classes inside character classes
	t.Run("NonCaptureGroupQuantifierAndNegatedCharacterClassesCharacterClasses", func(t *testing.T) {
		assert := assert.New(t)
		testString := "[^[:alpha:]](?ims-U:abc)(?:.*)?:test123[[^:digit:]](?-imUs:.*)"
		repository, tag, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("[^[:alpha:]](?ims-U:abc)(?:.*)?", repository)
		assert.Equal("test123[[^:digit:]](?-imUs:.*)", tag)
		assert.Equal(nil, err, "Error should be nil")
	})

	// Test invalid
	t.Run("NonCaptureGroupQuantifierAndNegatedCharacterClassesCharacterClasses", func(t *testing.T) {
		assert := assert.New(t)
		testString := "[^[:alpha:]](?ims-U:abc)(?:.*)?:test123[[^:digit:]](?-imUs:.*):"
		repository, tag, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("", repository)
		assert.Equal("", tag)
		assert.NotEqual(nil, err, "Error should not be nil")
	})

	// Test character class with colon (technically a tag or repo can't have a colon in the name -- but adding this for completeness)
	t.Run("NonCaptureGroupWithFlagsCharacterClassAndColonInCharacterClass", func(t *testing.T) {
		assert := assert.New(t)
		testString := "(?imsU:test):[[:digit:]][tes:]"
		repository, tag, err := repository.GetRepositoryAndTagRegex(testString)
		assert.Equal("(?imsU:test)", repository)
		assert.Equal("[[:digit:]][tes:]", tag)
		assert.Equal(nil, err, "Error should be nil")
	})
}

// TestGetLastTagFromResponse returns the last tag from response.
func TestGetLastTagFromResponse(t *testing.T) {
	t.Run("ReturnEmptyForNoHeaders", func(t *testing.T) {
		assert := assert.New(t)
		lastTag := repository.GetLastTagFromResponse(OneTagResult)
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
		lastTag := repository.GetLastTagFromResponse(ResultWithNoLinkHeader)
		assert.Equal("", lastTag)
	})

	t.Run("ReturnEmptyForNoQueryString", func(t *testing.T) {
		assert := assert.New(t)
		ResultWithNoQuery := &acr.RepositoryTagsType{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
					Header:     http.Header{headerLink: {"/acr/v1/&testRepo/_tags"}}},
			},
		}
		lastTag := repository.GetLastTagFromResponse(ResultWithNoQuery)
		assert.Equal("", lastTag)
	})

	t.Run("ReturnLastTagFromHeader", func(t *testing.T) {
		assert := assert.New(t)
		lastTag := repository.GetLastTagFromResponse(OneTagResultWithNext)
		assert.Equal("latest", lastTag)
	})

	t.Run("ReturnLastWithAmpersand", func(t *testing.T) {
		assert := assert.New(t)
		lastTag := repository.GetLastTagFromResponse(OneTagResultWithAmpersand)
		assert.Equal("123&latest", lastTag)
	})

	t.Run("ReturnLastWhenQueryEndingWithLast", func(t *testing.T) {
		assert := assert.New(t)
		lastTag := repository.GetLastTagFromResponse(OneTagResultQueryEndingWithLast)
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
		{"999999d", -1 * time.Duration(150*365) * 24 * time.Hour, nil},            // Capped at 150 years
		{"9999999d", -1 * time.Duration(150*365) * 24 * time.Hour, nil},           // Capped at 150 years
		{"999999999h", -1 * time.Duration(150*365) * 24 * time.Hour, nil},         // Capped at 150 years
		{"9999999d999999999h", -1 * time.Duration(150*365) * 24 * time.Hour, nil}, // Capped at 150 years
		{"0m", 0 * time.Minute, nil},
		{"-1d", 24 * time.Hour, nil}, // Negative durations pretty much just mean anything can be cleaned up
		{"15p", 0, errors.New("time: unknown unit \"p\" in duration \"15p\"")},
		{"15", 0 * time.Minute, errors.New("time: missing unit in duration \"15\"")},
		{"1d1h3m", -25*time.Hour - 3*time.Minute, nil},
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
	tagName                   = "latest"
	digest                    = "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283" //#nosec G101
	multiArchDigest           = "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119" //#nosec G101
	manifestWithSubjectDigest = "sha256:118811b833e6ca4f3c65559654ca6359410730e97c719f5090d0bfe4db0ab588" //#nosec G101
	deleteEnabled             = true
	deleteDisabled            = false
	writeEnabled              = true
	writeDisabled             = false
	lastUpdateTime            = time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano)
	lastUpdateTime1DayAgo     = time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
	lastUpdateTime2DaysAgo    = time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	lastUpdateTime3DaysAgo    = time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339Nano)
	invalidLastUpdateTime     = "date"
	OneTagResult              = &acr.RepositoryTagsType{
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
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
				Digest:               &digest,
			},
		},
	}
	OneTagResultWithNext = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
				Header:     http.Header{headerLink: {"</acr/v1/&testRepo/_tags?last=latest&n=3&orderby=timedesc>; rel=\"next\""}},
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
				Digest:               &digest,
			},
		},
	}
	OneTagResultWithAmpersand = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
				Header:     http.Header{headerLink: {"</acr/v1/&testRepo/_tags?last=123%26latest&n=3&orderby=>; rel=\"next\""}},
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
				Digest:               &digest,
			},
		},
	}
	OneTagResultQueryEndingWithLast = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
				Header:     http.Header{headerLink: {"</acr/v1/&testRepo/_tags?n=3&orderby=timedesc&last=123%26latest>; rel=\"next\""}},
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
				Digest:               &digest,
			},
		},
	}
	ManyRepositoriesResult = acr.Repositories{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Names: &[]string{testRepo, "foo", "baz", "foo/bar"},
	}
	MoreRepositoriesResult = acr.Repositories{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Names: &[]string{"foo1", "foo2", "foo3"},
	}
	NoRepositoriesResult = acr.Repositories{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Names: &[]string{},
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
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
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
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteDisabled, WriteEnabled: &writeEnabled},
				Digest:               &digest,
			},
		},
	}
	WriteDisabledOneTagResult = &acr.RepositoryTagsType{
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
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeDisabled},
				Digest:               &digest,
			},
		},
	}
	tagName1       = "v1"
	tagName2       = "v2"
	tagName3       = "v3"
	tagName4       = "v4"
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
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName3,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &multiArchDigest,
		}, {
			Name:                 &tagName4,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}},
	}
	FourTagsResultWithNext = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
				Header:     http.Header{headerLink: {"</acr/v1/&testRepo/_tags?last=v4&n=3&orderby=timedesc>; rel=\"next\""}},
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{{
			Name:                 &tagName1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName3,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &multiArchDigest,
		}, {
			Name:                 &tagName4,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}},
	}

	tagNameWithLoad = "v1-c-local.test"
	TagWithLocal    = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{{
			Name:                 &tagName1CommitA,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName1CommitB,
			LastUpdateTime:       &lastUpdateTime1DayAgo,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName1CommitC,
			LastUpdateTime:       &lastUpdateTime2DaysAgo,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &multiArchDigest,
		}, {
			Name:                 &tagNameWithLoad,
			LastUpdateTime:       &lastUpdateTime3DaysAgo,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}},
	}

	tagName1CommitA             = "v1-a"
	tagName1CommitB             = "v1-b"
	tagName1CommitC             = "v1-c"
	FourTagsWithRepoFilterMatch = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		TagsAttributes: &[]acr.TagAttributesBase{{
			Name:                 &tagName1CommitA,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName1CommitB,
			LastUpdateTime:       &lastUpdateTime1DayAgo,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName1CommitC,
			LastUpdateTime:       &lastUpdateTime2DaysAgo,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &multiArchDigest,
		}, {
			Name:                 &tagName2,
			LastUpdateTime:       &lastUpdateTime3DaysAgo,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
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
	dockerV2MediaType              = "application/vnd.docker.distribution.manifest.v2+json"
	dockerV2ListMediaType          = "application/vnd.docker.distribution.manifest.list.v2+json"
	ociMediaType                   = "application/vnd.oci.image.manifest.v1+json"
	ociListMediaType               = "application/vnd.oci.image.index.v1+json"
	singleManifestV2WithTagsResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
			MediaType:            &dockerV2MediaType,
			Tags:                 &[]string{"latest"},
		}},
	}
	deleteDisabledOneManifestResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteDisabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
			MediaType:            &dockerV2MediaType,
			Tags:                 &[]string{"latest"},
		}},
	}
	writeDisabledOneManifestResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteDisabled, WriteEnabled: &writeDisabled},
			Digest:               &digest,
			MediaType:            &dockerV2MediaType,
			Tags:                 &[]string{"latest"},
		}},
	}
	digest1                           = "sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683" //#nosec G101
	digest2                           = "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696" //#nosec G101
	doubleManifestV2WithoutTagsResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest1,
			MediaType:            &dockerV2MediaType,
			Tags:                 nil,
		}, {
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest2,
			MediaType:            &dockerV2MediaType,
			Tags:                 nil,
		}},
	}
	doubleOCIWithoutTagsResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest1,
			MediaType:            &ociMediaType,
			Tags:                 nil,
		}, {
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest2,
			MediaType:            &ociMediaType,
			Tags:                 nil,
		}},
	}
	singleMultiArchManifestV2WithTagsResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &multiArchDigest,
			MediaType:            &dockerV2ListMediaType,
			Tags:                 &[]string{"v3"},
		}},
	}
	singleMultiArchOCIWithTagsResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &multiArchDigest,
			MediaType:            &ociListMediaType,
			Tags:                 &[]string{"v3"},
		}},
	}
	singleManifestWithSubjectWithoutTagResult = &acr.Manifests{
		Registry:  &testLoginURL,
		ImageName: &testRepo,
		ManifestsAttributes: &[]acr.ManifestAttributesBase{{
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &manifestWithSubjectDigest,
			MediaType:            &ociMediaType,
			Tags:                 nil,
		}},
	}
	multiArchManifestV2Bytes = []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
		"manifests": [
			{
				"mediaType": "application/vnd.docker.image.manifest.v2+json",
				"size": 7143,
				"digest": "sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683",
				"platform": {
					"architecture": "ppc64le",
					"os": "linux"
				}
			}
		]
	}`)
	multiArchOCIBytes = []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.oci.image.index.v1+json",
		"manifests": [
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"size": 7143,
				"digest": "sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683",
				"platform": {
					"architecture": "ppc64le",
					"os": "linux"
				}
			}
		]
	}`)
	emptyManifestBytes = []byte(`{
		"mediaType": "application/vnd.oci.image.index.v1+json"
	}`)
	manifestWithSubjectOCIArtificate = []byte(`{
		"mediaType": "application/vnd.oci.artifact.manifest.v1+json",
		"artifactType": "application/vnd.example.sbom.v1",
		"subject": {
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"size": 1234,
			"digest": "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
		},
		"annotations": {
			"com.example.key1": "value1",
			"com.example.key2": "value2"
		  }
	}`)
)

// TestIncludeLockedFlag contains all tests for the --include-locked flag functionality
func TestIncludeLockedFlag(t *testing.T) {
	// Test that include-locked flag allows deletion of locked tags
	t.Run("IncludeLockedDeleteLockedTag", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(DeleteDisabledOneTagResult, nil).Once()
		mockClient.On("UpdateAcrTagAttributes", mock.Anything, testRepo, tagName, mock.MatchedBy(func(attrs *acr.ChangeableAttributes) bool {
			return attrs.DeleteEnabled != nil && *attrs.DeleteEnabled && attrs.WriteEnabled != nil && *attrs.WriteEnabled
		})).Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, tagName).Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", ".*", 0, 60, false, true)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test that include-locked flag allows deletion of write-disabled tags
	t.Run("IncludeLockedDeleteWriteDisabledTag", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(WriteDisabledOneTagResult, nil).Once()
		mockClient.On("UpdateAcrTagAttributes", mock.Anything, testRepo, tagName, mock.MatchedBy(func(attrs *acr.ChangeableAttributes) bool {
			return attrs.DeleteEnabled != nil && *attrs.DeleteEnabled && attrs.WriteEnabled != nil && *attrs.WriteEnabled
		})).Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, tagName).Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", ".*", 0, 60, false, true)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test that include-locked flag allows deletion of locked manifests
	t.Run("IncludeLockedDeleteLockedManifest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		// Create a manifest without tags (dangling) but with deleteDisabled
		deleteDisabledDanglingManifest := &acr.Manifests{
			Registry:  &testLoginURL,
			ImageName: &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{{
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteDisabled, WriteEnabled: &writeEnabled},
				Digest:               &digest,
				MediaType:            &dockerV2MediaType,
				Tags:                 nil, // No tags - this is a dangling manifest
			}},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(deleteDisabledDanglingManifest, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("UpdateAcrManifestAttributes", mock.Anything, testRepo, digest, mock.MatchedBy(func(attrs *acr.ChangeableAttributes) bool {
			return attrs.DeleteEnabled != nil && *attrs.DeleteEnabled && attrs.WriteEnabled != nil && *attrs.WriteEnabled
		})).Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, digest).Return(&deletedResponse, nil).Once()
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, false, true)
		assert.Equal(1, deletedManifests, "Number of deleted manifests should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test that without include-locked flag, locked tags are not deleted
	t.Run("NoIncludeLockedSkipLockedTag", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(DeleteDisabledOneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", ".*", 0, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test error handling when unlock fails but deletion continues
	t.Run("IncludeLockedUnlockError", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(DeleteDisabledOneTagResult, nil).Once()
		mockClient.On("UpdateAcrTagAttributes", mock.Anything, testRepo, tagName, mock.Anything).Return(nil, errors.New("unlock failed")).Once()
		// Even though unlock fails, we still attempt deletion
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, tagName).Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", ".*", 0, 60, false, true)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1 as deletion succeeded despite unlock failure")
		assert.Nil(err, "Error should be nil as deletion succeeded")
		mockClient.AssertExpectations(t)
	})
}

// TestDryRunWithIncludeLocked contains tests for dry-run behavior with include-locked flag
func TestDryRunWithIncludeLocked(t *testing.T) {
	// Test that dry-run with include-locked shows locked tags would be deleted but doesn't actually delete
	t.Run("DryRunWithIncludeLockedShowsLockedTags", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(DeleteDisabledOneTagResult, nil).Once()
		// No unlock or delete calls should be made in dry-run mode
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", ".*", 0, 60, true, true)
		assert.Equal(1, deletedTags, "Number of tags to be deleted should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test that dry-run with include-locked shows locked manifests would be deleted
	t.Run("DryRunWithIncludeLockedShowsLockedManifests", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		// Create a manifest without tags (dangling) but with deleteDisabled
		deleteDisabledDanglingManifest := &acr.Manifests{
			Registry:  &testLoginURL,
			ImageName: &testRepo,
			ManifestsAttributes: &[]acr.ManifestAttributesBase{{
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteDisabled, WriteEnabled: &writeEnabled},
				Digest:               &digest,
				MediaType:            &dockerV2MediaType,
				Tags:                 nil, // No tags - this is a dangling manifest
			}},
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(deleteDisabledDanglingManifest, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()
		// No unlock or delete calls should be made in dry-run mode
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, defaultAgo, nil, true, true)
		assert.Equal(1, deletedManifests, "Number of manifests to be deleted should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test that dry-run without include-locked does not show locked tags
	t.Run("DryRunWithoutIncludeLockedSkipsLockedTags", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(DeleteDisabledOneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", ".*", 0, 60, true, false)
		assert.Equal(0, deletedTags, "Number of tags to be deleted should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test mixed locked and unlocked tags with include-locked in dry-run
	t.Run("DryRunWithIncludeLockedMixedTags", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		// Create a response with both locked and unlocked tags
		mixedTagsResult := &acr.RepositoryTagsType{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			Registry:  &testLoginURL,
			ImageName: &testRepo,
			TagsAttributes: &[]acr.TagAttributesBase{
				{
					Name:                 &tagName1,
					LastUpdateTime:       &lastUpdateTime,
					ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
					Digest:               &digest,
				},
				{
					Name:                 &tagName2,
					LastUpdateTime:       &lastUpdateTime,
					ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteDisabled, WriteEnabled: &writeEnabled},
					Digest:               &digest,
				},
			},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(mixedTagsResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, "0m", ".*", 0, 60, true, true)
		assert.Equal(2, deletedTags, "Number of tags to be deleted should be 2 with include-locked")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}
