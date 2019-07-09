// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
	"github.com/Azure/acr-cli/cmd/worker"
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
	dangling     bool
	dryRun       bool
	numWorkers   int
}

var wg sync.WaitGroup

func newPurgeCmd(out io.Writer) *cobra.Command {
	var parameters purgeParameters
	cmd := &cobra.Command{
		Use:     "purge",
		Short:   "Delete images from a registry.",
		Long:    purgeLongMessage,
		Example: exampleMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			worker.StartDispatcher(&wg, parameters.numWorkers)
			ctx := context.Background()
			loginURL := api.LoginURL(parameters.registryName)
			var auth string
			if len(parameters.accessToken) > 0 && (len(parameters.username) > 0 || len(parameters.password) > 0) {
				return errors.New("bearer token and username, password are mutually exclusive")
			}
			if len(parameters.accessToken) > 0 {
				auth = api.BearerAuth(parameters.accessToken)
			} else {
				if len(parameters.username) > 0 && len(parameters.password) > 0 {
					auth = api.BasicAuth(parameters.username, parameters.password)
				} else {
					return errors.New("please specify authentication credentials")
				}
			}
			if !parameters.dryRun {
				if !parameters.dangling {
					err := PurgeTags(ctx, loginURL, auth, parameters.repoName, parameters.ago, parameters.filter)
					if err != nil {
						return err
					}
				}
				err := PurgeDanglingManifests(ctx, loginURL, auth, parameters.repoName)
				if err != nil {
					return err
				}
			} else {
				err := DryRunPurge(ctx, loginURL, auth, parameters.repoName, parameters.ago, parameters.filter, parameters.dangling)
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&parameters.registryName, "registry", "r", "", "Registry name")
	cmd.PersistentFlags().StringVarP(&parameters.username, "username", "u", "", "Registry username")
	cmd.PersistentFlags().StringVarP(&parameters.password, "password", "p", "", "Registry password")
	cmd.PersistentFlags().StringVar(&parameters.accessToken, "access-token", "", "Access token")
	cmd.Flags().BoolVar(&parameters.dangling, "dangling", false, "Just remove dangling manifests")
	cmd.Flags().IntVar(&parameters.numWorkers, "concurrency", defaultNumWorkers, "The number of concurrent requests sent to the registry")
	cmd.Flags().StringVar(&parameters.ago, "ago", "1d", "The images that were created before this time stamp will be deleted")
	cmd.Flags().BoolVar(&parameters.dryRun, "dry-run", false, "Don't actually remove any tag or manifest, instead, show if they would be deleted")
	cmd.Flags().StringVar(&parameters.repoName, "repository", "", "The repository which will be purged.")
	cmd.Flags().StringVarP(&parameters.filter, "filter", "f", "", "Given as a regular expression, if a tag matches the pattern and is older than the time specified in ago it gets deleted.")

	cmd.MarkPersistentFlagRequired("registry")
	cmd.MarkFlagRequired("repository")
	return cmd
}

// PurgeTags deletes all tags that are older than the ago value and that match the filter string (if present).
func PurgeTags(ctx context.Context, loginURL string, auth string, repoName string, ago string, filter string) error {
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
	tagsToDelete, lastTag, err := GetTagsToDelete(ctx, loginURL, auth, repoName, regex, timeToCompare, "")
	if err != nil {
		return err
	}
	for len(lastTag) > 0 {
		for _, tag := range *tagsToDelete {
			wg.Add(1)
			worker.QueuePurgeTag(loginURL, auth, repoName, *tag.Name)
		}
		wg.Wait()
		for len(worker.ErrorChannel) > 0 {
			wErr := <-worker.ErrorChannel
			if wErr.Error != nil {
				return wErr.Error
			}
		}
		tagsToDelete, lastTag, err = GetTagsToDelete(ctx, loginURL, auth, repoName, regex, timeToCompare, lastTag)
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
// and an error in case it occurred
func GetTagsToDelete(ctx context.Context,
	loginURL string,
	auth string,
	repoName string,
	filter *regexp.Regexp,
	timeToCompare time.Time,
	lastTag string) (*[]acr.TagAttributesBase, string, error) {
	var matches bool
	var lastUpdateTime time.Time
	resultTags, err := api.AcrListTags(ctx, loginURL, auth, repoName, "", lastTag)
	if err != nil {
		return nil, "", err
	}
	newLastTag := ""
	if resultTags != nil && resultTags.Tags != nil && len(*resultTags.Tags) > 0 {
		tags := *resultTags.Tags
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
func PurgeDanglingManifests(ctx context.Context, loginURL string, auth string, repoName string) error {
	lastManifestDigest := ""
	resultManifests, err := api.AcrListManifests(ctx, loginURL, auth, repoName, "", lastManifestDigest)
	if err != nil {
		return err
	}
	for resultManifests != nil && resultManifests.Manifests != nil {
		manifests := *resultManifests.Manifests
		for _, manifest := range manifests {
			if manifest.Tags == nil {
				wg.Add(1)
				worker.QueuePurgeManifest(loginURL, auth, repoName, *manifest.Digest)
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
		resultManifests, err = api.AcrListManifests(ctx, loginURL, auth, repoName, "", lastManifestDigest)
		if err != nil {
			return err
		}
	}
	return nil
}

// DryRunPurge outputs everything that would be deleted if the purge command was executed
func DryRunPurge(ctx context.Context, loginURL string, auth string, repoName string, ago string, filter string, dangling bool) error {
	deletedTags := map[string][]string{}
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
		tagsToDelete, lastTag, err := GetTagsToDelete(ctx, loginURL, auth, repoName, regex, timeToCompare, "")

		if err != nil {
			return err
		}
		for len(lastTag) > 0 {
			for _, tag := range *tagsToDelete {
				if _, ok := deletedTags[*tag.Digest]; ok {
					deletedTags[*tag.Digest] = append(deletedTags[*tag.Digest], *tag.Name)
				} else {
					deletedTags[*tag.Digest] = []string{*tag.Name}
				}
				fmt.Printf("%s/%s:%s\n", loginURL, repoName, *tag.Name)
			}
			tagsToDelete, lastTag, err = GetTagsToDelete(ctx, loginURL, auth, repoName, regex, timeToCompare, lastTag)
			if err != nil {
				return err
			}
		}
	}
	lastManifestDigest := ""
	resultManifests, err := api.AcrListManifests(ctx, loginURL, auth, repoName, "", lastManifestDigest)
	if err != nil {
		return err
	}
	for resultManifests != nil && resultManifests.Manifests != nil {
		manifests := *resultManifests.Manifests
		for _, manifest := range manifests {
			if manifest.Tags == nil || len(*manifest.Tags) == len(deletedTags[*manifest.Digest]) {
				fmt.Printf("%s/%s@%s\n", loginURL, repoName, *manifest.Digest)
			}
		}
		lastManifestDigest = *manifests[len(manifests)-1].Digest
		resultManifests, err = api.AcrListManifests(ctx, loginURL, auth, repoName, "", lastManifestDigest)
		if err != nil {
			return err
		}
	}
	return nil
}
