// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	acrapi "github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
)

// PurgeWorker defines a worker that can process PurgeJobs.
type PurgeWorker struct {
	Job         chan PurgeJob
	WorkerQueue chan chan PurgeJob
	StopChan    chan bool
	wg          *sync.WaitGroup
	acrClient   api.AcrCLIClient
	m           *sync.Mutex
}

// NewPurgeWorker creates a new worker.
func NewPurgeWorker(wg *sync.WaitGroup, workerQueue chan chan PurgeJob, acrClient api.AcrCLIClient, m *sync.Mutex) PurgeWorker {
	worker := PurgeWorker{
		Job:         make(chan PurgeJob),
		WorkerQueue: workerQueue,
		wg:          wg,
		acrClient:   acrClient,
		m:           m,
	}
	return worker
}

// Start starts a new purgeWork with an infinite loop inside a goroutine.
func (pw *PurgeWorker) Start() {
	go func() {
		for {
			// Free worker, insert worker into worker queue.
			WorkerQueue <- pw.Job

			select {
			// If the worker has a job assigned begin processing it.
			case job := <-pw.Job:
				ctx := context.Background()
				pw.ProcessJob(ctx, job)
			// If the worker needs to be stopped then return.
			case <-pw.StopChan:
				return
			}
		}
	}()
}

// Stop notifies the worker to stop handling purge jobs
func (pw *PurgeWorker) Stop() {
	go func() {
		pw.StopChan <- true
	}()
}

// ProcessJob processes any job (currently PurgeTag and PurgeManifest)
func (pw *PurgeWorker) ProcessJob(ctx context.Context, job PurgeJob) {
	go func() {
		defer pw.wg.Done()
		var wErr workerError
		switch job.JobType {
		case PurgeTag:
			err := pw.acrClient.DeleteAcrTag(ctx, job.RepoName, job.Tag)
			if err != nil {
				wErr = workerError{
					JobType: PurgeTag,
					Error:   err,
				}
			} else {
				fmt.Printf("%s/%s:%s\n", job.LoginURL, job.RepoName, job.Tag)
			}
			// If this is an archive job, then update the metadata
			if len(job.ArchiveRepository) > 0 {
				pw.m.Lock()
				manifestMetadata, err := pw.acrClient.GetAcrManifestMetadata(ctx, job.RepoName, job.Digest)
				if err != nil {
					//Metadata might be empty try initializing it
					tagMetadata := AcrTags{Name: job.Tag, ArchiveTime: job.TimeCreated.String()}
					tagsMetadataArray := make([]AcrTags, 0)
					metadataObject := &AcrManifestMetadata{Digest: job.Digest, OriginalRepo: job.RepoName, Tags: append(tagsMetadataArray, tagMetadata)}
					err = pw.acrClient.UpdateAcrManifestMetadata(ctx, job.RepoName, job.Digest, metadataObject)
					if err != nil {
						wErr = workerError{
							JobType: PurgeTag,
							Error:   err,
						}
						pw.m.Unlock()
						break
					}

				} else {
					var metadataObject AcrManifestMetadata
					err = json.Unmarshal([]byte(manifestMetadata), &metadataObject)
					if err != nil {
						wErr = workerError{
							JobType: PurgeTag,
							Error:   err,
						}
						pw.m.Unlock()
						break
					}
					tagMetadata := AcrTags{Name: job.Tag, ArchiveTime: job.TimeCreated.String()}
					metadataObject.Tags = append(metadataObject.Tags, tagMetadata)
					err = pw.acrClient.UpdateAcrManifestMetadata(ctx, job.RepoName, job.Digest, metadataObject)
					if err != nil {
						wErr = workerError{
							JobType: PurgeTag,
							Error:   err,
						}
						pw.m.Unlock()
						break
					}
				}
				pw.m.Unlock()
			}
		case PurgeManifest:
			if len(job.ArchiveRepository) > 0 {
				manifestMetadata, err := pw.acrClient.GetAcrManifestMetadata(ctx, job.RepoName, job.Digest)
				// If there was an error reading the metadata then just delete the manifest normally
				if err == nil {
					var metadataObject AcrManifestMetadata
					err = json.Unmarshal([]byte(manifestMetadata), &metadataObject)
					if err != nil {
						wErr = workerError{
							JobType: PurgeTag,
							Error:   err,
						}
						break
					}
					//Tags empty len 0
					manifestBytes, err := pw.acrClient.GetManifest(ctx, job.RepoName, job.Digest)
					if err != nil {
						wErr = workerError{
							JobType: PurgeTag,
							Error:   err,
						}
						break
					}
					var manifestV2 acrapi.Manifest
					err = json.Unmarshal(manifestBytes, &manifestV2)
					if err != nil {
						wErr = workerError{
							JobType: PurgeTag,
							Error:   err,
						}
						break
					}
					if *manifestV2.MediaType == "application/vnd.docker.distribution.manifest.list.v2+json" {
						// TODO support importing v2 manifest list
					} else {
						err = pw.acrClient.AcrCrossReferenceLayer(ctx, job.ArchiveRepository, *(*manifestV2.Config).Digest, job.RepoName)
						if err != nil {
							wErr = workerError{
								JobType: PurgeTag,
								Error:   err,
							}
							break
						}
						for _, layer := range *manifestV2.Layers {
							err = pw.acrClient.AcrCrossReferenceLayer(ctx, job.ArchiveRepository, *layer.Digest, job.RepoName)
							if err != nil {
								wErr = workerError{
									JobType: PurgeTag,
									Error:   err,
								}
								break
							}
						}
						newTagName := job.RepoName + (job.Digest)[len("sha256:"):len("sha256:")+8]
						err = pw.acrClient.PutManifest(ctx, job.ArchiveRepository, newTagName, string(manifestBytes))
						if err != nil {
							wErr = workerError{
								JobType: PurgeTag,
								Error:   err,
							}
							break
						}
						err = pw.acrClient.UpdateAcrTagMetadata(ctx, job.ArchiveRepository, newTagName, metadataObject)
						if err != nil {
							wErr = workerError{
								JobType: PurgeTag,
								Error:   err,
							}
							break
						}
					}
				}
			}
			err := pw.acrClient.DeleteManifest(ctx, job.RepoName, job.Digest)
			if err != nil {
				wErr = workerError{
					JobType: PurgeTag,
					Error:   err,
				}
			} else {
				fmt.Printf("%s/%s@%s\n", job.LoginURL, job.RepoName, job.Digest)
			}
		}
		ErrorChannel <- wErr
	}()
}
