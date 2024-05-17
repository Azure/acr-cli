// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cssc

import (
	"context"
	"os"
	"testing"

	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/acr-cli/internal/common"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestApplyFilterAndGetFilteredList(t *testing.T) {
	mockAcrClient := &mocks.AcrCLIClientInterface{}
	// 1. If filter contains only one repository and it is not specified, nothing will match the filter and an empty list should be returned
	t.Run("RepositoryNotSpecifiedTest", func(t *testing.T) {
		filter := Filter{
			Version: "v1",
			Repositories: []Repository{
				{
					Repository: "",
					Tags:       []string{common.TagName1, common.TagName2},
					Enabled:    boolPtr(true),
				},
			},
		}
		filteredRepositories, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Nil(t, filteredRepositories)
		assert.Len(t, filteredRepositories, 0)
	})
	//2.  If Tags are not specified for any repository in the filter, nothing will match the filter and an empty list should be returned
	t.Run("TagsNotSpecifiedTest", func(t *testing.T) {
		filter := Filter{
			Version: "v1",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       nil,
					Enabled:    boolPtr(true),
				},
				{
					Repository: common.RepoName2,
					Tags:       []string{""},
					Enabled:    boolPtr(true),
				},
			},
		}
		filteredRepositories, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Nil(t, filteredRepositories)
		assert.Len(t, filteredRepositories, 0)
	})
	//3. Error should be returned when GetAcrTags fails for a repository
	t.Run("GetAcrTagsFailsTest", func(t *testing.T) {
		filter := Filter{
			Version: "v1",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{common.TagName1, common.TagName2},
					Enabled:    boolPtr(true),
				},
			},
		}
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName1, "", "").Return(nil, errors.New("failed getting the tags")).Once()
		filteredRepositories, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "failed getting the tags")
		assert.Nil(t, filteredRepositories)
	})
	//4. If filter has a tag that doesn't exist in the repository, ignore it and return whatever exists that matches the filter
	t.Run("TagSpecifiedInFilterDoesNotExistTest", func(t *testing.T) {
		filter := Filter{
			Version: "v1",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{common.TagName, common.TagName1},
					Enabled:    boolPtr(true),
				},
			},
		}
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName1, "", "").Return(common.OneTagResult, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName1, "", common.TagName).Return(common.EmptyListTagsResult, nil).Once()
		filteredRepositories, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Len(t, filteredRepositories, 1)
		assert.Equal(t, common.RepoName1, filteredRepositories[0].Repository)
		assert.Equal(t, common.TagName, filteredRepositories[0].Tag)
	})
	// 5. Success scenario with all the combination of filters
	t.Run("AllFilterCombinationTest", func(t *testing.T) {
		filter := Filter{
			Version: "v1",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{common.TagName1, common.TagName2}, // tags specified
					Enabled:    boolPtr(true),
				},
				{
					Repository: common.RepoName2,
					Tags:       []string{"*"}, // * all tags
					Enabled:    boolPtr(true),
				},
				{
					Repository: common.RepoName3,
					Tags:       []string{common.TagName1, common.TagName2},
					Enabled:    nil, // nil means enabled
				},
				{
					Repository: common.RepoName4,
					Tags:       []string{common.TagName1, common.TagName2},
					Enabled:    boolPtr(false), // disabled repository for all tags
				},
			},
		}
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName1, "", "").Return(common.FourTagResultWithPatchTags, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName1, "", common.TagNamePatch2).Return(common.EmptyListTagsResult, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName2, "", "").Return(common.FourTagResultWithPatchTags, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName2, "", common.TagNamePatch2).Return(common.EmptyListTagsResult, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName3, "", "").Return(common.FourTagResultWithPatchTags, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName3, "", common.TagNamePatch2).Return(common.EmptyListTagsResult, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName4, "", "").Return(common.FourTagResultWithPatchTags, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName4, "", common.TagNamePatch2).Return(common.EmptyListTagsResult, nil).Once()
		filteredRepositories, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Len(t, filteredRepositories, 6)
		assert.Equal(t, common.RepoName1, filteredRepositories[0].Repository)
		assert.Equal(t, common.TagName1, filteredRepositories[0].Tag)
		assert.Equal(t, common.TagNamePatch1, filteredRepositories[0].PatchTag)
		assert.Equal(t, common.RepoName1, filteredRepositories[1].Repository)
		assert.Equal(t, common.TagName2, filteredRepositories[1].Tag)
		assert.Equal(t, common.TagNamePatch2, filteredRepositories[1].PatchTag)
		assert.Equal(t, common.RepoName2, filteredRepositories[2].Repository)
		assert.Equal(t, common.TagName1, filteredRepositories[2].Tag)
		assert.Equal(t, common.TagNamePatch1, filteredRepositories[2].PatchTag)
		assert.Equal(t, common.RepoName2, filteredRepositories[3].Repository)
		assert.Equal(t, common.TagName2, filteredRepositories[3].Tag)
		assert.Equal(t, common.TagNamePatch2, filteredRepositories[3].PatchTag)
		assert.Equal(t, common.RepoName3, filteredRepositories[4].Repository)
		assert.Equal(t, common.TagName1, filteredRepositories[4].Tag)
		assert.Equal(t, common.TagNamePatch1, filteredRepositories[4].PatchTag)
		assert.Equal(t, common.RepoName3, filteredRepositories[5].Repository)
		assert.Equal(t, common.TagName2, filteredRepositories[5].Tag)
		assert.Equal(t, common.TagNamePatch2, filteredRepositories[5].PatchTag)
	})
}

