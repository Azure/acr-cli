// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	orasauth "github.com/Azure/acr-cli/auth/oras"
	"github.com/Azure/acr-cli/cmd/api"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	newCsscCmdLongMessage  = `acr cssc: CSSC (Container Secure Supply Chain) operations for a registry. Use subcommands for more specific operations.`
	newPatchCmdLongMessage = `acr cssc patch: List all repositories and tags that match the filter.

	Use the --filter-policy flag to specify the repository:tag where the filter file exists. If no filter policy is provided, the default filter policy is continuouspatchpolicy:latest
	Example: acr cssc patch -r example --filter-policy continuouspatchpolicy:latest

	Use the --show-patch-tags flag to also get patch image tag (if it exists) for repositories and tags that match the filter.
	Example: acr cssc patch -r example --filter-policy continuouspatchpolicy:latest --show-patch-tags

	Use the --dry-run flag in combination with --file-path flag to read filter from file path instead of filter policy.
	Use this to validate the filter file without uploading it to the registry.
	Example: acr cssc patch -r example --dry-run --file-path /path/to/filter.json
	
	Filter JSON file format:
	{
		"version": "v1",
		"repositories": [
			{
				"repository": "example",
				"tags": ["tag1", "tag2"],
				"enabled": true
			}
		]
	}
`
)

type csscParameters struct {
	*rootParameters
	filterPolicy  string
	showPatchTags bool
	dryRun        bool
	filePath      string
}

type Repository struct {
	Repository string   `json:"repository"`
	Tags       []string `json:"tags"`
	Enabled    *bool    `json:"enabled"`
}

type Filter struct {
	Version      string       `json:"version"`
	Repositories []Repository `json:"repositories"`
}

type FilteredRepository struct {
	Repository string
	Tag        string
	PatchTag   string
}

// The cssc command can be used to list cssc configurations for a registry.
func newCsscCmd(rootParams *rootParameters) *cobra.Command {
	csscParams := csscParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:   "cssc",
		Short: "cssc operations for a registry",
		Long:  newCsscCmdLongMessage,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Help()
			return nil
		},
	}

	newCsscPatchFilterCmd := newPatchFilterCmd(&csscParams)

	cmd.AddCommand(
		newCsscPatchFilterCmd,
	)

	return cmd
}

// The patch subcommand can be used to list cssc continuous patch filters for a registry.
func newPatchFilterCmd(csscParams *csscParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "patch",
		Short: "Manage cssc patch operations for a registry",
		Long:  newPatchCmdLongMessage,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			registryName, err := csscParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, csscParams.username, csscParams.password, csscParams.configs)
			if err != nil {
				return err
			}

			filter := Filter{}
			if csscParams.dryRun == false && csscParams.filePath != "" {
				return errors.New("flag --file-path can only be used in combination with --dry-run flag")
			} else if csscParams.dryRun == true {
				if csscParams.filePath == "" {
					return errors.New("flag --file-path is required when using --dry-run flag")
				}
				fmt.Println("DRY RUN mode enabled. Reading filter from file path...")
				filter, err = getFilterFromFilePath(csscParams.filePath)
				if err != nil {
					return err
				}
			} else if csscParams.filterPolicy != "" {
				fmt.Println("Reading filter from filter policy...")
				filter, err = getFilterFromFilterPolicy(ctx, csscParams, loginURL)
				if err != nil {
					return err
				}
			}

			// Continue only if the filter is not empty
			if len(filter.Repositories) == 0 {
				fmt.Println("Filter is empty or invalid.")
				return nil
			}

			filteredResult, err := applyFilterAndGetFilteredList(ctx, acrClient, filter)
			if err != nil {
				return err
			}

			printFilteredResult(filteredResult, csscParams, loginURL)
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&csscParams.filterPolicy, "filter-policy", "continuouspatchpolicy:latest", "The filter policy defined by the filter json file uploaded in a repo:tag. For v1, it will be continuouspatchpolicy:latest")
	cmd.PersistentFlags().BoolVar(&csscParams.dryRun, "dry-run", false, "Use this in combination with --file-path to read filter from file path instead of filter policy. This allows to validate the filter file without uploading it to the registry.")
	cmd.PersistentFlags().StringVar(&csscParams.filePath, "file-path", "", "The file path of the JSON filter file. Use this in combination with --dry-run to simulate the operation without making any changes on the registry.")
	cmd.Flags().BoolVar(&csscParams.showPatchTags, "show-patch-tags", false, "Use this flag to get patch tag (if it exists) for repositories and tags that match the filter. Example: acr cssc patch --filter-policy continuouspatchpolicy:latest --show-patch-tags")
	return cmd
}

