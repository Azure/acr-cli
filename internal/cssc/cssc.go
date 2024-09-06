// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cssc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/internal/api"
	"github.com/Azure/acr-cli/internal/tag"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"

	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

type TagConvention string

const (
	Incremental TagConvention = "incremental"
	Floating    TagConvention = "floating"
)

func (tc TagConvention) IsValid() error {
	switch tc {
	case Incremental, Floating:
		return nil
	}
	return errors.New("tag-convention should be either incremental or floating")
}

var (
	floatingTagRegex    = regexp.MustCompile(`^(.+)-patched$`)            // Matches strings ending with -patched
	incrementalTagRegex = regexp.MustCompile(`^(.+)-([1-9]\d{0,2})$`)     // Matches strings ending with -1 to -999
	reservedSuffixRegex = regexp.MustCompile(`(-[1-9]\d{0,2}|-patched)$`) // Matches strings ending with -1 to -999 or -patched
)

// Filter struct to hold the filter policy
type Filter struct {
	Version       string        `json:"version"`
	TagConvention TagConvention `json:"tag-convention"`
	Repositories  []Repository  `json:"repositories"`
}

// Repository struct to hold the repository, tags and enabled flag
type Repository struct {
	Repository string   `json:"repository"`
	Tags       []string `json:"tags"`
	Enabled    *bool    `json:"enabled"`
}

// FilteredRepository struct to hold the filtered repository, tag and patch tag if any
type FilteredRepository struct {
	Repository string
	Tag        string
	PatchTag   string
}

// Reads the filter policy from the specified repository and tag and returns the Filter struct
func GetFilterFromFilterPolicy(ctx context.Context, filterPolicy string, loginURL string, username string, password string) (Filter, error) {
	filterPolicyPattern := `^[^:]+:[^:]+$`
	re := regexp.MustCompile(filterPolicyPattern)
	if !re.MatchString(filterPolicy) {
		return Filter{}, errors.New("filter-policy should be in the format repository:tag e.g. continuouspatchpolicy:latest")
	}
	repoTag := strings.Split(filterPolicy, ":")
	filterRepoName := repoTag[0]
	filterRepoTagName := repoTag[1]

	// Connect to the remote repository
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", loginURL, filterRepoName))
	if err != nil {
		return Filter{}, errors.Wrap(err, "error connecting to the repository when reading the filter policy")
	}
	repo.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
		Credential: auth.StaticCredential(loginURL, auth.Credential{
			Username: username,
			Password: password,
		}),
	}

	// Get manifest and read content
	_, pulledManifestContent, err := oras.FetchBytes(ctx, repo, filterRepoTagName, oras.DefaultFetchBytesOptions)
	if err != nil {
		return Filter{}, errors.Wrap(err, "error fetching filter manifest content when reading the filter policy")
	}
	var pulledManifest v1.Manifest
	if err := json.Unmarshal(pulledManifestContent, &pulledManifest); err != nil {
		return Filter{}, errors.Wrap(err, "error unmarshalling filter manifest content when reading the filter policy")
	}
	var fileContent []byte
	for _, layer := range pulledManifest.Layers {
		fileContent, err = content.FetchAll(ctx, repo, layer)
		if err != nil {
			return Filter{}, errors.Wrap(err, "error fetching filter content when reading the filter policy")
		}
	}

	// Unmarshal the JSON file data to Filter struct
	var filter = Filter{}
	if err := json.Unmarshal(fileContent, &filter); err != nil {
		return Filter{}, errors.Wrap(err, "error unmarshalling json content when reading the filter policy")
	}
	return filter, nil
}

// Reads the filter json from the specified file path and returns the Filter struct
func GetFilterFromFilePath(filePath string) (Filter, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return Filter{}, errors.Wrap(err, "error reading the filter json file from file path")
	}

	var filter = Filter{}
	if err := json.Unmarshal(file, &filter); err != nil {
		return Filter{}, errors.Wrap(err, "error unmarshalling json content when reading the filter file from file path")
	}

	return filter, nil
}

