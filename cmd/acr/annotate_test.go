// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.
package main

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/go-autorest/autorest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestAnnotateTags contains all the tests regarding the annotateTags method which is called when the --dry-run flag
// is not set.
func TestAnnotateTags(t *testing.T) {

	// If there are no tags on a registry, no error should show and no other methods should be called. (line 71)
	t.Run("EmptyRepositoryTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(EmptyListTagsResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testRegex, defaultRegexpMatchTimeoutSeconds)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// There is only one tag but it does not match the regex filter so no other method should be called. (line 94)
	t.Run("NoAnnotationFilterTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, "^i.*", defaultRegexpMatchTimeoutSeconds)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Invalid regex filter, an error should be returned. (line 106)
	t.Run("InvalidRegexTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, "[", defaultRegexpMatchTimeoutSeconds)
		assert.Equal(-1, annotatedTags, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Invalid artifact-type, an error should be returned --> not really possible because it's just a string
	// t.Run("InvalidArtifactTypeTest", func(t *testing.T) {
	// 	assert := assert.New(t)
	// 	mockClient := &mocks.AcrCLIClientInterface{}
	// 	annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testRegex, defaultRegexpMatchTimeoutSeconds)
	// 	assert.Equal(-1, annotatedTags, "Number of deleted elements should be -1")
	// 	assert.NotEqual(nil, err, "Error should be nil")
	// 	mockClient.AssertExpectations(t)
	// })

	// Error calling getTagsToAnnotate, with the call to GetAcrTags not a 404, returns -1
	t.Run("GetTagsToAnnotateError", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(nil, errors.New("error fetching tags")).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testRegex, defaultRegexpMatchTimeoutSeconds)
		assert.Equal(-1, annotatedTags, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)

	})

	// If a tag should be annotated but the write-enabled attribute is set ot false, it should not be annotated and
	// no error should show on the CLI output. (line 162)
	t.Run("OperationNotAllowedTagWriteDisabledTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(WriteDisabledOneTagResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testRegex, defaultRegexpMatchTimeoutSeconds)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// NOTE: AnnotateAcrTag, based off of DeleteAcrTag which is in purgejob.go!!!

	// The following tests involve annotating tags. (line 183)
	// There is only one tag and it should be annotated. The AnnotateAcrTag method should be called once.
	t.Run("OneTagAnnotationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "latest").Return(&deletedResponse, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, "^la.*", defaultRegexpMatchTimeoutSeconds)
		assert.Equal(1, annotatedTags, "Number of annotated elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// All tags should be annotated, 5 tags in total, separated into two GetAcrTags calls. There should be
	// 5 AnnotateAcrTag calls.
	t.Run("FiveAnnotationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "latest").Return(FourTagsResult, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "latest").Return(&annotatedResponse, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "v1").Return(&annotatedResponse, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "v2").Return(&annotatedResponse, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "v3").Return(&annotatedResponse, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "v4").Return(&annotatedResponse, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testRegex, defaultRegexpMatchTimeoutSeconds)
		assert.Equal(5, annotatedTags, "Number of annotated elements should be 5")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// Annotate tag with "local" in it (line 23)
	t.Run("Annotate tag with local in it", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(TagWithLocal, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "v1-c-local.test").Return(&annotatedResponse, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, ".*-?local[.].+", defaultRegexpMatchTimeoutSeconds)
		assert.Equal(1, annotatedTags, "Number of annotated elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// There are 5 tags, but only 4 match the filter. There should be 4 AnnotateAcrTag calls.
	t.Run("FiveAnnotationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "latest").Return(FourTagsResult, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "v1").Return(&annotatedResponse, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "v2").Return(&annotatedResponse, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "v3").Return(&annotatedResponse, nil).Once()
		// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "v4").Return(&annotatedResponse, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, "^v.*", defaultRegexpMatchTimeoutSeconds)
		assert.Equal(4, annotatedTags, "Number of annotated elements should be 4")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// There are 5 tags and none match the filter. There are no AnnotateAcrTag calls.
	t.Run("NoAnnotationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "latest").Return(FourTagsResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, "^i.*", defaultRegexpMatchTimeoutSeconds)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is a 404 error while annotating a tag, an error should not be returned. (line 215)
	// t.Run("DeleteNotFoundErrorTest", func(t *testing.T) {
	// 	assert := assert.New(t)
	// 	mockClient := &mocks.AcrCLIClientInterface{}
	// 	mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
	// 	// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "latest").Return(&notFoundResponse, errors.New("not found")).Once()
	// 	annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, "^la.*", defaultRegexpMatchTimeoutSeconds)
	// 	assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
	// 	assert.Equal(nil, err, "Error should be nil")
	// 	mockClient.AssertExpectations(t)
	// })

	// // If an error (other than a 404 error) occurs during annotate, an error should be returned. (line 227)
	// t.Run("DeleteNotFoundErrorTest", func(t *testing.T) {
	// 	assert := assert.New(t)
	// 	mockClient := &mocks.AcrCLIClientInterface{}
	// 	mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
	// 	// mockClient.On("AnnotateAcrTag", mock.Anything, testRepo, "latest").Return(nil, errors.New("error during annotate")).Once()
	// 	annotatedTags, err := annotateTags(testCtx, mockClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, "^la.*", defaultRegexpMatchTimeoutSeconds)
	// 	assert.Equal(-1, annotatedTags, "Number of annotated elements should be -1")
	// 	assert.Equal(nil, err, "Error should be nil")
	// 	mockClient.AssertExpectations(t)
	// })

}

// Contains all the tests for the getTagstoAnnotate method
func TestGetTagstoAnnotate(t *testing.T) {
	tagRegex, _ := buildRegexFilter("[\\s\\S]*", defaultRegexpMatchTimeoutSeconds)

	// If an error (other than a 404 error) occurs while calling GetAcrTags, an error should be returned.
	t.Run("GetAcrTagsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(nil, errors.New("error fetching tags")).Once()
		// returns *[]acr.TagAttributesBase -> tagsToAnnotate, string -> newLastTag, int -> newSkippedTagsCount, error
		_, testLastTag, err := getTagsToAnnotate(testCtx, mockClient, testRepo, tagRegex, "")
		assert.Equal("", testLastTag, "Last tag should be empty")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If a 404 error occurs while calling GetAcrTags, an error should be returned.
	t.Run("GetAcrTags404Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Once()
		_, testLastTag, err := getTagsToAnnotate(testCtx, mockClient, testRepo, tagRegex, "")
		assert.Equal("", testLastTag, "Last tag should be empty")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// GetTagstoAnnotate returns no error
	t.Run("Success case", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(TagWithLocal, nil).Once()
		tagsToAnnotate, testLastTag, err := getTagsToAnnotate(testCtx, mockClient, testRepo, tagRegex, "")
		assert.Equal(4, len(*tagsToAnnotate), "Number of tags to annotate should be 1")
		assert.Equal("", testLastTag, "Last tag should be empty")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

}

// All the variables used in the tests are defined here
var (
	testRegex        = "[\\s\\S]"
	testArtifactType = "application/vnd.microsoft.artifact.lifecycle"

	annotatedResponse = autorest.Response{
		Response: &http.Response{
			StatusCode: 200,
		},
	}
)
