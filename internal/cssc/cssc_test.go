// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cssc

import (
	"context"
	"os"
	"testing"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/acr-cli/internal/common"
	"github.com/Azure/acr-cli/internal/tag"
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
		filteredRepositories, artifactsNotFound, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Nil(t, artifactsNotFound)
		assert.Len(t, artifactsNotFound, 0)
		assert.Nil(t, filteredRepositories)
		assert.Len(t, filteredRepositories, 0)
	})
	// 2. If Tags are not specified for any repository in the filter, nothing will match the filter and an empty list should be returned
	t.Run("TagsNotSpecifiedTest", func(t *testing.T) {
		filter := Filter{
			Version: "v1",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       nil,
					Enabled:    boolPtr(true),
				},
			},
		}
		filteredRepositories, artifactsNotFound, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Nil(t, artifactsNotFound)
		assert.Len(t, artifactsNotFound, 0)
		assert.Nil(t, filteredRepositories)
		assert.Len(t, filteredRepositories, 0)
	})
	// 3. No error should be returned when GetAcrTags fails with ListTagsError and the tags that failed to be fetched should be returned in the artifactsNotFound list
	t.Run("GetAcrTagsFailsWithListTagsErrorTest", func(t *testing.T) {
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
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName1, "", "").Return(nil, errors.Wrap(&tag.ListTagsError{}, "failed to list tags")).Once()
		filteredRepositories, artifactsNotFound, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Len(t, artifactsNotFound, 2)
		assert.Equal(t, common.RepoName1, artifactsNotFound[0].Repository)
		assert.Equal(t, common.TagName1, artifactsNotFound[0].Tag)
		assert.Equal(t, common.RepoName1, artifactsNotFound[1].Repository)
		assert.Equal(t, common.TagName2, artifactsNotFound[1].Tag)
		assert.Nil(t, filteredRepositories)
	})
	// 4. If filter has a tag that is not present in the repository, that tag should be returned in the artifactsNotFound list and the rest of the tags should be returned in the filtered list
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
		filteredRepositories, artifactsNotFound, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Len(t, filteredRepositories, 1)
		assert.Equal(t, common.RepoName1, filteredRepositories[0].Repository)
		assert.Equal(t, common.TagName, filteredRepositories[0].Tag)
		assert.Equal(t, "N/A", filteredRepositories[0].PatchTag)
		assert.Len(t, artifactsNotFound, 1)
		assert.Equal(t, common.RepoName1, artifactsNotFound[0].Repository)
		assert.Equal(t, common.TagName1, artifactsNotFound[0].Tag)
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
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName1, "", "").Return(common.FourTagsResultWithPatchTags, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName1, "", common.TagName4FloatingTag).Return(common.EmptyListTagsResult, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName2, "", "").Return(common.FourTagsResultWithPatchTags, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName2, "", common.TagName4FloatingTag).Return(common.EmptyListTagsResult, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName3, "", "").Return(common.FourTagsResultWithPatchTags, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName3, "", common.TagName4FloatingTag).Return(common.EmptyListTagsResult, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName4, "", "").Return(common.FourTagsResultWithPatchTags, nil).Once()
		mockAcrClient.On("GetAcrTags", common.TestCtx, common.RepoName4, "", common.TagName4FloatingTag).Return(common.EmptyListTagsResult, nil).Once()
		filteredRepositories, artifactsNotFound, err := ApplyFilterAndGetFilteredList(context.Background(), mockAcrClient, filter)
		assert.NoError(t, err)
		assert.Nil(t, artifactsNotFound)
		assert.Len(t, artifactsNotFound, 0)
		assert.Len(t, filteredRepositories, 8)
		assert.True(t, isInFilteredList(filteredRepositories, common.RepoName1, common.TagName1, common.TagName1FloatingTag))
		assert.True(t, isInFilteredList(filteredRepositories, common.RepoName1, common.TagName2, common.TagName2FloatingTag))
		assert.True(t, isInFilteredList(filteredRepositories, common.RepoName2, common.TagName1, common.TagName1FloatingTag))
		assert.True(t, isInFilteredList(filteredRepositories, common.RepoName2, common.TagName2, common.TagName2FloatingTag))
		assert.True(t, isInFilteredList(filteredRepositories, common.RepoName2, common.TagName3, common.TagName3FloatingTag))
		assert.True(t, isInFilteredList(filteredRepositories, common.RepoName2, common.TagName4, common.TagName4FloatingTag))
		assert.True(t, isInFilteredList(filteredRepositories, common.RepoName3, common.TagName1, common.TagName1FloatingTag))
		assert.True(t, isInFilteredList(filteredRepositories, common.RepoName3, common.TagName2, common.TagName2FloatingTag))
	})
}

