package worker

import (
	"context"
	"sync"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
)

// Purger purges tags or manifests concurrently.
type Purger struct {
	pool      *pool
	acrClient api.AcrCLIClientInterface
	loginURL  string
	repoName  string
}

// NewPurger creates a new Purger.
func NewPurger(poolSize int, acrClient api.AcrCLIClientInterface, loginURL string, repoName string) *Purger {
	return &Purger{
		pool:      newPool(poolSize),
		acrClient: acrClient,
		loginURL:  loginURL,
		repoName:  repoName,
	}
}

// process starts purge jobs in worker pool, and returns a count of successful jobs and the first error occurred.
func (p *Purger) process(ctx context.Context, jobs *[]purgeJob) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var succ int64
	errChan := make(chan error)

	// Start purge jobs in worker pool.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, job := range *jobs {
			p.pool.start(ctx, job, p.acrClient, errChan, &wg, &succ)
		}
	}()

	// Wait for all purge jobs to finish.
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// If there are errors occurred during processing purge jobs, record the first error and cancel other jobs.
	var firstErr error
	for err := range errChan {
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}

	return int(succ), firstErr
}

// PurgeTags purges a list of tags concurrently, and returns a count of deleted tags and the first error occurred.
func (p *Purger) PurgeTags(ctx context.Context, tags *[]acr.TagAttributesBase) (int, error) {
	jobs := make([]purgeJob, len(*tags))
	for i, tag := range *tags {
		jobs[i] = newPurgeTagJob(p.loginURL, p.repoName, *tag.Name)
	}

	return p.process(ctx, &jobs)
}

// PurgeManifests purges a list of manifests concurrently, and returns a count of deleted manifests and the first error occurred.
func (p *Purger) PurgeManifests(ctx context.Context, manifests *[]acr.ManifestAttributesBase) (int, error) {
	jobs := make([]purgeJob, len(*manifests))
	for i, manifest := range *manifests {
		jobs[i] = newPurgeManifestJob(p.loginURL, p.repoName, *manifest.Digest)
	}

	return p.process(ctx, &jobs)
}
