// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

// Package api provides client implementations for Azure Container Registry and ORAS operations.
package api

import (
	"bytes"
	"context"
	"fmt"
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
	// currentScopes tracks the scopes that the current token was issued for
	currentScopes []string
	// isABAC indicates if this is an ABAC-enabled registry that requires repository-specific scopes
	isABAC bool
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
	// Try to get a token with both catalog and repository wildcard scope for non-ABAC registries
	// This maintains backward compatibility while supporting ABAC registries
	scope := "registry:catalog:* repository:*:pull"
	accessTokenResponse, err := newAcrCLIClient.AutorestClient.GetAcrAccessToken(ctx, loginURL, scope, refreshToken)
	isABAC := false
	if err != nil {
		// If the above fails (likely ABAC registry), fallback to catalog-only scope
		// Repository-specific scopes will be requested when needed
		accessTokenResponse, err = newAcrCLIClient.AutorestClient.GetAcrAccessToken(ctx, loginURL, "registry:catalog:*", refreshToken)
		if err != nil {
			return newAcrCLIClient, err
		}
		isABAC = true
	}
	token := &adal.Token{
		AccessToken:  *accessTokenResponse.AccessToken,
		RefreshToken: refreshToken,
	}
	newAcrCLIClient.token = token
	newAcrCLIClient.isABAC = isABAC
	newAcrCLIClient.AutorestClient.Authorizer = autorest.NewBearerAuthorizer(token)

	// Parse and store the scopes from the token
	scopes, _ := getScopesFromToken(token.AccessToken)
	newAcrCLIClient.currentScopes = scopes

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

// refreshAcrCLIClientToken obtains a new token with the specified scope and gets its expiration time.
func refreshAcrCLIClientToken(ctx context.Context, c *AcrCLIClient, scope string) error {
	accessTokenResponse, err := c.AutorestClient.GetAcrAccessToken(ctx, c.loginURL, scope, c.token.RefreshToken)
	if err != nil {
		return err
	}
	token := &adal.Token{
		AccessToken:  *accessTokenResponse.AccessToken,
		RefreshToken: c.token.RefreshToken,
	}
	c.token = token
	c.AutorestClient.Authorizer = autorest.NewBearerAuthorizer(token)

	// Parse and store the new scopes from the refreshed token
	scopes, _ := getScopesFromToken(token.AccessToken)
	c.currentScopes = scopes

	exp, err := getExpiration(token.AccessToken)
	if err != nil {
		return err
	}
	c.accessTokenExp = exp
	return nil
}

// refreshTokenForRepository obtains a new token scoped to a specific repository with all permissions.
// This supports both ABAC and non-ABAC registries.
func refreshTokenForRepository(ctx context.Context, c *AcrCLIClient, repoName string) error {
	// For ABAC-enabled registries, we need to specify exact permissions
	// Using pull,push,delete covers all necessary operations
	scope := fmt.Sprintf("repository:%s:pull,push,delete", repoName)
	return refreshAcrCLIClientToken(ctx, c, scope)
}

// getExpiration is used to obtain the expiration out of a jwt token using proper JWT methods.
func getExpiration(tokenStr string) (int64, error) {
	// Parse the token without verification to extract claims
	token, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return 0, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("unable to parse token claims")
	}

	if fExp, ok := claims["exp"].(float64); ok {
		return int64(fExp), nil
	}
	return 0, errors.New("unable to obtain expiration date for token")
}

// getScopesFromToken extracts the access scopes from a JWT token using proper JWT methods
func getScopesFromToken(tokenStr string) ([]string, error) {
	// Parse the token without verification to extract claims
	token, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("unable to parse token claims")
	}

	// ACR tokens typically have "access" claim with scopes
	if access, ok := claims["access"]; ok {
		if accessList, ok := access.([]interface{}); ok {
			var scopes []string
			for _, item := range accessList {
				if accessMap, ok := item.(map[string]interface{}); ok {
					if scope, ok := accessMap["type"].(string); ok {
						scopeStr := scope
						if name, ok := accessMap["name"].(string); ok {
							scopeStr += ":" + name
						}
						if actions, ok := accessMap["actions"].([]interface{}); ok {
							var actionStrs []string
							for _, action := range actions {
								if actionStr, ok := action.(string); ok {
									actionStrs = append(actionStrs, actionStr)
								}
							}
							if len(actionStrs) > 0 {
								scopeStr += ":" + strings.Join(actionStrs, ",")
							}
						}
						scopes = append(scopes, scopeStr)
					}
				}
			}
			return scopes, nil
		}
	}

	// Fallback: check for "scope" claim (some implementations use this)
	if scope, ok := claims["scope"].(string); ok {
		return strings.Split(scope, " "), nil
	}

	return []string{}, nil
}

