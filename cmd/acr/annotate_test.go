// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.
package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/acr-cli/cmd/common"
	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestAnnotateTags contains all the tests regarding the annotateTags method which is called when the --dry-run flag
// is not set.
func TestAnnotateTags(t *testing.T) {

	// If there are no tags on a registry, no error should show and no other methods should be called.
	t.Run("EmptyRepositoryTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(EmptyListTagsResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], testRegex, defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// There is only one tag but it does not match the regex filter so no other method should be called.
	t.Run("NoAnnotationFilterTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "^i.*", defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// Invalid regex filter, an error should be returned.
	t.Run("InvalidRegexTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "[", defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(-1, annotatedTags, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// Error calling getTagsToAnnotate, with the call to GetAcrTags not a 404, returns -1
	t.Run("GetTagsToAnnotateError", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(nil, errors.New("error fetching tags")).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], testRegex, defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(-1, annotatedTags, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)

	})

	// If a tag should be annotated but the write-enabled attribute is set ot false, it should not be annotated and
	// no error should show on the CLI output.
	t.Run("OperationNotAllowedTagWriteDisabledTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(WriteDisabledOneTagResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], testRegex, defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// Error annotating due to annotation not being formatted correctly
	t.Run("BadAnnotationFormat", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testBadAnnotations[:], testRegex, defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(-1, annotatedTags, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// The following tests involve annotating tags.
	// There is only one tag and it should be annotated. The Annotate method should be called once.
	t.Run("OneTagAnnotationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		ref := fmt.Sprintf("%s/%s:latest", testLoginURL, testRepo)
		digestRef := fmt.Sprintf("%s/%s@%s", testLoginURL, testRepo, digest)
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResult, nil).Once()
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "^la.*", defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(1, annotatedTags, "Number of annotated elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// All tags should be annotated, 5 tags in total, separated into two GetAcrTags calls. There should be
	// 5 Annotate calls.
	t.Run("FiveAnnotationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "latest").Return(FourTagsResult, nil).Once()

		ref := fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName1)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName2)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName3)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName4)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()

		digestRef := fmt.Sprintf("%s/%s@%s", testLoginURL, testRepo, digest)
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()
		digestRef = fmt.Sprintf("%s/%s@%s", testLoginURL, testRepo, multiArchDigest)
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()
		digestRef = fmt.Sprintf("%s/%s@%s", testLoginURL, testRepo, digest)
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()

		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], testRegex, defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(5, annotatedTags, "Number of annotated elements should be 5")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// Annotate tag with "local" in it
	t.Run("Annotate tag with local in it", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(TagWithLocal, nil).Once()
		ref := fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagNameWithLoad)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		digestRef := fmt.Sprintf("%s/%s@%s", testLoginURL, testRepo, digest)
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], ".*-?local[.].+", defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(1, annotatedTags, "Number of annotated elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// There are 5 tags, but only 4 match the filter. There should be 4 Annotate calls.
	t.Run("FourAnnotationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "latest").Return(FourTagsResult, nil).Once()

		ref := fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName1)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName2)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName3)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName4)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()

		digestRef := fmt.Sprintf("%s/%s@%s", testLoginURL, testRepo, digest)
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()
		digestRef = fmt.Sprintf("%s/%s@%s", testLoginURL, testRepo, digest)
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()
		digestRef = fmt.Sprintf("%s/%s@%s", testLoginURL, testRepo, multiArchDigest)
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()
		digestRef = fmt.Sprintf("%s/%s@%s", testLoginURL, testRepo, digest)
		mockOrasClient.On("Annotate", mock.Anything, digestRef, testArtifactType, annotationMap).Return(nil).Once()

		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "^v.*", defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(4, annotatedTags, "Number of annotated elements should be 4")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// There are 5 tags and none match the filter. There are no Annotate calls.
	t.Run("NoAnnotationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "latest").Return(FourTagsResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "^i.*", defaultRegexpMatchTimeoutSeconds, false)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

}

