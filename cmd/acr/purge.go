// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
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

// The constants for this file are defined here.
const (
	newPurgeCmdLongMessage = `acr purge: untag old images and delete dangling manifests.`
	purgeExampleMessage    = `  - Delete all tags that are older than 1 day in the example.azurecr.io registry inside the hello-world repository
    	acr purge -r example --filter "hello-world:.*" --ago 1d

  - Delete all tags that are older than 7 days and begin with hello in the example.azurecr.io registry inside the hello-world repository
    	acr purge -r example --filter "hello-world:^hello.*" --ago 7d 

  - Delete all tags that contain the word test in the tag name and are older than 5 days in the example.azurecr.io registry inside the hello-world 
    repository, after that, remove the dangling manifests in the same repository
	acr purge -r example --filter "hello-world:\w*test\w*" --ago 5d --untagged 

  - Delete all tags older than 1 day in the example.azurecr.io registry inside the hello-world repository using the credentials found in 
    the C://Users/docker/config.json path
	acr purge -r example --filter "hello-world:.*" --ago 1d --config C://Users/docker/config.json
`

	defaultNumWorkers       = 6
	manifestListContentType = "application/vnd.docker.distribution.manifest.list.v2+json"
	linkHeader              = "Link"
)

// purgeParameters defines the parameters that the purge command uses (including the registry name, username and password).
type purgeParameters struct {
	*rootParameters
	ago      string
	filters  []string
	untagged bool
	dryRun   bool
}

// The WaitGroup is used to make sure that the http requests are finished before exiting the program, and also to limit the
// amount of concurrent http calls to the defaultNumWorkers
var wg sync.WaitGroup

// newPurgeCmd defines the purge command.
func newPurgeCmd(out io.Writer, rootParams *rootParameters) *cobra.Command {
	purgeParams := purgeParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:     "purge",
		Short:   "Delete images from a registry.",
		Long:    newPurgeCmdLongMessage,
		Example: purgeExampleMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			// This context is used for all the http requests.
			ctx := context.Background()
			registryName, err := purgeParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			// An acrClient with authentication is generated, if the authentication cannot be resolved an error is returned.
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, purgeParams.username, purgeParams.password, purgeParams.configs)
			if err != nil {
				return err
			}
			// In order to only have a fixed amount of http requests a dispatcher is started that will keep forwarding the jobs
			// to the workers, which are goroutines that continuously fetch for tags/manifests to delete.
			worker.StartDispatcher(ctx, &wg, acrClient, defaultNumWorkers)
			// A map is used to keep the regex tags for every repository.
			tagFilters := map[string][]string{}
			for _, filter := range purgeParams.filters {
				repoName, tagRegex, err := getRepositoryAndTagRegex(filter)
				if err != nil {
					return err
				}
				if _, ok := tagFilters[repoName]; ok {
					tagFilters[repoName] = append(tagFilters[repoName], tagRegex)
				} else {
					tagFilters[repoName] = []string{tagRegex}
				}
			}

			// In order to print a summary of the deleted tags/manifests the counters get updated everytime a repo is purged.
			deletedTagsCount := 0
			deletedManifestsCount := 0
			for repoName, listOfTagRegex := range tagFilters {
				tagRegex := listOfTagRegex[0]
				for i := 1; i < len(listOfTagRegex); i++ {
					// To only iterate through a repo once a big regex filter is made of all the filters of a particular repo.
					tagRegex = tagRegex + "|" + listOfTagRegex[i]
				}
				if !purgeParams.dryRun {
					singleDeletedTagsCount, err := purgeTags(ctx, acrClient, loginURL, repoName, purgeParams.ago, tagRegex)
					if err != nil {
						return errors.Wrap(err, "failed to purge tags")
					}
					singleDeletedManifestsCount := 0
					// If the untagged flag is set then also manifests are deleted.
					if purgeParams.untagged {
						singleDeletedManifestsCount, err = purgeDanglingManifests(ctx, acrClient, loginURL, repoName)
						if err != nil {
							return errors.Wrap(err, "failed to purge manifests")
						}
					}
					// After every repository is purged the counters are updated.
					deletedTagsCount += singleDeletedTagsCount
					deletedManifestsCount += singleDeletedManifestsCount
				} else {
					// No tag or manifest will be deleted but the counters still will be updated.
					singleDeletedTagsCount, singleDeletedManifestsCount, err := dryRunPurge(ctx, acrClient, loginURL, repoName, purgeParams.ago, tagRegex, purgeParams.untagged)
					if err != nil {
						return errors.Wrap(err, "failed to dry-run purge")
					}
					deletedTagsCount += singleDeletedTagsCount
					deletedManifestsCount += singleDeletedManifestsCount

				}
			}
			// After all repos have been purged the summary is printed.
			fmt.Printf("\nNumber of deleted tags: %d\n", deletedTagsCount)
			fmt.Printf("Number of deleted manifests: %d\n", deletedManifestsCount)

			return nil
		},
	}

	cmd.Flags().BoolVar(&purgeParams.untagged, "untagged", false, "If the untagged flag is set all the manifests that do not have any tags associated to them will be also purged, except if they belong to a manifest list that contains at least one tag")
	cmd.Flags().BoolVar(&purgeParams.dryRun, "dry-run", false, "If the dry-run flag is set no manifest or tag will be deleted, the output would be the same as if they were deleted")
	cmd.Flags().StringVar(&purgeParams.ago, "ago", "", "The tags that were last updated before this duration will be deleted, the format is [number]d[string] where the first number represents an amount of days and the string is in a Go duration format (e.g. 2d3h6m selects images older than 2 days, 3 hours and 6 minutes)")
	cmd.Flags().StringArrayVarP(&purgeParams.filters, "filter", "f", nil, "Specify the repository and a regular expression filter for the tag name, if a tag matches the filter and is older than the duration specified in ago it will be deleted")
	cmd.Flags().StringArrayVarP(&purgeParams.configs, "config", "c", nil, "Authentication config paths (e.g. C://Users/docker/config.json)")
	cmd.Flags().BoolP("help", "h", false, "Print usage")
	cmd.MarkFlagRequired("filter")
	cmd.MarkFlagRequired("ago")
	return cmd
}

