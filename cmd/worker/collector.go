// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import "time"

// JobQueue is the queue that holds all the jobs that are going to be executed.
var JobQueue = make(chan PurgeJob, 100)

// ErrorChannel is a channel for communication with the cobra command to verify
// that no errors have happened so far.
var ErrorChannel = make(chan workerError, 100)

// QueuePurgeTag creates a PurgeTag job and queues it.
func QueuePurgeTag(loginURL string, repoName string, archive string, tag string, digest string) {
	newJob := PurgeJob{
		LoginURL:          loginURL,
		RepoName:          repoName,
		Tag:               tag,
		ArchiveRepository: archive,
		JobType:           PurgeTag,
		Digest:            digest,
		TimeCreated:       time.Now().UTC(),
	}
	JobQueue <- newJob
}

// QueuePurgeManifest creates a new PurgeManifest job and queues it.
func QueuePurgeManifest(loginURL string, repoName string, archive string, digest string) {
	newJob := PurgeJob{
		LoginURL:          loginURL,
		RepoName:          repoName,
		Digest:            digest,
		ArchiveRepository: archive,
		JobType:           PurgeManifest,
		TimeCreated:       time.Now().UTC(),
	}
	JobQueue <- newJob
}
