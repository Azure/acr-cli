// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.
package main

import (
	"errors"
	"testing"

	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Contains all the tests for the getTagstoAnnotate method
func TestGetTagstoAnnotate(t *testing.T) {
	tagRegex, _ := buildRegexFilter("[\\s\\S]*", defaultRegexpMatchTimeoutSeconds)

	// If an error (other than a 404 error) occurs while calling GetAcrTags, an error should be returned.
	t.Run("GetAcrTagsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(nil, errors.New("error fetching tags")).Once()
		// returns *[]acr.TagAttributesBase -> tagsToAnnotate, string -> newLastTag, int -> newSkippedTagsCount, error
		_, err := getTagsToAnnotate(testCtx, mockClient, testRepo, tagRegex, "")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If a 404 error occurs while calling GetAcrTags, an error should be returned.
	t.Run("GetAcrTags404Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Once()
		_, err := getTagsToAnnotate(testCtx, mockClient, testRepo, tagRegex, "")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// GetTagstoAnnotate returns no error
	t.Run("Success case", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(TagWithLocal, nil).Once()
		tagsToAnnotate, err := getTagsToAnnotate(testCtx, mockClient, testRepo, tagRegex, "")
		assert.Equal(4, len(*tagsToAnnotate), "Number of tags to annotate should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

}

// All the variables used in the tests are defined here
var ()
