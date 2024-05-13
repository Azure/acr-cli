// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"time"

	"github.com/Azure/acr-cli/cmd/api"
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
