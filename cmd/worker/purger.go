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

// FlushErrChan flushes the error channel and returns the last error occurred during processing PurgeJobs.
func (p *Purger) FlushErrChan() error {
	return p.workerPool.flushErrChan()
}

// BatchSize returns the size of a batch of PurgeJobs.
func (p *Purger) BatchSize() int {
	return p.workerPool.batchSize
}

// StartPurgeManifest starts a purge manifest job in worker pool.
func (p *Purger) StartPurgeManifest(ctx context.Context, loginURL string, repoName string, digest string) {
	job := newPurgeManifestJob(loginURL, repoName, digest)

	p.workerPool.start(ctx, job, p.acrClient, &p.wg)
}

// StartPurgeTag starts a purge tag job in worker pool.
func (p *Purger) StartPurgeTag(ctx context.Context, loginURL string, repoName string, tag string) {
	job := newPurgeTagJob(loginURL, repoName, tag)

	p.workerPool.start(ctx, job, p.acrClient, &p.wg)
}

// Wait waits for all the workers in worker pool to finish.
func (p *Purger) Wait() {
	p.wg.Wait()
}
