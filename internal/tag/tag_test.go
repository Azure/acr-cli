// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package tag

import (
	"errors"
	"testing"

	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/acr-cli/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestListTags(t *testing.T) {
	// First test, repository not found should return an error.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", common.TestCtx, common.TestRepo, "", "").Return(common.NotFoundTagResponse, errors.New("testRepo not found")).Once()
		tagList, err := ListTags(common.TestCtx, mockClient, common.TestRepo)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.Equal(0, len(tagList), "Tag list should be empty")
		mockClient.AssertExpectations(t)
	})
	// Second test, if an error is returned on any GetAcrTags call an error should be returned.
	t.Run("ErrorOnSecondPageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", common.TestCtx, common.TestRepo, "", "").Return(common.OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", common.TestCtx, common.TestRepo, "", "latest").Return(nil, errors.New("unauthorized")).Once()
		tagList, err := ListTags(common.TestCtx, mockClient, common.TestRepo)
		assert.Equal(0, len(tagList), "Tag list should be empty")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Third test, no errors
	t.Run("ListFiveTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", common.TestCtx, common.TestRepo, "", "").Return(common.OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", common.TestCtx, common.TestRepo, "", "latest").Return(common.FourTagsResult, nil).Once()
		mockClient.On("GetAcrTags", common.TestCtx, common.TestRepo, "", "v4").Return(common.EmptyListTagsResult, nil).Once()
		tagList, err := ListTags(common.TestCtx, mockClient, common.TestRepo)
		assert.Equal(nil, err, "Error should be nil")
		assert.Equal(5, len(tagList), "Tag list should have 5 tags")
		mockClient.AssertExpectations(t)
	})
}

func TestDeleteTags(t *testing.T) {
	args := []string{common.TagName, common.TagName1, common.TagName2, common.TagName3, common.TagName4}
	// First test, tag not found should return an error.
	t.Run("TagNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("DeleteAcrTag", common.TestCtx, common.TestRepo, "latest").Return(&common.NotFoundResponse, errors.New("not found")).Once()
		err := DeleteTags(common.TestCtx, mockClient, common.TestLoginURL, common.TestRepo, args)
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test, five tags deleted, regular behavior
	t.Run("DeleteFiveTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("DeleteAcrTag", common.TestCtx, common.TestRepo, "latest").Return(&common.DeletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", common.TestCtx, common.TestRepo, "v1").Return(&common.DeletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", common.TestCtx, common.TestRepo, "v2").Return(&common.DeletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", common.TestCtx, common.TestRepo, "v3").Return(&common.DeletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", common.TestCtx, common.TestRepo, "v4").Return(&common.DeletedResponse, nil).Once()
		err := DeleteTags(common.TestCtx, mockClient, common.TestLoginURL, common.TestRepo, args)
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}
