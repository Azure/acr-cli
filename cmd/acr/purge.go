// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
	"github.com/Azure/acr-cli/cmd/worker"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const (
	newPurgeCmdLongMessage = `acr purge: untag old images and delete dangling manifests.`
	purgeExampleMessage    = `  - Delete all tags that are older than 1 day
    acr purge -r MyRegistry --repository MyRepository --ago 1d

  - Delete all tags that are older than 1 day and begin with hello
    acr purge -r MyRegistry --repository MyRepository --ago 1d --filter "^hello.*"

  - Delete all dangling manifests
	acr purge -r MyRegistry --repository MyRepository --dangling
`

	defaultNumWorkers       = 6
	manifestListContentType = "application/vnd.docker.distribution.manifest.list.v2+json"
)

type purgeParameters struct {
	*rootParameters
	ago        string
	filters    []string
	untagged   bool
	dryRun     bool
	numWorkers int
}

var wg sync.WaitGroup

func newPurgeCmd(out io.Writer, rootParams *rootParameters) *cobra.Command {
	purgeParams := purgeParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:     "purge",
		Short:   "Delete images from a registry.",
		Long:    newPurgeCmdLongMessage,
		Example: purgeExampleMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			registryName, err := purgeParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, purgeParams.username, purgeParams.password, purgeParams.configs)
			if err != nil {
				return err
			}
			worker.StartDispatcher(&wg, *acrClient, purgeParams.numWorkers)
			tagFilters := map[string][]string{}
			for _, filter := range purgeParams.filters {
				repoName, tagRegex, err := GetRepositoryAndTagRegex(filter)
				if err != nil {
					return err
				}
				if _, ok := tagFilters[repoName]; ok {
					tagFilters[repoName] = append(tagFilters[repoName], tagRegex)
				} else {
					tagFilters[repoName] = []string{tagRegex}
				}
			}

			deletedTagsCount := 0
			deletedManifestsCount := 0
			for repoName, listOfTagRegex := range tagFilters {
				tagRegex := listOfTagRegex[0]
				for i := 1; i < len(listOfTagRegex); i++ {
					tagRegex = tagRegex + "|" + listOfTagRegex[i]
				}
				if !purgeParams.dryRun {
					singleDeletedTagsCount, err := PurgeTags(ctx, *acrClient, loginURL, repoName, purgeParams.ago, tagRegex)
					if err != nil {
						return errors.Wrap(err, "failed to purge tags")
					}

					singleDeletedManifestsCount := 0
					if purgeParams.untagged {
						deletedManifestsCount, err = PurgeDanglingManifests(ctx, *acrClient, loginURL, repoName)
						if err != nil {
							return errors.Wrap(err, "failed to purge manifests")
						}
					}
					deletedTagsCount += singleDeletedTagsCount
					deletedManifestsCount += singleDeletedManifestsCount
				} else {
					singleDeletedTagsCount, singleDeletedManifestsCount, err := DryRunPurge(ctx, *acrClient, loginURL, repoName, purgeParams.ago, tagRegex, purgeParams.untagged)
					if err != nil {
						return err
					}
					deletedTagsCount += singleDeletedTagsCount
					deletedManifestsCount += singleDeletedManifestsCount

				}
			}
			fmt.Printf("\nNumber of deleted tags: %d\n", deletedTagsCount)
			fmt.Printf("Number of deleted manifests: %d\n", deletedManifestsCount)

			return nil
		},
	}

	cmd.Flags().BoolVar(&purgeParams.untagged, "untagged", false, "If untagged is set all manifest that do not have any tags associated to them will be deleted")
	cmd.Flags().BoolVar(&purgeParams.dryRun, "dry-run", false, "Don't actually remove any tag or manifest, instead, show if they would be deleted")
	cmd.Flags().IntVar(&purgeParams.numWorkers, "concurrency", defaultNumWorkers, "The number of concurrent requests sent to the registry")
	cmd.Flags().StringVar(&purgeParams.ago, "ago", "1d", "The images that were created before this time stamp will be deleted")
	cmd.Flags().StringArrayVarP(&purgeParams.filters, "filter", "f", nil, "Given as a regular expression, if a tag matches the pattern and is older than the time specified in ago it gets deleted")
	cmd.Flags().StringArrayVarP(&purgeParams.configs, "config", "c", nil, "auth config paths")

	cmd.MarkFlagRequired("filter")
	return cmd
}

