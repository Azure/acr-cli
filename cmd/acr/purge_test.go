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

func TestPurgeTags(t *testing.T) {
	// First test if repository is not known PurgeTags should only call GetAcrTags and return no error.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(notFoundTagResponse, errors.New("repo not found")).Once()
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "1d", "[\\s\\S]*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test, if there are no tags on a registry no error should show and no other methods should be called.
	t.Run("EmptyRepositoryTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(EmptyListTagsResult, nil).Once()
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "1d", "[\\s\\S]*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Third test there is only one tag and it should not be deleted (according to the ago flag), GetAcrTags should be called twice
	// and no other methods should be called.
	t.Run("NoDeletionAgoTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "1d", "[\\s\\S]*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fourth test there is only one tag and it should be deleted according to the ago flag but it does not match a regex filter
	// so no other method should be called
	t.Run("NoDeletionFilterTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "0m", "^hello.*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Sixth test, invalid regex filter, an error should be returned.
	t.Run("InvalidRegexTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "0m", "[")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Seventh test, if a passed duration is invalid an error should be returned.
	t.Run("InvalidDurationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "0e", "^la.*")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Eighth test, if there is an error during a call to GetAcrTags (other than a 404) an error should be returned.
	t.Run("GetAcrTagsErrorSinglePageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(nil, errors.New("unauthorized")).Once()
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "1d", "[\\s\\S]*")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Ninth test, if there is an error during a call to GetAcrTags (other than a 404) an error should be returned.
	// similar to the previous test but the error occurs not on the first GetAcrTags call.
	t.Run("GetAcrTagsErrorMultiplePageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(nil, errors.New("unauthorized")).Once()
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "1d", "[\\s\\S]*")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Tenth test, if a tag should be deleted but the delete enabled attribute is set to true it should not be deleted
	// and no error should show on the CLI output.
	t.Run("OperationNotAllowedTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(DeleteDisabledOneTagResult, nil).Once()
		mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "0m", "^la.*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Eleventh test, if a tag has an invalid last update time attribute an error should be returned.
	t.Run("InvalidDurationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(InvalidDateOneTagResult, nil).Once()
		deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "0m", "^la.*")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// The following tests involve deleting tags.
	// Twelfth test, there is only one tag and it should be deleted, the DeleteAcrTag method should be called once.
	t.Run("OneTagDeletionTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(ctx, &wg, &mockClient, 6)
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("DeleteAcrTag", ctx, repo, "latest").Return(&deletedResponse, nil).Once()
		deletedTags, err := PurgeTags(ctx, &mockClient, loginURL, repo, "0m", "^la.*")
		worker.StopDispatcher()
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Thirteenth test, all tags should be deleted, 5 tags in total, separated into two GetAcrTags calls, there should be
	// 5 DeleteAcrTag calls.
	t.Run("FiveTagDeletionTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(ctx, &wg, &mockClient, 6)
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(FourTagsResult, nil).Once()
		mockClient.On("GetAcrTags", ctx, repo, "", "v4").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("DeleteAcrTag", ctx, repo, "latest").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", ctx, repo, "v1").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", ctx, repo, "v2").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", ctx, repo, "v3").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", ctx, repo, "v4").Return(&deletedResponse, nil).Once()
		deletedTags, err := PurgeTags(ctx, &mockClient, loginURL, repo, "0m", "[\\s\\S]*")
		worker.StopDispatcher()
		assert.Equal(5, deletedTags, "Number of deleted elements should be 5")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fourteenth, if an there is a 404 error while deleting a tag an error should not be returned.
	t.Run("DeleteNotFoundErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(ctx, &wg, &mockClient, 6)
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(nil, nil).Once()
		mockClient.On("DeleteAcrTag", ctx, repo, "latest").Return(&notFoundResponse, errors.New("not found")).Once()
		deletedTags, err := PurgeTags(ctx, &mockClient, loginURL, repo, "0m", "^la.*")
		worker.StopDispatcher()
		// If it is not found it can be assumed deleted.
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fifteenth, if an error (other than a 404 error) occurs during delete, an error should be returned.
	t.Run("DeleteErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(ctx, &wg, &mockClient, 6)
		mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("DeleteAcrTag", ctx, repo, "latest").Return(nil, errors.New("error during delete")).Once()
		deletedTags, err := PurgeTags(ctx, &mockClient, loginURL, repo, "0m", "^la.*")
		worker.StopDispatcher()
		assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
}

func TestGetRepositoryAndTagRegex(t *testing.T) {
	// First test normal functionality
	t.Run("NormalFunctionalityTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo:bar"
		repository, filter, err := GetRepositoryAndTagRegex(testString)
		assert.Equal("foo", repository)
		assert.Equal("bar", filter)
		assert.Equal(nil, err, "Error should be nil")
	})
	// Second test no colon
	t.Run("NoColonTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo"
		repository, filter, err := GetRepositoryAndTagRegex(testString)
		assert.Equal("", repository)
		assert.Equal("", filter)
		assert.NotEqual(nil, err, "Error should not be nil")
	})
	// Third test more than one colon
	t.Run("TwoColonsTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo:bar:zzz"
		repository, filter, err := GetRepositoryAndTagRegex(testString)
		assert.Equal("", repository)
		assert.Equal("", filter)
		assert.NotEqual(nil, err, "Error should not be nil")
	})
}

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
		durationResult, errorResult := ParseDuration(table.durationString)
		assert.Equal(table.duration, durationResult)
		assert.Equal(table.err, errorResult)
	}
}

var (
	ctx              = context.Background()
	loginURL         = "foo.azurecr.io"
	repo             = "bar"
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
	notFoundTagResponse = &acr.RepositoryTagsType{
		Response: notFoundResponse,
	}
	EmptyListTagsResult = &acr.RepositoryTagsType{
		Registry:       &loginURL,
		ImageName:      &repo,
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
		Registry:  &loginURL,
		ImageName: &repo,
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
		Registry:  &loginURL,
		ImageName: &repo,
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
		Registry:  &loginURL,
		ImageName: &repo,
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
		Registry:  &loginURL,
		ImageName: &repo,
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
)
