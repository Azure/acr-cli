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
	dangling     bool
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
				client, err := dockerAuth.NewClient(tagParams.configs...)
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
				// TODO: fetch token via oauth
				//auth = api.BearerAuth(password)
			} else {
				acrClient = api.NewAcrCLIClientWithBasicAuth(loginURL, purgeParams.username, purgeParams.password)
			}
			worker.StartDispatcher(&wg, acrClient, purgeParams.numWorkers)
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

			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&purgeParams.registryName, "registry", "r", "", "Registry name")
	cmd.PersistentFlags().StringVarP(&purgeParams.username, "username", "u", "", "Registry username")
	cmd.PersistentFlags().StringVarP(&purgeParams.password, "password", "p", "", "Registry password")
	cmd.PersistentFlags().StringVar(&purgeParams.accessToken, "access-token", "", "Access token")
	cmd.Flags().BoolVar(&purgeParams.dangling, "dangling", false, "Just remove dangling manifests")
	cmd.Flags().IntVar(&purgeParams.numWorkers, "concurrency", defaultNumWorkers, "The number of concurrent requests sent to the registry")
	cmd.Flags().StringVar(&purgeParams.ago, "ago", "1d", "The images that were created before this time stamp will be deleted")
	cmd.Flags().StringVar(&purgeParams.repoName, "repository", "", "The repository which will be purged.")
	cmd.Flags().StringVarP(&purgeParams.filter, "filter", "f", "", "Given as a regular expression, if a tag matches the pattern and is older than the time specified in ago it gets deleted.")
	cmd.Flags().StringVar(&purgeParams.archive, "archive-repository", "", "Instead of deleting manifests they will be moved to the repo specified here")

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
	var matches bool
	var lastUpdateTime time.Time
	lastTag := ""
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "", lastTag)
	if err != nil {
		return err
	}
	for resultTags != nil && resultTags.TagsAttributes != nil {
		tags := *resultTags.TagsAttributes
		for _, tag := range tags {
			tagName := *tag.Name
			//A regex filter was specified
			if len(filter) > 0 {
				matches = regex.MatchString(tagName)
				if !matches {
					continue
				}
			}
			lastUpdateTime, err = time.Parse(time.RFC3339Nano, *tag.LastUpdateTime)
			if err != nil {
				return err
			}
			if lastUpdateTime.Before(timeToCompare) {
				wg.Add(1)
				worker.QueuePurgeTag(loginURL, repoName, archive, tagName, *tag.Digest)
			}
		}
		wg.Wait()
		for len(worker.ErrorChannel) > 0 {
			wErr := <-worker.ErrorChannel
			if wErr.Error != nil {
				return wErr.Error
			}
		}
		lastTag = *tags[len(tags)-1].Name
		resultTags, err = acrClient.GetAcrTags(ctx, repoName, "", lastTag)
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
