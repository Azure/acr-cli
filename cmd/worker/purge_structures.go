// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import "time"

// PurgeJob describes a purge job, contains all necessary parameters to execute job.
type PurgeJob struct {
	LoginURL    string
	RepoName    string
	Tag         string
	Digest      string
	TimeCreated time.Time
	JobType     JobTypeEnum
}

// JobTypeEnum describes the type of PurgeJob.
type JobTypeEnum string

const (
	// PurgeTag refers to a tag deletion job
	PurgeTag JobTypeEnum = "purgetag"

	//PurgeManifest refers to a manifest deletion job
	PurgeManifest JobTypeEnum = "purgemanifest"
)

// workerError describes an error which occured inside a worker.
type workerError struct {
	JobType JobTypeEnum
	Error   error
}
