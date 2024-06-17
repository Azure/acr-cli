// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
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
			t.Logf("equal")
		} else {
			t.Logf("not equal")
		}
		assert.Equal(m, annotationMap, "Maps should be equal")
	})

	t.Run("Failure", func(t *testing.T) {
		assert := assert.New(t)
		annotations := []string{"vnd.microsoft.artifact.lifecycle.end-of-life.date=2024-03-21", "testKey"}
		_, err := convertListToMap(annotations)
		assert.NotEqual(nil, err, "Error should not be nil")
	})

}
