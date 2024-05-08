// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"bytes"
	"context"
	"io/ioutil"
	"strings"
	"time"

	// The autorest generated SDK is used, this file is just a wrapper to it.
	acrapi "github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/auth/oras"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/golang-jwt/jwt/v4"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// Constants that are used throughout this file.
const (
	prefixHTTPS                      = "https://"
	registryURL                      = ".azurecr.io"
	manifestTagFetchCount            = 100
	manifestORASArtifactContentType  = "application/vnd.cncf.oras.artifact.manifest.v1+json"
	manifestOCIArtifactContentType   = "application/vnd.oci.artifact.manifest.v1+json"
	manifestOCIImageContentType      = v1.MediaTypeImageManifest
	manifestOCIImageIndexContentType = v1.MediaTypeImageIndex
	manifestImageContentType         = "application/vnd.docker.distribution.manifest.v2+json"
	manifestListContentType          = "application/vnd.docker.distribution.manifest.list.v2+json"
	manifestAcceptHeader             = "*/*, " + manifestORASArtifactContentType +
		", " + manifestOCIArtifactContentType +
		", " + manifestOCIImageContentType +
		", " + manifestOCIImageIndexContentType +
		", " + manifestImageContentType +
		", " + manifestListContentType
)

// The AcrCLIClient is the struct that will be in charge of doing the http requests to the registry.
// it implements the AcrCLIClientInterface.
type AcrCLIClient struct {
	AutorestClient acrapi.BaseClient
	// manifestTagFetchCount refers to how many tags or manifests can be retrieved in a single http request.
	manifestTagFetchCount int32
	loginURL              string
	// token refers to an ACR access token for use with bearer authentication.
	token *adal.Token
	// accessTokenExp refers to the expiration time for the access token, it is in a unix time format represented by a
	// 64 bit integer.
	accessTokenExp int64
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

// newAcrCLIClient creates a client that does not have any authentication.
func newAcrCLIClient(loginURL string) AcrCLIClient {
	loginURLPrefix := LoginURLWithPrefix(loginURL)
	return AcrCLIClient{
		AutorestClient: acrapi.NewWithoutDefaults(loginURLPrefix),
		// The manifestTagFetchCount is set to the default which is 100
		manifestTagFetchCount: manifestTagFetchCount,
		loginURL:              loginURL,
	}
}

// newAcrCLIClientWithBasicAuth creates a client that uses basic authentication.
func newAcrCLIClientWithBasicAuth(loginURL string, username string, password string) AcrCLIClient {
	newAcrCLIClient := newAcrCLIClient(loginURL)
	newAcrCLIClient.AutorestClient.Authorizer = autorest.NewBasicAuthorizer(username, password)
	return newAcrCLIClient
}

// newAcrCLIClientWithBearerAuth creates a client that uses bearer token authentication.
func newAcrCLIClientWithBearerAuth(loginURL string, refreshToken string) (AcrCLIClient, error) {
	newAcrCLIClient := newAcrCLIClient(loginURL)
	ctx := context.Background()
	accessTokenResponse, err := newAcrCLIClient.AutorestClient.GetAcrAccessToken(ctx, loginURL, "registry:catalog:* repository:*:*", refreshToken)
	if err != nil {
		return newAcrCLIClient, err
	}
	token := &adal.Token{
		AccessToken:  *accessTokenResponse.AccessToken,
		RefreshToken: refreshToken,
	}
	newAcrCLIClient.token = token
	newAcrCLIClient.AutorestClient.Authorizer = autorest.NewBearerAuthorizer(token)
	// The expiration time is stored in the struct to make it easy to determine if a token is expired.
	exp, err := getExpiration(token.AccessToken)
	if err != nil {
		return newAcrCLIClient, err
	}
	newAcrCLIClient.accessTokenExp = exp
	return newAcrCLIClient, nil
}

// GetAcrCLIClientWithAuth obtains a client that has authentication for making ACR http requests
func GetAcrCLIClientWithAuth(loginURL string, username string, password string, configs []string) (*AcrCLIClient, error) {
	if username == "" && password == "" {
		// If both username and password are empty then the docker config file will be used, it can be found in the default
		// location or in a location specified by the configs string array
		store, err := oras.NewStore(configs...)
		if err != nil {
			return nil, errors.Wrap(err, "error resolving authentication")
		}
		cred, err := store.Credential(context.Background(), loginURL)
		if err != nil {
			return nil, errors.Wrap(err, "error resolving authentication")
		}
		username = cred.Username
		password = cred.Password

		// fallback to refresh token if it is available
		if password == "" && cred.RefreshToken != "" {
			password = cred.RefreshToken
		}
	}
	// If the password is empty then the authentication failed.
	if password == "" {
		return nil, errors.New("unable to resolve authentication, missing identity token or password")
	}
	var acrClient AcrCLIClient
	if username == "" || username == "00000000-0000-0000-0000-000000000000" {
		// If the username is empty an ACR refresh token was used.
		var err error
		acrClient, err = newAcrCLIClientWithBearerAuth(loginURL, password)
		if err != nil {
			return nil, errors.Wrap(err, "error resolving authentication")
		}
		return &acrClient, nil
	}
	// if both the username and password were specified basic authentication can be assumed.
	acrClient = newAcrCLIClientWithBasicAuth(loginURL, username, password)
	return &acrClient, nil
}

// refreshAcrCLIClientToken obtains a new token and gets its expiration time.
func refreshAcrCLIClientToken(ctx context.Context, c *AcrCLIClient) error {
	accessTokenResponse, err := c.AutorestClient.GetAcrAccessToken(ctx, c.loginURL, "repository:*:*", c.token.RefreshToken)
	if err != nil {
		return err
	}
	token := &adal.Token{
		AccessToken:  *accessTokenResponse.AccessToken,
		RefreshToken: c.token.RefreshToken,
	}
	c.token = token
	c.AutorestClient.Authorizer = autorest.NewBearerAuthorizer(token)
	exp, err := getExpiration(token.AccessToken)
	if err != nil {
		return err
	}
	c.accessTokenExp = exp
	return nil
}

// getExpiration is used to obtain the expiration out of a jwt token.
func getExpiration(token string) (int64, error) {
	parser := jwt.Parser{SkipClaimsValidation: true}
	mapC := jwt.MapClaims{}
	// Since we only need the expiration time there is no need for verifying the signature of the token.
	_, _, err := parser.ParseUnverified(token, mapC)
	if err != nil {
		return 0, err
	}
	if fExp, ok := mapC["exp"].(float64); ok {
		return int64(fExp), nil
	}
	return 0, errors.New("unable to obtain expiration date for token")
}

// isExpired return true when the token inside an acrClient is expired and a new should be requested.
func (c *AcrCLIClient) isExpired() bool {
	if c.token == nil {
		// there is no token so basic auth can be assumed.
		return false
	}
	// 5 minutes are subtracted to make sure that there won't be a case were a client with an expired token tries doing a request.
	return (time.Now().Add(5 * time.Minute)).Unix() > c.accessTokenExp
}

// GetAcrTags list the tags of a repository with their attributes.
func (c *AcrCLIClient) GetAcrTags(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.RepositoryTagsType, error) {
	if c.isExpired() {
		if err := refreshAcrCLIClientToken(ctx, c); err != nil {
			return nil, err
		}
	}
	tags, err := c.AutorestClient.GetAcrTags(ctx, repoName, last, &c.manifestTagFetchCount, orderBy, "")
	if err != nil {
		// tags might contain information such as status codes, so it a pointer to it is returned instead of nil.
		return &tags, err
	}
	return &tags, nil
}

// DeleteAcrTag deletes the tag by reference.
func (c *AcrCLIClient) DeleteAcrTag(ctx context.Context, repoName string, reference string) (*autorest.Response, error) {
	if c.isExpired() {
		if err := refreshAcrCLIClientToken(ctx, c); err != nil {
			return nil, err
		}
	}
	resp, err := c.AutorestClient.DeleteAcrTag(ctx, repoName, reference)
	if err != nil {
		return &resp, err
	}
	return &resp, nil
}

// GetAcrManifests list all the manifest in a repository with their attributes.
func (c *AcrCLIClient) GetAcrManifests(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.Manifests, error) {
	if c.isExpired() {
		if err := refreshAcrCLIClientToken(ctx, c); err != nil {
			return nil, err
		}
	}
	manifests, err := c.AutorestClient.GetAcrManifests(ctx, repoName, last, &c.manifestTagFetchCount, orderBy)
	if err != nil {
		return &manifests, err
	}
	return &manifests, nil
}

// DeleteManifest deletes a manifest using the digest as a reference.
func (c *AcrCLIClient) DeleteManifest(ctx context.Context, repoName string, reference string) (*autorest.Response, error) {
	if c.isExpired() {
		if err := refreshAcrCLIClientToken(ctx, c); err != nil {
			return nil, err
		}
	}
	resp, err := c.AutorestClient.DeleteManifest(ctx, repoName, reference)
	if err != nil {
		return &resp, err
	}
	return &resp, nil
}

// GetManifest fetches a manifest (could be a Manifest List or a v2 manifest) and returns it as a byte array.
// This is used when a manifest list is wanted, first the bytes are obtained and then unmarshalled into a new struct.
func (c *AcrCLIClient) GetManifest(ctx context.Context, repoName string, reference string) ([]byte, error) {
	if c.isExpired() {
		if err := refreshAcrCLIClientToken(ctx, c); err != nil {
			return nil, err
		}
	}
	var result acrapi.SetObject
	req, err := c.AutorestClient.GetManifestPreparer(ctx, repoName, reference, manifestAcceptHeader)
	if err != nil {
		err = autorest.NewErrorWithError(err, "acr.BaseClient", "GetManifest", nil, "Failure preparing request")
		return nil, err
	}

	resp, err := c.AutorestClient.GetManifestSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		err = autorest.NewErrorWithError(err, "acr.BaseClient", "GetManifest", resp, "Failure sending request")
		return nil, err
	}

	var manifestBytes []byte
	if resp.Body != nil {
		manifestBytes, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	}

	resp.Body = ioutil.NopCloser(bytes.NewBuffer(manifestBytes))

	_, err = c.AutorestClient.GetManifestResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "acr.BaseClient", "GetManifest", resp, "Failure responding to request")
		return nil, err
	}

	return manifestBytes, nil
}

func (c *AcrCLIClient) GetAcrRepositories(ctx context.Context, last string, n *int32) (*acrapi.Repositories, error) {
	if c.isExpired() {
		if err := refreshAcrCLIClientToken(ctx, c); err != nil {
			return nil, err
		}
	}
	repos, err := c.AutorestClient.GetAcrRepositories(ctx, last, n)
	if err != nil {
		return &repos, err
	}
	return &repos, nil
}

// AcrCLIClientInterface defines the required methods that the acr-cli will need to use.
type AcrCLIClientInterface interface {
	GetAcrTags(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.RepositoryTagsType, error)
	DeleteAcrTag(ctx context.Context, repoName string, reference string) (*autorest.Response, error)
	GetAcrManifests(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.Manifests, error)
	DeleteManifest(ctx context.Context, repoName string, reference string) (*autorest.Response, error)
	GetManifest(ctx context.Context, repoName string, reference string) ([]byte, error)
	GetAcrRepositories(ctx context.Context, last string, n *int32) (*acrapi.Repositories, error)
}
