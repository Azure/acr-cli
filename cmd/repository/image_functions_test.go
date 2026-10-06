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
	"github.com/stretchr/testify/mock"
)

func TestEvaluateRetention(t *testing.T) {
	cutoff := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	old := cutoff.Add(-time.Hour)
	recent := cutoff.Add(time.Hour)

	t.Run("Nil cutoff disables age but not maximum", func(t *testing.T) {
		age, maximum := EvaluateRetention(old, nil, 3, -1, 2)
		assert.False(t, age)
		assert.True(t, maximum)

		age, maximum = EvaluateRetention(old, nil, 2, -1, 2)
		assert.False(t, age)
		assert.False(t, maximum)

		age, maximum = EvaluateRetention(old, nil, 3, 0, -1)
		assert.False(t, age)
		assert.False(t, maximum)
	})

	t.Run("Minimum protects old items at its boundary", func(t *testing.T) {
		age, maximum := EvaluateRetention(old, &cutoff, 2, 2, -1)
		assert.False(t, age)
		assert.False(t, maximum)

		age, maximum = EvaluateRetention(old, &cutoff, 3, 2, -1)
		assert.True(t, age)
		assert.False(t, maximum)
	})

	t.Run("Combined policy reports independent causes", func(t *testing.T) {
		age, maximum := EvaluateRetention(old, &cutoff, 2, 1, 3)
		assert.True(t, age)
		assert.False(t, maximum)

		age, maximum = EvaluateRetention(recent, &cutoff, 4, 1, 3)
		assert.False(t, age)
		assert.True(t, maximum)

		age, maximum = EvaluateRetention(old, &cutoff, 4, 1, 3)
		assert.True(t, age)
		assert.True(t, maximum)
	})

	t.Run("Cutoff equality is not older and zero limits are active", func(t *testing.T) {
		age, maximum := EvaluateRetention(cutoff, &cutoff, 1, 0, -1)
		assert.False(t, age)
		assert.False(t, maximum)

		age, maximum = EvaluateRetention(old, &cutoff, 1, 0, 0)
		assert.True(t, age)
		assert.True(t, maximum)
	})

	t.Run("Unset counts preserve age-only selection", func(t *testing.T) {
		age, maximum := EvaluateRetention(old, &cutoff, 1, -1, -1)
		assert.True(t, age)
		assert.False(t, maximum)
	})
}

func TestDeletionReasonString(t *testing.T) {
	tests := []struct {
		reason   DeletionReason
		expected string
	}{
		{DeletionReasonUntagged, "untagged"},
		{DeletionReasonAge, "age"},
		{DeletionReasonMaximumCount, "maximum count"},
		{DeletionReasonAgeAndMaximumCount, "age and maximum count"},
	}
	for _, test := range tests {
		assert.Equal(t, test.expected, test.reason.String())
	}
}