// PurgeTags deletes all tags that are older than the ago value and that match the filter string (if present).
func PurgeTags(ctx context.Context, acrClient api.AcrCLIClient, loginURL string, repoName string, ago string, tagFilter string) (int, error) {
	fmt.Printf("Deleting tags for repository: %s\n", repoName)
	agoDuration, err := ParseDuration(ago)
	deletedTagsCount := 0
	if err != nil {
		return -1, err
	}
	timeToCompare := time.Now().UTC()
	timeToCompare = timeToCompare.Add(agoDuration)
	tagRegex, err := regexp.Compile(tagFilter)
	if err != nil {
		return -1, err
	}
	lastTag := ""
	tagsToDelete, lastTag, err := GetTagsToDelete(ctx, acrClient, repoName, tagRegex, timeToCompare, "")
	if err != nil {
		return -1, err
	}
	for len(lastTag) > 0 {
		for _, tag := range *tagsToDelete {
			wg.Add(1)
			worker.QueuePurgeTag(loginURL, repoName, *tag.Name, *tag.Digest)
			deletedTagsCount++
		}
		wg.Wait()
		for len(worker.ErrorChannel) > 0 {
			wErr := <-worker.ErrorChannel
			if wErr.Error != nil {
				return -1, wErr.Error
			}
		}
		tagsToDelete, lastTag, err = GetTagsToDelete(ctx, acrClient, repoName, tagRegex, timeToCompare, lastTag)
		if err != nil {
			return -1, err
		}
	}
	return deletedTagsCount, nil
}

func GetRepositoryAndTagRegex(filter string) (string, string, error) {
	repoAndRegex := strings.Split(filter, ":")
	if len(repoAndRegex) != 2 {
		return "", "", errors.New("unable to correctly parse filter flag")
	}
	return repoAndRegex[0], repoAndRegex[1], nil
}

// ParseDuration analog to time.ParseDuration() but with days added.
func ParseDuration(ago string) (time.Duration, error) {
	var days int
	var durationString string
	if strings.Contains(ago, "d") {
		if _, err := fmt.Sscanf(ago, "%dd%s", &days, &durationString); err != nil {
			fmt.Sscanf(ago, "%dd", &days)
			durationString = ""
		}
	} else {
		days = 0
		if _, err := fmt.Sscanf(ago, "%s", &durationString); err != nil {
			return time.Duration(0), err
		}
	}
	duration := time.Duration(days) * 24 * time.Hour
	if len(durationString) > 0 {
		agoDuration, err := time.ParseDuration(durationString)
		if err != nil {
			return time.Duration(0), err
		}
		duration = duration + agoDuration
	}
	return (-1 * duration), nil
}

// GetTagsToDelete gets all tags that should be deleted according to the ago flag and the filter flag, this will at most return 100 tags,
// returns a pointer to a slice that contains the tags that will be deleted, the last tag obtained through the AcrListTags method
// and an error in case it occurred, the fourth return value contains a map that is used to determine how many tags a manifest has
func GetTagsToDelete(ctx context.Context,
	acrClient api.AcrCLIClient,
	repoName string,
	filter *regexp.Regexp,
	timeToCompare time.Time,
	lastTag string) (*[]acr.TagAttributesBase, string, error) {
	var matches bool
	var lastUpdateTime time.Time
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "", lastTag)
	if err != nil {
		return nil, "", err
	}
	newLastTag := ""
	if resultTags != nil && resultTags.TagsAttributes != nil && len(*resultTags.TagsAttributes) > 0 {
		tags := *resultTags.TagsAttributes
		tagsToDelete := []acr.TagAttributesBase{}
		for _, tag := range tags {
			matches = filter.MatchString(*tag.Name)
			if !matches {
				continue
			}
			lastUpdateTime, err = time.Parse(time.RFC3339Nano, *tag.LastUpdateTime)
			if err != nil {
				return nil, "", err
			}
			if lastUpdateTime.Before(timeToCompare) {
				tagsToDelete = append(tagsToDelete, tag)
			}
		}
		newLastTag = *tags[len(tags)-1].Name
		return &tagsToDelete, newLastTag, nil
	}
	// In case there are no more tags return empty string as lastTag
	return nil, "", nil
}

