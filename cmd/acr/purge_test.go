// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/acr-cli/cmd/worker"
	"github.com/Azure/go-autorest/autorest"
	"github.com/stretchr/testify/assert"
)

var ctx = context.TODO()

func TestDelete(t *testing.T) {
	var k key = "test"
	loginURL := "foo.azurecr.io"
	repo := "bar"
	deletedResponse := autorest.Response{
		Response: &http.Response{
			StatusCode: 200,
		},
	}
	EmptyListTagsResult := &acr.RepositoryTagsType{
		Registry:       &loginURL,
		ImageName:      &repo,
		TagsAttributes: nil,
	}
	tagName := "latest"
	digest := "sha:abc"
	multiArchDigest := "sha:356"
	deleteEnabled := true
	lastUpdateTime := time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano) //Creation time -15minutes from current time

	OneTagResult := &acr.RepositoryTagsType{
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
	tagName1 := "v1"
	tagName2 := "v2"
	tagName3 := "v3"
	tagName4 := "v4"

	FourTagsResult := &acr.RepositoryTagsType{
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
	ctx = context.WithValue(ctx, k, "Delete Test")

	t.Run("first", func(t *testing.T) {
		assert := assert.New(t)
		mockClientDeleteOneTag := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(ctx, &wg, &mockClientDeleteOneTag, 6)
		mockClientDeleteOneTag.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
		mockClientDeleteOneTag.On("GetAcrTags", ctx, repo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		mockClientDeleteOneTag.On("DeleteAcrTag", ctx, repo, "latest").Return(&deletedResponse, nil).Once()
		deletedTags, err := PurgeTags(ctx, &mockClientDeleteOneTag, loginURL, repo, "0m", "^la.*")
		assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClientDeleteOneTag.AssertExpectations(t)
	})
	t.Run("Second", func(t *testing.T) {
		assert := assert.New(t)
		mockClientDeleteFourTags := mocks.AcrCLIClientInterface{}
		worker.StartDispatcher(ctx, &wg, &mockClientDeleteFourTags, 6)
		mockClientDeleteFourTags.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
		mockClientDeleteFourTags.On("GetAcrTags", ctx, repo, "", "latest").Return(FourTagsResult, nil).Once()
		mockClientDeleteFourTags.On("GetAcrTags", ctx, repo, "", "v4").Return(EmptyListTagsResult, nil).Once()
		mockClientDeleteFourTags.On("DeleteAcrTag", ctx, repo, "latest").Return(&deletedResponse, nil).Once()
		mockClientDeleteFourTags.On("DeleteAcrTag", ctx, repo, "v1").Return(&deletedResponse, nil).Once()
		mockClientDeleteFourTags.On("DeleteAcrTag", ctx, repo, "v2").Return(&deletedResponse, nil).Once()
		mockClientDeleteFourTags.On("DeleteAcrTag", ctx, repo, "v3").Return(&deletedResponse, nil).Once()
		mockClientDeleteFourTags.On("DeleteAcrTag", ctx, repo, "v4").Return(&deletedResponse, nil).Once()
		deletedTags, err := PurgeTags(ctx, &mockClientDeleteFourTags, loginURL, repo, "0m", "[\\s\\S]*")
		assert.Equal(5, deletedTags, "Number of deleted elements should be 5")
		assert.Equal(nil, err, "Error should be nil")
		mockClientDeleteFourTags.AssertExpectations(t)
	})
}

// func TestThreeDeletes(t *testing.T) {
// 	ctx := context.TODO()
// 	var k key = "test"
// 	loginURL := "foo.azurecr.io"
// 	repo := "bar"
// 	notFoundResponse := autorest.Response{
// 		Response: &http.Response{
// 			StatusCode: 404,
// 		},
// 	}
// 	tagName := "latest"
// 	digest := "sha:abc"
// 	deleteEnabled := true
// 	lastUpdateTime := time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano) //Creation time -15minutes from current time

// 	OneTagResult := &acr.RepositoryTagsType{
// 		Registry:  &loginURL,
// 		ImageName: &repo,
// 		TagsAttributes: &[]acr.TagAttributesBase{
// 			{
// 				Name:                 &tagName,
// 				LastUpdateTime:       &lastUpdateTime,
// 				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
// 				Digest:               &digest,
// 			},
// 		},
// 	}
// 	assert := assert.New(t)
// 	ctx = context.WithValue(ctx, k, "3")
// 	mockClient := mocks.AcrCLIClientInterface{}
// 	worker.StartDispatcher(ctx, &wg, &mockClient, 6)
// 	mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
// 	mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(nil, nil).Once()
// 	mockClient.On("DeleteAcrTag", ctx, repo, "latest").Return(&notFoundResponse, errors.New("not found")).Once()
// 	deletedTags, err := PurgeTags(ctx, &mockClient, loginURL, repo, "0m", "^la.*")
// 	// If it is not found it can be assumed deleted.
// 	assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
// 	assert.Equal(nil, err, "Error should be nil")
// 	mockClient.AssertExpectations(t)
// }
// func TestFourDeletes(t *testing.T) {
// 	ctx := context.TODO()
// 	var k key = "test"
// 	loginURL := "foo.azurecr.io"
// 	repo := "bar"
// 	tagName := "latest"
// 	digest := "sha:abc"
// 	deleteEnabled := true
// 	lastUpdateTime := time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano) //Creation time -15minutes from current time

// 	OneTagResult := &acr.RepositoryTagsType{
// 		Registry:  &loginURL,
// 		ImageName: &repo,
// 		TagsAttributes: &[]acr.TagAttributesBase{
// 			{
// 				Name:                 &tagName,
// 				LastUpdateTime:       &lastUpdateTime,
// 				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
// 				Digest:               &digest,
// 			},
// 		},
// 	}

// 	ctx = context.WithValue(ctx, k, "4")
// 	assert := assert.New(t)
// 	mockClient := mocks.AcrCLIClientInterface{}
// 	worker.StartDispatcher(ctx, &wg, &mockClient, 6)
// 	mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
// 	mockClient.On("DeleteAcrTag", ctx, repo, "latest").Return(nil, errors.New("error during delete")).Once()
// 	deletedTags, err := PurgeTags(ctx, &mockClient, loginURL, repo, "0m", "^la.*")
// 	assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
// 	assert.NotEqual(nil, err, "Error should not be nil")
// 	mockClient.AssertExpectations(t)
// }

type key string
