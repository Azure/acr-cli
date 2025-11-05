package repository

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/Azure/acr-cli/acr"
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

func TestAddDependentManifestsToIgnoreList(t *testing.T) {
	ctx := context.Background()
	err404 := azure.RequestError{}
	err404.StatusCode = 404
	repoName := "test-repo"

	t.Run("Simple case with non-list manifests", func(t *testing.T) {
		ignoreList := &sync.Map{}
		mockClient := &mocks.AcrCLIClientInterface{}

		dependentManifests := []dependentManifestResult{
			{Digest: "digest1", IsList: false},
			{Digest: "digest2", IsList: false},
		}

		err := addDependentManifestsToIgnoreList(ctx, dependentManifests, mockClient, repoName, ignoreList)
		assert.NoError(t, err)

		// Check that both manifests are in ignore list
		_, exists1 := ignoreList.Load("digest1")
		assert.True(t, exists1)
		_, exists2 := ignoreList.Load("digest2")
		assert.True(t, exists2)

		// No mock expectations since non-list manifests don't get fetched
		mockClient.AssertExpectations(t)
	})

	t.Run("Mixed list and non-list manifests", func(t *testing.T) {
		ignoreList := &sync.Map{}
		mockClient := &mocks.AcrCLIClientInterface{}

		dependentManifests := []dependentManifestResult{
			{Digest: "list1", IsList: true},
			{Digest: "nonlist1", IsList: false},
		}

		// Mock response for the list manifest
		mockClient.On("GetManifest", ctx, repoName, "list1").Return([]byte(`{
			"manifests": [
				{"digest": "child1", "mediaType": "application/vnd.oci.image.manifest.v1+json"}
			]
		}`), nil)

		err := addDependentManifestsToIgnoreList(ctx, dependentManifests, mockClient, repoName, ignoreList)
		assert.NoError(t, err)

		// Check that all manifests are in ignore list
		_, exists1 := ignoreList.Load("list1")
		assert.True(t, exists1)
		_, exists2 := ignoreList.Load("nonlist1")
		assert.True(t, exists2)
		_, exists3 := ignoreList.Load("child1")
		assert.True(t, exists3)

		mockClient.AssertExpectations(t)
	})
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
		depth := secureRandomNum(2, maxDepth)      // depth between 2 and maxDepth
		branching := secureRandomNum(2, maxBranch) // branching between 2 and maxBranch

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

			// Convert root digest to dependent manifest for testing
			dependentManifests := []dependentManifestResult{
				{Digest: rootDigest, IsList: true},
			}

			err := addDependentManifestsToIgnoreList(ctx, dependentManifests, mockClient, repoName, ignoreList)
			assert.NoError(t, err, "Expected no error while processing recursive manifests")

			// Check that root digest is in ignore list
			_, exists := ignoreList.Load(rootDigest)
			assert.True(t, exists, "Expected root manifest %s in ignore list", rootDigest)

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
			mediaType := mediaTypes[secureRandomNum(0, len(mediaTypes))]

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

func TestCheckOCIArtifactDeletability(t *testing.T) {
	testCases := []struct {
		name         string
		manifestJSON string
		mediaType    string
		canDelete    bool
		expectError  bool
	}{
		{
			name:         "OCI manifest without subject",
			manifestJSON: `{"schemaVersion": 2}`,
			mediaType:    v1.MediaTypeImageManifest,
			canDelete:    true,
			expectError:  false,
		},
		{
			name:         "OCI manifest with subject - referrer",
			manifestJSON: `{"schemaVersion": 2, "subject": {"digest": "sha256:abc123", "mediaType": "application/vnd.oci.image.manifest.v1+json"}}`,
			mediaType:    v1.MediaTypeImageManifest,
			canDelete:    false,
			expectError:  false,
		},
		{
			name:         "OCI artifact manifest without subject",
			manifestJSON: `{"schemaVersion": 2}`,
			mediaType:    mediaTypeArtifactManifest,
			canDelete:    true,
			expectError:  false,
		},
		{
			name:         "OCI artifact manifest with subject - referrer",
			manifestJSON: `{"schemaVersion": 2, "subject": {"digest": "sha256:def456", "mediaType": "application/vnd.oci.image.manifest.v1+json"}}`,
			mediaType:    mediaTypeArtifactManifest,
			canDelete:    false,
			expectError:  false,
		},
		{
			name:         "Non-OCI media type",
			manifestJSON: `{"schemaVersion": 2}`,
			mediaType:    "application/vnd.docker.distribution.manifest.v2+json",
			canDelete:    true,
			expectError:  false,
		},
		{
			name:         "Invalid JSON",
			manifestJSON: `{invalid json}`,
			mediaType:    v1.MediaTypeImageManifest,
			canDelete:    false,
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			canDelete, err := checkOCIArtifactDeletability([]byte(tc.manifestJSON), tc.mediaType)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.canDelete, canDelete)
			}
		})
	}
}