// hasRequiredScope checks if the current token has the required scope for a repository operation
func (c *AcrCLIClient) hasRequiredScope(repoName string) bool {
	if c.token == nil || len(c.currentScopes) == 0 {
		// No token or no scopes tracked
		return false
	}

	// Check if we have a wildcard repository scope (for non-ABAC registries)
	for _, scope := range c.currentScopes {
		if scope == "repository:*:pull" || scope == "repository:*:*" {
			return true
		}
		// Check for specific repository scope
		if strings.HasPrefix(scope, fmt.Sprintf("repository:%s:", repoName)) {
			// Check if we have at least pull permission
			parts := strings.Split(scope, ":")
			if len(parts) >= 3 {
				permissions := strings.Split(parts[2], ",")
				for _, perm := range permissions {
					if perm == "pull" || perm == "push" || perm == "delete" || perm == "*" {
						return true
					}
				}
			}
		}
	}

	return false
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
	// Check if token is expired OR if we don't have the required scope for this repository
	if c.isExpired() || (c.isABAC && !c.hasRequiredScope(repoName)) {
		if err := refreshTokenForRepository(ctx, c, repoName); err != nil {
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
	// Check if token is expired OR if we don't have the required scope for this repository
	if c.isExpired() || (c.isABAC && !c.hasRequiredScope(repoName)) {
		if err := refreshTokenForRepository(ctx, c, repoName); err != nil {
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
	// Check if token is expired OR if we don't have the required scope for this repository
	if c.isExpired() || (c.isABAC && !c.hasRequiredScope(repoName)) {
		if err := refreshTokenForRepository(ctx, c, repoName); err != nil {
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
	// Check if token is expired OR if we don't have the required scope for this repository
	if c.isExpired() || (c.isABAC && !c.hasRequiredScope(repoName)) {
		if err := refreshTokenForRepository(ctx, c, repoName); err != nil {
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
	// Check if token is expired OR if we don't have the required scope for this repository
	if c.isExpired() || (c.isABAC && !c.hasRequiredScope(repoName)) {
		if err := refreshTokenForRepository(ctx, c, repoName); err != nil {
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

// GetAcrManifestAttributes gets the attributes of a manifest.
func (c *AcrCLIClient) GetAcrManifestAttributes(ctx context.Context, repoName string, reference string) (*acrapi.ManifestAttributes, error) {
	// Check if token is expired OR if we don't have the required scope for this repository
	if c.isExpired() || (c.isABAC && !c.hasRequiredScope(repoName)) {
		if err := refreshTokenForRepository(ctx, c, repoName); err != nil {
			return nil, err
		}
	}
	manifestAttrs, err := c.AutorestClient.GetAcrManifestAttributes(ctx, repoName, reference)
	if err != nil {
		return &manifestAttrs, err
	}
	return &manifestAttrs, nil
}

// UpdateAcrTagAttributes updates tag attributes to enable/disable deletion and writing.
func (c *AcrCLIClient) UpdateAcrTagAttributes(ctx context.Context, repoName string, reference string, value *acrapi.ChangeableAttributes) (*autorest.Response, error) {
	// Check if token is expired OR if we don't have the required scope for this repository
	if c.isExpired() || (c.isABAC && !c.hasRequiredScope(repoName)) {
		if err := refreshTokenForRepository(ctx, c, repoName); err != nil {
			return nil, err
		}
	}
	resp, err := c.AutorestClient.UpdateAcrTagAttributes(ctx, repoName, reference, value)
	if err != nil {
		return &resp, err
	}
	return &resp, nil
}

// UpdateAcrManifestAttributes updates manifest attributes to enable/disable deletion and writing.
func (c *AcrCLIClient) UpdateAcrManifestAttributes(ctx context.Context, repoName string, reference string, value *acrapi.ChangeableAttributes) (*autorest.Response, error) {
	// Check if token is expired OR if we don't have the required scope for this repository
	if c.isExpired() || (c.isABAC && !c.hasRequiredScope(repoName)) {
		if err := refreshTokenForRepository(ctx, c, repoName); err != nil {
			return nil, err
		}
	}
	resp, err := c.AutorestClient.UpdateAcrManifestAttributes(ctx, repoName, reference, value)
	if err != nil {
		return &resp, err
	}
	return &resp, nil
}

// AcrCLIClientInterface defines the required methods that the acr-cli will need to use.
type AcrCLIClientInterface interface {
	GetAcrTags(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.RepositoryTagsType, error)
	DeleteAcrTag(ctx context.Context, repoName string, reference string) (*autorest.Response, error)
	GetAcrManifests(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.Manifests, error)
	DeleteManifest(ctx context.Context, repoName string, reference string) (*autorest.Response, error)
	GetManifest(ctx context.Context, repoName string, reference string) ([]byte, error)
	GetAcrManifestAttributes(ctx context.Context, repoName string, reference string) (*acrapi.ManifestAttributes, error)
	UpdateAcrTagAttributes(ctx context.Context, repoName string, reference string, value *acrapi.ChangeableAttributes) (*autorest.Response, error)
	UpdateAcrManifestAttributes(ctx context.Context, repoName string, reference string, value *acrapi.ChangeableAttributes) (*autorest.Response, error)
}
