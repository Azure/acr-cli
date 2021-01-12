package worker

import (
	"context"
	"runtime"
	"time"

	"github.com/Azure/acr-cli/cmd/api"
)

// Purger defines a worker pool that can process PurgeJobs.
type Purger struct {
	wp *workerPool
}

// NewPurger creates a new Purger.
func NewPurger(batchSize int, acrClient api.AcrCLIClientInterface) *Purger {
	procNum := int64(runtime.GOMAXPROCS(0))
	worker := newPurgeWorker(acrClient)
	workerPool := newWorkerPool(worker, procNum, batchSize)
	return &Purger{
		wp: workerPool,
	}
}

// ErrChan returns a channel that contains workerError occurred during processing PurgeJobs.
func (p *Purger) ErrChan() chan WorkerError {
	return p.wp.errChan
}

// StartPurgeManifest starts a purge manifest job in worker pool.
func (p *Purger) StartPurgeManifest(ctx context.Context, loginURL string, repoName string, digest string) {
	newJob := PurgeJob{
		LoginURL:    loginURL,
		RepoName:    repoName,
		Digest:      digest,
		JobType:     PurgeManifest,
		TimeCreated: time.Now().UTC(),
	}
	p.wp.start(ctx, newJob)
}

// StartPurgeTag starts a purge tag job in worker pool.
func (p *Purger) StartPurgeTag(ctx context.Context, loginURL string, repoName string, tag string, digest string) {
	newJob := PurgeJob{
		LoginURL:    loginURL,
		RepoName:    repoName,
		Tag:         tag,
		JobType:     PurgeTag,
		Digest:      digest,
		TimeCreated: time.Now().UTC(),
	}
	p.wp.start(ctx, newJob)
}

// Wait waits for all the workers in worker pool to finish.
func (p *Purger) Wait(ctx context.Context) {
	p.wp.wait(ctx)
}