func TestGetFilterFromFilterPolicy(t *testing.T) {
	username := "username"
	password := "password"
	// 1. Error should be returned when filter policy is not in the correct format
	t.Run("FilterPolicyNotCorrectFormatTest", func(t *testing.T) {
		filterPolicy := "notcorrectformat"
		filter, err := GetFilterFromFilterPolicy(context.Background(), filterPolicy, common.TestLoginURL, username, password)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.Equal(t, "filter-policy should be in the format repository:tag e.g. continuouspatchpolicy:latest", err.Error())
		assert.Equal(t, Filter{}, filter)
	})
	// 2. Error should be returned when filter policy has more than one colon
	t.Run("FilterPolicyMoreThanOneColonTest", func(t *testing.T) {
		filterPolicy := "repo1:something:anotherthing"
		filter, err := GetFilterFromFilterPolicy(context.Background(), filterPolicy, common.TestLoginURL, username, password)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.Equal(t, "filter-policy should be in the format repository:tag e.g. continuouspatchpolicy:latest", err.Error())
		assert.Equal(t, Filter{}, filter)
	})
	// 3. Error should be returned when fetching repository manifest fails
	t.Run("FetchBytesFailsTest", func(t *testing.T) {
		filterPolicy := "repo1:tag1"
		filter, err := GetFilterFromFilterPolicy(context.Background(), filterPolicy, common.TestLoginURL, username, password)
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.ErrorContains(t, err, "error fetching filter manifest content when reading the filter policy")
		assert.Equal(t, Filter{}, filter)
	})
}