func TestSortManifestsByTimePreservesReasons(t *testing.T) {
	oldTime, recentTime := "2024-10-01T12:00:00Z", "2024-11-01T12:00:00Z"
	oldDigest, firstDigest, secondDigest := "sha256:old", "sha256:a", "sha256:b"
	manifests := []ManifestToDelete{
		{ManifestAttributesBase: acr.ManifestAttributesBase{Digest: &oldDigest, LastUpdateTime: &oldTime}, Reason: DeletionReasonAge},
		{ManifestAttributesBase: acr.ManifestAttributesBase{Digest: &secondDigest, LastUpdateTime: &recentTime}, Reason: DeletionReasonAgeAndMaximumCount},
		{ManifestAttributesBase: acr.ManifestAttributesBase{Digest: &firstDigest, LastUpdateTime: &recentTime}, Reason: DeletionReasonMaximumCount},
	}

	SortManifestsByTime(manifests)

	assert.Equal(t, firstDigest, *manifests[0].Digest)
	assert.Equal(t, DeletionReasonMaximumCount, manifests[0].Reason)
	assert.Equal(t, secondDigest, *manifests[1].Digest)
	assert.Equal(t, DeletionReasonAgeAndMaximumCount, manifests[1].Reason)
	assert.Equal(t, oldDigest, *manifests[2].Digest)
	assert.Equal(t, DeletionReasonAge, manifests[2].Reason)
}

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

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            -1,
			MaxManifests:            -1,
		}
		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, untaggedManifestsOptions, nil)

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

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            -1,
			MaxManifests:            -1,
		}
		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, untaggedManifestsOptions, nil)

		assert.NoError(t, err)
		assert.Equal(t, 0, len(result), "Recent manifest should be protected")
		mockClient.AssertExpectations(t)
	})

	t.Run("Untagged manifest at cutoff is protected", func(t *testing.T) {
		mockClient := &mocks.AcrCLIClientInterface{}
		timestamp := "2024-11-01T12:00:00Z"
		manifests := createManifestsResult([]manifestTestData{
			{digest: "sha256:cutoff", tags: nil, lastUpdate: timestamp, mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
		})

		mockClient.On("GetAcrManifests", ctx, repoName, "", "").Return(manifests, nil).Once()
		mockClient.On("GetAcrManifests", ctx, repoName, "", "sha256:cutoff").Return(createEmptyManifestsResult(), nil).Once()

		cutoff := parseTime(t, timestamp)
		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            -1,
			MaxManifests:            -1,
		}
		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, untaggedManifestsOptions, nil)

		assert.NoError(t, err)
		assert.Empty(t, result, "Manifest exactly at cutoff should be protected")
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

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            -1,
			MaxManifests:            -1,
		}
		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, untaggedManifestsOptions, nil)

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

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            -1,
			MaxManifests:            -1,
		}
		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, untaggedManifestsOptions, nil)

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

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            -1,
			MaxManifests:            -1,
		}
		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, untaggedManifestsOptions, nil)

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

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            nil,
			MinManifests:            -1,
			MaxManifests:            -1,
		}
		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, untaggedManifestsOptions, nil)

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

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  true,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            -1,
			MaxManifests:            -1,
		}
		result, err := GetUntaggedManifests(ctx, poolSize, mockClient, repoName, untaggedManifestsOptions, nil)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(result), "Dry run should still apply age criteria")
		assert.Equal(t, "sha256:old1", *result[0].Digest)
		mockClient.AssertExpectations(t)
	})
}

func TestIsLiteralRegex(t *testing.T) {
	tests := []struct {
		pattern  string
		expected bool
	}{
		{"myrepo", true},
		{"my-repo/sub-repo", true},
		{"repo123", true},
		{"my.repo", false},
		{"repo*", false},
		{"repo+", false},
		{"repo?", false},
		{"(repo)", false},
		{"[repo]", false},
		{`repo\d`, false},
		{"repo|other", false},
		{"^repo", false},
		{"repo$", false},
		{"repo{1}", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			assert.Equal(t, tt.expected, isLiteralRegex(tt.pattern))
		})
	}
}

