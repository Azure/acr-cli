// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"time"

	"github.com/Azure/acr-cli/cmd/api"
)

type purgeManifestJob struct {
	jobBase
	client api.AcrCLIClientInterface
	digest string
}

type purgeTagJob struct {
	jobBase
	client api.AcrCLIClientInterface
	tag    string
}

func newPurgeManifestJob(loginURL string, repoName string, client api.AcrCLIClientInterface, digest string) *purgeManifestJob {
	base := jobBase{
		loginURL:    loginURL,
		repoName:    repoName,
		timeCreated: time.Now().UTC(),
	}

	return &purgeManifestJob{
		jobBase: base,
		client:  client,
		digest:  digest,
	}
}

func newPurgeTagJob(loginURL string, repoName string, client api.AcrCLIClientInterface, tag string) *purgeTagJob {
	base := jobBase{
		loginURL:    loginURL,
		repoName:    repoName,
		timeCreated: time.Now().UTC(),
	}

	return &purgeTagJob{
		jobBase: base,
		client:  client,
		tag:     tag,
	}
}
