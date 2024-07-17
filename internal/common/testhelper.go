package common

import (
	"context"
	"net/http"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/go-autorest/autorest"
)

var (
	TestCtx      = context.Background()
	TestLoginURL = "foo.azurecr.io"
	TestRepo     = "bar"

	TagName         = "latest"
	TagName1        = "v1"
	TagName2        = "v2"
	TagName3        = "v3"
	TagName4        = "v4"
	TagNamePatch1   = "v1-patched"
	TagNamePatch2   = "v2-patched"
	RepoName1       = "repo1"
	RepoName2       = "repo2"
	RepoName3       = "repo3"
	RepoName4       = "repo4"
	deleteEnabled   = true
	lastUpdateTime  = time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano)
	writeEnabled    = true
	digest          = "sha256:2830cc0fcddc1bc2bd4aeab0ed5ee7087dab29a49e65151c77553e46a7ed5283" //#nosec G101
	multiArchDigest = "sha256:d88fb54ba4424dada7c928c6af332ed1c49065ad85eafefb6f26664695015119" //#nosec G101

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

	FourTagResultWithPatchTags = &acr.RepositoryTagsType{
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
			Name:                 &TagNamePatch1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagName2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}, {
			Name:                 &TagNamePatch2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled, WriteEnabled: &writeEnabled},
			Digest:               &digest,
		}},
	}
)
