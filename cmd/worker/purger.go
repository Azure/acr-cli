package worker

import (
	"context"

	"github.com/Azure/acr-cli/cmd/api"
)

// Purger defines a worker pool, an ACR client and a wait group to manage concurrent PurgeJobs.
type Purger struct {
	*Pool
	acrClient api.AcrCLIClientInterface
}

// NewPurger creates a new Purger.
func NewPurger(ctx context.Context, poolSize int, acrClient api.AcrCLIClientInterface) *Purger {
	pool := NewPool(ctx, poolSize)
	return &Purger{
		Pool:      pool,
		acrClient: acrClient,
	}
}

// StartPurgeManifest starts a purge manifest job in worker pool.
func (p *Purger) StartPurgeManifest(loginURL string, repoName string, digest string) {
	job := newPurgeManifestJob(loginURL, repoName, digest)

	p.Start(job, p.acrClient)
}

// StartPurgeTag starts a purge tag job in worker pool.
func (p *Purger) StartPurgeTag(loginURL string, repoName string, tag string) {
	job := newPurgeTagJob(loginURL, repoName, tag)

	p.Start(job, p.acrClient)
}