// purgeTags deletes all tags that are older than the ago value and that match the tagFilter string.
func purgeTags(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, ago string, tagFilter string) (int, error) {
	fmt.Printf("Deleting tags for repository: %s\n", repoName)
	deletedTagsCount := 0
	agoDuration, err := parseDuration(ago)
	if err != nil {
		return -1, err
	}
	timeToCompare := time.Now().UTC()
	// Since the parseDuration function returns a negative duration, it is added to the current duration in order to be able to easily compare
	// with the LastUpdatedTime attribute a tag has.
	timeToCompare = timeToCompare.Add(agoDuration)
	tagRegex, err := regexp.Compile(tagFilter)
	if err != nil {
		return -1, err
	}
	lastTag := ""

	// GetTagsToDelete will return an empty lastTag when there are no more tags.
	for {
		tagsToDelete, newLastTag, err := getTagsToDelete(ctx, acrClient, repoName, tagRegex, timeToCompare, lastTag)
		lastTag = newLastTag
		if err != nil {
			return -1, err
		}
		if tagsToDelete != nil {
			for _, tag := range *tagsToDelete {
				wg.Add(1)
				// The purge job is queued, after a purge worker picks it up the tag will be deleted.
				worker.QueuePurgeTag(loginURL, repoName, *tag.Name, *tag.Digest)
				deletedTagsCount++
			}
			// To not overflow the error channel capacity the purgeTags function waits for a whole block of
			// 100 jobs to be finished before continuing.
			wg.Wait()
			for len(worker.ErrorChannel) > 0 {
				wErr := <-worker.ErrorChannel
				if wErr.Error != nil {
					return -1, wErr.Error
				}
			}
		}
		if len(lastTag) == 0 {
			break
		}

	}
	return deletedTagsCount, nil
}

// getRepositoryAndTagRegex splits the strings that are in the form <repository>:<regex filter>
func getRepositoryAndTagRegex(filter string) (string, string, error) {
	repoAndRegex := strings.Split(filter, ":")
	if len(repoAndRegex) != 2 {
		return "", "", errors.New("unable to correctly parse filter flag")
	}
	return repoAndRegex[0], repoAndRegex[1], nil
}

// parseDuration analog to time.ParseDuration() but with days added.
func parseDuration(ago string) (time.Duration, error) {
	var days int
	var durationString string
	// The supported format is %d%s where the string is a valid go duration string.
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
	// The number of days gets converted to hours.
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

// getTagsToDelete gets all tags that should be deleted according to the ago flag and the filter flag, this will at most return 100 tags,
// returns a pointer to a slice that contains the tags that will be deleted, the last tag obtained through the AcrListTags function
// and an error in case it occurred, the fourth return value contains a map that is used to determine how many tags a manifest has
func getTagsToDelete(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	repoName string,
	filter *regexp.Regexp,
	timeToCompare time.Time,
	lastTag string) (*[]acr.TagAttributesBase, string, error) {

	var matches bool
	var lastUpdateTime time.Time
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "", lastTag)
	if err != nil {
		if resultTags != nil && resultTags.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return nil, "", nil
		}
		// An empty lastTag string is returned so there will not be any tag purged.
		return nil, "", err
	}
	newLastTag := ""
	if resultTags != nil && resultTags.TagsAttributes != nil && len(*resultTags.TagsAttributes) > 0 {
		tags := *resultTags.TagsAttributes
		tagsToDelete := []acr.TagAttributesBase{}
		for _, tag := range tags {
			matches = filter.MatchString(*tag.Name)
			if !matches {
				// If a tag does not match the regex then it not added to the list no matter the LastUpdateTime
				continue
			}
			lastUpdateTime, err = time.Parse(time.RFC3339Nano, *tag.LastUpdateTime)
			if err != nil {
				return nil, "", err
			}
			// If a tag did match the regex filter, is older than the specified duration and can be deleted then it is returned
			// as a tag to delete.
			if lastUpdateTime.Before(timeToCompare) && *(*tag.ChangeableAttributes).DeleteEnabled {
				tagsToDelete = append(tagsToDelete, tag)
			}
		}
		newLastTag = getLastTagFromResponse(resultTags)
		return &tagsToDelete, newLastTag, nil
	}
	// In case there are no more tags return empty string as lastTag so that the purgeTags function stops
	return nil, "", nil
}

