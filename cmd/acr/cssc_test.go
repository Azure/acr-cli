package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/go-autorest/autorest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestNewCsscCmd(t *testing.T) {
	rootParams := &rootParameters{}
	cmd := newCsscCmd(rootParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "cssc", cmd.Use)
	assert.Equal(t, newCsscCmdLongMessage, cmd.Long)
}

func TestNewPatchFilterCmd(t *testing.T) {
	rootParams := &rootParameters{}
	csscParams := csscParameters{rootParameters: rootParams}
	cmd := newPatchFilterCmd(&csscParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "patch", cmd.Use)
	assert.Equal(t, newPatchFilterCmdLongMessage, cmd.Long)
}

func TestGetAndFilterRepositories(t *testing.T) {
	mockAcrClient := &mocks.AcrCLIClientInterface{}

	// 1. If Repository is not specified in the filter, return an empty list
	t.Run("RepositoryNotSpecifiedTest", func(t *testing.T) {
		filter := []Filter{
			{
				Repository: "",
				Tags:       []string{csscTestTag1, csscTestTag2},
				Enabled:    boolPtr(true),
			},
		}
		filteredRepositories, err := getAndFilterRepositories(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Nil(t, filteredRepositories)
		assert.Len(t, filteredRepositories, 0)
	})

	//2.  If Tag is not specified in the filter, return an empty list
	t.Run("TagsNotSpecifiedTest", func(t *testing.T) {
		filter := []Filter{
			{
				Repository: csscTestRepo1,
				Tags:       nil,
				Enabled:    boolPtr(true),
			},
			{
				Repository: csscTestRepo2,
				Tags:       []string{""},
				Enabled:    boolPtr(true),
			},
		}
		filteredRepositories, err := getAndFilterRepositories(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Nil(t, filteredRepositories)
		assert.Len(t, filteredRepositories, 0)
	})

	//3. Error should be returned when GetAcrTags fails for a repository
	t.Run("GetAcrTagsFailsTest", func(t *testing.T) {
		filter := []Filter{
			{
				Repository: csscTestRepo1,
				Tags:       []string{csscTestTag1, csscTestTag2},
				Enabled:    boolPtr(true),
			},
		}
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo1, "", "").Return(nil, errors.New("failed getting the tags")).Once()
		filteredRepositories, err := getAndFilterRepositories(context.Background(), mockAcrClient, filter)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "failed getting the tags")
		assert.Nil(t, filteredRepositories)
	})

	//4. If filter has a tag that doesn't exist in the repository, ignore it and return what exists and matches the filter
	t.Run("TagSpecifiedInFilterDoesNotExistTest", func(t *testing.T) {
		filter := []Filter{
			{
				Repository: csscTestRepo1,
				Tags:       []string{csscTestTag1, csscTestTag2},
				Enabled:    boolPtr(true),
			},
		}
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo1, "", "").Return(CsscTestOneTagResult, nil).Once()
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo1, "", csscTestTag1).Return(EmptyListTagsResult, nil).Once()

		filteredRepositories, err := getAndFilterRepositories(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Len(t, filteredRepositories, 1) //
		assert.Equal(t, csscTestRepo1, filteredRepositories[0].Repository)
		assert.Equal(t, csscTestTag1, filteredRepositories[0].Tag)
	})

	// 5. Success scenario with all the combination of filters
	t.Run("AllFilterCombinationTest", func(t *testing.T) {
		filter := []Filter{
			{
				Repository: csscTestRepo1,
				Tags:       []string{csscTestTag1, csscTestTag2}, // tags specified
				Enabled:    boolPtr(true),
			},
			{
				Repository: csscTestRepo2,
				Tags:       []string{"*"}, // * all tags
				Enabled:    boolPtr(true),
			},
			{
				Repository: csscTestRepo3,
				Tags:       []string{csscTestTag1, csscTestTag2},
				Enabled:    nil, // nil means enabled
			},
			{
				Repository: csscTestRepo4,
				Tags:       []string{csscTestTag1, csscTestTag2},
				Enabled:    boolPtr(false), // disabled repository for all tags
			},
		}
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo1, "", "").Return(CsscTestTagResult, nil).Once()
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo1, "", csscTestPatch2).Return(EmptyListTagsResult, nil).Once()
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo2, "", "").Return(CsscTestTagResult, nil).Once()
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo2, "", csscTestPatch2).Return(EmptyListTagsResult, nil).Once()
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo3, "", "").Return(CsscTestTagResult, nil).Once()
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo3, "", csscTestPatch2).Return(EmptyListTagsResult, nil).Once()
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo4, "", "").Return(CsscTestTagResult, nil).Once()
		mockAcrClient.On("GetAcrTags", csscTestCtx, csscTestRepo4, "", csscTestPatch2).Return(EmptyListTagsResult, nil).Once()

		filteredRepositories, err := getAndFilterRepositories(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Len(t, filteredRepositories, 6)
		assert.Equal(t, csscTestRepo1, filteredRepositories[0].Repository)
		assert.Equal(t, csscTestTag1, filteredRepositories[0].Tag)
		assert.Equal(t, csscTestPatch1, filteredRepositories[0].PatchTag)
		assert.Equal(t, csscTestRepo1, filteredRepositories[1].Repository)
		assert.Equal(t, csscTestTag2, filteredRepositories[1].Tag)
		assert.Equal(t, csscTestPatch2, filteredRepositories[1].PatchTag)

		assert.Equal(t, csscTestRepo2, filteredRepositories[2].Repository)
		assert.Equal(t, csscTestTag1, filteredRepositories[2].Tag)
		assert.Equal(t, csscTestPatch1, filteredRepositories[2].PatchTag)
		assert.Equal(t, csscTestRepo2, filteredRepositories[3].Repository)
		assert.Equal(t, csscTestTag2, filteredRepositories[3].Tag)
		assert.Equal(t, csscTestPatch2, filteredRepositories[3].PatchTag)

		assert.Equal(t, csscTestRepo3, filteredRepositories[4].Repository)
		assert.Equal(t, csscTestTag1, filteredRepositories[4].Tag)
		assert.Equal(t, csscTestPatch1, filteredRepositories[4].PatchTag)
		assert.Equal(t, csscTestRepo3, filteredRepositories[5].Repository)
		assert.Equal(t, csscTestTag2, filteredRepositories[5].Tag)
		assert.Equal(t, csscTestPatch2, filteredRepositories[5].PatchTag)
	})
}

