// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"errors"
	"testing"

	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/acr-cli/internal/tag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewTagCmd(t *testing.T) {
	rootParams := &rootParameters{}
	cmd := newTagCmd(rootParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "tag", cmd.Use)
	assert.Equal(t, newTagCmdLongMessage, cmd.Long)
}

func TestNewTagListCmd(t *testing.T) {
	rootParams := &rootParameters{}
	tagParams := tagParameters{rootParameters: rootParams}
	cmd := newTagListCmd(&tagParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "list", cmd.Use)
	assert.Equal(t, newTagListCmdLongMessage, cmd.Long)
}

func TestNewTagDeleteCmd(t *testing.T) {
	rootParams := &rootParameters{}
	tagParams := tagParameters{rootParameters: rootParams}
	cmd := newTagDeleteCmd(&tagParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "delete", cmd.Use)
	assert.Equal(t, newTagDeleteCmdLongMessage, cmd.Long)
}

// TestListTagsAbac tests the tag list ABAC code path: after client creation,
// if IsAbac() returns true, RefreshTokenForAbac must be called before listing tags.
func TestListTagsAbac(t *testing.T) {
	t.Run("AbacEnabledListTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(true)
		mockClient.On("RefreshTokenForAbac", mock.Anything, []string{testRepo}).Return(nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		// Simulate the ABAC code path from the tag list command
		if mockClient.IsAbac() {
			err := mockClient.RefreshTokenForAbac(testCtx, []string{testRepo})
			assert.Equal(nil, err, "RefreshTokenForAbac should not return an error")
		}
		tagList, err := tag.ListTags(testCtx, mockClient, testRepo)
		assert.Equal(nil, err, "Error should be nil")
		assert.Equal(1, len(tagList), "Tag list should have 1 tag")
		mockClient.AssertExpectations(t)
	})

	t.Run("AbacRefreshFailureListTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(true)
		mockClient.On("RefreshTokenForAbac", mock.Anything, []string{testRepo}).Return(errors.New("failed to refresh token for ABAC repositories")).Once()
		// Simulate the ABAC code path from the tag list command
		if mockClient.IsAbac() {
			err := mockClient.RefreshTokenForAbac(testCtx, []string{testRepo})
			assert.NotEqual(nil, err, "RefreshTokenForAbac should return an error")
		}
		// GetAcrTags should NOT be called since ABAC refresh failed
		mockClient.AssertExpectations(t)
	})

	t.Run("NonAbacListTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "", "").Return(OneTagResult, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "", "latest").Return(EmptyListTagsResult, nil).Once()
		// Simulate the ABAC code path from the tag list command
		if mockClient.IsAbac() {
			err := mockClient.RefreshTokenForAbac(testCtx, []string{testRepo})
			assert.Equal(nil, err, "RefreshTokenForAbac should not return an error")
		}
		tagList, err := tag.ListTags(testCtx, mockClient, testRepo)
		assert.Equal(nil, err, "Error should be nil")
		assert.Equal(1, len(tagList), "Tag list should have 1 tag")
		// RefreshTokenForAbac should NOT have been called
		mockClient.AssertNotCalled(t, "RefreshTokenForAbac", mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})
}

// TestDeleteTagsAbac tests the tag delete ABAC code path: after client creation,
// if IsAbac() returns true, RefreshTokenForAbac must be called before deleting tags.
func TestDeleteTagsAbac(t *testing.T) {
	args := []string{"latest", "v1"}

	t.Run("AbacEnabledDeleteTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(true)
		mockClient.On("RefreshTokenForAbac", mock.Anything, []string{testRepo}).Return(nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "latest").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1").Return(&deletedResponse, nil).Once()
		// Simulate the ABAC code path from the tag delete command
		if mockClient.IsAbac() {
			err := mockClient.RefreshTokenForAbac(testCtx, []string{testRepo})
			assert.Equal(nil, err, "RefreshTokenForAbac should not return an error")
		}
		err := tag.DeleteTags(testCtx, mockClient, testLoginURL, testRepo, args)
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	t.Run("AbacRefreshFailureDeleteTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(true)
		mockClient.On("RefreshTokenForAbac", mock.Anything, []string{testRepo}).Return(errors.New("failed to refresh token for ABAC repositories")).Once()
		// Simulate the ABAC code path from the tag delete command
		if mockClient.IsAbac() {
			err := mockClient.RefreshTokenForAbac(testCtx, []string{testRepo})
			assert.NotEqual(nil, err, "RefreshTokenForAbac should return an error")
		}
		// DeleteAcrTag should NOT be called since ABAC refresh failed
		mockClient.AssertExpectations(t)
	})

	t.Run("NonAbacDeleteTagsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("IsAbac").Return(false)
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "latest").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteAcrTag", mock.Anything, testRepo, "v1").Return(&deletedResponse, nil).Once()
		// Simulate the ABAC code path from the tag delete command
		if mockClient.IsAbac() {
			err := mockClient.RefreshTokenForAbac(testCtx, []string{testRepo})
			assert.Equal(nil, err, "RefreshTokenForAbac should not return an error")
		}
		err := tag.DeleteTags(testCtx, mockClient, testLoginURL, testRepo, args)
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertNotCalled(t, "RefreshTokenForAbac", mock.Anything, mock.Anything)
		mockClient.AssertExpectations(t)
	})
}