func getLastTagFromResponse(resultTags *acr.RepositoryTagsType) string {
	newLastTag := ""
	// The lastTag is updated to keep the for loop going.
	if resultTags.Header != nil {
		link := resultTags.Header.Get(linkHeader)
		if len(link) > 0 {
			queryString := strings.Split(link, "?")
			if len(queryString) > 1 {
				stripRel := strings.Split(queryString[1], ";")

				queryParams := strings.Split(stripRel[0], "&")

				for _, queryParam := range queryParams {
					lastParam := strings.Split(queryParam, "last=")
					if len(lastParam) > 1 {
						newLastTag = lastParam[1]
					}
				}

			}

		}
	}
	return newLastTag
}

// purgeDanglingManifests deletes all manifests that do not have any tags associated with them.
func purgeDanglingManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string) (int, error) {
	fmt.Printf("Deleting manifests for repository: %s\n", repoName)
	deletedManifestsCount := 0
	// Contrary to getTagsToDelete, getManifestsToDelete gets all the Manifests at once, this was done because if there is a manifest that has no
	// tag but is referenced by a multiarch manifest that has tags then it should not be deleted.
	manifestsToDelete, err := getManifestsToDelete(ctx, acrClient, repoName)
	if err != nil {
		return -1, err
	}
	i := 0
	for _, manifest := range *manifestsToDelete {
		wg.Add(1)
		worker.QueuePurgeManifest(loginURL, repoName, *manifest.Digest)
		deletedManifestsCount++
		// Because the worker ErrorChannel has a capacity of 100 it has to periodically be checked for errors
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
	// Wait for all the worker jobs to finish.
	wg.Wait()
	for len(worker.ErrorChannel) > 0 {
		wErr := <-worker.ErrorChannel
		if wErr.Error != nil {
			return -1, wErr.Error
		}
	}
	return deletedManifestsCount, nil
}

// getManifestsToDelete gets all the manifests that should be deleted, this means that do not have any tag and that do not form part
// of a manifest list that has tags referencing it.
func getManifestsToDelete(ctx context.Context, acrClient api.AcrCLIClientInterface, repoName string) (*[]acr.ManifestAttributesBase, error) {
	lastManifestDigest := ""
	manifestsToDelete := []acr.ManifestAttributesBase{}
	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
	if err != nil {
		if resultManifests != nil && resultManifests.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return &manifestsToDelete, nil
		}
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
				// If a manifest list is found and it has tags then all the dependent digests are
				// marked to not be deleted.
				var manifestListBytes []byte
				manifestListBytes, err = acrClient.GetManifest(ctx, repoName, *manifest.Digest)
				if err != nil {
					return nil, err
				}
				var manifestList multiArchManifest
				err = json.Unmarshal(manifestListBytes, &manifestList)
				if err != nil {
					return nil, err
				}
				for _, dependentDigest := range manifestList.Manifests {
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
	// Remove all manifests that should not be deleted
	for i := 0; i < len(candidatesToDelete); i++ {
		if _, ok := doNotDelete[*candidatesToDelete[i].Digest]; !ok {
			// if a manifest has no tags, is not part of a manifest list and can be deleted then it is added to the
			// manifestToDelete array.
			if *(*candidatesToDelete[i].ChangeableAttributes).DeleteEnabled {
				manifestsToDelete = append(manifestsToDelete, candidatesToDelete[i])
			}
		}
	}
	return &manifestsToDelete, nil
}

// dryRunPurge outputs everything that would be deleted if the purge command was executed
func dryRunPurge(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, ago string, filter string, untagged bool) (int, int, error) {
	deletedTagsCount := 0
	deletedManifestsCount := 0
	// In order to keep track if a manifest would get deleted a map is defined that as a  key has the manifest
	// digest and as the value the number of tags (referencing said manifests) that were deleted.
	deletedTags := map[string]int{}
	fmt.Printf("Deleting tags for repository: %s\n", repoName)
	agoDuration, err := parseDuration(ago)
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
	// The loop to get the deleted tags follows the same logic as the one in the purgeTags function
	for {
		tagsToDelete, newLastTag, err := getTagsToDelete(ctx, acrClient, repoName, regex, timeToCompare, lastTag)
		lastTag = newLastTag
		if err != nil {
			return -1, -1, err
		}

		if tagsToDelete != nil {
			for _, tag := range *tagsToDelete {
				// For every tag that would be deleted first check if it exists in the map, if it doesn't add a new key
				// with value 1 and if it does just add 1 to the existent value.
				if _, exists := deletedTags[*tag.Digest]; exists {
					deletedTags[*tag.Digest]++
				} else {
					deletedTags[*tag.Digest] = 1
				}
				fmt.Printf("%s/%s:%s\n", loginURL, repoName, *tag.Name)
				deletedTagsCount++
			}
		}
		if len(lastTag) == 0 {
			break
		}
	}
	if untagged {
		fmt.Printf("Deleting manifests for repository: %s\n", repoName)
		// The countMap contains a map that for every digest contains how many tags are referencing it.
		countMap, err := countTagsByManifest(ctx, acrClient, repoName)
		if err != nil {
			return -1, -1, err
		}
		lastManifestDigest := ""
		resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			if resultManifests != nil && resultManifests.StatusCode == http.StatusNotFound {
				fmt.Printf("%s repository not found\n", repoName)
				return 0, 0, nil
			}
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
				// If the manifest is manifest list and would not get deleted then mark it's dependant manifests as not deletable.
				if *manifest.MediaType == manifestListContentType && (*countMap)[*manifest.Digest] != deletedTags[*manifest.Digest] {
					var manifestListBytes []byte
					manifestListBytes, err = acrClient.GetManifest(ctx, repoName, *manifest.Digest)
					if err != nil {
						return -1, -1, err
					}
					var manifestList multiArchManifest
					err = json.Unmarshal(manifestListBytes, &manifestList)
					if err != nil {
						return -1, -1, err
					}
					for _, dependentDigest := range manifestList.Manifests {
						doNotDelete[dependentDigest.Digest] = true
					}
				} else if (*countMap)[*manifest.Digest] == deletedTags[*manifest.Digest] {
					// If the manifest has the same amount of tags as the amount of tags deleted then it is a candidate for deletion.
					candidatesToDelete = append(candidatesToDelete, manifest)
				}
			}
			lastManifestDigest = *manifests[len(manifests)-1].Digest
			resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
			if err != nil {
				return -1, -1, err
			}
		}
		// Just print manifests that would be deleted.
		for i := 0; i < len(candidatesToDelete); i++ {
			if _, ok := doNotDelete[*candidatesToDelete[i].Digest]; !ok {
				fmt.Printf("%s/%s@%s\n", loginURL, repoName, *candidatesToDelete[i].Digest)
				deletedManifestsCount++
			}
		}
	}

	return deletedTagsCount, deletedManifestsCount, nil
}

