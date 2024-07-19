// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/acr-cli/internal/api"
)

type annotateJob struct {
	jobBase
	client       api.ORASClientInterface
	artifactType string
	annotations  map[string]string
	ref          string
}

func newAnnotateJob(loginURL string, repoName string, artifactType string, annotations map[string]string, client api.ORASClientInterface, ref string) *annotateJob {
	base := jobBase{
		loginURL:    loginURL,
		repoName:    repoName,
		timeCreated: time.Now().UTC(),
	}

	return &annotateJob{
		jobBase:      base,
		client:       client,
		artifactType: artifactType,
		annotations:  annotations,
		ref:          ref,
	}
}

// execute calls acrClient to annotate a manifest or tag.
func (job *annotateJob) execute(ctx context.Context) error {
	ref := fmt.Sprintf("%s/%s@%s", job.loginURL, job.repoName, job.ref)
	err := job.client.Annotate(ctx, ref, job.artifactType, job.annotations)
	if err == nil {
		fmt.Printf("Annotated %s/%s@%s\n", job.loginURL, job.repoName, job.ref)
		return nil
	}

	return err
}
