// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cssc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/acr-cli/internal/tag"

	"github.com/Azure/acr-cli/cmd/api"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"

	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// Filter struct to hold the filter policy
type Filter struct {
	Version      string       `json:"version"`
	Repositories []Repository `json:"repositories"`
}

type Repository struct {
	Repository string   `json:"repository"`
	Tags       []string `json:"tags"`
	Enabled    *bool    `json:"enabled"`
}

// Repository struct to hold the filtered repository, tag and patch tag if any
type FilteredRepository struct {
	Repository string
	Tag        string
	PatchTag   string
}

// Reads the filter policy from the specified repository and tag and returns the Filter struct
func GetFilterFromFilterPolicy(ctx context.Context, filterPolicy string, loginURL string, username string, password string) (Filter, error) {
	if !strings.Contains(filterPolicy, ":") {
		return Filter{}, errors.New("filter-policy should be in the format repo:tag")
	}
	repoTag := strings.Split(filterPolicy, ":")
	filterRepoName := repoTag[0]
	filterRepoTagName := repoTag[1]

	// Connect to the remote repository
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", loginURL, filterRepoName))
	if err != nil {
		panic(err)
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
		return Filter{}, errors.Wrap(err, "error fetching manifest content when reading the filter policy")
	}
	var pulledManifest v1.Manifest
	if err := json.Unmarshal(pulledManifestContent, &pulledManifest); err != nil {
		panic(err)
	}
	var fileContent []byte
	for _, layer := range pulledManifest.Layers {
		fileContent, err = content.FetchAll(ctx, repo, layer)
		if err != nil {
			panic(err)
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

// Applies filter to filter out the repositories and tags from the ACR according to the specified criteria and returns the FilteredRepository struct
func ApplyFilterAndGetFilteredList(ctx context.Context, acrClient api.AcrCLIClientInterface, filter Filter) ([]FilteredRepository, error) {
	var filteredRepos []FilteredRepository

	for _, filterRepo := range filter.Repositories {
		if filterRepo.Enabled != nil && !*filterRepo.Enabled {
			continue
		}
		if filterRepo.Repository == "" || filterRepo.Tags == nil || len(filterRepo.Tags) == 0 || filterRepo.Tags[0] == "" {
			continue
		}

		tagList, err := tag.ListTags(ctx, acrClient, filterRepo.Repository)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				continue
			}
			return nil, err
		}

		if len(filterRepo.Tags) == 1 && filterRepo.Tags[0] == "*" {
			for _, tag := range tagList {
				if strings.Contains(*tag.Name, "-patched") {
					originalTag := strings.Split(*tag.Name, "-patched")[0]
					matchingRepo := FilteredRepository{Repository: filterRepo.Repository, Tag: originalTag, PatchTag: *tag.Name}
					filteredRepos = AppendElement(filteredRepos, matchingRepo)
				} else {
					matchingRepo := FilteredRepository{Repository: filterRepo.Repository, Tag: *tag.Name, PatchTag: *tag.Name}
					filteredRepos = AppendElement(filteredRepos, matchingRepo)
				}
			}
		} else {
			for _, ftag := range filterRepo.Tags {
				for _, tag := range tagList {
					if *tag.Name == ftag {
						matchingRepo := FilteredRepository{Repository: filterRepo.Repository, Tag: *tag.Name, PatchTag: *tag.Name}
						filteredRepos = AppendElement(filteredRepos, matchingRepo)
					} else if *tag.Name == ftag+"-patched" {
						matchingRepo := FilteredRepository{Repository: filterRepo.Repository, Tag: ftag, PatchTag: ftag + "-patched"}
						filteredRepos = AppendElement(filteredRepos, matchingRepo)
					}
				}
			}
		}
	}
	return filteredRepos, nil
}

// Prints the filtered result to the console
func PrintFilteredResult(filteredResult []FilteredRepository, showPatchTags bool, loginURL string) {
	if len(filteredResult) == 0 {
		fmt.Println("No matching repository and tag found!")
	}
	if showPatchTags {
		fmt.Println("Listing repositories and tags matching the filter with corrosponding patch tag (if present):")
		for _, result := range filteredResult {
			fmt.Printf("%s/%s:%s,%s\n", loginURL, result.Repository, result.Tag, result.PatchTag)
		}
	} else {
		fmt.Println("Listing repositories and tags matching the filter:")
		for _, result := range filteredResult {
			fmt.Printf("%s/%s:%s\n", loginURL, result.Repository, result.Tag)
		}
	}
	fmt.Println("Total matches found:", len(filteredResult))
}

// Appends the element to the slice if it does not already exist in the slice
func AppendElement(slice []FilteredRepository, element FilteredRepository) []FilteredRepository {
	for _, existing := range slice {
		if existing.Repository == element.Repository && existing.Tag == element.Tag {
			// Remove the existing element from the slice
			for i, v := range slice {
				if v.Repository == element.Repository && v.Tag == element.Tag {
					slice = append(slice[:i], slice[i+1:]...)
					break
				}
			}
		}
	}
	// Append the new element to the slice
	return append(slice, element)
}
