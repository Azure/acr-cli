package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/go-autorest/autorest/azure"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
)

func TestFindDirectDependentManifests(t *testing.T) {
	ctx := context.Background()
	mockClient := &mocks.AcrCLIClientInterface{}
	repoName := "test-repo"
	err404 := azure.RequestError{}
	err404.StatusCode = 404

	// Define multiple test cases
	testCases := []struct {
		name            string
		manifestDigest  string
		mockResponse    interface{}
		expectedResults []dependentManifestResult
		expectedError   error
	}{
		{
			name:           "Valid multiarch manifest",
			manifestDigest: "test-digest",
			mockResponse: struct {
				Manifests []v1.Descriptor `json:"manifests"`
			}{
				Manifests: []v1.Descriptor{
					{Digest: "digest1", MediaType: mediaTypeDockerManifestList},
					{Digest: "digest2", MediaType: v1.MediaTypeImageIndex},
					{Digest: "digest3", MediaType: "other"},
				},
			},
			expectedResults: []dependentManifestResult{
				{Digest: "digest1", IsList: true},
				{Digest: "digest2", IsList: true},
				{Digest: "digest3", IsList: false},
			},
			expectedError: nil,
		},
		{
			name:            "Manifest not found",
			manifestDigest:  "missing-digest",
			mockResponse:    err404,
			expectedResults: []dependentManifestResult{},
			expectedError:   nil, // Should return an empty slice without error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if bytes, ok := tc.mockResponse.([]byte); ok {
				mockClient.On("GetManifest", ctx, repoName, tc.manifestDigest).Return(bytes, nil)
			} else if err, ok := tc.mockResponse.(error); ok {
				mockClient.On("GetManifest", ctx, repoName, tc.manifestDigest).Return(nil, err)
			} else {
				bytes, _ := json.Marshal(tc.mockResponse)
				mockClient.On("GetManifest", ctx, repoName, tc.manifestDigest).Return(bytes, nil)
			}

			results, err := findDirectDependentManifests(ctx, tc.manifestDigest, mockClient, repoName)

			if tc.expectedError != nil {
				assert.ErrorContains(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResults, results)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestAddIndexDependenciesToIgnoreList(t *testing.T) {
	ctx := context.Background()
	err404 := azure.RequestError{}
	err404.StatusCode = 404
	repoName := "test-repo"

	testCases := []struct {
		name               string
		manifestDigest     string
		mockResponses      map[string]string // Multiple responses for recursion
		mockErrorResponses map[string]error  // Simulate errors for specific digests
		expectedKeys       []string
		expectedError      bool
	}{
		{
			name:           "Valid multiarch manifest with recursion",
			manifestDigest: "test-digest",
			mockResponses: map[string]string{
				"test-digest": `{
					"manifests": [
						{"digest": "digest1", "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json"},
						{"digest": "digest2", "mediaType": "application/vnd.oci.image.index.v1+json"},
						{"digest": "digest3", "mediaType": "application/vnd.oci.image.manifest.v1+json"}
					]
				}`,
				"digest1": `{
					"manifests": [
						{"digest": "digest1-1", "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json"},
						{"digest": "digest1-2", "mediaType": "application/vnd.oci.image.index.v1+json"},
						{"digest": "digest1-3", "mediaType": "application/vnd.oci.image.manifest.v1+json"}
					]
				}`,
				"digest2": `{
					"manifests": [
						{"digest": "digest2-1", "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json"},
						{"digest": "digest2-2", "mediaType": "application/vnd.oci.image.index.v1+json"},
						{"digest": "digest2-3", "mediaType": "application/vnd.oci.image.manifest.v1+json"}
					]
				}`,
				"digest1-1": `{"manifests": []}`,
				"digest1-2": `{"manifests": []}`,
				"digest2-1": `{"manifests": []}`,
				"digest2-2": `{"manifests": []}`,
			},
			expectedKeys:  []string{"digest1", "digest2", "digest3", "digest1-1", "digest1-2", "digest1-3", "digest2-1", "digest2-2", "digest2-3"},
			expectedError: false,
		},
		{
			name:           "Some Manifests not found",
			manifestDigest: "test-digest",
			mockResponses: map[string]string{
				"test-digest": `{
					"manifests": [
						{"digest": "digest1", "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json"},
						{"digest": "digest2", "mediaType": "application/vnd.oci.image.index.v1+json"},
						{"digest": "digest3", "mediaType": "application/vnd.oci.image.manifest.v1+json"}
					]
				}`,
				"digest2": `{
					"manifests": [
						{"digest": "digest2-1", "mediaType": "application/vnd.oci.image.manifest.v1+json"}
					]
				}`,
			},
			mockErrorResponses: map[string]error{
				"digest1": err404, // Simulate 404 for digest1
			},
			expectedKeys: []string{"digest1", "digest2", "digest3", "digest2-1"},
		},
		{
			name:           "Encountered error that could not be resolved",
			manifestDigest: "test-digest",
			mockResponses: map[string]string{
				"test-digest": `{
					"manifests": [
						{"digest": "digest1", "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json"},
						{"digest": "digest2", "mediaType": "application/vnd.oci.image.index.v1+json"},
						{"digest": "digest3", "mediaType": "application/vnd.oci.image.manifest.v1+json"}
					]
				}`,
				"digest1": `{
					"manifests": [
						{"digest": "digest1-1", "mediaType": "application/vnd.oci.image.manifest.v1+json"}
					]
				}`,
			},
			mockErrorResponses: map[string]error{
				"digest2": errors.New("this is an unexpected error that purge cannot resolve"),
			},
			expectedKeys:  []string{"digest1", "digest2", "digest3", "digest1-1"},
			expectedError: true, // Expect an error due to the unresolved error for digest1
		},
	}

	for _, tt := range testCases {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			ignoreList := &sync.Map{}
			mockClient := &mocks.AcrCLIClientInterface{}

			// Mock responses dynamically
			for digest, response := range tc.mockResponses {
				mockClient.On("GetManifest", ctx, repoName, digest).Return([]byte(response), nil)
			}

			// Mock error responses
			for digest, err := range tc.mockErrorResponses {
				mockClient.On("GetManifest", ctx, repoName, digest).Return(nil, err)
			}

			err := addIndexDependenciesToIgnoreList(ctx, tc.manifestDigest, mockClient, repoName, ignoreList)
			if tc.expectedError {
				assert.Error(t, err, "Expected an error due to unresolved manifest")
			} else {
				assert.NoError(t, err, "Expected no error while processing manifests")
			}

			for _, key := range tc.expectedKeys {
				_, exists := ignoreList.Load(key)
				assert.True(t, exists, "Expected manifest %s in ignore list", key)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

// TestAddIndexDependenciesToIgnoreListComplex tests the recursive structure of manifests
// with varying depths and branching factors to ensure the function can handle complex cases.
// It generates a random tree structure of manifests and verifies that all expected manifests
// are added to the ignore list. This test is designed to cover edge cases and ensure robustness,
// the constants can be updated locally to increase the complexity of the test cases.
func TestAddIndexDependenciesToIgnoreListComplex(t *testing.T) {
	const (
		numTests  = 11
		maxDepth  = 5
		maxBranch = 4
	)

	for i := 0; i < numTests; i++ {
		depth := rand.Intn(maxDepth-1) + 2      // depth between 2 and maxDepth
		branching := rand.Intn(maxBranch-1) + 2 // branching between 2 and maxBranch

		t.Run(fmt.Sprintf("Depth%d_Branching%d", depth, branching), func(t *testing.T) {
			t.Parallel()
			rootDigest, mockResponses, expectedKeys := generateRecursiveTestCase(depth, branching)

			ctx := context.Background()
			repoName := "test-repo"
			ignoreList := &sync.Map{}
			mockClient := &mocks.AcrCLIClientInterface{}

			// Mock responses for the recursive structure
			for digest, response := range mockResponses {
				mockClient.On("GetManifest", ctx, repoName, digest).Return([]byte(response), nil)
			}

			err := addIndexDependenciesToIgnoreList(ctx, rootDigest, mockClient, repoName, ignoreList)
			assert.NoError(t, err, "Expected no error while processing recursive manifests")

			for _, key := range expectedKeys {
				_, exists := ignoreList.Load(key)
				assert.True(t, exists, "Expected manifest %s in ignore list", key)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func generateRecursiveTestCase(depth, branching int) (string, map[string]string, []string) {
	type manifest struct {
		Digest    string `json:"digest"`
		MediaType string `json:"mediaType"`
	}

	type manifestList struct {
		Manifests []manifest `json:"manifests"`
	}

	mockResponses := make(map[string]string)
	expectedKeys := []string{}
	root := "root-digest"

	type queueItem struct {
		Digest string
		Level  int
	}

	queue := []queueItem{{Digest: root, Level: 0}}

	mediaTypes := []string{
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.oci.image.manifest.v1+json",
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.Level >= depth {
			mockResponses[item.Digest] = `{"manifests":[]}`
			continue
		}

		manifests := []manifest{}
		for i := 0; i < branching; i++ {
			child := fmt.Sprintf("%s-%d", item.Digest, i)
			mediaType := mediaTypes[rand.Intn(len(mediaTypes))]

			// Only add children to mockResponses if they are index types
			if mediaType == "application/vnd.docker.distribution.manifest.list.v2+json" ||
				mediaType == "application/vnd.oci.image.index.v1+json" {
				queue = append(queue, queueItem{Digest: child, Level: item.Level + 1})
				expectedKeys = append(expectedKeys, child)
				manifests = append(manifests, manifest{Digest: child, MediaType: mediaType})
			}
		}

		data, _ := json.Marshal(manifestList{Manifests: manifests})
		mockResponses[item.Digest] = string(data)
	}

	return root, mockResponses, expectedKeys
}
