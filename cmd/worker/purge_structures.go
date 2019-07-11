// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import "time"

// PurgeJob describes a purge job, contains all necessary parameters to execute job.
type PurgeJob struct {
	LoginURL          string
	RepoName          string
	ArchiveRepository string
	Tag               string
	Digest            string
	TimeCreated       time.Time
	JobType           JobTypeEnum
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

// AcrManifestMetadata the struct that is used to store original repository info
type AcrManifestMetadata struct {
	Digest         string    `json:"digest,omitempty"`
	OriginalRepo   string    `json:"repository,omitempty"`
	LastUpdateTime string    `json:"lastUpdateTime,omitempty"`
	Tags           []AcrTags `json:"tags,omitempty"`
}

// AcrTags stores the tag and the time it was archived
type AcrTags struct {
	Name        string `json:"name,omitempty"`
	ArchiveTime string `json:"archiveTime,omitempty"`
}
