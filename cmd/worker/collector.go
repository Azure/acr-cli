// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

// JobQueue is the queue that holds all the jobs that are going to be executed.
var JobQueue = make(chan PurgeJob, 100)

// ErrorChannel is a channel for communication with the cobra command to verify
// that no errors have happened so far.
var ErrorChannel = make(chan WorkerError, 100)

// QueuePurgeTag creates a PurgeTag job and queues it.
func QueuePurgeTag(loginURL string,
	auth string,
	repoName string,
	tag string) {
	newJob := PurgeJob{
		LoginURL: loginURL,
		Auth:     auth,
		RepoName: repoName,
		Tag:      tag,
		JobType:  PurgeTag,
	}
	JobQueue <- newJob
}

// QueuePurgeManifest creates a new PurgeManifest job and queues it.
func QueuePurgeManifest(loginURL string,
	auth string,
	repoName string,
	digest string) {
	newJob := PurgeJob{
		LoginURL: loginURL,
		Auth:     auth,
		RepoName: repoName,
		Digest:   digest,
		JobType:  PurgeManifest,
	}
	JobQueue <- newJob
}
