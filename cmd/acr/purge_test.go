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

func TestPurgetags(t *testing.T) {
	// Setup for all the mocked responses from the ACR API
	loginURL := "foo.azurecr.io"
	repo := "bar"
	notFoundTagResponse := &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 404,
			},
		},
	}
	EmptyListTagsResult := &acr.RepositoryTagsType{
		Registry:       &loginURL,
		ImageName:      &repo,
		TagsAttributes: nil,
	}
	lastUpdateTime := time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano) //Creation time -15minutes from current time
	tagName := "latest"
	digest := "sha"
	deleteEnabled := true
	deleteDisabled := false
	tag := acr.TagAttributesBase{
		Name:                 &tagName,
		LastUpdateTime:       &lastUpdateTime,
		ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
		Digest:               &digest,
	}
	singleTag := []acr.TagAttributesBase{tag}
	OneTagResult := &acr.RepositoryTagsType{
		Registry:       &loginURL,
		ImageName:      &repo,
		TagsAttributes: &singleTag,
	}
	tagName1 := "v1"
	tag1 := acr.TagAttributesBase{
		Name:                 &tagName1,
		LastUpdateTime:       &lastUpdateTime,
		ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
		Digest:               &digest,
	}
	multipleTags := []acr.TagAttributesBase{tag1}
	tagName2 := "v2"
	tag2 := acr.TagAttributesBase{
		Name:                 &tagName2,
		LastUpdateTime:       &lastUpdateTime,
		ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
		Digest:               &digest,
	}
	multipleTags = append(multipleTags, tag2)
	tagName3 := "v3"
	tag3 := acr.TagAttributesBase{
		Name:                 &tagName3,
		LastUpdateTime:       &lastUpdateTime,
		ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
		Digest:               &digest,
	}
	multipleTags = append(multipleTags, tag3)
	tagName4 := "v4"
	tag4 := acr.TagAttributesBase{
		Name:                 &tagName4,
		LastUpdateTime:       &lastUpdateTime,
		ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
		Digest:               &digest,
	}
	multipleTags = append(multipleTags, tag4)
	FourTagsResult := &acr.RepositoryTagsType{
		Registry:       &loginURL,
		ImageName:      &repo,
		TagsAttributes: &multipleTags,
	}
	// First test if repository is not known PurgeTags should only call GetAcrTags and return no error
	t.Run("RepositoryNotFound", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", context.Background(), repo, "", "").Return(notFoundTagResponse, errors.New("repo not found")).Once()
		deletedTags, err := PurgeTags(context.Background(), mockClient, loginURL, repo, "1d", "[\\s\\S]*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test, if there are no tags on a registry no error should show and no other methods should be called.
	t.Run("EmptyRepository", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", context.Background(), repo, "", "").Return(EmptyListTagsResult, nil).Once()
		deletedTags, err := PurgeTags(context.Background(), mockClient, loginURL, repo, "1d", "[\\s\\S]*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Third test only one tag and it should not be delete (ago flag), GetAcrTags should be called twice and no other methods should be called
	t.Run("NoDeletionAgo", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", context.Background(), repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", context.Background(), repo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		deletedTags, err := PurgeTags(context.Background(), mockClient, loginURL, repo, "1d", "[\\s\\S]*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fourth test only one tag and it should be deleted according to ago flag but it does not match a regex filter
	t.Run("NoDeletionFilter", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", context.Background(), repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", context.Background(), repo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		deletedTags, err := PurgeTags(context.Background(), mockClient, loginURL, repo, "0m", "^hello.*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Fifth test, only one tag it should be deleted
	t.Run("OneTagDeletion", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(&wg, mockClient, 6)
		mockClient.On("GetAcrTags", context.Background(), repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", context.Background(), repo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("DeleteAcrTag", context.Background(), repo, "latest").Return(nil, nil).Once()
		deletedTags, err := PurgeTags(context.Background(), mockClient, loginURL, repo, "0m", "^la.*")
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Sixth test, all tags should be deleted, 5 tags in total, separated into two GetAcrTags calls.
	t.Run("FiveTagDeletion", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(&wg, mockClient, 6)
		mockClient.On("GetAcrTags", context.Background(), repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", context.Background(), repo, "", "latest").Return(FourTagsResult, nil).Once()
		mockClient.On("GetAcrTags", context.Background(), repo, "", "v4").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("DeleteAcrTag", context.Background(), repo, "latest").Return(nil, nil).Once()
		mockClient.On("DeleteAcrTag", context.Background(), repo, "v1").Return(nil, nil).Once()
		mockClient.On("DeleteAcrTag", context.Background(), repo, "v2").Return(nil, nil).Once()
		mockClient.On("DeleteAcrTag", context.Background(), repo, "v3").Return(nil, nil).Once()
		mockClient.On("DeleteAcrTag", context.Background(), repo, "v4").Return(nil, nil).Once()
		deletedTags, err := PurgeTags(context.Background(), mockClient, loginURL, repo, "0m", "[\\s\\S]*")
		assert.Equal(5, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Seventh test, invalid regex filter
	t.Run("InvalidRegex", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		deletedTags, err := PurgeTags(context.Background(), mockClient, loginURL, repo, "0m", "[")
		assert.Equal(-1, deletedTags, "Number of deleted elements should be 1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
	// Eight test, operation not allowed, no tag should be deleted
	(*(*(OneTagResult.TagsAttributes))[0].ChangeableAttributes).DeleteEnabled = &deleteDisabled
	t.Run("OperationNotAllowed", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(&wg, mockClient, 6)
		mockClient.On("GetAcrTags", context.Background(), repo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", context.Background(), repo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		deletedTags, err := PurgeTags(context.Background(), mockClient, loginURL, repo, "0m", "^la.*")
		assert.Equal(0, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

//TODO add manifest test
//TODO add dry-run test
//TODO add simple joint test

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
	for _, table := range tables {
		durationResult, errorResult := ParseDuration(table.durationString)
		if durationResult != table.duration {
			t.Fatalf("ParseDuration of %s incorrect, expected %v got %v", table.durationString, table.duration, durationResult)
		}
		if errorResult != table.err {
			if errorResult.Error() != table.err.Error() {
				t.Fatalf("ParseDuration of %s incorrect, expected %v got %v", table.durationString, table.err, errorResult)
			}
		}
	}
}
