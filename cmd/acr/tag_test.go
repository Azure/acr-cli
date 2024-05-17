// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"errors"
	"testing"

	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/stretchr/testify/assert"
)

func TestListTags(t *testing.T) {
	// First test, repository not found should return an error.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Once()
		tagList, err := listTags(testCtx, mockClient, testRepo)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.Equal(0, len(tagList), "Tag list should be empty")
		mockClient.AssertExpectations(t)
	})
	// Second test, if an error is returned on any GetAcrTags call an error should be returned.
	t.Run("ErrorOnSecondPageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "latest").Return(nil, errors.New("unauthorized")).Once()
		tagList, err := listTags(testCtx, mockClient, testRepo)
		assert.Equal(0, len(tagList), "Tag list should be empty")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Third test, no errors
	t.Run("ListFiveTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "latest").Return(FourTagsResult, nil).Once()
		mockClient.On("GetAcrTags", testCtx, testRepo, "", "v4").Return(EmptyListTagsResult, nil).Once()
		tagList, err := listTags(testCtx, mockClient, testRepo)
		assert.Equal(nil, err, "Error should be nil")
		assert.Equal(5, len(tagList), "Tag list should have 5 tags")
		mockClient.AssertExpectations(t)
	})
}

func TestDeleteTags(t *testing.T) {
	args := []string{"latest", "v1", "v2", "v3", "v4"}
	// First test, tag not found should return an error.
	t.Run("TagNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "latest").Return(&notFoundResponse, errors.New("not found")).Once()
		err := deleteTags(testCtx, mockClient, testLoginURL, testRepo, args)
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test, five tags deleted, regular behavior
	t.Run("DeleteFiveTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "latest").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "v1").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "v2").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "v3").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testCtx, testRepo, "v4").Return(&deletedResponse, nil).Once()
		err := deleteTags(testCtx, mockClient, testLoginURL, testRepo, args)
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}
