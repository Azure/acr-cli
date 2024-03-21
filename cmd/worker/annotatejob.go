// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"

	// "fmt"
	// "net/http"
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
	err := orasClient.Annotate(ctx, job.repoName, job.digest, job.artifactType, job.annotations)
	if err == nil {
		fmt.Printf("Deleted %s/%s@%s\n", job.loginURL, job.repoName, job.digest)
		return nil
	}

	// if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
	// 	// If the manifest is not found it can be assumed to have been deleted.
	// 	fmt.Printf("Skipped %s/%s@%s, HTTP status: %d\n", job.loginURL, job.repoName, job.digest, resp.StatusCode)
	// 	return nil
	// }

	return err
	// return nil
}

// process calls acrClient to annotate a tag.
func (job *annotateTagJob) processAnnotate(ctx context.Context, orasClient api.ORASClientInterface) error {
	err := orasClient.Annotate(ctx, job.repoName, job.tag, job.artifactType, job.annotations)
	if err == nil {
		fmt.Printf("Deleted %s/%s:%s\n", job.loginURL, job.repoName, job.tag)
		return nil
	}

	// if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
	// 	// If the tag is not found it can be assumed to have been deleted.
	// 	fmt.Printf("Skipped %s/%s:%s, HTTP status: %d\n", job.loginURL, job.repoName, job.tag, resp.StatusCode)
	// 	return nil
	// }

	return err
}