func TestCollectTagFiltersLiteralSkipsRepoListing(t *testing.T) {
	ctx := context.Background()

	t.Run("Literal repo name skips GetAllRepositoryNames", func(t *testing.T) {
		mockClient := &mocks.BaseClientAPI{}
		// No GetRepositories expectation set — if called, mock will fail.
		filters := []string{"myrepo:.*"}
		result, err := CollectTagFilters(ctx, filters, mockClient, 60, 100)
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"myrepo": ".*"}, result)
		mockClient.AssertExpectations(t)
	})

	t.Run("Regex repo name calls GetAllRepositoryNames", func(t *testing.T) {
		mockClient := &mocks.BaseClientAPI{}
		pageSize := int32(100)
		names := []string{"repo-a", "repo-b"}
		mockClient.On("GetRepositories", ctx, "", &pageSize).Return(acr.Repositories{Names: &names}, nil).Once()
		mockClient.On("GetRepositories", ctx, "repo-b", &pageSize).Return(acr.Repositories{}, nil).Once()

		filters := []string{"repo-.*:latest"}
		result, err := CollectTagFilters(ctx, filters, mockClient, 60, pageSize)
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"repo-a": "latest", "repo-b": "latest"}, result)
		mockClient.AssertExpectations(t)
	})

	t.Run("Mixed literal and regex filters", func(t *testing.T) {
		mockClient := &mocks.BaseClientAPI{}
		pageSize := int32(100)
		names := []string{"alpha", "beta", "gamma"}
		mockClient.On("GetRepositories", ctx, "", &pageSize).Return(acr.Repositories{Names: &names}, nil).Once()
		mockClient.On("GetRepositories", ctx, "gamma", &pageSize).Return(acr.Repositories{}, nil).Once()

		filters := []string{"alpha:v1", "bet.*:v2"}
		result, err := CollectTagFilters(ctx, filters, mockClient, 60, pageSize)
		assert.NoError(t, err)
		assert.Equal(t, "v1", result["alpha"])
		assert.Equal(t, "v2", result["beta"])
		_, hasGamma := result["gamma"]
		assert.False(t, hasGamma)
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

func TestGetUntaggedManifestsWithMinAndAgeCriteria(t *testing.T) {
	ctx := context.Background()
	repoName := "test-repo"
	mediaType := "application/vnd.docker.distribution.manifest.v2+json"
	cutoff := parseTime(t, "2024-11-01T12:00:00Z")

	t.Run("Paginated old manifests count locks and break minimum ties by digest", func(t *testing.T) {
		first := createManifestsResult([]manifestTestData{
			{digest: "sha256:c", lastUpdate: "2024-10-02T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:e", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:a", lastUpdate: "2024-10-03T12:00:00Z", mediaType: mediaType},
		})
		second := createManifestsResult([]manifestTestData{
			{digest: "sha256:f", lastUpdate: "2024-09-30T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:d", lastUpdate: "2024-10-02T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:b", lastUpdate: "2024-10-02T12:00:00Z", mediaType: mediaType},
		})
		*(*first.ManifestsAttributes)[1].ChangeableAttributes.WriteEnabled = false
		*(*first.ManifestsAttributes)[2].ChangeableAttributes.DeleteEnabled = false

		t.Run("Locked manifests occupy positions before exclusion", func(t *testing.T) {
			client := &mocks.AcrCLIClientInterface{}
			client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:a").Return(second, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:b").Return(createEmptyManifestsResult(), nil).Once()

			untaggedManifestsOptions := UntaggedManifestsOptions{
				PreserveAllOCIManifests: false,
				DryRun:                  false,
				IncludeLocked:           false,
				DeleteCutoff:            &cutoff,
				MinManifests:            2,
				MaxManifests:            -1,
			}
			result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
			assert.NoError(t, err)
			var digests []string
			for _, manifest := range result {
				digests = append(digests, *manifest.Digest)
			}
			assert.Equal(t, []string{"sha256:c", "sha256:d", "sha256:f"}, digests)
			client.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
		})

		t.Run("Include locked selects eligible locks but retains the old minimum", func(t *testing.T) {
			client := &mocks.AcrCLIClientInterface{}
			client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:a").Return(second, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:b").Return(createEmptyManifestsResult(), nil).Once()

			untaggedManifestsOptions := UntaggedManifestsOptions{
				PreserveAllOCIManifests: false,
				DryRun:                  false,
				IncludeLocked:           true,
				DeleteCutoff:            &cutoff,
				MinManifests:            2,
				MaxManifests:            -1,
			}
			result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
			assert.NoError(t, err)
			var digests []string
			for _, manifest := range result {
				digests = append(digests, *manifest.Digest)
			}
			assert.Equal(t, []string{"sha256:c", "sha256:d", "sha256:e", "sha256:f"}, digests)
			client.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
		})
	})

	t.Run("Young manifests occupy the minimum and cutoff equality is retained", func(t *testing.T) {
		client := &mocks.AcrCLIClientInterface{}
		page := createManifestsResult([]manifestTestData{
			{digest: "sha256:old", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:young", lastUpdate: "2024-11-03T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:cutoff", lastUpdate: "2024-11-01T12:00:00Z", mediaType: mediaType},
		})
		client.On("GetAcrManifests", ctx, repoName, "", "").Return(page, nil).Once()
		client.On("GetAcrManifests", ctx, repoName, "", "sha256:cutoff").Return(createEmptyManifestsResult(), nil).Once()

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            1,
			MaxManifests:            -1,
		}
		result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
		assert.NoError(t, err)
		if assert.Len(t, result, 1) {
			assert.Equal(t, "sha256:old", *result[0].Digest)
		}
		client.AssertExpectations(t)
	})

	t.Run("Zero minimum retains missing and malformed timestamps and their dependencies", func(t *testing.T) {
		for _, retainedType := range []string{mediaType, mediaTypeDockerManifestList, v1.MediaTypeImageIndex} {
			t.Run(retainedType, func(t *testing.T) {
				client := &mocks.AcrCLIClientInterface{}
				page := createManifestsResult([]manifestTestData{
					{digest: "sha256:missing", mediaType: retainedType},
					{digest: "sha256:invalid", lastUpdate: "invalid", mediaType: retainedType},
					{digest: "sha256:child-missing", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
					{digest: "sha256:child-invalid", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
					{digest: "sha256:free", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
				})
				client.On("GetAcrManifests", ctx, repoName, "", "").Return(page, nil).Once()
				client.On("GetAcrManifests", ctx, repoName, "", "sha256:free").Return(createEmptyManifestsResult(), nil).Once()
				expected := []string{"sha256:child-invalid", "sha256:child-missing", "sha256:free"}
				if retainedType != mediaType {
					reads := 1
					if retainedType == v1.MediaTypeImageIndex {
						reads = 2
					}
					for _, state := range []string{"missing", "invalid"} {
						content := []byte(fmt.Sprintf(`{"manifests":[{"digest":"sha256:child-%s","mediaType":%q}]}`, state, mediaType))
						client.On("GetManifest", ctx, repoName, "sha256:"+state).Return(content, nil).Times(reads)
					}
					expected = []string{"sha256:free"}
				}

				untaggedManifestsOptions := UntaggedManifestsOptions{
					PreserveAllOCIManifests: false,
					DryRun:                  false,
					IncludeLocked:           false,
					DeleteCutoff:            &cutoff,
					MinManifests:            0,
					MaxManifests:            -1,
				}
				result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
				assert.NoError(t, err)
				var digests []string
				for _, manifest := range result {
					digests = append(digests, *manifest.Digest)
				}
				assert.Equal(t, expected, digests)
				client.AssertExpectations(t)
			})
		}
	})

	t.Run("Retained index dependencies do not trigger replacement deletions", func(t *testing.T) {
		first := createManifestsResult([]manifestTestData{
			{digest: "sha256:child", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:parent", lastUpdate: "2024-10-03T12:00:00Z", mediaType: mediaTypeDockerManifestList},
		})
		second := createManifestsResult([]manifestTestData{
			{digest: "sha256:free", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:retained", lastUpdate: "2024-10-02T12:00:00Z", mediaType: mediaType},
		})
		content := []byte(fmt.Sprintf(`{"manifests":[{"digest":"sha256:child","mediaType":%q}]}`, mediaType))

		t.Run("Normal discovery", func(t *testing.T) {
			client := &mocks.AcrCLIClientInterface{}
			client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:parent").Return(second, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:retained").Return(createEmptyManifestsResult(), nil).Once()
			client.On("GetManifest", ctx, repoName, "sha256:parent").Return(content, nil).Once()

			untaggedManifestsOptions := UntaggedManifestsOptions{
				PreserveAllOCIManifests: false,
				DryRun:                  false,
				IncludeLocked:           false,
				DeleteCutoff:            &cutoff,
				MinManifests:            2,
				MaxManifests:            -1,
			}
			result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
			assert.NoError(t, err)
			if assert.Len(t, result, 1) {
				assert.Equal(t, "sha256:free", *result[0].Digest)
			}
			client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
		})

		t.Run("Dry run discovery", func(t *testing.T) {
			client := &mocks.AcrCLIClientInterface{}
			client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:parent").Return(second, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:retained").Return(createEmptyManifestsResult(), nil).Once()
			client.On("GetManifest", ctx, repoName, "sha256:parent").Return(content, nil).Once()

			untaggedManifestsOptions := UntaggedManifestsOptions{
				PreserveAllOCIManifests: false,
				DryRun:                  true,
				IncludeLocked:           false,
				DeleteCutoff:            &cutoff,
				MinManifests:            2,
				MaxManifests:            -1,
			}
			result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
			assert.NoError(t, err)
			if assert.Len(t, result, 1) {
				assert.Equal(t, "sha256:free", *result[0].Digest)
				assert.Equal(t, DeletionReasonAge, result[0].Reason)
			}
			client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
		})
	})
}

func TestGetUntaggedManifestsWithMinMaxAndAgeCriteria(t *testing.T) {
	ctx := context.Background()
	repoName := "test-repo"
	mediaType := "application/vnd.docker.distribution.manifest.v2+json"
	cutoff := parseTime(t, "2024-11-01T12:00:00Z")

	t.Run("Paginated young middle is retained but tied overflow is selected", func(t *testing.T) {
		first := createManifestsResult([]manifestTestData{
			{digest: "sha256:d", lastUpdate: "2024-11-03T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:f", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:a", lastUpdate: "2024-11-05T12:00:00Z", mediaType: mediaType},
		})
		second := createManifestsResult([]manifestTestData{
			{digest: "sha256:e", lastUpdate: "2024-11-02T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:b", lastUpdate: "2024-11-04T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:c", lastUpdate: "2024-11-03T12:00:00Z", mediaType: mediaType},
		})
		*(*first.ManifestsAttributes)[2].ChangeableAttributes.WriteEnabled = false
		*(*second.ManifestsAttributes)[0].ChangeableAttributes.DeleteEnabled = false

		t.Run("Locked overflow is excluded without shifting positions", func(t *testing.T) {
			client := &mocks.AcrCLIClientInterface{}
			client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:a").Return(second, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:c").Return(createEmptyManifestsResult(), nil).Once()

			untaggedManifestsOptions := UntaggedManifestsOptions{
				PreserveAllOCIManifests: false,
				DryRun:                  false,
				IncludeLocked:           false,
				DeleteCutoff:            &cutoff,
				MinManifests:            1,
				MaxManifests:            3,
			}
			result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
			assert.NoError(t, err)
			var digests []string
			for _, manifest := range result {
				digests = append(digests, *manifest.Digest)
			}
			assert.Equal(t, []string{"sha256:d", "sha256:f"}, digests)
			assert.Equal(t, DeletionReasonMaximumCount, result[0].Reason)
			assert.Equal(t, DeletionReasonAgeAndMaximumCount, result[1].Reason)
			client.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
		})

		t.Run("Include locked selects overflow without unlocking it", func(t *testing.T) {
			client := &mocks.AcrCLIClientInterface{}
			client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:a").Return(second, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:c").Return(createEmptyManifestsResult(), nil).Once()

			untaggedManifestsOptions := UntaggedManifestsOptions{
				PreserveAllOCIManifests: false,
				DryRun:                  false,
				IncludeLocked:           true,
				DeleteCutoff:            &cutoff,
				MinManifests:            1,
				MaxManifests:            3,
			}
			result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
			assert.NoError(t, err)
			var digests []string
			for _, manifest := range result {
				digests = append(digests, *manifest.Digest)
			}
			assert.Equal(t, []string{"sha256:d", "sha256:e", "sha256:f"}, digests)
			client.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
		})
	})

	t.Run("Old minimum is retained while old middle and overflow are selected", func(t *testing.T) {
		client := &mocks.AcrCLIClientInterface{}
		first := createManifestsResult([]manifestTestData{
			{digest: "sha256:b", lastUpdate: "2024-10-03T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:d", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
		})
		second := createManifestsResult([]manifestTestData{
			{digest: "sha256:c", lastUpdate: "2024-10-02T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:a", lastUpdate: "2024-10-03T12:00:00Z", mediaType: mediaType},
		})
		client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
		client.On("GetAcrManifests", ctx, repoName, "", "sha256:d").Return(second, nil).Once()
		client.On("GetAcrManifests", ctx, repoName, "", "sha256:a").Return(createEmptyManifestsResult(), nil).Once()

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            1,
			MaxManifests:            3,
		}
		result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
		assert.NoError(t, err)
		var digests []string
		for _, manifest := range result {
			digests = append(digests, *manifest.Digest)
		}
		assert.Equal(t, []string{"sha256:b", "sha256:c", "sha256:d"}, digests)
		client.AssertExpectations(t)
	})

	t.Run("Equal bounds retain exactly the minimum even when overflow is young", func(t *testing.T) {
		client := &mocks.AcrCLIClientInterface{}
		page := createManifestsResult([]manifestTestData{
			{digest: "sha256:b", lastUpdate: "2024-11-03T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:a", lastUpdate: "2024-11-03T12:00:00Z", mediaType: mediaType},
		})
		client.On("GetAcrManifests", ctx, repoName, "", "").Return(page, nil).Once()
		client.On("GetAcrManifests", ctx, repoName, "", "sha256:a").Return(createEmptyManifestsResult(), nil).Once()

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            1,
			MaxManifests:            1,
		}
		result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
		assert.NoError(t, err)
		if assert.Len(t, result, 1) {
			assert.Equal(t, "sha256:b", *result[0].Digest)
		}
		client.AssertExpectations(t)
	})

	t.Run("Zero bounds select valid timestamps regardless of age", func(t *testing.T) {
		client := &mocks.AcrCLIClientInterface{}
		page := createManifestsResult([]manifestTestData{
			{digest: "sha256:old", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:missing", mediaType: mediaType},
			{digest: "sha256:young", lastUpdate: "2024-11-03T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:invalid", lastUpdate: "invalid", mediaType: mediaType},
		})
		client.On("GetAcrManifests", ctx, repoName, "", "").Return(page, nil).Once()
		client.On("GetAcrManifests", ctx, repoName, "", "sha256:invalid").Return(createEmptyManifestsResult(), nil).Once()

		untaggedManifestsOptions := UntaggedManifestsOptions{
			PreserveAllOCIManifests: false,
			DryRun:                  false,
			IncludeLocked:           false,
			DeleteCutoff:            &cutoff,
			MinManifests:            0,
			MaxManifests:            0,
		}
		result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
		assert.NoError(t, err)
		var digests []string
		for _, manifest := range result {
			digests = append(digests, *manifest.Digest)
		}
		assert.Equal(t, []string{"sha256:young", "sha256:old"}, digests)
		client.AssertExpectations(t)
	})
}

func TestGetUntaggedManifestsWithMaxCriteria(t *testing.T) {
	ctx := context.Background()
	repoName := "test-repo"
	mediaType := "application/vnd.docker.distribution.manifest.v2+json"

	t.Run("Paginated recent overflow uses digest order and counts locked manifests", func(t *testing.T) {
		first := createManifestsResult([]manifestTestData{
			{digest: "sha256:c", lastUpdate: "2024-11-03T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:e", lastUpdate: "2024-11-02T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:a", lastUpdate: "2024-11-04T12:00:00Z", mediaType: mediaType},
		})
		second := createManifestsResult([]manifestTestData{
			{digest: "sha256:f", lastUpdate: "2024-11-01T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:d", lastUpdate: "2024-11-03T12:00:00Z", mediaType: mediaType},
			{digest: "sha256:b", lastUpdate: "2024-11-03T12:00:00Z", mediaType: mediaType},
		})
		*(*first.ManifestsAttributes)[1].ChangeableAttributes.DeleteEnabled = false
		*(*first.ManifestsAttributes)[2].ChangeableAttributes.WriteEnabled = false

		t.Run("Locked overflow is retained without a replacement deletion", func(t *testing.T) {
			client := &mocks.AcrCLIClientInterface{}
			client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:a").Return(second, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:b").Return(createEmptyManifestsResult(), nil).Once()

			untaggedManifestsOptions := UntaggedManifestsOptions{
				PreserveAllOCIManifests: false,
				DryRun:                  false,
				IncludeLocked:           false,
				DeleteCutoff:            nil,
				MinManifests:            -1,
				MaxManifests:            2,
			}
			result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
			assert.NoError(t, err)
			var digests []string
			for _, manifest := range result {
				digests = append(digests, *manifest.Digest)
				assert.Equal(t, DeletionReasonMaximumCount, manifest.Reason)
			}
			assert.Equal(t, []string{"sha256:c", "sha256:d", "sha256:f"}, digests)
			client.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
		})

		t.Run("Include locked selects overflow but does not mutate manifests", func(t *testing.T) {
			client := &mocks.AcrCLIClientInterface{}
			client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:a").Return(second, nil).Once()
			client.On("GetAcrManifests", ctx, repoName, "", "sha256:b").Return(createEmptyManifestsResult(), nil).Once()

			untaggedManifestsOptions := UntaggedManifestsOptions{
				PreserveAllOCIManifests: false,
				DryRun:                  false,
				IncludeLocked:           true,
				DeleteCutoff:            nil,
				MinManifests:            -1,
				MaxManifests:            2,
			}
			result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
			assert.NoError(t, err)
			var digests []string
			for _, manifest := range result {
				digests = append(digests, *manifest.Digest)
			}
			assert.Equal(t, []string{"sha256:c", "sha256:d", "sha256:e", "sha256:f"}, digests)
			client.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
		})
	})

	t.Run("Zero maximum retains missing and malformed timestamps and their dependencies", func(t *testing.T) {
		for _, retainedType := range []string{mediaType, mediaTypeDockerManifestList, v1.MediaTypeImageIndex} {
			t.Run(retainedType, func(t *testing.T) {
				client := &mocks.AcrCLIClientInterface{}
				page := createManifestsResult([]manifestTestData{
					{digest: "sha256:missing", mediaType: retainedType},
					{digest: "sha256:invalid", lastUpdate: "invalid", mediaType: retainedType},
					{digest: "sha256:child-missing", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
					{digest: "sha256:child-invalid", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
					{digest: "sha256:free", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
				})
				client.On("GetAcrManifests", ctx, repoName, "", "").Return(page, nil).Once()
				client.On("GetAcrManifests", ctx, repoName, "", "sha256:free").Return(createEmptyManifestsResult(), nil).Once()
				expected := []string{"sha256:child-invalid", "sha256:child-missing", "sha256:free"}
				if retainedType != mediaType {
					reads := 1
					if retainedType == v1.MediaTypeImageIndex {
						reads = 2
					}
					for _, state := range []string{"missing", "invalid"} {
						content := []byte(fmt.Sprintf(`{"manifests":[{"digest":"sha256:child-%s","mediaType":%q}]}`, state, mediaType))
						client.On("GetManifest", ctx, repoName, "sha256:"+state).Return(content, nil).Times(reads)
					}
					expected = []string{"sha256:free"}
				}

				untaggedManifestsOptions := UntaggedManifestsOptions{
					PreserveAllOCIManifests: false,
					DryRun:                  false,
					IncludeLocked:           false,
					DeleteCutoff:            nil,
					MinManifests:            -1,
					MaxManifests:            0,
				}
				result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
				assert.NoError(t, err)
				var digests []string
				for _, manifest := range result {
					digests = append(digests, *manifest.Digest)
				}
				assert.Equal(t, expected, digests)
				client.AssertExpectations(t)
			})
		}
	})

	t.Run("Old retained indexes protect children and referrers without replacement deletions", func(t *testing.T) {
		for _, indexType := range []string{mediaTypeDockerManifestList, v1.MediaTypeImageIndex} {
			t.Run(indexType, func(t *testing.T) {
				first := createManifestsResult([]manifestTestData{
					{digest: "sha256:child", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
					{digest: "sha256:parent", lastUpdate: "2024-10-03T12:00:00Z", mediaType: indexType},
				})
				second := createManifestsResult([]manifestTestData{
					{digest: "sha256:referrer", lastUpdate: "2024-11-03T12:00:00Z", mediaType: v1.MediaTypeImageManifest},
					{digest: "sha256:free", lastUpdate: "2024-10-01T12:00:00Z", mediaType: mediaType},
					{digest: "sha256:retained", lastUpdate: "2024-10-02T12:00:00Z", mediaType: mediaType},
				})
				*(*first.ManifestsAttributes)[1].ChangeableAttributes.DeleteEnabled = false
				content := []byte(fmt.Sprintf(`{"manifests":[{"digest":"sha256:child","mediaType":%q}]}`, mediaType))
				referrerContent := []byte(`{"subject":{"digest":"sha256:free"}}`)
				parentReads := 1
				if indexType == v1.MediaTypeImageIndex {
					parentReads = 2
				}

				t.Run("Normal discovery counts the retained locked index", func(t *testing.T) {
					client := &mocks.AcrCLIClientInterface{}
					client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
					client.On("GetAcrManifests", ctx, repoName, "", "sha256:parent").Return(second, nil).Once()
					client.On("GetAcrManifests", ctx, repoName, "", "sha256:retained").Return(createEmptyManifestsResult(), nil).Once()
					client.On("GetManifest", ctx, repoName, "sha256:parent").Return(content, nil).Times(parentReads)
					client.On("GetManifest", ctx, repoName, "sha256:referrer").Return(referrerContent, nil).Once()

					untaggedManifestsOptions := UntaggedManifestsOptions{
						PreserveAllOCIManifests: false,
						DryRun:                  false,
						IncludeLocked:           false,
						DeleteCutoff:            nil,
						MinManifests:            -1,
						MaxManifests:            2,
					}
					result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
					assert.NoError(t, err)
					if assert.Len(t, result, 1) {
						assert.Equal(t, "sha256:free", *result[0].Digest)
					}
					client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
					client.AssertExpectations(t)
				})

				t.Run("Include locked still protects retained index children", func(t *testing.T) {
					client := &mocks.AcrCLIClientInterface{}
					client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
					client.On("GetAcrManifests", ctx, repoName, "", "sha256:parent").Return(second, nil).Once()
					client.On("GetAcrManifests", ctx, repoName, "", "sha256:retained").Return(createEmptyManifestsResult(), nil).Once()
					client.On("GetManifest", ctx, repoName, "sha256:parent").Return(content, nil).Times(parentReads)
					client.On("GetManifest", ctx, repoName, "sha256:referrer").Return(referrerContent, nil).Once()

					untaggedManifestsOptions := UntaggedManifestsOptions{
						PreserveAllOCIManifests: false,
						DryRun:                  false,
						IncludeLocked:           true,
						DeleteCutoff:            nil,
						MinManifests:            -1,
						MaxManifests:            2,
					}
					result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
					assert.NoError(t, err)
					if assert.Len(t, result, 1) {
						assert.Equal(t, "sha256:free", *result[0].Digest)
					}
					client.AssertNotCalled(t, "UpdateAcrManifestAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
					client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
					client.AssertExpectations(t)
				})

				t.Run("Dry run preserves the same dependencies and minimum count", func(t *testing.T) {
					client := &mocks.AcrCLIClientInterface{}
					client.On("GetAcrManifests", ctx, repoName, "", "").Return(first, nil).Once()
					client.On("GetAcrManifests", ctx, repoName, "", "sha256:parent").Return(second, nil).Once()
					client.On("GetAcrManifests", ctx, repoName, "", "sha256:retained").Return(createEmptyManifestsResult(), nil).Once()
					client.On("GetManifest", ctx, repoName, "sha256:parent").Return(content, nil).Times(parentReads)
					client.On("GetManifest", ctx, repoName, "sha256:referrer").Return(referrerContent, nil).Once()

					untaggedManifestsOptions := UntaggedManifestsOptions{
						PreserveAllOCIManifests: false,
						DryRun:                  true,
						IncludeLocked:           false,
						DeleteCutoff:            nil,
						MinManifests:            -1,
						MaxManifests:            2,
					}
					result, err := GetUntaggedManifests(ctx, 1, client, repoName, untaggedManifestsOptions, nil)
					assert.NoError(t, err)
					if assert.Len(t, result, 1) {
						assert.Equal(t, "sha256:free", *result[0].Digest)
					}
					client.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything, mock.Anything)
					client.AssertExpectations(t)
				})
			})
		}
	})
}