// Contains all the tests for the getTagstoAnnotate method
func TestGetManifeststoAnnotate(t *testing.T) {
	tagRegex, _ := common.BuildRegexFilter("[\\s\\S]*", defaultRegexpMatchTimeoutSeconds)

	// If an error (other than a 404 error) occurs while calling GetAcrTags, an error should be returned.
	t.Run("GetAcrTagsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(nil, errors.New("error fetching tags")).Once()
		_, testLastTag, err := getManifestsToAnnotate(testCtx, mockClient, mockOrasClient, testLoginURL, testRepo, tagRegex, "", testArtifactType, false)
		assert.Equal("", testLastTag, "Last tag should be empty")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If a 404 error occurs while calling GetAcrTags, an error should be returned.
	t.Run("GetAcrTags404Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Once()
		_, testLastTag, err := getManifestsToAnnotate(testCtx, mockClient, mockOrasClient, testLoginURL, testRepo, tagRegex, "", testArtifactType, false)
		assert.Equal("", testLastTag, "Last tag should be empty")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// GetTagstoAnnotate returns no error
	t.Run("Success case", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(TagWithLocal, nil).Once()
		ref := fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName1CommitA)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName1CommitB)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName1CommitC)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagNameWithLoad)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		tagsToAnnotate, testLastTag, err := getManifestsToAnnotate(testCtx, mockClient, mockOrasClient, testLoginURL, testRepo, tagRegex, "", testArtifactType, false)
		assert.Equal(4, len(*tagsToAnnotate), "Number of tags to annotate should be 1")
		assert.Equal("", testLastTag, "Last tag should be empty")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

}

