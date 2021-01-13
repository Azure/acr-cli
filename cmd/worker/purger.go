package worker

import (
	"context"
	"sync"

	"github.com/Azure/acr-cli/cmd/api"
)

// Purger defines a worker pool, an ACR client and a wait group to manage concurrent PurgeJobs.
type Purger struct {
	workerPool *workerPool
	acrClient  api.AcrCLIClientInterface
	wg         sync.WaitGroup
}

// NewPurger creates a new Purger.
func NewPurger(workerNum int, batchSize int, acrClient api.AcrCLIClientInterface) *Purger {
	workerPool := newWorkerPool(workerNum, batchSize)
	return &Purger{
		workerPool: workerPool,
		acrClient:  acrClient,
		wg:         sync.WaitGroup{},
	}
}

// ErrChan returns a channel that stores workerError occurred during processing PurgeJobs.
func (p *Purger) ErrChan() chan PurgeJobError {
	return p.workerPool.errChan
}

// BatchSize returns the size of a batch of PurgeJobs.
func (p *Purger) BatchSize() int {
	return p.workerPool.batchSize
}

// StartPurgeManifest starts a purge manifest job in worker pool.
func (p *Purger) StartPurgeManifest(ctx context.Context, loginURL string, repoName string, digest string) {
	pmJob := NewPurgeManifestJob(loginURL, repoName, digest)

	p.workerPool.start(ctx, pmJob, p.acrClient, &p.wg)
}

// StartPurgeTag starts a purge tag job in worker pool.
func (p *Purger) StartPurgeTag(ctx context.Context, loginURL string, repoName string, digest string, tag string) {
	ptJob := NewPurgeTagJob(loginURL, repoName, digest, tag)

	p.workerPool.start(ctx, ptJob, p.acrClient, &p.wg)
}

// Wait waits for all the workers in worker pool to finish.
func (p *Purger) Wait() {
	p.wg.Wait()
}