func getFilterFromFilterPolicy(ctx context.Context, csscParams *csscParameters, loginURL string) (Filter, error) {
	// Validate the filter policy format
	if !strings.Contains(csscParams.filterPolicy, ":") {
		return Filter{}, errors.New("filter-policy should be in the format repo:tag")
	}
	repoTag := strings.Split(csscParams.filterPolicy, ":")
	filterRepoName := repoTag[0]
	filterRepoTagName := repoTag[1]

	// Connect to the remote repository
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", loginURL, filterRepoName))
	if err != nil {
		panic(err)
	}
	getRegistryCredsFromStore(csscParams, loginURL)
	repo.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
		Credential: auth.StaticCredential(loginURL, auth.Credential{
			Username: csscParams.username,
			Password: csscParams.password,
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

func getFilterFromFilePath(filePath string) (Filter, error) {
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

func applyFilterAndGetFilteredList(ctx context.Context, acrClient api.AcrCLIClientInterface, filter Filter) ([]FilteredRepository, error) {
	var filteredRepos []FilteredRepository

	for _, filterRepo := range filter.Repositories {
		if filterRepo.Enabled != nil && *filterRepo.Enabled == false {
			continue
		}
		if filterRepo.Repository == "" || filterRepo.Tags == nil || len(filterRepo.Tags) == 0 || filterRepo.Tags[0] == "" {
			continue
		}

		tagList, err := listTags(ctx, acrClient, filterRepo.Repository)
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
					filteredRepos = appendElement(filteredRepos, matchingRepo)
				} else {
					matchingRepo := FilteredRepository{Repository: filterRepo.Repository, Tag: *tag.Name, PatchTag: *tag.Name}
					filteredRepos = appendElement(filteredRepos, matchingRepo)
				}
			}
		} else {
			for _, ftag := range filterRepo.Tags {
				for _, tag := range tagList {
					if *tag.Name == ftag {
						matchingRepo := FilteredRepository{Repository: filterRepo.Repository, Tag: *tag.Name, PatchTag: *tag.Name}
						filteredRepos = appendElement(filteredRepos, matchingRepo)
					} else if *tag.Name == ftag+"-patched" {
						matchingRepo := FilteredRepository{Repository: filterRepo.Repository, Tag: ftag, PatchTag: ftag + "-patched"}
						filteredRepos = appendElement(filteredRepos, matchingRepo)
					}
				}
			}
		}
	}
	return filteredRepos, nil
}

func getRegistryCredsFromStore(csscParams *csscParameters, loginURL string) {
	// If both username and password are empty then the docker config file will be used, it can be found in the default
	// location or in a location specified by the configs string array
	if csscParams.username == "" || csscParams.password == "" {
		store, err := orasauth.NewStore(csscParams.configs...)
		if err != nil {
			errors.Wrap(err, "error resolving authentication")
		}
		cred, err := store.Credential(context.Background(), loginURL)
		if err != nil {
			errors.Wrap(err, "error resolving authentication")
		}
		csscParams.username = cred.Username
		csscParams.password = cred.Password

		// fallback to refresh token if it is available
		if csscParams.password == "" && cred.RefreshToken != "" {
			csscParams.password = cred.RefreshToken
		}
	}
}

func appendElement(slice []FilteredRepository, element FilteredRepository) []FilteredRepository {
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

func printFilteredResult(filteredResult []FilteredRepository, csscParams *csscParameters, loginURL string) {
	if len(filteredResult) == 0 {
		fmt.Println("No matching repository and tag found!")
	}
	if csscParams.showPatchTags {
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
