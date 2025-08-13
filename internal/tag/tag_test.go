// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package tag

import (
	"errors"
	"testing"

	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/acr-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func TestListTags(t *testing.T) {
	// First test, repository not found should return an error.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testutil.TestCtx, testutil.TestRepo, "", "").Return(testutil.NotFoundTagResponse, errors.New("testRepo not found")).Once()
		tagList, err := ListTags(testutil.TestCtx, mockClient, testutil.TestRepo)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.Equal(0, len(tagList), "Tag list should be empty")
		mockClient.AssertExpectations(t)
	})
	// Second test, if an error is returned on any GetAcrTags call an error should be returned.
	t.Run("ErrorOnSecondPageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testutil.TestCtx, testutil.TestRepo, "", "").Return(testutil.OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", testutil.TestCtx, testutil.TestRepo, "", "latest").Return(nil, errors.New("unauthorized")).Once()
		tagList, err := ListTags(testutil.TestCtx, mockClient, testutil.TestRepo)
		assert.Equal(0, len(tagList), "Tag list should be empty")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Third test, no errors
	t.Run("ListFiveTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", testutil.TestCtx, testutil.TestRepo, "", "").Return(testutil.OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", testutil.TestCtx, testutil.TestRepo, "", "latest").Return(testutil.FourTagsResult, nil).Once()
		mockClient.On("GetAcrTags", testutil.TestCtx, testutil.TestRepo, "", testutil.TagName4).Return(testutil.EmptyListTagsResult, nil).Once()
		tagList, err := ListTags(testutil.TestCtx, mockClient, testutil.TestRepo)
		assert.Equal(nil, err, "Error should be nil")
		assert.Equal(5, len(tagList), "Tag list should have 5 tags")
		mockClient.AssertExpectations(t)
	})
}

func TestDeleteTags(t *testing.T) {
	args := []string{testutil.TagName, "v1", "v2", "v3", "v4"}
	// First test, tag not found should return an error.
	t.Run("TagNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("DeleteAcrTag", testutil.TestCtx, testutil.TestRepo, "latest").Return(&testutil.NotFoundResponse, errors.New("not found")).Once()
		err := DeleteTags(testutil.TestCtx, mockClient, testutil.TestLoginURL, testutil.TestRepo, args)
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test, five tags deleted, regular behavior
	t.Run("DeleteFiveTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("DeleteAcrTag", testutil.TestCtx, testutil.TestRepo, "latest").Return(&testutil.DeletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testutil.TestCtx, testutil.TestRepo, "v1").Return(&testutil.DeletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testutil.TestCtx, testutil.TestRepo, "v2").Return(&testutil.DeletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testutil.TestCtx, testutil.TestRepo, "v3").Return(&testutil.DeletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", testutil.TestCtx, testutil.TestRepo, "v4").Return(&testutil.DeletedResponse, nil).Once()
		err := DeleteTags(testutil.TestCtx, mockClient, testutil.TestLoginURL, testutil.TestRepo, args)
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}