func TestExtractSubmanifestsFromBytes(t *testing.T) {
	testCases := []struct {
		name         string
		manifestJSON string
		expected     []dependentManifestResult
		expectError  bool
	}{
		{
			name:         "Empty manifests array",
			manifestJSON: `{"manifests": []}`,
			expected:     nil,
			expectError:  false,
		},
		{
			name: "Mixed manifest types",
			manifestJSON: `{
				"manifests": [
					{"digest": "sha256:abc123", "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json"},
					{"digest": "sha256:def456", "mediaType": "application/vnd.oci.image.index.v1+json"},
					{"digest": "sha256:ghi789", "mediaType": "application/vnd.oci.image.manifest.v1+json"}
				]
			}`,
			expected: []dependentManifestResult{
				{Digest: "sha256:abc123", IsList: true},
				{Digest: "sha256:def456", IsList: true},
				{Digest: "sha256:ghi789", IsList: false},
			},
			expectError: false,
		},
		{
			name:         "No manifests field",
			manifestJSON: `{"schemaVersion": 2}`,
			expected:     nil,
			expectError:  false,
		},
		{
			name:         "Invalid JSON",
			manifestJSON: `{invalid json}`,
			expected:     nil,
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := extractSubmanifestsFromBytes([]byte(tc.manifestJSON))

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func secureRandomNum(minDepth, maxDepth int) int {
	if maxDepth <= minDepth {
		// This is a function for testing so a panic is acceptable here
		panic(fmt.Sprintf("maxDepth (%d) must be greater than minDepth (%d)", maxDepth, minDepth))
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(maxDepth-minDepth)))
	if err != nil {
		return 0
	}
	return int(n.Int64()) + minDepth
}

// TestGetUntaggedManifestsWithAgeCriteria tests the age-based filtering logic for untagged manifests
func TestGetUntaggedManifestsWithAgeCriteria(t *testing.T) {
	ctx := context.Background()
	repoName := "test-repo"
	poolSize := 1

	// Create test timestamps
	oldTimestamp := "2024-10-01T12:00:00Z"    // More than 30 days old
	recentTimestamp := "2024-11-03T12:00:00Z" // Less than 30 days old

	t.Run("Untagged manifest older than cutoff is deleted", func(t *testing.T) {
		mockClient := &mocks.AcrCLIClientInterface{}

		manifests := createManifestsResult([]manifestTestData{
			{digest: "sha256:old1", tags: nil, lastUpdate: oldTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
		})

		mockClient.On("GetAcrManifests", ctx, repoName, "", "").Return(manifests, nil).Once()
		mockClient.On("GetAcrManifests", ctx, repoName, "", "sha256:old1").Return(createEmptyManifestsResult(), nil).Once()

		cutoff := parseTime(t, "2024-11-01T12:00:00Z") // 30 days ago from "now"

		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, false, nil, false, false, &cutoff)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, "sha256:old1", *result[0].Digest)
		mockClient.AssertExpectations(t)
	})

	t.Run("Untagged manifest newer than cutoff is protected", func(t *testing.T) {
		mockClient := &mocks.AcrCLIClientInterface{}

		manifests := createManifestsResult([]manifestTestData{
			{digest: "sha256:recent1", tags: nil, lastUpdate: recentTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
		})

		mockClient.On("GetAcrManifests", ctx, repoName, "", "").Return(manifests, nil).Once()
		mockClient.On("GetAcrManifests", ctx, repoName, "", "sha256:recent1").Return(createEmptyManifestsResult(), nil).Once()

		cutoff := parseTime(t, "2024-11-01T12:00:00Z")

		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, false, nil, false, false, &cutoff)

		assert.NoError(t, err)
		assert.Equal(t, 0, len(result), "Recent manifest should be protected")
		mockClient.AssertExpectations(t)
	})

	t.Run("Untagged manifest with nil timestamp is protected", func(t *testing.T) {
		mockClient := &mocks.AcrCLIClientInterface{}

		manifests := createManifestsResult([]manifestTestData{
			{digest: "sha256:notime1", tags: nil, lastUpdate: "", mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
		})

		mockClient.On("GetAcrManifests", ctx, repoName, "", "").Return(manifests, nil).Once()
		mockClient.On("GetAcrManifests", ctx, repoName, "", "sha256:notime1").Return(createEmptyManifestsResult(), nil).Once()

		cutoff := parseTime(t, "2024-11-01T12:00:00Z")

		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, false, nil, false, false, &cutoff)

		assert.NoError(t, err)
		assert.Equal(t, 0, len(result), "Manifest with nil timestamp should be protected")
		mockClient.AssertExpectations(t)
	})

	t.Run("Tagged manifest ignores age criteria", func(t *testing.T) {
		mockClient := &mocks.AcrCLIClientInterface{}

		manifests := createManifestsResult([]manifestTestData{
			{digest: "sha256:oldtagged", tags: []string{"v1"}, lastUpdate: oldTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
		})

		mockClient.On("GetAcrManifests", ctx, repoName, "", "").Return(manifests, nil).Once()
		mockClient.On("GetAcrManifests", ctx, repoName, "", "sha256:oldtagged").Return(createEmptyManifestsResult(), nil).Once()

		cutoff := parseTime(t, "2024-11-01T12:00:00Z")

		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, false, nil, false, false, &cutoff)

		assert.NoError(t, err)
		assert.Equal(t, 0, len(result), "Tagged manifest should be protected regardless of age")
		mockClient.AssertExpectations(t)
	})

	t.Run("Mixed manifests - only old untagged are deleted", func(t *testing.T) {
		mockClient := &mocks.AcrCLIClientInterface{}

		manifests := createManifestsResult([]manifestTestData{
			{digest: "sha256:old1", tags: nil, lastUpdate: oldTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
			{digest: "sha256:recent1", tags: nil, lastUpdate: recentTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
			{digest: "sha256:oldtagged", tags: []string{"v1"}, lastUpdate: oldTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
		})

		mockClient.On("GetAcrManifests", ctx, repoName, "", "").Return(manifests, nil).Once()
		mockClient.On("GetAcrManifests", ctx, repoName, "", "sha256:oldtagged").Return(createManifestsResult([]manifestTestData{
			{digest: "sha256:old1", tags: nil, lastUpdate: oldTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
			{digest: "sha256:recent1", tags: nil, lastUpdate: recentTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
		}), nil).Once()
		mockClient.On("GetAcrManifests", ctx, repoName, "", "sha256:recent1").Return(createEmptyManifestsResult(), nil).Once()

		cutoff := parseTime(t, "2024-11-01T12:00:00Z")

		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, false, nil, false, false, &cutoff)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, "sha256:old1", *result[0].Digest)
		mockClient.AssertExpectations(t)
	})

	t.Run("No cutoff specified - age not checked", func(t *testing.T) {
		mockClient := &mocks.AcrCLIClientInterface{}

		manifests := createManifestsResult([]manifestTestData{
			{digest: "sha256:old1", tags: nil, lastUpdate: oldTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
			{digest: "sha256:recent1", tags: nil, lastUpdate: recentTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
		})

		mockClient.On("GetAcrManifests", ctx, repoName, "", "").Return(manifests, nil).Once()
		mockClient.On("GetAcrManifests", ctx, repoName, "", "sha256:recent1").Return(createEmptyManifestsResult(), nil).Once()

		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, false, nil, false, false, nil)

		assert.NoError(t, err)
		assert.Equal(t, 2, len(result), "All untagged manifests should be candidates when no cutoff is specified")
		mockClient.AssertExpectations(t)
	})

	t.Run("Dry run with age criteria", func(t *testing.T) {
		mockClient := &mocks.AcrCLIClientInterface{}

		manifests := createManifestsResult([]manifestTestData{
			{digest: "sha256:old1", tags: nil, lastUpdate: oldTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
			{digest: "sha256:recent1", tags: nil, lastUpdate: recentTimestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
		})

		mockClient.On("GetAcrManifests", ctx, repoName, "", "").Return(manifests, nil).Once()
		mockClient.On("GetAcrManifests", ctx, repoName, "", "sha256:recent1").Return(createEmptyManifestsResult(), nil).Once()

		cutoff := parseTime(t, "2024-11-01T12:00:00Z")

		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, false, nil, true, false, &cutoff)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(result), "Dry run should still apply age criteria")
		assert.Equal(t, "sha256:old1", *result[0].Digest)
		mockClient.AssertExpectations(t)
	})
}

// Helper types and functions for testing

type manifestTestData struct {
	digest     string
	tags       []string
	lastUpdate string
	mediaType  string
}

func createManifestsResult(manifests []manifestTestData) *acr.Manifests {
	attributes := make([]acr.ManifestAttributesBase, len(manifests))

	for i, m := range manifests {
		deleteEnabled := true
		writeEnabled := true

		attr := acr.ManifestAttributesBase{
			Digest:    &m.digest,
			MediaType: &m.mediaType,
			ChangeableAttributes: &acr.ChangeableAttributes{
				DeleteEnabled: &deleteEnabled,
				WriteEnabled:  &writeEnabled,
			},
		}

		if len(m.tags) > 0 {
			attr.Tags = &m.tags
		}

		if m.lastUpdate != "" {
			attr.LastUpdateTime = &m.lastUpdate
		}

		attributes[i] = attr
	}

	return &acr.Manifests{
		ManifestsAttributes: &attributes,
	}
}

func createEmptyManifestsResult() *acr.Manifests {
	return &acr.Manifests{
		ManifestsAttributes: nil,
	}
}

func parseTime(t *testing.T, timeStr string) time.Time {
	parsed, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		t.Fatalf("Failed to parse time %s: %v", timeStr, err)
	}
	return parsed
}
