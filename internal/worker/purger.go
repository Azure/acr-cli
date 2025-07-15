// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/internal/api"
	"github.com/alitto/pond/v2"
)

// Purger purges tags or manifests concurrently.
type Purger struct {
	Executer
	acrClient api.AcrCLIClientInterface
	includeLocked bool
}

// NewPurger creates a new Purger. Purgers are currently repository specific
func NewPurger(repoParallelism int, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, includeLocked bool) *Purger {
	executeBase := Executer{
		// Use a queue size 3x the pool size to buffer enough tasks and keep workers busy and avoiding
		// slowdown due to task scheduling blocking.
		pool:     pond.NewPool(repoParallelism, pond.WithQueueSize(repoParallelism*3), pond.WithNonBlocking(false)),
		loginURL: loginURL,
		repoName: repoName,
	}
	return &Purger{
		Executer:  executeBase,
		acrClient: acrClient,
		includeLocked: includeLocked,
	}
}

// PurgeTags purges a list of tags concurrently, and returns a count of deleted tags and the first error occurred.
func (p *Purger) PurgeTags(ctx context.Context, tags []acr.TagAttributesBase) (int, error) {
	var deletedTags atomic.Int64 // Count of successfully deleted tags
	group := p.pool.NewGroup()
	for _, tag := range tags {
		group.SubmitErr(func() error {
			// If include-locked is enabled and tag is locked, unlock it first
			if p.includeLocked && tag.ChangeableAttributes != nil {
				if (tag.ChangeableAttributes.DeleteEnabled != nil && !*tag.ChangeableAttributes.DeleteEnabled) ||
					(tag.ChangeableAttributes.WriteEnabled != nil && !*tag.ChangeableAttributes.WriteEnabled) {
					
					enabledTrue := true
					unlockAttrs := &acr.ChangeableAttributes{
						DeleteEnabled: &enabledTrue,
						WriteEnabled:  &enabledTrue,
					}
					
					_, unlockErr := p.acrClient.UpdateAcrTagAttributes(ctx, p.repoName, *tag.Name, unlockAttrs)
					if unlockErr != nil {
						fmt.Printf("Failed to unlock %s/%s:%s, error: %v\n", p.loginURL, p.repoName, *tag.Name, unlockErr)
						return unlockErr
					}
					fmt.Printf("Unlocked %s/%s:%s\n", p.loginURL, p.repoName, *tag.Name)
				}
			}
			
			resp, err := p.acrClient.DeleteAcrTag(ctx, p.repoName, *tag.Name)
			if err == nil {
				fmt.Printf("Deleted %s/%s:%s\n", p.loginURL, p.repoName, *tag.Name)
				// Increment the count of successfully deleted tags atomically
				deletedTags.Add(1)
				return nil
			}

			if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
				// If the tag is not found it can be assumed to have been deleted.
				deletedTags.Add(1)
				fmt.Printf("Skipped %s/%s:%s, HTTP status: %d\n", p.loginURL, p.repoName, *tag.Name, resp.StatusCode)
				return nil
			}

			fmt.Printf("Failed to delete %s/%s:%s, error: %v\n", p.loginURL, p.repoName, *tag.Name, err)
			return err
		})
	}
	err := group.Wait() // Error should be nil
	return int(deletedTags.Load()), err
}

// PurgeManifests purges a list of manifests concurrently, and returns a count of deleted manifests and the first error occurred.
func (p *Purger) PurgeManifests(ctx context.Context, manifests []string) (int, error) {
	var deletedManifests atomic.Int64 // Count of successfully deleted tags
	group := p.pool.NewGroup()
	for _, manifest := range manifests {
		group.SubmitErr(func() error {
			// If include-locked is enabled, we need to check if manifest is locked and unlock it first
			if p.includeLocked {
				manifestAttrs, err := p.acrClient.GetAcrManifestAttributes(ctx, p.repoName, manifest)
				if err == nil && manifestAttrs.ManifestAttributes != nil && manifestAttrs.ManifestAttributes.ChangeableAttributes != nil {
					if (manifestAttrs.ManifestAttributes.ChangeableAttributes.DeleteEnabled != nil && !*manifestAttrs.ManifestAttributes.ChangeableAttributes.DeleteEnabled) ||
						(manifestAttrs.ManifestAttributes.ChangeableAttributes.WriteEnabled != nil && !*manifestAttrs.ManifestAttributes.ChangeableAttributes.WriteEnabled) {
						
						enabledTrue := true
						unlockAttrs := &acr.ChangeableAttributes{
							DeleteEnabled: &enabledTrue,
							WriteEnabled:  &enabledTrue,
						}
						
						_, unlockErr := p.acrClient.UpdateAcrManifestAttributes(ctx, p.repoName, manifest, unlockAttrs)
						if unlockErr != nil {
							fmt.Printf("Failed to unlock %s/%s@%s, error: %v\n", p.loginURL, p.repoName, manifest, unlockErr)
							return unlockErr
						}
						fmt.Printf("Unlocked %s/%s@%s\n", p.loginURL, p.repoName, manifest)
					}
				}
			}
			
			resp, err := p.acrClient.DeleteManifest(ctx, p.repoName, manifest)
			if err == nil {
				fmt.Printf("Deleted %s/%s@%s\n", p.loginURL, p.repoName, manifest)
				// Increment the count of successfully deleted tags atomically
				deletedManifests.Add(1)
				return nil
			}

			if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
				// If the manifest is not found it can be assumed to have been deleted.
				deletedManifests.Add(1)
				fmt.Printf("Skipped %s/%s@%s, HTTP status: %d\n", p.loginURL, p.repoName, manifest, resp.StatusCode)
				return nil
			}

			fmt.Printf("Failed to delete %s/%s@%s, error: %v\n", p.loginURL, p.repoName, manifest, err)
			return err

		})
	}
	err := group.Wait()
	return int(deletedManifests.Load()), err
}
