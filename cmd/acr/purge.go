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

	maxConcurrentWorkers = 6
)

type purgeParameters struct {
	registryName string
	username     string
	password     string
	accessToken  string
	ago          string
	dangling     bool
	filter       string
	repoName     string
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
			worker.StartDispatcher(&wg, maxConcurrentWorkers)
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

			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&parameters.registryName, "registry", "r", "", "Registry name")
	cmd.PersistentFlags().StringVarP(&parameters.username, "username", "u", "", "Registry username")
	cmd.PersistentFlags().StringVarP(&parameters.password, "password", "p", "", "Registry password")
	cmd.PersistentFlags().StringVar(&parameters.accessToken, "access-token", "", "Access token")
	cmd.Flags().StringVar(&parameters.ago, "ago", "1d", "The images that were created before this time stamp will be deleted")
	cmd.Flags().BoolVar(&parameters.dangling, "dangling", false, "Just remove dangling manifests")
	cmd.Flags().StringVarP(&parameters.filter, "filter", "f", "", "Given as a regular expression, if a tag matches the pattern and is older than the time specified in ago it gets deleted.")
	cmd.Flags().StringVar(&parameters.repoName, "repository", "", "The repository which will be purged.")

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
	var matches bool
	var lastUpdateTime time.Time
	lastTag := ""
	resultTags, err := api.AcrListTags(ctx, loginURL, auth, repoName, "", lastTag)
	if err != nil {
		return err
	}
	for resultTags != nil && resultTags.Tags != nil {
		tags := *resultTags.Tags
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
				worker.QueuePurgeTag(loginURL, auth, repoName, tagName)
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
		resultTags, err = api.AcrListTags(ctx, loginURL, auth, repoName, "", lastTag)
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