// PurgeDanglingManifests deletes all manifests that do not have any tags associated with them.
func PurgeDanglingManifests(ctx context.Context, acrClient api.AcrCLIClient, loginURL string, repoName string) (int, error) {
	fmt.Printf("Deleting manifests for repository: %s\n", repoName)
	deletedManifestsCount := 0
	manifestsToDelete, err := GetManifestsToDelete(ctx, acrClient, repoName)
	if err != nil {
		return -1, err
	}
	i := 0
	for _, manifest := range *manifestsToDelete {
		wg.Add(1)
		worker.QueuePurgeManifest(loginURL, repoName, *manifest.Digest)
		deletedManifestsCount++
		// Because the worker ErrorChannel has a capacity of 100 if has to periodically be checked
		if math.Mod(float64(i), 100) == 0 {
			wg.Wait()
			for len(worker.ErrorChannel) > 0 {
				wErr := <-worker.ErrorChannel
				if wErr.Error != nil {
					return -1, wErr.Error
				}
			}
		}
		i++
	}
	wg.Wait()
	for len(worker.ErrorChannel) > 0 {
		wErr := <-worker.ErrorChannel
		if wErr.Error != nil {
			return -1, wErr.Error
		}
	}
	return deletedManifestsCount, nil
}

// GetManifestsToDelete gets all the manifests that should be deleted, this means that do not have any tag and that do not form part
// of a manifest list that has tags refrerencing it.
func GetManifestsToDelete(ctx context.Context, acrClient api.AcrCLIClient, repoName string) (*[]acr.ManifestAttributesBase, error) {
	lastManifestDigest := ""
	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
	if err != nil {
		return nil, err
	}
	// This will act as a set if a key is present then it should not be deleted because it is referenced by a multiarch manifest
	// that will not be deleted
	doNotDelete := map[string]bool{}
	candidatesToDelete := []acr.ManifestAttributesBase{}
	// Iterate over all manifests to discover multiarchitecture manifests
	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
		manifests := *resultManifests.ManifestsAttributes
		for _, manifest := range manifests {
			if *manifest.MediaType == manifestListContentType && manifest.Tags != nil {
				var manifestList []byte
				manifestList, err = acrClient.GetManifest(ctx, repoName, *manifest.Digest)
				if err != nil {
					return nil, err
				}
				var multiArchManifest MultiArchManifest
				err = json.Unmarshal(manifestList, &multiArchManifest)
				if err != nil {
					return nil, err
				}
				for _, dependentDigest := range multiArchManifest.Manifests {
					doNotDelete[dependentDigest.Digest] = true
				}
			} else if manifest.Tags == nil {
				// If the manifest has no tags left it is a candidate for deletion
				candidatesToDelete = append(candidatesToDelete, manifest)
			}
		}
		lastManifestDigest = *manifests[len(manifests)-1].Digest
		resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			return nil, err
		}
	}
	manifestsToDelete := []acr.ManifestAttributesBase{}
	// Remove all manifests that should not be deleted
	for i := 0; i < len(candidatesToDelete); i++ {
		if _, ok := doNotDelete[*candidatesToDelete[i].Digest]; !ok {
			manifestsToDelete = append(manifestsToDelete, candidatesToDelete[i])
		}
	}
	return &manifestsToDelete, nil
}