func TestGetFilterFromFilterPolicy(t *testing.T) {
	username := "username"
	password := "password"
	//1. Error should be returned when filter policy is not in the correct format
	t.Run("FilterPolicyNotCorrectFormatTest", func(t *testing.T) {
		filterPolicy := "notcorrectformat"
		filter, err := GetFilterFromFilterPolicy(context.Background(), filterPolicy, common.TestLoginURL, username, password)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.Equal(t, "filter-policy should be in the format repo:tag", err.Error())
		assert.Equal(t, Filter{}, filter)
	})
	//2. Error should be returned when fetching repository manifest fails
	t.Run("FetchBytesFailsTest", func(t *testing.T) {
		filterPolicy := "repo1:tag1"
		filter, err := GetFilterFromFilterPolicy(context.Background(), filterPolicy, common.TestLoginURL, username, password)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.ErrorContains(t, err, "error fetching manifest content when reading the filter policy")
		assert.Equal(t, Filter{}, filter)
	})
}

func TestGetFilterFromFilePath(t *testing.T) {
	//1. Error should be returned when filter file does not exist
	t.Run("FileDoesNotExistTest", func(t *testing.T) {
		filter, err := GetFilterFromFilePath("idontexist")
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.ErrorContains(t, err, "error reading the filter json file from file path")
		assert.Equal(t, Filter{}, filter)
	})
	//2. Error should be returned when filter file is not in the correct format
	t.Run("FileNotCorrectFormatTest", func(t *testing.T) {
		var filterFile = []byte(`i am not a json file`)
		err := os.WriteFile("filter-wrongformat.json", filterFile, 0600)
		assert.Nil(t, err, "Error should be nil")
		filter, err := GetFilterFromFilePath("filter-wrongformat.json")
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.ErrorContains(t, err, "error unmarshalling json content when reading the filter file from file path")
		assert.Equal(t, Filter{}, filter)
		err = os.Remove("filter-wrongformat.json")
		assert.Nil(t, err, "Error should be nil")
	})
	//3. Success scenario with correct filter file
	t.Run("SuccessTest", func(t *testing.T) {
		var filterFile = []byte(`{
			"version": "v1",
			"repositories": [
				{
					"repository": "repo1",
					"tags": ["v1", "v2"],
					"enabled": true
				},
				{
					"repository": "repo2",
					"tags": ["v1"],
					"enabled": true
				}
			]
		}`)
		err := os.WriteFile("filter.json", filterFile, 0600)
		assert.Nil(t, err, "Error should be nil")
		filter, err := GetFilterFromFilePath("filter.json")
		assert.Nil(t, err, "Error should be nil")
		assert.Equal(t, "v1", filter.Version)
		assert.Len(t, filter.Repositories, 2)
		assert.Equal(t, common.RepoName1, filter.Repositories[0].Repository)
		assert.Equal(t, common.TagName1, filter.Repositories[0].Tags[0])
		assert.Equal(t, common.TagName2, filter.Repositories[0].Tags[1])
		assert.Equal(t, true, *filter.Repositories[0].Enabled)
		assert.Equal(t, common.RepoName2, filter.Repositories[1].Repository)
		assert.Equal(t, common.TagName1, filter.Repositories[1].Tags[0])
		assert.Equal(t, true, *filter.Repositories[1].Enabled)
		err = os.Remove("filter.json")
		assert.Nil(t, err, "Error should be nil")
	})
}

func TestAppendElement(t *testing.T) {
	// 1. Should append the element to the slice when the slice does not contain the element
	t.Run("AppendElementTest", func(t *testing.T) {
		slice := []FilteredRepository{
			{
				Repository: common.RepoName1,
				Tag:        common.TagName1,
				PatchTag:   common.TagNamePatch1,
			},
		}
		element := FilteredRepository{
			Repository: common.RepoName2,
			Tag:        common.TagName2,
			PatchTag:   common.TagNamePatch2,
		}
		newSlice := AppendElement(slice, element)
		assert.Len(t, newSlice, 2)
		assert.Equal(t, common.RepoName1, newSlice[0].Repository)
		assert.Equal(t, common.TagName1, newSlice[0].Tag)
		assert.Equal(t, common.TagNamePatch1, newSlice[0].PatchTag)
		assert.Equal(t, common.RepoName2, newSlice[1].Repository)
		assert.Equal(t, common.TagName2, newSlice[1].Tag)
		assert.Equal(t, common.TagNamePatch2, newSlice[1].PatchTag)
	})
	// 2. Should not append the element to the slice when the slice already contains the element
	t.Run("AppendElementAlreadyExistsTest", func(t *testing.T) {
		slice := []FilteredRepository{
			{
				Repository: common.RepoName1,
				Tag:        common.TagName1,
				PatchTag:   common.TagNamePatch1,
			},
		}
		element := FilteredRepository{
			Repository: common.RepoName1,
			Tag:        common.TagName1,
			PatchTag:   common.TagNamePatch1,
		}
		newSlice := AppendElement(slice, element)
		assert.Len(t, newSlice, 1)
		assert.Equal(t, common.RepoName1, newSlice[0].Repository)
		assert.Equal(t, common.TagName1, newSlice[0].Tag)
		assert.Equal(t, common.TagNamePatch1, newSlice[0].PatchTag)
	})
}

// Helper function to create a pointer to a bool
func boolPtr(v bool) *bool {
	return &v
}
