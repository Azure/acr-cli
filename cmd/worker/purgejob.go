// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/acr-cli/cmd/api"
)

type purgeJob interface {
	process(context.Context, api.AcrCLIClientInterface) error
}

type purgeJobBase struct {
	loginURL    string
	repoName    string
	timeCreated time.Time
}

type purgeManifestJob struct {
	purgeJobBase
	digest string
}

type purgeTagJob struct {
	purgeJobBase
	tag string
}

func newPurgeManifestJob(loginURL string, repoName string, digest string) *purgeManifestJob {
	base := purgeJobBase{
		loginURL:    loginURL,
		repoName:    repoName,
		timeCreated: time.Now().UTC(),
	}

	return &purgeManifestJob{
		purgeJobBase: base,
		digest:       digest,
	}
}

func newPurgeTagJob(loginURL string, repoName string, tag string) *purgeTagJob {
	base := purgeJobBase{
		loginURL:    loginURL,
		repoName:    repoName,
		timeCreated: time.Now().UTC(),
	}

	return &purgeTagJob{
		purgeJobBase: base,
		tag:          tag,
	}
}

// process calls acrClient to delete a manifest
func (job *purgeManifestJob) process(ctx context.Context, acrClient api.AcrCLIClientInterface) error {
	resp, err := acrClient.DeleteManifest(ctx, job.repoName, job.digest)
	if err == nil {
		fmt.Printf("Deleted %s/%s@%s\n", job.loginURL, job.repoName, job.digest)
		return nil
	}

	if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
		// If the manifest is not found it can be assumed to have been deleted.
		fmt.Printf("Skipped %s/%s@%s, HTTP status: %d\n", job.loginURL, job.repoName, job.digest, resp.StatusCode)
		return nil
	}

	return err
}

// process calls acrClient to delete a tag
func (job *purgeTagJob) process(ctx context.Context, acrClient api.AcrCLIClientInterface) error {
	resp, err := acrClient.DeleteAcrTag(ctx, job.repoName, job.tag)
	if err == nil {
		fmt.Printf("Deleted %s/%s:%s\n", job.loginURL, job.repoName, job.tag)
		return nil
	}

	if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
		// If the tag is not found it can be assumed to have been deleted.
		fmt.Printf("Skipped %s/%s:%s, HTTP status: %d\n", job.loginURL, job.repoName, job.tag, resp.StatusCode)
		return nil
	}

	return err
}
