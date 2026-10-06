// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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

var defaultAgoDuration time.Duration

func init() {
	defaultAgoDuration = mustParseDuration(defaultAgo)
}

// Helper function to parse ago string for tests
func mustParseDuration(ago string) time.Duration {
	d, err := parseDuration(ago)
	if err != nil {
		panic(err)
	}
	return d
}

// TestPurgeTags contains all the tests regarding the purgeTags method which is called when the --dry-run flag is
// not set.
func TestPurgeTags(t *testing.T) {
	t.Run("Delete tag with local in it", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(TagWithLocal, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-c-local.test").Return(&deletedResponse, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*-?local[.].+", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "v1(?!-a)", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "v1-*[abc]+(?<!-[a])", 0, -1, -1, 60, false, false)
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
		agoDuration := mustParseDuration("1d")
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, "[\\s\\S]*", 0, -1, -1, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there are no tags on a registry no error should show and no other methods should be called.
	t.Run("EmptyRepositoryTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(EmptyListTagsResult, nil).Once()
		agoDuration := mustParseDuration("1d")
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, "[\\s\\S]*", 0, -1, -1, 60, false, false)
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
		agoDuration := mustParseDuration("1d")
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, "[\\s\\S]*", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "^hello.*", 0, -1, -1, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Invalid regex filter, an error should be returned.
	t.Run("InvalidRegexTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "[", 0, -1, -1, 60, false, false)
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error during a call to GetAcrTags (other than a 404) an error should be returned.
	t.Run("GetAcrTagsErrorSinglePageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(nil, errors.New("unauthorized")).Once()
		agoDuration := mustParseDuration("1d")
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, "[\\s\\S]*", 0, -1, -1, 60, false, false)
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
		agoDuration := mustParseDuration("1d")
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, "[\\s\\S]*", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "^la.*", 0, -1, -1, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("OperationNotAllowedTagWriteDisabledTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(WriteDisabledOneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "^la.*", 0, -1, -1, 60, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If a tag has an invalid last update time attribute an error should be returned.
	t.Run("InvalidDurationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(InvalidDateOneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "^la.*", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "^la.*", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "[\\s\\S]*", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "^la.*", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "^la.*", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "[\\s\\S]*", 1, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, "v1-.*", 1, -1, -1, 60, false, false)
		assert.Equal(2, deletedTags, "Number of deleted elements should be 2")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("Keep 1 tag when repo filter doesn't match all results and not all results match due to ago filter", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsWithRepoFilterMatch, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1-c").Return(&deletedResponse, nil).Once()
		agoDuration := mustParseDuration("30m")
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, "v1-.*", 1, -1, -1, 60, false, false)
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunAgeOnlyReportsAge", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*", 0, -1, -1, 60, true, false)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Equal(fmt.Sprintf("Would delete tags for repository: %s\nWould delete: %s/%s:latest (reason: age)\n", testRepo, testLoginURL, testRepo), string(output))
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunKeepAndAgeReportsOnlyUnkeptTags", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsResult, nil).Once()
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*", 1, -1, -1, 60, true, false)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.NoError(err)
		assert.Equal(3, deletedTags)
		assert.Equal(fmt.Sprintf("Would delete tags for repository: %s\nWould delete: %s/%s:v2 (reason: age)\nWould delete: %s/%s:v3 (reason: age)\nWould delete: %s/%s:v4 (reason: age)\n", testRepo, testLoginURL, testRepo, testLoginURL, testRepo, testLoginURL, testRepo), string(output))
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxProtectsOldMinAndPrunesMiddleAcrossFilteredPages", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newest := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)
		middle := now.Add(-72 * time.Hour).Format(time.RFC3339Nano)
		oldest := now.Add(-96 * time.Hour).Format(time.RFC3339Nano)
		other, first, second, third := "other", "release-new", "release-middle", "release-old"
		firstDigest, secondDigest, thirdDigest := "sha256:new", "sha256:middle", "sha256:old"
		firstPage := []acr.TagAttributesBase{
			{Name: &other},
			{Name: &first, Digest: &firstDigest, LastUpdateTime: &newest},
			{Name: &second, Digest: &secondDigest, LastUpdateTime: &middle},
		}
		secondPage := []acr.TagAttributesBase{{Name: &third, Digest: &thirdDigest, LastUpdateTime: &oldest}}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response:       autorest.Response{Response: &http.Response{Header: http.Header{"Link": {"</tags?last=release-middle>"}}}},
			TagsAttributes: &firstPage,
		}, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", second).Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &secondPage,
		}, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, second).Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, third).Return(&deletedResponse, nil).Once()

		agoDuration := mustParseDuration("1d")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, "^release-", 0, 1, 2, 60, false, false)

		assert.NoError(err)
		assert.Equal(2, deletedTags)
		assert.Equal(map[string]int{secondDigest: 1, thirdDigest: 1}, byDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, first)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, other)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxRetainsRecentMiddleButDeletesRecentOverflow", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newest := now.Add(-time.Hour).Format(time.RFC3339Nano)
		middle := now.Add(-2 * time.Hour).Format(time.RFC3339Nano)
		oldest := now.Add(-3 * time.Hour).Format(time.RFC3339Nano)
		first, second, third := "new", "middle", "old"
		firstDigest, secondDigest, thirdDigest := "sha256:new", "sha256:middle", "sha256:old"
		tags := []acr.TagAttributesBase{
			{Name: &first, Digest: &firstDigest, LastUpdateTime: &newest},
			{Name: &second, Digest: &secondDigest, LastUpdateTime: &middle},
			{Name: &third, Digest: &thirdDigest, LastUpdateTime: &oldest},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, third).Return(&deletedResponse, nil).Once()

		agoDuration := mustParseDuration("1d")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, ".*", 0, 1, 2, 60, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Equal(map[string]int{thirdDigest: 1}, byDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, first)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, second)
		mockClient.AssertExpectations(t)
	})

	t.Run("MaxWithoutAgoRetainsOldTagsBelowLimit", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		name, digest := "old", "sha256:old"
		tags := []acr.TagAttributesBase{{Name: &name, Digest: &digest, LastUpdateTime: &old}}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()

		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, nil, ".*", 0, -1, 2, 60, false, false)

		assert.NoError(err)
		assert.Zero(deletedTags)
		assert.Empty(byDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("ZeroAgoAndZeroMinPrunesBelowMaxButRetainsFuture", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		old := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)
		future := now.Add(time.Hour).Format(time.RFC3339Nano)
		oldName, futureName, oldDigest, futureDigest := "old", "future", "sha256:old", "sha256:future"
		tags := []acr.TagAttributesBase{
			{Name: &futureName, Digest: &futureDigest, LastUpdateTime: &future},
			{Name: &oldName, Digest: &oldDigest, LastUpdateTime: &old},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, oldName).Return(&deletedResponse, nil).Once()

		agoDuration := mustParseDuration("0s")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, ".*", 0, 0, 2, 60, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Equal(map[string]int{oldDigest: 1}, byDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, futureName)
		mockClient.AssertExpectations(t)
	})

	t.Run("ZeroMinMaxDeletesEvenFutureTags", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
		name, digest := "future", "sha256:future"
		tags := []acr.TagAttributesBase{{Name: &name, Digest: &digest, LastUpdateTime: &future}}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, name).Return(&deletedResponse, nil).Once()

		agoDuration := mustParseDuration("0s")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, ".*", 0, 0, 0, 60, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Equal(map[string]int{digest: 1}, byDigest)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxAbovePopulationProtectsAllOldTags", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		name, digest := "old", "sha256:old"
		tags := []acr.TagAttributesBase{{Name: &name, Digest: &digest, LastUpdateTime: &old}}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()

		agoDuration := mustParseDuration("1d")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, ".*", 0, 2, 3, 60, false, false)

		assert.NoError(err)
		assert.Zero(deletedTags)
		assert.Empty(byDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("RetentionFlagValidation", func(t *testing.T) {
		if value, exists := os.LookupEnv("ACR_DEFAULT_REGISTRY"); exists {
			assert.NoError(t, os.Unsetenv("ACR_DEFAULT_REGISTRY"))
			t.Cleanup(func() { _ = os.Setenv("ACR_DEFAULT_REGISTRY", value) })
		}
		for _, flag := range []string{"min-tags", "max-tags", "min-untagged-manifests", "max-untagged-manifests"} {
			for _, value := range []string{"-1", "-2"} {
				t.Run(flag+value, func(t *testing.T) {
					cmd := newPurgeCmd(&rootParameters{})
					cmd.SetArgs([]string{"--filter", "repo:.*", "--ago", "3d", "--untagged", "--" + flag, value})
					assert.ErrorContains(t, cmd.Execute(), "--"+flag+" must be nonnegative")
				})
			}
			for _, keep := range []string{"0", "2"} {
				t.Run(flag+"-keep-"+keep, func(t *testing.T) {
					cmd := newPurgeCmd(&rootParameters{})
					cmd.SetArgs([]string{"--filter", "repo:.*", "--ago", "3d", "--untagged", "--" + flag, "0", "--keep", keep})
					assert.ErrorContains(t, cmd.Execute(), "none of the others can be")
				})
			}
			t.Run(flag+"-negative-without-age", func(t *testing.T) {
				cmd := newPurgeCmd(&rootParameters{})
				cmd.SetArgs([]string{"--untagged-only", "--" + flag, "-1"})
				assert.EqualError(t, cmd.Execute(), "--"+flag+" must be nonnegative")
			})
			t.Run(flag+"-negative-with-keep", func(t *testing.T) {
				cmd := newPurgeCmd(&rootParameters{})
				cmd.SetArgs([]string{"--untagged-only", "--" + flag, "-1", "--keep", "0"})
				assert.ErrorContains(t, cmd.Execute(), "none of the others can be")
			})
		}
		invalid := map[string][]string{
			"--min-tags requires --ago":                                         {"--filter", "repo:.*", "--min-tags", "0", "--max-tags", "5"},
			"--min-untagged-manifests requires --ago":                           {"--untagged-only", "--min-untagged-manifests", "0"},
			"--min-tags must not exceed --max-tags":                             {"--filter", "repo:.*", "--ago", "3d", "--min-tags", "3", "--max-tags", "2"},
			"--min-untagged-manifests must not exceed --max-untagged-manifests": {"--untagged-only", "--ago", "3d", "--min-untagged-manifests", "3", "--max-untagged-manifests", "2"},
			"--max-tags cannot be combined with --untagged-only":                {"--untagged-only", "--max-tags", "1"},
			"--min-tags cannot be combined with --untagged-only":                {"--untagged-only", "--ago", "3d", "--min-tags", "1"},
			"--max-untagged-manifests requires --untagged or --untagged-only":   {"--filter", "repo:.*", "--ago", "3d", "--max-untagged-manifests", "1"},
			"--min-untagged-manifests requires --untagged or --untagged-only":   {"--filter", "repo:.*", "--ago", "3d", "--min-untagged-manifests", "1"},
			"--ago or --max-tags is required":                                   {"--filter", "repo:.*", "--untagged", "--max-untagged-manifests", "1"},
			"at least one of the flags in the group":                            {"--max-tags", "1"},
			"none of the others can be":                                         {"--untagged", "--untagged-only"},
		}
		for message, args := range invalid {
			t.Run(message, func(t *testing.T) {
				cmd := newPurgeCmd(&rootParameters{})
				cmd.SetArgs(args)
				assert.ErrorContains(t, cmd.Execute(), message)
			})
		}
		for i, args := range [][]string{
			{"--filter", "repo:.*", "--max-tags", "0"},
			{"--filter", "repo:.*", "--ago", "0s", "--min-tags", "0", "--max-tags", "0"},
			{"--filter", "repo:.*", "--max-tags", "2", "--untagged"},
			{"--filter", "repo:.*", "--max-tags", "2", "--untagged", "--max-untagged-manifests", "1"},
			{"--untagged-only", "--max-untagged-manifests", "0"},
			{"--untagged-only", "--ago", "0s", "--min-untagged-manifests", "0", "--max-untagged-manifests", "0"},
			{"--filter", "repo:.*", "--ago", "3d", "--keep", "2"},
		} {
			t.Run(fmt.Sprintf("valid-%d", i), func(t *testing.T) {
				cmd := newPurgeCmd(&rootParameters{})
				cmd.SetArgs(args)
				assert.EqualError(t, cmd.Execute(), "unable to determine registry name, please use --registry flag")
			})
		}
	})

	t.Run("MinMaxCountsProtectedLocksBeforeSelectingOverflow", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
		protected, overflow := "protected", "overflow"
		protectedDigest, overflowDigest := "sha256:protected", "sha256:overflow"
		disabled := false
		tags := []acr.TagAttributesBase{
			{Name: &protected, Digest: &protectedDigest, LastUpdateTime: &recent, ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &disabled}},
			{Name: &overflow, Digest: &overflowDigest, LastUpdateTime: &recent},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, overflow).Return(&deletedResponse, nil).Once()

		agoDuration := mustParseDuration("1d")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, ".*", 0, 1, 1, 60, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Equal(map[string]int{overflowDigest: 1}, byDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, protected)
		mockClient.AssertNotCalled(t, "UpdateAcrTagAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxIncludeLockedUnlocksOnlySelectedTags", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		protected, middle, overflow := "protected", "middle", "overflow"
		protectedDigest, middleDigest, overflowDigest := "sha256:protected", "sha256:middle", "sha256:overflow"
		enabled, disabled := true, false
		tags := []acr.TagAttributesBase{
			{Name: &protected, Digest: &protectedDigest, LastUpdateTime: &old, ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &disabled}},
			{Name: &middle, Digest: &middleDigest, LastUpdateTime: &old, ChangeableAttributes: &acr.ChangeableAttributes{WriteEnabled: &disabled}},
			{Name: &overflow, Digest: &overflowDigest, LastUpdateTime: &old, ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &disabled}},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		unlockAttrs := &acr.ChangeableAttributes{DeleteEnabled: &enabled, WriteEnabled: &enabled}
		unlockMiddle := mockClient.On("UpdateAcrTagAttributes", mock.Anything, testRepo, middle, unlockAttrs).Return(&deletedResponse, nil).Once()
		unlockOverflow := mockClient.On("UpdateAcrTagAttributes", mock.Anything, testRepo, overflow, unlockAttrs).Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, middle).Return(&deletedResponse, nil).Once().NotBefore(unlockMiddle)
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, overflow).Return(&deletedResponse, nil).Once().NotBefore(unlockOverflow)

		agoDuration := mustParseDuration("1d")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, ".*", 0, 1, 2, 60, false, true)

		assert.NoError(err)
		assert.Equal(2, deletedTags)
		assert.Equal(map[string]int{middleDigest: 1, overflowDigest: 1}, byDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, protected)
		mockClient.AssertNotCalled(t, "UpdateAcrTagAttributes", mock.Anything, testRepo, protected, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxDryRunIncludeLockedReportsOldMiddleAndOverflowWithoutMutations", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		protected, middle, overflow := "protected", "middle", "overflow"
		protectedDigest, middleDigest, overflowDigest := "sha256:protected", "sha256:middle", "sha256:overflow"
		disabled := false
		tags := []acr.TagAttributesBase{
			{Name: &protected, Digest: &protectedDigest, LastUpdateTime: &old, ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &disabled}},
			{Name: &middle, Digest: &middleDigest, LastUpdateTime: &old, ChangeableAttributes: &acr.ChangeableAttributes{WriteEnabled: &disabled}},
			{Name: &overflow, Digest: &overflowDigest, LastUpdateTime: &old, ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &disabled}},
		}
		firstPage, secondPage := tags[:2], tags[2:]
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{Header: http.Header{"Link": {"</tags?last=middle>"}}}}, TagsAttributes: &firstPage,
		}, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", middle).Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &secondPage,
		}, nil).Once()
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })

		agoDuration := mustParseDuration("1d")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, ".*", 0, 1, 2, 60, true, true)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.NoError(err)
		assert.Equal(2, deletedTags)
		assert.Equal(map[string]int{middleDigest: 1, overflowDigest: 1}, byDigest)
		assert.Equal(fmt.Sprintf("Would delete tags for repository: %s\nWould delete: %s/%s:middle (reason: age)\nWould delete: %s/%s:overflow (reason: age and maximum count)\n", testRepo, testLoginURL, testRepo, testLoginURL, testRepo), string(output))
		assert.False(*tags[0].ChangeableAttributes.DeleteEnabled)
		assert.False(*tags[1].ChangeableAttributes.WriteEnabled)
		assert.False(*tags[2].ChangeableAttributes.DeleteEnabled)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrTagAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxDryRunReportsRecentOverflowAndRetainedLock", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
		protected, locked, overflow := "protected", "locked", "overflow"
		protectedDigest, lockedDigest, overflowDigest := "sha256:protected", "sha256:locked", "sha256:overflow"
		disabled := false
		tags := []acr.TagAttributesBase{
			{Name: &protected, Digest: &protectedDigest, LastUpdateTime: &recent},
			{Name: &locked, Digest: &lockedDigest, LastUpdateTime: &recent, ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &disabled}},
			{Name: &overflow, Digest: &overflowDigest, LastUpdateTime: &recent},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })

		agoDuration := mustParseDuration("1d")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, ".*", 0, 1, 1, 60, true, false)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Equal(map[string]int{overflowDigest: 1}, byDigest)
		assert.Equal(fmt.Sprintf("Would delete tags for repository: %s\nWarning: Retaining locked tag %s:locked (reason: maximum count)\nWould delete: %s/%s:overflow (reason: maximum count)\n", testRepo, testRepo, testLoginURL, testRepo), string(output))
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrTagAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxWarnsForWriteLockWhileAgePrunesUnlockedTag", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		protected, middle, locked := "protected", "middle", "locked"
		protectedDigest, middleDigest, lockedDigest := "sha256:protected", "sha256:middle", "sha256:locked"
		disabled := false
		tags := []acr.TagAttributesBase{
			{Name: &protected, Digest: &protectedDigest, LastUpdateTime: &old},
			{Name: &middle, Digest: &middleDigest, LastUpdateTime: &old},
			{Name: &locked, Digest: &lockedDigest, LastUpdateTime: &old, ChangeableAttributes: &acr.ChangeableAttributes{WriteEnabled: &disabled}},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, middle).Return(&deletedResponse, nil).Once()
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })

		agoDuration := mustParseDuration("1d")
		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, 1, testLoginURL, testRepo, &agoDuration, ".*", 0, 1, 2, 60, false, false)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Equal(map[string]int{middleDigest: 1}, byDigest)
		assert.Contains(string(output), fmt.Sprintf("Warning: Retaining locked tag %s:locked (reason: age and maximum count)\n", testRepo))
		assert.NotContains(string(output), "not satisfied")
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, locked)
		mockClient.AssertNotCalled(t, "UpdateAcrTagAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MaxReportsSkippedTagWhenServerRejectsDeletion", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		protected, blocked := "protected", "blocked"
		protectedDigest, blockedDigest := "sha256:protected", "sha256:blocked"
		tags := []acr.TagAttributesBase{
			{Name: &protected, Digest: &protectedDigest, LastUpdateTime: &old},
			{Name: &blocked, Digest: &blockedDigest, LastUpdateTime: &old},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, blocked).Return(&autorest.Response{
			Response: &http.Response{StatusCode: http.StatusMethodNotAllowed},
		}, errors.New("operation not allowed")).Once()
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })

		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, 1, testLoginURL, testRepo, nil, ".*", 0, -1, 1, 60, false, false)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.NoError(err)
		assert.Zero(deletedTags)
		assert.Equal(map[string]int{blockedDigest: 1}, byDigest)
		assert.Equal(fmt.Sprintf("Deleting tags for repository: %s\nSkipped %s/%s:blocked, operation not allowed, HTTP status: 405\n", testRepo, testLoginURL, testRepo), string(output))
		assert.NotContains(string(output), "Warning: Retaining locked tag")
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, protected)
		mockClient.AssertExpectations(t)
	})

	t.Run("MaxProtectedFirstPageDoesNotMaskLaterListingFailure", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		name, digest := "protected", "sha256:protected"
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		tags := []acr.TagAttributesBase{{Name: &name, Digest: &digest, LastUpdateTime: &old}}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response:       autorest.Response{Response: &http.Response{Header: http.Header{"Link": {"</tags?last=protected>"}}}},
			TagsAttributes: &tags,
		}, nil).Once()
		listingErr := errors.New("listing failed")
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", name).Return(nil, listingErr).Once()

		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, nil, ".*", 0, -1, 1, 60, false, false)

		assert.ErrorIs(err, listingErr)
		assert.Equal(-1, deletedTags)
		assert.Empty(byDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MaxProtectedFirstPageDoesNotMaskLaterTimestampError", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		firstName, secondName, firstDigest, secondDigest := "protected", "invalid", "sha256:protected", "sha256:invalid"
		old, invalid := time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339Nano), "invalid"
		first := []acr.TagAttributesBase{{Name: &firstName, Digest: &firstDigest, LastUpdateTime: &old}}
		second := []acr.TagAttributesBase{{Name: &secondName, Digest: &secondDigest, LastUpdateTime: &invalid}}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response:       autorest.Response{Response: &http.Response{Header: http.Header{"Link": {"</tags?last=protected>"}}}},
			TagsAttributes: &first,
		}, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", firstName).Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &second,
		}, nil).Once()

		deletedTags, byDigest, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, nil, ".*", 0, -1, 1, 60, false, false)

		var parseErr *time.ParseError
		if assert.ErrorAs(err, &parseErr) {
			assert.Equal("invalid", parseErr.Value)
			assert.Equal(time.RFC3339Nano, parseErr.Layout)
		}
		assert.Equal(-1, deletedTags)
		assert.Empty(byDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error (different to a 404 error) getting the first set of manifests an error should be returned.
	t.Run("GetAcrManifestsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(nil, errors.New("unauthorized")).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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

		agoDuration := mustParseDuration("1h")
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, 0, -1, -1, nil, false, false)
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

		agoDuration := mustParseDuration("24h")
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		// This call may or may not happen depending on goroutine timing: the
		// tagged parent's goroutine may add digest1 to the ignoreList before
		// the main loop reaches this untagged manifest.
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683").Return(emptyManifestBytes, nil).Maybe()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(emptyManifestBytes, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(doubleOCIWithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(nil, nil).Once()
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
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
		deletedTags, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, false)
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunKeepAndAgeReportsOnlyUnkeptManifests", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		manifests := []acr.ManifestAttributesBase{
			createManifestWithTime("sha256:kept", now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
			createManifestWithTime("sha256:old", now.Add(-72*time.Hour).Format(time.RFC3339Nano)),
		}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:old").Return(EmptyListManifestsResult, nil).Once()
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })
		agoDuration := mustParseDuration("1d")
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, 1, testLoginURL, testRepo, &agoDuration, 1, -1, -1, nil, true, false)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.NoError(err)
		assert.Equal(1, deletedManifests)
		assert.Equal(fmt.Sprintf("Would delete manifests for repository: %s\nWould delete: %s/%s@sha256:old (reason: age)\n", testRepo, testLoginURL, testRepo), string(output))
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxProtectsOldMinAndPrunesMiddleAfterAllPages", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		newest := createManifestWithTime("sha256:new", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		middle := createManifestWithTime("sha256:middle", now.Add(-72*time.Hour).Format(time.RFC3339Nano))
		oldest := createManifestWithTime("sha256:old", now.Add(-96*time.Hour).Format(time.RFC3339Nano))
		first := []acr.ManifestAttributesBase{oldest, middle}
		second := []acr.ManifestAttributesBase{newest, middle}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &first}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *middle.Digest).Return(&acr.Manifests{ManifestsAttributes: &second}, nil).Once()
		discovered := mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *middle.Digest).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, *middle.Digest).Return(&deletedResponse, nil).Once().NotBefore(discovered)
		mockClient.On("DeleteManifest", mock.Anything, testRepo, *oldest.Digest).Return(&deletedResponse, nil).Once().NotBefore(discovered)

		agoDuration := mustParseDuration("1d")
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, 0, 1, 2, nil, false, false)

		assert.NoError(err)
		assert.Equal(2, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, *newest.Digest)
		mockClient.AssertNumberOfCalls(t, "GetAcrManifests", 3)
		mockClient.AssertExpectations(t)
	})

	t.Run("MaxWithoutAgoRetainsOldManifestsBelowLimit", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		old := createManifestWithTime("sha256:old", time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339Nano))
		manifests := []acr.ManifestAttributesBase{old}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *old.Digest).Return(EmptyListManifestsResult, nil).Once()

		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, nil, 0, -1, 2, nil, false, false)

		assert.NoError(err)
		assert.Zero(deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("ZeroAgoAndZeroMinPrunesBelowMaxButRetainsFuture", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		old := createManifestWithTime("sha256:old", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		future := createManifestWithTime("sha256:future", now.Add(time.Hour).Format(time.RFC3339Nano))
		manifests := []acr.ManifestAttributesBase{old, future}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *future.Digest).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, *old.Digest).Return(&deletedResponse, nil).Once()

		agoDuration := mustParseDuration("0s")
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, 0, 0, 2, nil, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, *future.Digest)
		mockClient.AssertExpectations(t)
	})

	t.Run("ZeroMinMaxDeletesEvenFutureManifests", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		future := createManifestWithTime("sha256:future", time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano))
		manifests := []acr.ManifestAttributesBase{future}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *future.Digest).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, *future.Digest).Return(&deletedResponse, nil).Once()

		agoDuration := mustParseDuration("0s")
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &agoDuration, 0, 0, 0, nil, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedManifests)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxRetainedNestedIndexesProtectEarlierChildrenWithoutReranking", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		parent := createManifestWithTime("sha256:parent", now.Add(-time.Hour).Format(time.RFC3339Nano))
		nested := createManifestWithTime("sha256:nested", now.Add(-2*time.Hour).Format(time.RFC3339Nano))
		leaf := createManifestWithTime("sha256:leaf", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		other := createManifestWithTime("sha256:other", now.Add(-72*time.Hour).Format(time.RFC3339Nano))
		parent.MediaType, nested.MediaType = &dockerV2ListMediaType, &dockerV2ListMediaType
		first := []acr.ManifestAttributesBase{leaf, nested}
		second := []acr.ManifestAttributesBase{parent, other}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &first}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *nested.Digest).Return(&acr.Manifests{ManifestsAttributes: &second}, nil).Once()
		discovered := mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *other.Digest).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, *parent.Digest).Return([]byte(fmt.Sprintf(`{"manifests":[{"digest":"sha256:nested","mediaType":%q}]}`, dockerV2ListMediaType)), nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, *nested.Digest).Return([]byte(fmt.Sprintf(`{"manifests":[{"digest":"sha256:leaf","mediaType":%q}]}`, dockerV2MediaType)), nil).Twice()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, *other.Digest).Return(&deletedResponse, nil).Once().NotBefore(discovered)

		agoDuration := mustParseDuration("1d")
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, 1, testLoginURL, testRepo, &agoDuration, 0, 1, 2, nil, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, *parent.Digest)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, *nested.Digest)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, *leaf.Digest)
		mockClient.AssertNumberOfCalls(t, "GetAcrManifests", 3)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxRetainedIndexInspectionFailurePreventsDeletion", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		parent := createManifestWithTime("sha256:parent", now.Add(-time.Hour).Format(time.RFC3339Nano))
		other := createManifestWithTime("sha256:other", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		parent.MediaType = &dockerV2ListMediaType
		manifests := []acr.ManifestAttributesBase{parent, other}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *other.Digest).Return(EmptyListManifestsResult, nil).Once()
		inspectionErr := errors.New("dependency inspection failed")
		mockClient.On("GetManifest", mock.Anything, testRepo, *parent.Digest).Return(nil, inspectionErr).Once()

		agoDuration := mustParseDuration("1d")
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, 1, testLoginURL, testRepo, &agoDuration, 0, 1, 1, nil, false, false)

		assert.ErrorIs(err, inspectionErr)
		assert.Equal(-1, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxDryRunNestedIndexInspectionFailureIsReturned", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		parent := createManifestWithTime("sha256:parent", now.Add(-time.Hour).Format(time.RFC3339Nano))
		nested := createManifestWithTime("sha256:nested", now.Add(-48*time.Hour).Format(time.RFC3339Nano))
		parent.MediaType, nested.MediaType = &dockerV2ListMediaType, &dockerV2ListMediaType
		manifests := []acr.ManifestAttributesBase{nested, parent}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *parent.Digest).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, *parent.Digest).Return([]byte(fmt.Sprintf(`{"manifests":[{"digest":"sha256:nested","mediaType":%q}]}`, dockerV2ListMediaType)), nil).Once()
		inspectionErr := errors.New("nested dependency inspection failed")
		mockClient.On("GetManifest", mock.Anything, testRepo, *nested.Digest).Return(nil, inspectionErr).Once()

		agoDuration := mustParseDuration("1d")
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, 1, testLoginURL, testRepo, &agoDuration, 0, 1, 1, nil, true, false)

		assert.ErrorIs(err, inspectionErr)
		assert.Equal(-1, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("TagAndManifestMinMaxAreIndependentPerRepository", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		now := time.Now().UTC()
		recent, old := now.Add(-time.Hour).Format(time.RFC3339Nano), now.Add(-5*24*time.Hour).Format(time.RFC3339Nano)
		newName, oldName, anotherName := "release-new", "release-old", "release-older"
		newDigest, oldDigest := "sha256:new", "sha256:old"
		catalog := &mocks.BaseClientAPI{}
		repos, err := repository.CollectTagFilters(testCtx, []string{"repo-a:^release-", "repo-a:old$", "repo-b:^release-", "repo-b:old$"}, catalog, 60, defaultRepoPageSize)
		if !assert.NoError(err) {
			t.FailNow()
		}
		assert.Len(repos, 2)
		for repo := range repos {
			tags := []acr.TagAttributesBase{
				{Name: &newName, Digest: &newDigest, LastUpdateTime: &old},
				{Name: &oldName, Digest: &oldDigest, LastUpdateTime: &old},
				{Name: &anotherName, Digest: &oldDigest, LastUpdateTime: &old},
			}
			mockClient.On("GetAcrTags", mock.Anything, repo, "timedesc", "").Return(&acr.RepositoryTagsType{
				Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
			}, nil).Once()
			firstDeletion := mockClient.On("DeleteAcrTag", mock.Anything, repo, oldName).Return(&deletedResponse, nil).Once()
			secondDeletion := mockClient.On("DeleteAcrTag", mock.Anything, repo, anotherName).Return(&deletedResponse, nil).Once()
			tagged := createManifestWithTime(newDigest, recent)
			tagged.Tags = &[]string{newName}
			newlyDangling := createManifestWithTime(oldDigest, recent)
			dangling := createManifestWithTime("sha256:dangling", old)
			manifests := []acr.ManifestAttributesBase{tagged, newlyDangling, dangling}
			mockClient.On("GetAcrManifests", mock.Anything, repo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once().NotBefore(firstDeletion, secondDeletion)
			mockClient.On("GetAcrManifests", mock.Anything, repo, "", *dangling.Digest).Return(EmptyListManifestsResult, nil).Once()
			mockClient.On("DeleteManifest", mock.Anything, repo, *dangling.Digest).Return(&deletedResponse, nil).Once()
		}

		agoDuration := mustParseDuration("1d")
		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 1, &agoDuration, 0, 1, 1, 1, 1, 60, true, false, repos, false, false, false)

		assert.NoError(err)
		assert.Equal(4, deletedTags)
		assert.Equal(2, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, oldDigest)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, newDigest)
		catalog.AssertExpectations(t)
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunTagAndManifestMinMaxCountNewlyDanglingManifestsPerRepository", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		now := time.Now().UTC()
		recent, old := now.Add(-time.Hour).Format(time.RFC3339Nano), now.Add(-5*24*time.Hour).Format(time.RFC3339Nano)
		newName, oldName, anotherName := "release-new", "release-old", "release-older"
		newDigest, oldDigest := "sha256:new", "sha256:old"
		catalog := &mocks.BaseClientAPI{}
		repos, err := repository.CollectTagFilters(testCtx, []string{"repo-a:^release-", "repo-a:old$", "repo-b:^release-", "repo-b:old$"}, catalog, 60, defaultRepoPageSize)
		if !assert.NoError(err) {
			t.FailNow()
		}
		assert.Len(repos, 2)
		for repo := range repos {
			tags := []acr.TagAttributesBase{
				{Name: &newName, Digest: &newDigest, LastUpdateTime: &old},
				{Name: &oldName, Digest: &oldDigest, LastUpdateTime: &old},
				{Name: &anotherName, Digest: &oldDigest, LastUpdateTime: &old},
			}
			mockClient.On("GetAcrTags", mock.Anything, repo, "timedesc", "").Return(&acr.RepositoryTagsType{
				Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
			}, nil).Once()
			tagged := createManifestWithTime(newDigest, recent)
			tagged.Tags = &[]string{newName}
			newlyDangling := createManifestWithTime(oldDigest, recent)
			newlyDangling.Tags = &[]string{oldName, anotherName}
			dangling := createManifestWithTime("sha256:dangling", old)
			manifests := []acr.ManifestAttributesBase{tagged, newlyDangling, dangling}
			mockClient.On("GetAcrManifests", mock.Anything, repo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
			mockClient.On("GetAcrManifests", mock.Anything, repo, "", *dangling.Digest).Return(EmptyListManifestsResult, nil).Once()
		}

		agoDuration := mustParseDuration("1d")
		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 1, &agoDuration, 0, 1, 1, 1, 1, 60, true, false, repos, true, false, false)

		assert.NoError(err)
		assert.Equal(4, deletedTags)
		assert.Equal(2, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrTagAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		catalog.AssertExpectations(t)
		mockClient.AssertExpectations(t)
	})

	t.Run("TagMaxWithoutManifestLimitsCleansAllNewlyDanglingManifests", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
		first, second, digest := "release-a", "release-b", "sha256:newly-dangling"
		tags := []acr.TagAttributesBase{
			{Name: &first, Digest: &digest, LastUpdateTime: &recent},
			{Name: &second, Digest: &digest, LastUpdateTime: &recent},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		firstDeletion := mockClient.On("DeleteAcrTag", mock.Anything, testRepo, first).Return(&deletedResponse, nil).Once()
		secondDeletion := mockClient.On("DeleteAcrTag", mock.Anything, testRepo, second).Return(&deletedResponse, nil).Once()
		manifests := []acr.ManifestAttributesBase{createManifestWithTime(digest, recent)}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once().NotBefore(firstDeletion, secondDeletion)
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, digest).Return(&deletedResponse, nil).Once()

		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 1, nil, 0, -1, 0, -1, -1, 60, true, false, map[string]string{testRepo: "^release-"}, false, false, false)

		assert.NoError(err)
		assert.Equal(2, deletedTags)
		assert.Equal(1, deletedManifests)
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunTagMaxWithoutManifestLimitsPredictsNewlyDanglingCleanup", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
		future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
		first, second, digest := "release-a", "release-b", "sha256:newly-dangling"
		tags := []acr.TagAttributesBase{
			{Name: &first, Digest: &digest, LastUpdateTime: &future},
			{Name: &second, Digest: &digest, LastUpdateTime: &recent},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		newlyDangling := createManifestWithTime(digest, recent)
		newlyDangling.Tags = &[]string{first, second}
		manifests := []acr.ManifestAttributesBase{newlyDangling}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()

		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })
		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 1, nil, 0, -1, 0, -1, -1, 60, true, false, map[string]string{testRepo: "^release-"}, true, false, false)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.Contains(string(output), fmt.Sprintf("Would delete tags for repository: %s\nWould delete: %s/%s:release-a (reason: maximum count)\nWould delete: %s/%s:release-b (reason: maximum count)\nWould delete manifests for repository: %s\nWould delete: %s/%s@%s (reason: untagged)\n", testRepo, testLoginURL, testRepo, testLoginURL, testRepo, testRepo, testLoginURL, testRepo, digest))
		assert.NoError(err)
		assert.Equal(2, deletedTags)
		assert.Equal(1, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrTagAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunRemainingNonmatchingTagProtectsManifestFromZeroMax", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		first, second, remaining, digest := "release-a", "release-b", "not-matching", "sha256:tagged"
		tags := []acr.TagAttributesBase{
			{Name: &first, Digest: &digest, LastUpdateTime: &old},
			{Name: &second, Digest: &digest, LastUpdateTime: &old},
			{Name: &remaining, Digest: &digest, LastUpdateTime: &old},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		tagged := createManifestWithTime(digest, old)
		tagged.Tags = &[]string{first, second, remaining}
		manifests := []acr.ManifestAttributesBase{tagged}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()

		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 1, nil, 0, -1, 0, -1, 0, 60, true, false, map[string]string{testRepo: "^release-"}, true, false, false)

		assert.NoError(err)
		assert.Equal(2, deletedTags)
		assert.Zero(deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunLockedTagDoesNotMakeManifestDangling", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		locked, unlocked, digest := "locked", "unlocked", "sha256:tagged"
		disabled := false
		tags := []acr.TagAttributesBase{
			{Name: &locked, Digest: &digest, LastUpdateTime: &old, ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &disabled}},
			{Name: &unlocked, Digest: &digest, LastUpdateTime: &old},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		tagged := createManifestWithTime(digest, old)
		tagged.Tags = &[]string{locked, unlocked}
		manifests := []acr.ManifestAttributesBase{tagged}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()

		agoDuration := mustParseDuration("0s")
		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 1, &agoDuration, 0, 0, 0, 0, 0, 60, true, false, map[string]string{testRepo: ".*"}, true, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Zero(deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrTagAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("ServerRejectedTagDeletionDoesNotMakeManifestDangling", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
		blocked, deleted, digest := "blocked", "deleted", "sha256:tagged"
		tags := []acr.TagAttributesBase{
			{Name: &blocked, Digest: &digest, LastUpdateTime: &old},
			{Name: &deleted, Digest: &digest, LastUpdateTime: &old},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		rejection := mockClient.On("DeleteAcrTag", mock.Anything, testRepo, blocked).Return(&autorest.Response{
			Response: &http.Response{StatusCode: http.StatusMethodNotAllowed},
		}, errors.New("operation not allowed")).Once()
		deletion := mockClient.On("DeleteAcrTag", mock.Anything, testRepo, deleted).Return(&deletedResponse, nil).Once()
		tagged := createManifestWithTime(digest, old)
		tagged.Tags = &[]string{blocked}
		manifests := []acr.ManifestAttributesBase{tagged}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once().NotBefore(rejection, deletion)
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()

		agoDuration := mustParseDuration("0s")
		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 1, &agoDuration, 0, 0, 0, 0, 0, 60, true, false, map[string]string{testRepo: ".*"}, false, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Zero(deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("TagMinWithManifestAgeRetainsRecentlyUntaggedManifest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		now := time.Now().UTC()
		recent, old := now.Add(-time.Hour).Format(time.RFC3339Nano), now.Add(-5*24*time.Hour).Format(time.RFC3339Nano)
		protectedName, deletedName := "protected", "deleted"
		protectedDigest, deletedDigest := "sha256:protected", "sha256:deleted"
		tags := []acr.TagAttributesBase{
			{Name: &protectedName, Digest: &protectedDigest, LastUpdateTime: &old},
			{Name: &deletedName, Digest: &deletedDigest, LastUpdateTime: &old},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		deletion := mockClient.On("DeleteAcrTag", mock.Anything, testRepo, deletedName).Return(&deletedResponse, nil).Once()
		protected := createManifestWithTime(protectedDigest, old)
		protected.Tags = &[]string{protectedName}
		newlyDangling := createManifestWithTime(deletedDigest, recent)
		dangling := createManifestWithTime("sha256:dangling", old)
		manifests := []acr.ManifestAttributesBase{protected, newlyDangling, dangling}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once().NotBefore(deletion)
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *dangling.Digest).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, *dangling.Digest).Return(&deletedResponse, nil).Once()

		agoDuration := mustParseDuration("3d")
		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 1, &agoDuration, 0, 1, -1, -1, -1, 60, true, false, map[string]string{testRepo: ".*"}, false, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Equal(1, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, testRepo, deletedDigest)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, testRepo, protectedName)
		mockClient.AssertExpectations(t)
	})

	t.Run("DryRunTagAgeAndManifestMaxUseIndependentPopulations", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		now := time.Now().UTC()
		recent, old := now.Add(-time.Hour).Format(time.RFC3339Nano), now.Add(-5*24*time.Hour).Format(time.RFC3339Nano)
		protectedName, deletedName := "protected", "deleted"
		protectedDigest, deletedDigest := "sha256:protected", "sha256:deleted"
		tags := []acr.TagAttributesBase{
			{Name: &protectedName, Digest: &protectedDigest, LastUpdateTime: &recent},
			{Name: &deletedName, Digest: &deletedDigest, LastUpdateTime: &old},
		}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(&acr.RepositoryTagsType{
			Response: autorest.Response{Response: &http.Response{}}, TagsAttributes: &tags,
		}, nil).Once()
		protected := createManifestWithTime(protectedDigest, recent)
		protected.Tags = &[]string{protectedName}
		newlyDangling := createManifestWithTime(deletedDigest, recent)
		newlyDangling.Tags = &[]string{deletedName}
		dangling := createManifestWithTime("sha256:dangling", old)
		manifests := []acr.ManifestAttributesBase{protected, newlyDangling, dangling}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *dangling.Digest).Return(EmptyListManifestsResult, nil).Once()

		agoDuration := mustParseDuration("3d")
		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 1, &agoDuration, 0, -1, -1, -1, 1, 60, true, false, map[string]string{testRepo: ".*"}, true, false, false)

		assert.NoError(err)
		assert.Equal(1, deletedTags)
		assert.Equal(1, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteAcrTag", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MaxListingFailurePreventsDeletionOfEarlierCandidates", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		old := createManifestWithTime("sha256:old", time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339Nano))
		manifests := []acr.ManifestAttributesBase{old}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		listingErr := errors.New("listing failed")
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *old.Digest).Return(nil, listingErr).Once()

		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, nil, 0, -1, 0, nil, false, false)

		assert.ErrorIs(err, listingErr)
		assert.Equal(-1, deletedManifests)
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MinMaxDryRunWarnsForSelectedLockWithoutAggregateManifestWarning", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		now := time.Now().UTC()
		protected := createManifestWithTime("sha256:protected", now.Add(-time.Hour).Format(time.RFC3339Nano))
		locked := createManifestWithTime("sha256:locked", now.Add(-2*time.Hour).Format(time.RFC3339Nano))
		overflow := createManifestWithTime("sha256:overflow", now.Add(-3*time.Hour).Format(time.RFC3339Nano))
		disabled := false
		protected.ChangeableAttributes.DeleteEnabled = &disabled
		locked.ChangeableAttributes.WriteEnabled = &disabled
		manifests := []acr.ManifestAttributesBase{protected, locked, overflow}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *overflow.Digest).Return(EmptyListManifestsResult, nil).Once()
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })

		agoDuration := mustParseDuration("1d")
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, 1, testLoginURL, testRepo, &agoDuration, 0, 1, 1, nil, true, false)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.NoError(err)
		assert.Equal(1, deletedManifests)
		assert.Equal(fmt.Sprintf("Would delete manifests for repository: %s\nWarning: Retaining locked manifest %s@sha256:locked (reason: maximum count)\nWould delete: %s/%s@sha256:overflow (reason: maximum count)\n", testRepo, testRepo, testLoginURL, testRepo), string(output))
		mockClient.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})

	t.Run("MaxSkipsServerBlockedManifestWithoutAggregateWarning", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		old := createManifestWithTime("sha256:old", time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339Nano))
		manifests := []acr.ManifestAttributesBase{old}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(&acr.Manifests{ManifestsAttributes: &manifests}, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", *old.Digest).Return(EmptyListManifestsResult, nil).Once()
		mockClient.On("DeleteManifest", mock.Anything, testRepo, *old.Digest).Return(&autorest.Response{
			Response: &http.Response{StatusCode: http.StatusMethodNotAllowed},
		}, errors.New("operation not allowed")).Once()
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if !assert.NoError(err) {
			t.FailNow()
		}
		os.Stdout = writer
		t.Cleanup(func() { os.Stdout = oldStdout; _ = reader.Close(); _ = writer.Close() })

		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, 1, testLoginURL, testRepo, nil, 0, -1, 0, nil, false, false)
		assert.NoError(writer.Close())
		os.Stdout = oldStdout
		output, readErr := io.ReadAll(reader)

		assert.NoError(readErr)
		assert.NoError(err)
		assert.Zero(deletedManifests)
		assert.NotContains(string(output), "not satisfied")
		mockClient.AssertExpectations(t)
	})
}