// DryRunPurge outputs everything that would be deleted if the purge command was executed
func DryRunPurge(ctx context.Context, acrClient api.AcrCLIClient, loginURL string, repoName string, ago string, filter string, untagged bool) (int, int, error) {
	deletedTagsCount := 0
	deletedManifestsCount := 0
	deletedTags := map[string]int{}
	fmt.Printf("Deleting tags for repository: %s\n", repoName)
	agoDuration, err := ParseDuration(ago)
	if err != nil {
		return -1, -1, err
	}
	timeToCompare := time.Now().UTC()
	timeToCompare = timeToCompare.Add(agoDuration)
	regex, err := regexp.Compile(filter)
	if err != nil {
		return -1, -1, err
	}
	lastTag := ""
	tagsToDelete, lastTag, err := GetTagsToDelete(ctx, acrClient, repoName, regex, timeToCompare, "")

	if err != nil {
		return -1, -1, err
	}
	for len(lastTag) > 0 {
		for _, tag := range *tagsToDelete {
			if _, exists := deletedTags[*tag.Digest]; exists {
				deletedTags[*tag.Digest]++
			} else {
				deletedTags[*tag.Digest] = 1
			}
			fmt.Printf("%s/%s:%s\n", loginURL, repoName, *tag.Name)
			deletedTagsCount++
		}
		tagsToDelete, lastTag, err = GetTagsToDelete(ctx, acrClient, repoName, regex, timeToCompare, lastTag)
		if err != nil {
			return -1, -1, err
		}
	}
	if untagged {
		fmt.Printf("Deleting manifests for repository: %s\n", repoName)
		countMap, err := CountTagsByManifest(ctx, acrClient, repoName)
		if err != nil {
			return -1, -1, err
		}
		lastManifestDigest := ""
		resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			return -1, -1, err
		}
		// This will act as a set if a key is present then it should not be deleted because it is referenced by a multiarch manifest
		// that will not be deleted
		doNotDelete := map[string]bool{}
		candidatesToDelete := []acr.ManifestAttributesBase{}
		// Iterate over all manifests to discover multiarchitecture manifests
		for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
			manifests := *resultManifests.ManifestsAttributes
			for _, manifest := range manifests {
				if *manifest.MediaType == manifestListContentType && (*countMap)[*manifest.Digest] != deletedTags[*manifest.Digest] {
					var manifestList []byte
					manifestList, err = acrClient.GetManifest(ctx, repoName, *manifest.Digest)
					if err != nil {
						return -1, -1, err
					}
					var multiArchManifest MultiArchManifest
					err = json.Unmarshal(manifestList, &multiArchManifest)
					if err != nil {
						return -1, -1, err
					}
					for _, dependentDigest := range multiArchManifest.Manifests {
						doNotDelete[dependentDigest.Digest] = true
					}
				} else if (*countMap)[*manifest.Digest] == deletedTags[*manifest.Digest] {
					// If the manifest has no tags left it is a candidate for deletion
					candidatesToDelete = append(candidatesToDelete, manifest)
				}
			}
			lastManifestDigest = *manifests[len(manifests)-1].Digest
			resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
			if err != nil {
				return -1, -1, err
			}
		}
		// Just print manifests that should be deleted.
		for i := 0; i < len(candidatesToDelete); i++ {
			if _, ok := doNotDelete[*candidatesToDelete[i].Digest]; !ok {
				fmt.Printf("%s/%s@%s\n", loginURL, repoName, *candidatesToDelete[i].Digest)
				deletedManifestsCount++
			}
		}
	}

	return deletedTagsCount, deletedManifestsCount, nil
}

// CountTagsByManifest returns a map that for a given manifest digest contains the number of tags associated to it.
func CountTagsByManifest(ctx context.Context, acrClient api.AcrCLIClient, repoName string) (*map[string]int, error) {
	countMap := map[string]int{}
	lastTag := ""
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "", lastTag)
	if err != nil {
		return nil, err
	}
	for resultTags != nil && resultTags.TagsAttributes != nil {
		tags := *resultTags.TagsAttributes
		for _, tag := range tags {
			if _, exists := countMap[*tag.Digest]; exists {
				countMap[*tag.Digest]++
			} else {
				countMap[*tag.Digest] = 1
			}
		}

		lastTag = *tags[len(tags)-1].Name
		resultTags, err = acrClient.GetAcrTags(ctx, repoName, "", lastTag)
		if err != nil {
			return nil, err
		}
	}
	return &countMap, nil
}

type MultiArchManifest struct {
	Manifests     []Manifest `json:"manifests"`
	MediaType     string     `json:"mediaType"`
	SchemaVersion int        `json:"schemaVersion"`
}

type Manifest struct {
	Digest    string   `json:"digest"`
	MediaType string   `json:"mediaType"`
	Platform  Platform `json:"platform"`
	Size      int      `json:"size"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	Os           string `json:"os"`
}
