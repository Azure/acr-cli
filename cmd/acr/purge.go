// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Azure/acr-cli/acr"
	dockerAuth "github.com/Azure/acr-cli/auth/docker"
	"github.com/Azure/acr-cli/cmd/api"
	"github.com/Azure/acr-cli/cmd/worker"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const (
	purgeLongMessage = `acr purge: untag old images and delete dangling manifests.`
	exampleMessage   = `Untag old images and delete dangling manifests

Examples:

  - Delete all tags that are older than 1 day
    acr purge -r MyRegistry --repository MyRepository --ago 1d

  - Delete all tags that are older than 1 day and begin with hello
    acr purge -r MyRegistry --repository MyRepository --ago 1d --filter "^hello.*"

  - Delete all dangling manifests
	acr purge -r MyRegistry --repository MyRepository --dangling
`

	defaultNumWorkers = 6
)

type purgeParameters struct {
	registryName string
	username     string
	password     string
	accessToken  string
	ago          string
	filter       string
	repoName     string
	archive      string
	configs      []string
	dangling     bool
	dryRun       bool
	numWorkers   int
}

var wg sync.WaitGroup

func newPurgeCmd(out io.Writer) *cobra.Command {
	var purgeParams purgeParameters
	cmd := &cobra.Command{
		Use:     "purge",
		Short:   "Delete images from a registry.",
		Long:    purgeLongMessage,
		Example: exampleMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			loginURL := api.LoginURL(purgeParams.registryName)

			if purgeParams.username == "" && purgeParams.password == "" {
				client, err := dockerAuth.NewClient(purgeParams.configs...)
				if err != nil {
					return err
				}
				purgeParams.username, purgeParams.password, err = client.GetCredential(loginURL)
				if err != nil {
					return err
				}
			}

			var acrClient api.AcrCLIClient
			if purgeParams.username == "" {
				var err error
				acrClient, err = api.NewAcrCLIClientWithBearerAuth(loginURL, purgeParams.password)
				if err != nil {
					return errors.Wrap(err, "failed to purge repository")
				}
			} else {
				acrClient = api.NewAcrCLIClientWithBasicAuth(loginURL, purgeParams.username, purgeParams.password)
			}

			worker.StartDispatcher(&wg, acrClient, purgeParams.numWorkers)

			if !purgeParams.dryRun {
				if !purgeParams.dangling {
					err := PurgeTags(ctx, acrClient, loginURL, purgeParams.repoName, purgeParams.ago, purgeParams.filter, purgeParams.archive)
					if err != nil {
						return errors.Wrap(err, "failed to purge tags")
					}
				}
				err := PurgeDanglingManifests(ctx, acrClient, loginURL, purgeParams.repoName, purgeParams.archive)
				if err != nil {
					return errors.Wrap(err, "failed to purge manifests")
				}
			} else {
				err := DryRunPurge(ctx, acrClient, loginURL, purgeParams.repoName, purgeParams.ago, purgeParams.filter, purgeParams.dangling)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&purgeParams.registryName, "registry", "r", "", "Registry name")
	cmd.PersistentFlags().StringVarP(&purgeParams.username, "username", "u", "", "Registry username")
	cmd.PersistentFlags().StringVarP(&purgeParams.password, "password", "p", "", "Registry password")
	cmd.PersistentFlags().StringVar(&purgeParams.accessToken, "access-token", "", "Access token")
	cmd.Flags().BoolVar(&purgeParams.dangling, "dangling", false, "Just remove dangling manifests")
	cmd.Flags().BoolVar(&purgeParams.dryRun, "dry-run", false, "Don't actually remove any tag or manifest, instead, show if they would be deleted")
	cmd.Flags().IntVar(&purgeParams.numWorkers, "concurrency", defaultNumWorkers, "The number of concurrent requests sent to the registry")
	cmd.Flags().StringVar(&purgeParams.ago, "ago", "1d", "The images that were created before this time stamp will be deleted")
	cmd.Flags().StringVar(&purgeParams.repoName, "repository", "", "The repository which will be purged.")
	cmd.Flags().StringVarP(&purgeParams.filter, "filter", "f", "", "Given as a regular expression, if a tag matches the pattern and is older than the time specified in ago it gets deleted.")
	cmd.Flags().StringVar(&purgeParams.archive, "archive-repository", "", "Instead of deleting manifests they will be moved to the repo specified here")
	cmd.Flags().StringArrayVarP(&purgeParams.configs, "config", "c", nil, "auth config paths")

	cmd.MarkPersistentFlagRequired("registry")
	cmd.MarkFlagRequired("repository")
	return cmd
}

// PurgeTags deletes all tags that are older than the ago value and that match the filter string (if present).
func PurgeTags(ctx context.Context, acrClient api.AcrCLIClient, loginURL string, repoName string, ago string, filter string, archive string) error {
	agoDuration, err := ParseDuration(ago)
	if err != nil {
		return err
	}
	timeToCompare := time.Now().UTC()
	timeToCompare = timeToCompare.Add(agoDuration)
	regex, err := regexp.Compile(filter)
	if err != nil {
		return err
	}
	lastTag := ""
	tagsToDelete, lastTag, err := GetTagsToDelete(ctx, acrClient, repoName, regex, timeToCompare, "")
	if err != nil {
		return err
	}
	for len(lastTag) > 0 {
		for _, tag := range *tagsToDelete {
			wg.Add(1)
			worker.QueuePurgeTag(loginURL, repoName, archive, *tag.Name, *tag.Digest)
		}
		wg.Wait()
		for len(worker.ErrorChannel) > 0 {
			wErr := <-worker.ErrorChannel
			if wErr.Error != nil {
				return wErr.Error
			}
		}
		tagsToDelete, lastTag, err = GetTagsToDelete(ctx, acrClient, repoName, regex, timeToCompare, lastTag)
		if err != nil {
			return err
		}
	}
	return nil
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
			//A regex filter was specified
			if len(filter.String()) > 0 {
				matches = filter.MatchString(*tag.Name)
				if !matches {
					continue
				}
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
func PurgeDanglingManifests(ctx context.Context, acrClient api.AcrCLIClient, loginURL string, repoName string, archive string) error {
	lastManifestDigest := ""
	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
	if err != nil {
		return err
	}
	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
		manifests := *resultManifests.ManifestsAttributes
		for _, manifest := range manifests {
			if manifest.Tags == nil {
				wg.Add(1)
				worker.QueuePurgeManifest(loginURL, repoName, archive, *manifest.Digest)
			}
		}
		wg.Wait()
		for len(worker.ErrorChannel) > 0 {
			wErr := <-worker.ErrorChannel
			if wErr.Error != nil {
				return wErr.Error
			}
		}
		lastManifestDigest = *manifests[len(manifests)-1].Digest
		resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			return err
		}
	}
	return nil
}

// DryRunPurge outputs everything that would be deleted if the purge command was executed
func DryRunPurge(ctx context.Context, acrClient api.AcrCLIClient, loginURL string, repoName string, ago string, filter string, dangling bool) error {
	deletedTags := map[string]int{}
	if !dangling {
		agoDuration, err := ParseDuration(ago)
		if err != nil {
			return err
		}
		timeToCompare := time.Now().UTC()
		timeToCompare = timeToCompare.Add(agoDuration)
		regex, err := regexp.Compile(filter)
		if err != nil {
			return err
		}
		lastTag := ""
		tagsToDelete, lastTag, err := GetTagsToDelete(ctx, acrClient, repoName, regex, timeToCompare, "")

		if err != nil {
			return err
		}
		for len(lastTag) > 0 {
			for _, tag := range *tagsToDelete {
				if _, exists := deletedTags[*tag.Digest]; exists {
					deletedTags[*tag.Digest]++
				} else {
					deletedTags[*tag.Digest] = 1
				}
				fmt.Printf("%s/%s:%s\n", loginURL, repoName, *tag.Name)
			}
			tagsToDelete, lastTag, err = GetTagsToDelete(ctx, acrClient, repoName, regex, timeToCompare, lastTag)
			if err != nil {
				return err
			}
		}
	}
	countMap, err := CountTagsByManifest(ctx, acrClient, repoName)
	if err != nil {
		return err
	}
	lastManifestDigest := ""
	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
	if err != nil {
		return err
	}
	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
		manifests := *resultManifests.ManifestsAttributes
		for _, manifest := range manifests {
			if manifest.Tags == nil || countMap[*manifest.Digest] == deletedTags[*manifest.Digest] {
				fmt.Printf("%s/%s@%s\n", loginURL, repoName, *manifest.Digest)
			}
		}
		lastManifestDigest = *manifests[len(manifests)-1].Digest
		resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			return err
		}
	}
	return nil
}

// CountTagsByManifest returns a map that for a given manifest digest contains the number of tags associated to it.
func CountTagsByManifest(ctx context.Context, acrClient api.AcrCLIClient, repoName string) (map[string]int, error) {
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
	return countMap, nil
}
