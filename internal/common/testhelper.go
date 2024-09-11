package common

import (
	"context"
	"net/http"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/go-autorest/autorest"
)

var (
	TestCtx              = context.Background()
	TestLoginURL         = "foo.azurecr.io"
	TestRepo             = "bar"
	TagName              = "latest"
	TagName1             = "jammy"
	TagName2             = "jammy-20240808"
	TagName3             = "jammy-20240627.1"
	TagName4             = "20.04"
	TagName1FloatingTag  = "jammy-patched"
	TagName2FloatingTag  = "jammy-20240808-patched"
	TagName3FloatingTag  = "jammy-20240627.1-patched"
	TagName4FloatingTag  = "20.04-patched"
	TagName1Incremental1 = "jammy-1"
	TagName2Incremental1 = "jammy-20240808-1"
	TagName3Incremental1 = "jammy-20240627.1-1"
	TagName4Incremental1 = "20.04-1"
	TagName1Incremental2 = "jammy-2"
	TagName2Incremental2 = "jammy-20240808-2"
	TagName3Incremental2 = "jammy-20240627.1-2"
	TagName4Incremental2 = "20.04-2"
	RepoName1            = "repo1"
	RepoName2            = "repo2"
	RepoName3            = "repo3"
	RepoName4            = "repo4"
	deleteEnabled        = true
	lastUpdateTime       = time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano)
	writeEnabled         = true
	digest               = "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283" //#nosec G101
	multiArchDigest      = "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119" //#nosec G101

	NotFoundResponse = autorest.Response{
		Response: &http.Response{
			StatusCode: 404,
		},
	}
	DeletedResponse = autorest.Response{
		Response: &http.Response{
			StatusCode: 200,
		},
	}

	// Response for the GetAcrTags when the repository is not found.
	NotFoundTagResponse = &acr.RepositoryTagsType{
		Response: NotFoundResponse,
	}

	// Response for the GetAcrTags when there are no tags on the testRepo.
	EmptyListTagsResult = &acr.RepositoryTagsType{
		Registry:       &TestLoginURL,
		ImageName:      &TestRepo,
		TagsAttributes: nil,
	}

	// Response for the GetAcrTags when there is one tag on the testRepo.
	OneTagResult = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &TestLoginURL,
		ImageName: &TestRepo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &TagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
				Digest:               &digest,
			},
		},
	}

	FourTagsResult = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &TestLoginURL,
		ImageName: &TestRepo,
		TagsAttributes: &[]acr.TagAttributesBase{{
			Name:                 &TagName1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName3,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &multiArchDigest,
		}, {
			Name:                 &TagName4,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}},
	}

	FourTagsResultWithPatchTags = &acr.RepositoryTagsType{
		Response: autorest.Response{
			Response: &http.Response{
				StatusCode: 200,
			},
		},
		Registry:  &TestLoginURL,
		ImageName: &TestRepo,
		TagsAttributes: &[]acr.TagAttributesBase{{
			Name:                 &TagName1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName1Incremental1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName1Incremental2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName1FloatingTag,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName2Incremental1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName2Incremental2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName2FloatingTag,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName3,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName3Incremental1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName3Incremental2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName3FloatingTag,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName4,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName4Incremental1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName4Incremental2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName4FloatingTag,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}},
	}
)