func TestListRepositoriesAndTagsMatchingFilterPolicy(t *testing.T) {
	rootParams := &rootParameters{}
	rootParams.username = "username"
	rootParams.password = "password"
	csscParams := csscParameters{rootParameters: rootParams}
	loginURL := testLoginURL
	mockAcrClient := &mocks.AcrCLIClientInterface{}

	//1. Error should be returned when filter policy is not in the correct format
	t.Run("FilterPolicyNotCorrectFormatTest", func(t *testing.T) {
		csscParams.filterPolicy = "notcorrectformat"
		err := listRepositoriesAndTagsMatchingFilterPolicy(context.Background(), &csscParams, loginURL, mockAcrClient)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.Equal(t, "--filter-policy should be in the format repo:tag", err.Error())
	})

	//2. Error should be returned when fetching repository manifest fails
	t.Run("FetchBytesFailsTest", func(t *testing.T) {
		csscParams.filterPolicy = "repo1:tag1"
		err := listRepositoriesAndTagsMatchingFilterPolicy(context.Background(), &csscParams, loginURL, mockAcrClient)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.ErrorContains(t, err, "Error fetching manifest by tag for the repository and tag specified in the filter policy")
	})
}

func TestGetRegistryCredsFromStore(t *testing.T) {
	rootParams := &rootParameters{}
	rootParams.configs = []string{"config1", "config2"}
	csscParams := csscParameters{rootParameters: rootParams}
	loginURL := testLoginURL

	// 1. Should not get the creds from the store when creds are provided
	t.Run("CredsProvidedTest", func(t *testing.T) {
		rootParams.username = "username"
		rootParams.password = "password"
		getRegistryCredsFromStore(&csscParams, loginURL)
		assert.Equal(t, "username", csscParams.username)
		assert.Equal(t, "password", csscParams.password)
	})

	// 2. When creds are not provided, should get the creds from the store
	t.Run("CredsNotProvidedTest", func(t *testing.T) {
		rootParams.username = ""
		rootParams.password = ""
		getRegistryCredsFromStore(&csscParams, loginURL)
		assert.Equal(t, "", csscParams.username)
		assert.Equal(t, "", csscParams.password)
	})
}

