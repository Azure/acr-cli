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

// JobTypeEnum describes the type of PurgeJob.
type JobTypeEnum string

const (
	// PurgeTag refers to a tag deletion job
	PurgeTag JobTypeEnum = "purgetag"

	//PurgeManifest refers to a manifest deletion job
	PurgeManifest JobTypeEnum = "purgemanifest"
)

// PurgeJobError describes an error which occurred during processing PurgeJob.
type PurgeJobError struct {
	JobType JobTypeEnum
	Error   error
}

type PurgeJob interface {
	Process(context.Context, api.AcrCLIClientInterface) PurgeJobError
}

type PurgeJobBase struct {
	LoginURL    string
	RepoName    string
	Digest      string
	TimeCreated time.Time
	JobType     JobTypeEnum
}

type PurgeManifestJob struct {
	PurgeJobBase
}

type PurgeTagJob struct {
	PurgeJobBase
	Tag string
}

func NewPurgeManifestJob(loginURL string, repoName string, digest string) *PurgeManifestJob {
	base := PurgeJobBase{
		LoginURL:    loginURL,
		RepoName:    repoName,
		Digest:      digest,
		JobType:     PurgeManifest,
		TimeCreated: time.Now().UTC(),
	}

	return &PurgeManifestJob{
		PurgeJobBase: base,
	}
}

func NewPurgeTagJob(loginURL string, repoName string, digest string, tag string) *PurgeTagJob {
	base := PurgeJobBase{
		LoginURL:    loginURL,
		RepoName:    repoName,
		Digest:      digest,
		JobType:     PurgeTag,
		TimeCreated: time.Now().UTC(),
	}

	return &PurgeTagJob{
		PurgeJobBase: base,
		Tag:          tag,
	}
}

func (job *PurgeManifestJob) Process(ctx context.Context, acrClient api.AcrCLIClientInterface) (pjErr PurgeJobError) {
	// // In case a manifest is going to be purged DeleteManifest method is used.
	resp, err := acrClient.DeleteManifest(ctx, job.RepoName, job.Digest)
	if err != nil {
		if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
			// If the manifest is not found it can be assumed to have been deleted.
			fmt.Printf("Skipped %s/%s@%s, HTTP status: %d\n", job.LoginURL, job.RepoName, job.Digest, resp.StatusCode)
		} else {
			pjErr = PurgeJobError{
				JobType: PurgeTag,
				Error:   err,
			}
		}
	} else {
		fmt.Printf("%s/%s@%s\n", job.LoginURL, job.RepoName, job.Digest)
	}

	return
}

func (job *PurgeTagJob) Process(ctx context.Context, acrClient api.AcrCLIClientInterface) (pjErr PurgeJobError) {
	// In case a tag is going to be purged DeleteAcrTag method is used.
	resp, err := acrClient.DeleteAcrTag(ctx, job.RepoName, job.Tag)
	if err != nil {
		if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
			// If the tag is not found it can be assumed to have been deleted.
			fmt.Printf("Skipped %s/%s:%s, HTTP status: %d\n", job.LoginURL, job.RepoName, job.Tag, resp.StatusCode)
		} else {
			pjErr = PurgeJobError{
				JobType: PurgeTag,
				Error:   err,
			}
		}
	} else {
		fmt.Printf("%s/%s:%s\n", job.LoginURL, job.RepoName, job.Tag)
	}

	return
}