// TestAnnotateManifests contains the tests for the annotateUntaggedManifests method, which calls the getManifests method.
// It is invoked when the --untagged flag is set and the --dry-run flag is not set
func TestAnnotateManifests(t *testing.T) {
	// If a repository is not known, annotateUntaggedManifests should only call GetAcrManifests once and return no error.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(notFoundManifestResponse, errors.New("testRepo not found")).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(0, annotatedManifests, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// If there is an error (different to a 404 error) getting the first set of manifests, an error should be returned.
	t.Run("GetAcrManifestsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(nil, errors.New("unauthorized")).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(-1, annotatedManifests, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// No manifest should be annotated. If all the manifests have at least one tag they should not be annotated.
	t.Run("NoAnnotationManifestTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(EmptyListManifestsResult, nil).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(0, annotatedManifests, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// If there is an error (different to a 404 error) getting the second set of manifests, an error should be returned.
	t.Run("GetAcrManifestsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(nil, errors.New("error getting manifests")).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(-1, annotatedManifests, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// The following tests involve multiarch manifests
	// If there is an error while getting the multiarch manifest, an error should be returned.
	t.Run("MultiArchErrorGettingManifestTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(nil, errors.New("error getting manifest")).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(-1, annotatedManifests, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// If a multiarch manifest returns an invalid JSON, an error should be returned.
	t.Run("MultiArchInvalidJsonTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(nil, errors.New("error getting manifest")).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(-1, annotatedManifests, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// Error annotating due to annotation not being formatted correctly
	t.Run("BadAnnotationFormat", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(EmptyListManifestsResult, nil).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testBadAnnotations[:], false)
		assert.Equal(-1, annotatedManifests, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// The following tests involve annotating manifests.
	// There are three manifests split into two GetAcrManifests calls, and one is linked to a tag so there should
	// only be 2 annotations, hence the 2 Annotate calls
	t.Run("AnnotateTwoManifestsTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		ref := fmt.Sprintf("%s/%s@sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683", testLoginURL, testRepo)
		mockOrasClient.On("Annotate", mock.Anything, ref, testArtifactType, annotationMap).Return(nil).Once()
		ref = fmt.Sprintf("%s/%s@sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696", testLoginURL, testRepo)
		mockOrasClient.On("Annotate", mock.Anything, ref, testArtifactType, annotationMap).Return(nil).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(2, annotatedManifests, "Number of annotated elements should be 2")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// If there is an error while annotating the manifest and it is a 404 error, return an error.
	t.Run("ErrorManifestAnnotateNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		ref := fmt.Sprintf("%s/%s@sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683", testLoginURL, testRepo)
		mockOrasClient.On("Annotate", mock.Anything, ref, testArtifactType, annotationMap).Return(nil).Once()
		ref = fmt.Sprintf("%s/%s@sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696", testLoginURL, testRepo)
		mockOrasClient.On("Annotate", mock.Anything, ref, testArtifactType, annotationMap).Return(errors.New("manifest not found")).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(1, annotatedManifests, "Number of annotated elements should be 1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// If there is an error while annotating the manifest and it is different from a 404 error, an error should be returned.
	t.Run("ErrorManifestAnnotateTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		ref := fmt.Sprintf("%s/%s@sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683", testLoginURL, testRepo)
		mockOrasClient.On("Annotate", mock.Anything, ref, testArtifactType, annotationMap).Return(errors.New("error annotating manifest")).Once()
		ref = fmt.Sprintf("%s/%s@sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696", testLoginURL, testRepo)
		mockOrasClient.On("Annotate", mock.Anything, ref, testArtifactType, annotationMap).Return(nil).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(1, annotatedManifests, "Number of annotated elements should be 1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// If there is an error while annotating the manifest and it is different than a 404 error, and error should be returned.
	// Similar to the previous test but the error occurs in the second manifest that should be annotated.
	t.Run("ErrorManifestAnnotate2Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		ref := fmt.Sprintf("%s/%s@sha256:63532043b5af6247377a472ad075a42bde35689918de1cf7f807714997e0e683", testLoginURL, testRepo)
		mockOrasClient.On("Annotate", mock.Anything, ref, testArtifactType, annotationMap).Return(nil).Once()
		ref = fmt.Sprintf("%s/%s@sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696", testLoginURL, testRepo)
		mockOrasClient.On("Annotate", mock.Anything, ref, testArtifactType, annotationMap).Return(errors.New("error annotating manifest")).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(1, annotatedManifests, "Number of annotated elements should be 1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// There are three manifests, two of them have tags, but one of them belongs to a multiarch image that has tags so it should
	// not be annotated. Only one call to Annotate should be made because the manifest that does not belong to the
	// multiarch manifest and has no tags should be annotated.
	t.Run("MultiArchAnnotateTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(multiArchManifestV2Bytes, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		ref := fmt.Sprintf("%s/%s@sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696", testLoginURL, testRepo)
		mockOrasClient.On("Annotate", mock.Anything, ref, testArtifactType, annotationMap).Return(nil).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(1, annotatedManifests, "Number of annotated elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// If a manifest should be annotated but the delete-enabled attribute is set to false it should not be deleted
	// and no error should show on the CLI output.
	t.Run("OperationNotAllowedManifestDeleteDisabledTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(deleteDisabledOneManifestResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(0, annotatedManifests, "Number of deleted elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})

	// If a manifest should be annotated but the write-enabled attribute is set to false, it should not be annotated
	// and no error should show on the CLI output.
	t.Run("OperationNotAllowedManifestWriteDisabledTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(writeDisabledOneManifestResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", digest).Return(EmptyListManifestsResult, nil).Once()
		annotatedManifests, err := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], false)
		assert.Equal(0, annotatedManifests, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
		mockOrasClient.AssertExpectations(t)
	})
}

// TestDryRun contains the tests for dry-runs of annotateTags and annotateUntaggedManifests.
//
//	It is called when the --dry-run flag is set.
func TestDryRunAnnotate(t *testing.T) {
	// If a repository is not known, annotateTags and annotateUntaggedManifests should not return an error,
	// and there should not be any tags or manifests annotated.
	t.Run("RepositoryNotFoundTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(notFoundManifestResponse, errors.New("testRepo not found")).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(notFoundTagResponse, errors.New("testRepo not found")).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "[\\s\\S]*", defaultRegexpMatchTimeoutSeconds, true)
		annotatedManifests, errManifests := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], true)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(0, annotatedManifests, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		assert.Equal(nil, errManifests, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an invalid regex, an error should be returned as well as the invalid counters.
	t.Run("InvalidRegexTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "[", defaultRegexpMatchTimeoutSeconds, true)
		assert.Equal(-1, annotatedTags, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// There are 4 tags that should be annotated. Note how there are no AnnotateAcrTag calls because this a dry-run.
	t.Run("FourTagAnnotationDryRunTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsResult, nil).Once()
		ref := fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName1)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName2)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName3)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		ref = fmt.Sprintf("%s/%s:%s", testLoginURL, testRepo, tagName4)
		mockOrasClient.On("DiscoverLifecycleAnnotation", mock.Anything, ref, testArtifactType).Return(false, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "[\\s\\S]*", defaultRegexpMatchTimeoutSeconds, true)
		assert.Equal(4, annotatedTags, "Number of annotated elements should be 4")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error on the first GetAcrTags call (different to a 404), an error should be returned.
	t.Run("GetAcrTagsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(nil, errors.New("error fetching tags")).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "[\\s\\S]*", defaultRegexpMatchTimeoutSeconds, true)
		assert.Equal(-1, annotatedTags, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error on the second GetAcrTags call (different to a 404), an error should be returned.
	t.Run("GetAcrTagsError2Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(nil, errors.New("error fetching tags")).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "[\\s\\S]*", defaultRegexpMatchTimeoutSeconds, true)
		assert.Equal(-1, annotatedTags, "Number of annotated elements should be -1")
		assert.NotEqual(nil, err, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})
	// If there is an error on the first GetAcrManifests call (different to a 404), an error should be returned.
	t.Run("GetAcrManifestsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(nil, errors.New("testRepo not found")).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "[\\s\\S]*", defaultRegexpMatchTimeoutSeconds, true)
		annotatedManifests, errManifests := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], true)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(-1, annotatedManifests, "Number of annotated elements should be -1")
		assert.Equal(nil, err, "Error should be nil")
		assert.NotEqual(nil, errManifests, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is an error on the second GetAcrManifests call (different to a 404), an error should be returned.
	t.Run("GetAcrManifestsError2Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(EmptyListTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(nil, errors.New("error fetching tags")).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "[\\s\\S]*", defaultRegexpMatchTimeoutSeconds, true)
		annotatedManifests, errManifests := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], true)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(-1, annotatedManifests, "Number of annotated elements should be -1")
		assert.Equal(nil, err, "Error should be nil")
		assert.NotEqual(nil, errManifests, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If there is a GetManifest error for the multiarch scenario, an error should be returned.
	t.Run("MultiArchGetManifestErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(nil, errors.New("error getting manifest")).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "^lat.*", defaultRegexpMatchTimeoutSeconds, true)
		annotatedManifests, errManifests := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], true)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(-1, annotatedManifests, "Number of annotated elements should be -1")
		assert.Equal(nil, err, "Error should be nil")
		assert.NotEqual(nil, errManifests, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// If the returned multiarch manifest json is invalid, an error should be returned.
	t.Run("MultiArchInvalidJSONTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return([]byte("invalid json"), nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "^lat.*", defaultRegexpMatchTimeoutSeconds, true)
		annotatedManifests, errManifests := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], true)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(-1, annotatedManifests, "Number of annotated elements should be -1")
		assert.Equal(nil, err, "Error should be nil")
		assert.NotEqual(nil, errManifests, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// Error on the fourth GetAcrTags, an error should be returned.
	t.Run("MultiArchGetAcrTagsErrorTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(multiArchManifestV2Bytes, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(nil, errors.New("error fetching manifests")).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "^lat.*", defaultRegexpMatchTimeoutSeconds, true)
		annotatedManifests, errManifests := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], true)
		assert.Equal(0, annotatedTags, "Number of annotated tags should be 0")
		assert.Equal(-1, annotatedManifests, "Number of annotated manifests should be -1")
		assert.Equal(nil, err, "Error should be nil")
		assert.NotEqual(nil, errManifests, "Error should not be nil")
		mockClient.AssertExpectations(t)
	})

	// One image that has no tags belongs to a multiarch image that has tags so it should not be annotated, but there is one manifest
	// that should be annotated.
	t.Run("MultiArchGetAcrTagsError2Test", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(FourTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "").Return(singleMultiArchManifestV2WithTagsResult, nil).Once()
		mockClient.On("GetManifest", mock.Anything, testRepo, "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(multiArchManifestV2Bytes, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119").Return(doubleManifestV2WithoutTagsResult, nil).Once()
		mockClient.On("GetAcrManifests", mock.Anything, testRepo, "", "sha256:6305e31b9b0081d2532397a1e08823f843f329a7af2ac98cb1d7f0355a3e3696").Return(EmptyListManifestsResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "^lat.*", defaultRegexpMatchTimeoutSeconds, true)
		annotatedManifests, errManifests := annotateUntaggedManifests(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], true)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(1, annotatedManifests, "Number of annotated elements should be 1")
		assert.Equal(nil, err, "Error should be nil")
		assert.Equal(nil, errManifests, "Error should be nil")
		mockClient.AssertExpectations(t)
	})

	// There are 5 tags and none match the filter. There are no AnnotateAcrTag calls.
	t.Run("NoAnnotationTest", func(t *testing.T) {
		assert := assert.New(t)
		mockClient := &mocks.AcrCLIClientInterface{}
		mockOrasClient := &mocks.ORASClientInterface{}
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(OneTagResultWithNext, nil).Once()
		mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "latest").Return(FourTagsResult, nil).Once()
		annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], "^i.*", defaultRegexpMatchTimeoutSeconds, true)
		assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
		assert.Equal(nil, err, "Error should be nil")
		mockClient.AssertExpectations(t)
	})
}

// All the variables used in the tests are defined here
var (
	testRegex          = "[\\s\\S]"
	testArtifactType   = "application/vnd.microsoft.artifact.lifecycle"
	testAnnotations    = [1]string{"vnd.microsoft.artifact.lifecycle.end-of-life.date=2024-03-21"}
	testBadAnnotations = [1]string{"test"}
	annotationMap      = map[string]string{
		"vnd.microsoft.artifact.lifecycle.end-of-life.date": "2024-03-21",
	}
)
