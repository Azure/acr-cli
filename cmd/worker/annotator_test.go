// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.
package worker

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertListToMap(t *testing.T) {

	t.Run("Success", func(t *testing.T) {
		assert := assert.New(t)
		annotations := []string{"vnd.microsoft.artifact.lifecycle.end-of-life.date=2024-03-21", "testKey=testVal"}
		annotationMap := map[string]string{
			"vnd.microsoft.artifact.lifecycle.end-of-life.date": "2024-03-21",
			"testKey": "testVal",
		}

		m, _ := convertListToMap(annotations)
		if reflect.DeepEqual(m, annotationMap) {
			fmt.Printf("equal")
		} else {
			fmt.Printf("not equal")
		}
		assert.Equal(m, annotationMap, "Maps should be equal")
	})

	t.Run("Failure", func(t *testing.T) {
		assert := assert.New(t)
		annotations := []string{"vnd.microsoft.artifact.lifecycle.end-of-life.date=2024-03-21", "testKey"}
		_, err := convertListToMap(annotations)
		assert.NotEqual(nil, err, "Error should not be nil")
	})

	// t.Run("EmptyRepositoryTest", func(t *testing.T) {
	// 	assert := assert.New(t)
	// 	mockClient := &mocks.AcrCLIClientInterface{}
	// 	mockOrasClient := &mocks.ORASClientInterface{}
	// 	mockClient.On("GetAcrTags", mock.Anything, testRepo, "timedesc", "").Return(EmptyListTagsResult, nil).Once()
	// 	annotatedTags, err := annotateTags(testCtx, mockClient, mockOrasClient, defaultPoolSize, testLoginURL, testRepo, testArtifactType, testAnnotations[:], testRegex, defaultRegexpMatchTimeoutSeconds)
	// 	assert.Equal(0, annotatedTags, "Number of annotated elements should be 0")
	// 	assert.Equal(nil, err, "Error should be nil")
	// 	mockClient.AssertExpectations(t)
	// })

}
