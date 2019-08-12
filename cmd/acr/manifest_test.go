// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"errors"
	"testing"

	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/stretchr/testify/assert"
)

func TestListManifests(t *testing.T) {
	// First test, repository not found should return an error.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(notFoundManifestResponse, errors.New("testRepo not found")).Once()
		err := listManifests(testCtx, mockClient, testLoginURL, testRepo)
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test, if an error is returned on any GetAcrTags call an error should be returned.
	t.Run("ErrorOnSecondPageTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:abc").Return(nil, errors.New("unauthorized")).Once()
		err := listManifests(testCtx, mockClient, testLoginURL, testRepo)
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Third test, no errors
	t.Run("ListThreeManifestsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:abc").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", testCtx, testRepo, "", "sha:234").Return(EmptyListManifestsResult, nil).Once()
		err := listManifests(testCtx, mockClient, testLoginURL, testRepo)
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

func TestDeleteManifests(t *testing.T) {
	args := []string{"sha:123", "sha:124", "sha:125"}
	// First test, manifest not found should return an error.
	t.Run("ManifestNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:123").Return(&notFoundResponse, errors.New("not found")).Once()
		err := deleteManifests(testCtx, mockClient, testLoginURL, testRepo, args)
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// Second test, three manifests deleted, regular behavior
	t.Run("DeleteFiveManifestsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:123").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:124").Return(&deletedResponse, nil).Once()
		mockClient.On("DeleteManifest", testCtx, testRepo, "sha:125").Return(&deletedResponse, nil).Once()
		err := deleteManifests(testCtx, mockClient, testLoginURL, testRepo, args)
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}