// Test appending element to a slice which does not contain the element. It should be appended.
func TestAppendElement(t *testing.T) {

	// 1. Should append the element to the slice when the slice does not contain the element
	t.Run("AppendElementTest", func(t *testing.T) {
		slice := []FilteredRepository{
			{
				Repository: csscTestRepo1,
				Tag:        csscTestTag1,
				PatchTag:   csscTestPatch1,
			},
		}
		element := FilteredRepository{
			Repository: csscTestRepo2,
			Tag:        csscTestTag2,
			PatchTag:   csscTestPatch2,
		}
		newSlice := appendElement(slice, element)
		assert.Len(t, newSlice, 2)
		assert.Equal(t, csscTestRepo1, newSlice[0].Repository)
		assert.Equal(t, csscTestTag1, newSlice[0].Tag)
		assert.Equal(t, csscTestPatch1, newSlice[0].PatchTag)
		assert.Equal(t, csscTestRepo2, newSlice[1].Repository)
		assert.Equal(t, csscTestTag2, newSlice[1].Tag)
		assert.Equal(t, csscTestPatch2, newSlice[1].PatchTag)
	})

	// 2. Should not append the element to the slice when the slice already contains the element
	t.Run("AppendElementAlreadyExistsTest", func(t *testing.T) {
		slice := []FilteredRepository{
			{
				Repository: csscTestRepo1,
				Tag:        csscTestTag1,
				PatchTag:   csscTestPatch1,
			},
		}
		element := FilteredRepository{
			Repository: csscTestRepo1,
			Tag:        csscTestTag1,
			PatchTag:   csscTestPatch1,
		}
		newSlice := appendElement(slice, element)
		assert.Len(t, newSlice, 1)
		assert.Equal(t, csscTestRepo1, newSlice[0].Repository)
		assert.Equal(t, csscTestTag1, newSlice[0].Tag)
		assert.Equal(t, csscTestPatch1, newSlice[0].PatchTag)
	})
}

// Helper function to create a pointer to a bool
func boolPtr(v bool) *bool {
	return &v
}

// All variables used in the tests
var (
	csscTestCtx      = context.Background()
	csscTestLoginURL = "foo.azurecr.io"
	csscTestRegistry = "registry"
	csscTestRepo1    = "repo1"
	csscTestRepo2    = "repo2"
	csscTestRepo3    = "repo3"
	csscTestRepo4    = "repo4"
	csscTestRepo5    = "repo5"
	csscTestRepo6    = "repo6"
	csscTestTag1     = "tag1"
	csscTestTag2     = "tag2"
	csscTestPatch1   = "tag1-patched"
	csscTestPatch2   = "tag2-patched"

	CsscTestTagResult = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &csscTestRegistry,
		ImageName: &csscTestRepo1,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name: &csscTestTag1,
			},
			{
				Name: &csscTestPatch1,
			},
			{
				Name: &csscTestTag2,
			},
			{
				Name: &csscTestPatch2,
			},
		},
	}

	CsscTestOneTagResult = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &csscTestRegistry,
		ImageName: &csscTestRepo1,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name: &csscTestTag1,
			},
		},
	}
)
