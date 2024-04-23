// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/acr-cli/cmd/api"
)

type annotateJob interface {
	processAnnotate(context.Context, api.ORASClientInterface) error
}

type annotateJobBase struct {
	loginURL     string
	repoName     string
	timeCreated  time.Time
	artifactType string
	annotations  map[string]string
}

type annotateManifestJob struct {
	annotateJobBase
	digest string
}

type annotateTagJob struct {
	annotateJobBase
	tag string
}

func newAnnotateManifestJob(loginURL string, repoName string, artifactType string, annotations map[string]string, digest string) *annotateManifestJob {
	base := annotateJobBase{
		loginURL:     loginURL,
		repoName:     repoName,
		timeCreated:  time.Now().UTC(),
		artifactType: artifactType,
		annotations:  annotations,
	}

	return &annotateManifestJob{
		annotateJobBase: base,
		digest:          digest,
	}
}

func newAnnotateTagJob(loginURL string, repoName string, artifactType string, annotations map[string]string, tag string) *annotateTagJob {
	base := annotateJobBase{
		loginURL:     loginURL,
		repoName:     repoName,
		timeCreated:  time.Now().UTC(),
		artifactType: artifactType,
		annotations:  annotations,
	}

	return &annotateTagJob{
		annotateJobBase: base,
		tag:             tag,
	}
}

// process calls acrClient to annotate a manifest.
func (job *annotateManifestJob) processAnnotate(ctx context.Context, orasClient api.ORASClientInterface) error {
	ref := fmt.Sprintf("%s/%s@%s", job.loginURL, job.repoName, job.digest)
	err := orasClient.Annotate(ctx, ref, job.artifactType, job.annotations)
	if err == nil {
		fmt.Printf("Annotated %s/%s@%s\n", job.loginURL, job.repoName, job.digest)
		return nil
	}

	return err
}

// process calls acrClient to annotate a tag.
func (job *annotateTagJob) processAnnotate(ctx context.Context, orasClient api.ORASClientInterface) error {
	ref := fmt.Sprintf("%s/%s:%s", job.loginURL, job.repoName, job.tag)
	err := orasClient.Annotate(ctx, ref, job.artifactType, job.annotations)
	if err == nil {
		fmt.Printf("Annotated %s/%s:%s\n", job.loginURL, job.repoName, job.tag)
		return nil
	}

	return err
}