// Validates the filter and returns an error if the filter is invalid
func (filter *Filter) ValidateFilter() error {
	fmt.Println("Validating filter...")
	const versionV1 = "v1"
	if filter.Version == "" || filter.Version != versionV1 {
		return errors.New("Version is required in the filter and should be " + versionV1)
	}
	if len(filter.Repositories) == 0 {
		return errors.New("Repositories is required in the filter")
	}
	if filter.TagConvention != "" {
		err := filter.TagConvention.IsValid()
		if err != nil {
			return err
		}
	}

	allErrors := []string{}
	for _, repo := range filter.Repositories {
		if repo.Repository == "" {
			return errors.New("Repository is required in the filter")
		}
		if repo.Tags == nil || len(repo.Tags) == 0 {
			return errors.New("Tags is required in the filter")
		}

		incorrectTags := []string{}
		for _, tag := range repo.Tags {
			if tag == "" {
				return errors.New("Tag is required in the filter")
			}
			if endsWithIncrementalOrFloatingPattern(tag) {
				incorrectTags = append(incorrectTags, tag)
			}
		}
		if len(incorrectTags) > 0 {
			allErrors = append(allErrors, fmt.Sprintf("Repo:%s Invalid Tag(s): %s", repo.Repository, strings.Join(incorrectTags, ", ")))
		}
	}
	if len(allErrors) > 0 {
		allErrors = append(allErrors, "Tags in filter json should not end with -1 to -999 or -patched")
		return errors.New(strings.Join(allErrors, "\n"))
	}
	return nil
}

// Applies filter to filter out the repositories and tags from the ACR according to the specified criteria and returns the FilteredRepository struct
func ApplyFilterAndGetFilteredList(ctx context.Context, acrClient api.AcrCLIClientInterface, filter Filter) ([]FilteredRepository, []FilteredRepository, error) {

	var filteredRepos []FilteredRepository
	var artifactsNotFound []FilteredRepository

	// Default is floating tag regex, only if tag convention is incremental, then use incremental tag regex
	patchTagRegex := floatingTagRegex
	if filter.TagConvention == Incremental {
		patchTagRegex = incrementalTagRegex
	}

	for _, filterRepo := range filter.Repositories {
		// Create a tag map for each repository, where the key will be the base tag and the value will be the list of tags matching the tag convention from the filter
		tagMap := make(map[string][]string)
		if filterRepo.Enabled != nil && !*filterRepo.Enabled {
			continue
		}
		if filterRepo.Repository == "" || filterRepo.Tags == nil || len(filterRepo.Tags) == 0 {
			continue
		}

		tagList, err := tag.ListTags(ctx, acrClient, filterRepo.Repository)
		if err != nil {
			var listTagsErr *tag.ListTagsError
			if errors.As(err, &listTagsErr) {
				for _, tag := range filterRepo.Tags {
					artifactsNotFound = append(artifactsNotFound, FilteredRepository{
						Repository: filterRepo.Repository,
						Tag:        tag,
					})
				}
				continue
			}
			return nil, nil, errors.Wrap(err, "Some unexpected error occurred while listing tags for repository-"+filterRepo.Repository)
		}

		if len(filterRepo.Tags) == 1 && filterRepo.Tags[0] == "*" { // If the repo has * as tags defined in the filter, then all tags are considered for that repo
			for _, tag := range tagList {
				matches := patchTagRegex.FindStringSubmatch(*tag.Name)
				if matches != nil {
					baseTag := matches[1]
					tagMap[baseTag] = append(tagMap[baseTag], *tag.Name)
				} else if !endsWithIncrementalOrFloatingPattern(*tag.Name) {
					tagMap[*tag.Name] = append(tagMap[*tag.Name], *tag.Name)
				}
			}
		} else {
			for _, ftag := range filterRepo.Tags { // If the repo has specific tags defined in the filter, then only those tags are considered
				// If tag from filter is not found in the tag list obtained for the repository, then add it to artifactsNotFound and continue with the next tag
				if !containsTag(tagList, ftag) {
					artifactsNotFound = append(artifactsNotFound, FilteredRepository{
						Repository: filterRepo.Repository,
						Tag:        ftag,
					})
					continue
				}
				// This is needed to evaluate all versions of patch tags when original tag is specified in the filter
				var re *regexp.Regexp
				if filter.TagConvention == Incremental {
					re = regexp.MustCompile(`^` + ftag + `(-[1-9]\d{0,2})?$`)
				} else {
					re = regexp.MustCompile(`^` + ftag + `(-patched\d*)?$`)
				}
				for _, tag := range tagList {
					if re.MatchString(*tag.Name) {
						matches := patchTagRegex.FindStringSubmatch(*tag.Name)
						if matches != nil {
							baseTag := matches[1]
							tagMap[baseTag] = append(tagMap[baseTag], *tag.Name)
						} else if !endsWithIncrementalOrFloatingPattern(*tag.Name) {
							tagMap[*tag.Name] = append(tagMap[*tag.Name], *tag.Name)
						}
					}
				}
			}
		}

		// Iterate over the tagMap to generate the FilteredRepository list
		for baseTag, tags := range tagMap {
			sort.Slice(tags, func(i, j int) bool {
				return compareTags(tags[i], tags[j])
			})
			latestTag := tags[len(tags)-1]

			if latestTag == baseTag {
				latestTag = "N/A"
			}
			filteredRepos = append(filteredRepos, FilteredRepository{
				Repository: filterRepo.Repository,
				Tag:        baseTag,
				PatchTag:   latestTag,
			})
		}
	}
	return filteredRepos, artifactsNotFound, nil
}