func TestGetFilterFromFilePath(t *testing.T) {
	// 1. Error should be returned when filter file does not exist
	t.Run("FileDoesNotExistTest", func(t *testing.T) {
		filter, err := GetFilterFromFilePath("idontexist")
		assert.NotEqual(nil, err, "Error should not be nil")
		assert.ErrorContains(t, err, "error reading the filter json file from file path")
		assert.Equal(t, Filter{}, filter)
	})
	// 2. Error should be returned when filter file is not in the correct format
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
	// 3. Success scenario with correct filter file
	t.Run("SuccessTest", func(t *testing.T) {
		var filterFile = []byte(`{
			"version": "v1",
			"repositories": [
				{
					"repository": "repo1",
					"tags": ["jammy", "jammy-20240808"],
					"enabled": true
				},
				{
					"repository": "repo2",
					"tags": ["jammy"],
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

func TestIsTagConventionValid(t *testing.T) {
	filter := Filter{
		Version: "v1",
	}
	// 1. Should return no error when the tag convention is incremental
	t.Run("ValidTagConventionIncrementalTest", func(t *testing.T) {
		filter.TagConvention = "incremental"
		fil := filter.TagConvention.IsValid()
		assert.Nil(t, fil)
	})
	// 2. Should return no error when the tag convention is floating
	t.Run("ValidTagConventionFloatingTest", func(t *testing.T) {
		filter.TagConvention = "floating"
		fil := filter.TagConvention.IsValid()
		assert.Nil(t, fil)
	})
	// 3. Should return error when the tag convention is not incremental or floating
	t.Run("InvalidTagConventionTest", func(t *testing.T) {
		filter.TagConvention = "invalid"
		fil := filter.TagConvention.IsValid()
		assert.ErrorContains(t, fil, "tag-convention should be either incremental or floating")
	})
	// 4. Should return error when the tag convention is empty
	t.Run("EmptyTagConventionTest", func(t *testing.T) {
		filter.TagConvention = ""
		fil := filter.TagConvention.IsValid()
		assert.ErrorContains(t, fil, "tag-convention should be either incremental or floating")
	})
	// 5. Should return error when the tag convention is nil
	t.Run("NilTagConventionTest", func(t *testing.T) {
		fil := filter.TagConvention.IsValid()
		assert.ErrorContains(t, fil, "tag-convention should be either incremental or floating")
	})
}

func TestValidateFilter(t *testing.T) {
	// 1. Should return error when the version is empty
	t.Run("EmptyVersionTest", func(t *testing.T) {
		filter := Filter{
			Version: "",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{common.TagName1, common.TagName2},
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.ErrorContains(t, err, "Version is required in the filter and should be v1")
	})
	// 2. Should return error when the repositories list is empty
	t.Run("EmptyRepositoriesListTest", func(t *testing.T) {
		filter := Filter{
			Version:      "v1",
			Repositories: []Repository{},
		}
		err := filter.ValidateFilter()
		assert.ErrorContains(t, err, "Repositories is required in the filter")
	})
	// 3. Should return no error even when optional tag convention is not specified
	t.Run("ValidFilterTestWithNoTagConvention", func(t *testing.T) {
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
		err := filter.ValidateFilter()
		assert.Nil(t, err)
	})
	// 4. Should return no error when optional tag convention is specified as incremental
	t.Run("ValidFilterTestWithIncrementalTagConvention", func(t *testing.T) {
		filter := Filter{
			Version:       "v1",
			TagConvention: "incremental",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{common.TagName1, common.TagName2},
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.Nil(t, err)
	})
	// 5. Should return no error when optional tag convention is specified as floating
	t.Run("ValidFilterTestWithFloatingTagConvention", func(t *testing.T) {
		filter := Filter{
			Version:       "v1",
			TagConvention: "floating",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{common.TagName1, common.TagName2},
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.Nil(t, err)
	})
	// 6. Should return error when optional tag convention is specified and is invalid
	t.Run("InvalidTagConventionTest", func(t *testing.T) {
		filter := Filter{
			Version:       "v1",
			TagConvention: "invalid",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{common.TagName1, common.TagName2},
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.ErrorContains(t, err, "tag-convention should be either incremental or floating")
	})
	// 7. Should return error when the repository name is empty
	t.Run("EmptyRepositoryNameTest", func(t *testing.T) {
		filter := Filter{
			Version:       "v1",
			TagConvention: "incremental",
			Repositories: []Repository{
				{
					Repository: "",
					Tags:       []string{common.TagName1, common.TagName2},
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.ErrorContains(t, err, "Repository is required in the filter")
	})
	// 8. Should return error when the tags list is empty
	t.Run("EmptyTagsListTest", func(t *testing.T) {
		filter := Filter{
			Version:       "v1",
			TagConvention: "incremental",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{},
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.ErrorContains(t, err, "Tags is required in the filter")
	})
	// 9. Should return error when the tag list is nil
	t.Run("NilTagsListTest", func(t *testing.T) {
		filter := Filter{
			Version:       "v1",
			TagConvention: "incremental",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       nil,
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.ErrorContains(t, err, "Tags is required in the filter")
	})
	// 10. Should return error when tag list has empty tag
	t.Run("EmptyTagTest", func(t *testing.T) {
		filter := Filter{
			Version:       "v1",
			TagConvention: "incremental",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{""},
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.ErrorContains(t, err, "Tag is required in the filter")
	})
	// 11. Should return error when filter json contains tags ending with incremental or floating pattern
	t.Run("TagEndsWithIncrementalOrFloatingPatternTest", func(t *testing.T) {
		filter := Filter{
			Version:       "v1",
			TagConvention: "incremental",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{"jammy-1", "jammy-patched"},
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.ErrorContains(t, err, "Tags in filter json should not end with -1 to -999 or -patched")
		assert.ErrorContains(t, err, "Invalid Tag(s): jammy-1, jammy-patched")
	})
	// 12. Should return no error when filter json contains tag ending with number outside the range of 1-999
	t.Run("TagEndsWithNumberOutsideRangeTest", func(t *testing.T) {
		filter := Filter{
			Version:       "v1",
			TagConvention: "incremental",
			Repositories: []Repository{
				{
					Repository: common.RepoName1,
					Tags:       []string{"jammy-1000"},
					Enabled:    boolPtr(true),
				},
			},
		}
		err := filter.ValidateFilter()
		assert.Nil(t, err)
	})
}

func TestCompareTags(t *testing.T) {
	// 1. Should return true when the first tag is greater than the second tag
	t.Run("FirstTagGreaterThanSecondTest", func(t *testing.T) {
		assert.False(t, compareTags("10.1.1-10", "10.1.1-9"))
	})
	// 2. Should return false when the first tag is less than the second tag
	t.Run("FirstTagLessThanSecondTest", func(t *testing.T) {
		assert.True(t, compareTags("10.1.1-9", "10.1.1-10"))
	})
	// 3. Should return false when the first tag is equal to the second tag
	t.Run("FirstTagEqualToSecondTest", func(t *testing.T) {
		assert.False(t, compareTags("10.1.1-10", "10.1.1-10"))
	})
	t.Run("LexicographicalComparisonTest", func(t *testing.T) {
		assert.False(t, compareTags("10.1.1-patched", "10.1.1"))
		assert.True(t, compareTags("10.1.1", "10.1.1-patched"))
	})
}

func TestIsNumeric(t *testing.T) {
	// 1. Should return true when the string is numeric
	t.Run("NumericTest", func(t *testing.T) {
		assert.True(t, isNumeric("10"))
	})
	// 2. Should return false when the string is not numeric
	t.Run("NotNumericTest", func(t *testing.T) {
		assert.False(t, isNumeric("patched"))
	})
}

func TestContainsTag(t *testing.T) {
	tagList := []acr.TagAttributesBase{
		{Name: &common.TagName1},
		{Name: &common.TagName2},
	}
	// 1. Should return true when the list contains the tag
	t.Run("ContainsTagTest", func(t *testing.T) {
		assert.True(t, containsTag(tagList, common.TagName1))
	})
	// 2. Should return false when the list does not contain the tag
	t.Run("DoesNotContainTagTest", func(t *testing.T) {
		assert.False(t, containsTag(tagList, "tagnotinlist"))
	})
	// 3. Should return false when the list is empty
	t.Run("EmptyListTest", func(t *testing.T) {
		assert.False(t, containsTag([]acr.TagAttributesBase{}, common.TagName1))
	})
	//4. Should return false when the tag is nil
	t.Run("TagIsNilTest", func(t *testing.T) {
		tagList = []acr.TagAttributesBase{}
		assert.False(t, containsTag(tagList, ""))
	})
}

func TestEndsWithIncrementalOrFloatingPattern(t *testing.T) {
	// 1. Should return true when the tag ends with incremental pattern ending only between 1-999
	t.Run("EndsWithIncrementalPatternTest", func(t *testing.T) {
		assert.True(t, endsWithIncrementalOrFloatingPattern("10.1.1-1"))
		assert.True(t, endsWithIncrementalOrFloatingPattern("10.1.1-10"))
		assert.True(t, endsWithIncrementalOrFloatingPattern("10.1.1-999"))
		assert.False(t, endsWithIncrementalOrFloatingPattern("10.1.1-1000"))
	})
	// 2. Should return true when the tag ends with floating pattern
	t.Run("EndsWithFloatingPatternTest", func(t *testing.T) {
		assert.True(t, endsWithIncrementalOrFloatingPattern("10.1.1-patched"))
	})
	// 3. Should return false when the tag does not end with incremental or floating pattern
	t.Run("DoesNotEndWithPatternTest", func(t *testing.T) {
		assert.False(t, endsWithIncrementalOrFloatingPattern("10.1.1"))
	})
}

// Helper function to create a pointer to a bool
func boolPtr(v bool) *bool {
	return &v
}

// Helper function to check if the repository combination exists in the filtered list of repositories
func isInFilteredList(filteredRepositories []FilteredRepository, repository string, tag string, patchTag string) bool {
	for _, filteredRepository := range filteredRepositories {
		if filteredRepository.Repository == repository && filteredRepository.Tag == tag && filteredRepository.PatchTag == patchTag {
			return true
		}
	}
	return false
}