// TestDryRun contains the tests for the dryRunPurge method, it is called when the --dry-run flag is set.
func TestDryRun(t *testing.T) {
	// If repository is not know DryRun should not return an error, and there should not be any tags or manifest deleted.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		// Mock IsAbac to return false (non-ABAC registry) to use standard wildcard token flow
		mockClient.On("IsAbac").Return(false)
		// Need a .Maybe() since it's only called for ABAC registries (this test mocks IsAbac to return false)
		mockClient.On("IsTokenExpired").Return(false).Maybe()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(notFoundManifestResponse, errors.New("testRepo not found")).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Once()
		agoDuration := -24 * time.Hour
		deletedTags, deletedManifests, err := purge(testCtx, mockClient, testLoginURL, 60, &agoDuration, 0, -1, -1, -1, -1, 1, true, false, map[string]string{testRepo: "[\\s\\S]*"}, true, false, false)
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
		// "bar" is a literal repo name, so GetRepositories should not be called.
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
		// "ba" is a literal repo name, so GetRepositories should not be called.
		// The literal name "ba" is used directly without verifying against the registry.
		filters, err := repository.CollectTagFilters(testCtx, []string{"ba:.*"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Literal repo name should be passed through")
		assert.Equal(".*", filters["ba"], "Filter for ba repo should be .*")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NameWithSlash", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		// "foo/bar" is a literal repo name, so GetRepositories should not be called.
		filters, err := repository.CollectTagFilters(testCtx, []string{"foo/bar:.*"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Number of found repos should be one")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("NameWithSlashAndNonCaptureGroupInTag", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		// "foo/bar" is a literal repo name, so GetRepositories should not be called.
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
		// "bar" is a literal repo name, so GetRepositories should not be called.
		// The literal name is used directly without verifying against the registry.
		filters, err := repository.CollectTagFilters(testCtx, []string{testRepo + ":.*"}, mockClient, 60, defaultRepoPageSize)
		assert.Equal(1, len(filters), "Literal repo name should be passed through")
		assert.Equal(".*", filters[testRepo], "Filter for test repo should be .*")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("EmptyRepoRegex", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		// Parsing fails before any repo listing happens.
		_, err := repository.CollectTagFilters(testCtx, []string{":.*"}, mockClient, 60, defaultRepoPageSize)
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("EmptyTagRegex", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.BaseClientAPI{}
		// Parsing fails before any repo listing happens.
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*", 0, -1, -1, 60, false, true)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*", 0, -1, -1, 60, false, true)
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
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, false, true)
		assert.Equal(1, deletedManifests, "Number of deleted manifests should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test that without include-locked flag, locked tags are not deleted
	t.Run("NoIncludeLockedSkipLockedTag", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(DeleteDisabledOneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*", 0, -1, -1, 60, false, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*", 0, -1, -1, 60, false, true)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*", 0, -1, -1, 60, true, true)
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
		deletedManifests, err := purgeDanglingManifests(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, 0, -1, -1, nil, true, true)
		assert.Equal(1, deletedManifests, "Number of manifests to be deleted should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Test that dry-run without include-locked does not show locked tags
	t.Run("DryRunWithoutIncludeLockedSkipsLockedTags", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(DeleteDisabledOneTagResult, nil).Once()
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*", 0, -1, -1, 60, true, false)
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
		deletedTags, _, err := purgeTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, &defaultAgoDuration, ".*", 0, -1, -1, 60, true, true)
		assert.Equal(2, deletedTags, "Number of tags to be deleted should be 2 with include-locked")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}