// Prints the filtered result to the console
func PrintFilteredResult(filteredResult []FilteredRepository, showPatchTags bool, loginURL string) {
	if len(filteredResult) == 0 {
		fmt.Println("No matching repository and tag found!")
	} else if showPatchTags {
		fmt.Println("Listing repositories and tags matching the filter with corresponding latest patch tag (if present):")
		fmt.Printf("%s,%s,%s\n", "Repo", "Tag", "LatestPatchTag")
		for _, result := range filteredResult {
			fmt.Printf("%s,%s,%s\n", result.Repository, result.Tag, result.PatchTag)
		}
	} else {
		fmt.Println("Listing repositories and tags matching the filter:")
		fmt.Printf("%s,%s\n", "Repo", "Tag")
		for _, result := range filteredResult {
			fmt.Printf("%s,%s\n", result.Repository, result.Tag)
		}
	}
	fmt.Println("Matches found:", len(filteredResult))
}

// Prints the artifacts not found to the console
func PrintNotFoundArtifacts(artifactsNotFound []FilteredRepository) {
	if len(artifactsNotFound) > 0 {
		fmt.Printf("%s\n", "Artifacts specified in the filter that do not exist:")
		fmt.Printf("%s,%s\n", "Repo", "Tag")
		for _, result := range artifactsNotFound {
			fmt.Printf("%s,%s\n", result.Repository, result.Tag)
		}
		fmt.Println("Not found:", len(artifactsNotFound))
	}
}

// Helper function that compares two tags and returns true if tag1 is less than tag2
func compareTags(tag1, tag2 string) bool {
	// If tag1 and tag2 ends with incremental or floating pattern, then extract suffix based on last occurrence of "-" and compare the suffixes
	if endsWithIncrementalOrFloatingPattern(tag1) && endsWithIncrementalOrFloatingPattern(tag2) {
		tag1Index := strings.LastIndex(tag1, "-")
		tag2Index := strings.LastIndex(tag2, "-")
		var tag1Suffix, tag2Suffix string
		if tag1Index != -1 {
			tag1Suffix = tag1[tag1Index+1:]
		} else {
			tag1Suffix = tag1
		}
		if tag2Index != -1 {
			tag2Suffix = tag2[tag2Index+1:]
		} else {
			tag2Suffix = tag2
		}
		// Compare the suffixes
		if isNumeric(tag1Suffix) && isNumeric(tag2Suffix) {
			aNum, _ := strconv.Atoi(tag1Suffix)
			bNum, _ := strconv.Atoi(tag2Suffix)
			return aNum < bNum
		}
	}
	// Fallback to lexicographic comparison
	return tag1 < tag2
}

// Helper function to check if a string is numeric
func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// Helper function to check if tagList contains the specified tag
func containsTag(tagList []acr.TagAttributesBase, tag string) bool {
	for _, item := range tagList {
		if item.Name != nil && *item.Name == tag {
			return true
		}
	}
	return false
}

// Helper function to check if the string ends with incremental or floating pattern
func endsWithIncrementalOrFloatingPattern(str string) bool {
	return reservedSuffixRegex.MatchString(str)
}
