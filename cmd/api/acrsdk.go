// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/Azure/go-autorest/autorest"

	acrapi "github.com/Azure/acr-cli/acr"
)

const (
	prefixHTTPS           = "https://"
	registryURL           = ".azurecr.io"
	manifestTagFetchCount = 100
)

type AcrCLIClient struct {
	AutorestClient        acrapi.BaseClient
	manifestTagFetchCount int32
}

// BearerAuth returns the authentication header in case an access token was specified.
func BearerAuth(accessToken string) string {
	return "Bearer " + accessToken
}

// BasicAuth returns the username and the passwrod encoded in base 64.
func BasicAuth(username string, password string) string {
	auth := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

// LoginURL returns the FQDN for a registry.
func LoginURL(registryName string) string {
	// TODO: if the registry is in another cloud (i.e. dogfood) a full FQDN for the registry should be specified.
	if strings.Contains(registryName, ".") {
		return registryName
	}
	return registryName + registryURL
}

// LoginURLWithPrefix return the hostname of a registry.
func LoginURLWithPrefix(loginURL string) string {
	urlWithPrefix := loginURL
	if !strings.HasPrefix(loginURL, prefixHTTPS) {
		urlWithPrefix = prefixHTTPS + loginURL
	}
	return urlWithPrefix
}

func NewAcrCLIClient(loginURL string) AcrCLIClient {
	loginURL = LoginURLWithPrefix(loginURL)
	return AcrCLIClient{
		acrapi.NewWithoutDefaults(loginURL),
		100,
	}
}

func NewAcrCLIClientWithBasicAuth(loginURL string, username string, password string) AcrCLIClient {
	newAcrCLIClient := NewAcrCLIClient(loginURL)
	newAcrCLIClient.AutorestClient.Authorizer = autorest.NewBasicAuthorizer(username, password)
	return newAcrCLIClient
}

// AcrListTags list the tags of a repository with their attributes.
func (c *AcrCLIClient) GetAcrTags(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.RepositoryTagsType, error) {
	tags, err := c.AutorestClient.GetAcrTags(ctx, repoName, last, &c.manifestTagFetchCount, orderBy, "")
	if err != nil {
		return nil, err
	}
	return &tags, nil
}

// DeleteTag deletes the tag by reference.
func (c *AcrCLIClient) DeleteAcrTag(ctx context.Context, repoName string, reference string) error {
	_, err := c.AutorestClient.DeleteAcrTag(ctx, repoName, reference)
	if err != nil {
		return err
	}
	return nil
}

// ListManifestsAttributes list all the manifest in a repository with their attributes.
func (c *AcrCLIClient) GetAcrManifests(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.Manifests, error) {
	manifests, err := c.AutorestClient.GetAcrManifests(ctx, repoName, last, &c.manifestTagFetchCount, orderBy)
	if err != nil {
		return nil, err
	}
	return &manifests, nil
}

// DeleteManifestByDigest deletes a manifest using the digest as a reference.
func (c *AcrCLIClient) DeleteManifest(ctx context.Context, repoName string, reference string) error {
	_, err := c.AutorestClient.DeleteManifest(ctx, repoName, reference)
	if err != nil {
		return err
	}
	return nil
}

type AcrCLIClientInterface interface {
	AcrListTags(ctx context.Context, loginURL string, auth string, repoName string, orderBy string, last string)
}