// countTagsByManifest returns a map that for a given manifest digest contains the number of tags associated to it.
func countTagsByManifest(ctx context.Context, acrClient api.AcrCLIClientInterface, repoName string) (*map[string]int, error) {
	countMap := map[string]int{}
	lastTag := ""
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "", lastTag)
	if err != nil {
		if resultTags != nil && resultTags.StatusCode == http.StatusNotFound {
			//Repository not found, will be handled in the GetAcrManifests call
			return nil, nil
		}
		return nil, err
	}
	for resultTags != nil && resultTags.TagsAttributes != nil {
		tags := *resultTags.TagsAttributes
		for _, tag := range tags {
			// if a digest already exists in the map then add 1 to the number of tags it has.
			if _, exists := countMap[*tag.Digest]; exists {
				countMap[*tag.Digest]++
			} else {
				countMap[*tag.Digest] = 1
			}
		}

		lastTag = *tags[len(tags)-1].Name
		// Keep on iterating until the resultTags or resultTags.TagsAttributes is nil
		resultTags, err = acrClient.GetAcrTags(ctx, repoName, "", lastTag)
		if err != nil {
			return nil, err
		}
	}
	return &countMap, nil
}

// In order to parse the content of a mutliarch manifest string the following structs were defined.
type multiArchManifest struct {
	Manifests     []manifest `json:"manifests"`
	MediaType     string     `json:"mediaType"`
	SchemaVersion int        `json:"schemaVersion"`
}

type manifest struct {
	Digest    string   `json:"digest"`
	MediaType string   `json:"mediaType"`
	Platform  platform `json:"platform"`
	Size      int64    `json:"size"`
}

type platform struct {
	Architecture string `json:"architecture"`
	Os           string `json:"os"`
}
