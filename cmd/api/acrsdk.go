// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	acrapi "github.com/Azure/acr-cli/acr"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const (
	prefixHTTPS = "https://"
	registryURL = ".azurecr.io"
)

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

// AcrListTags list the tags of a repository with their attributes.
func AcrListTags(ctx context.Context,
	loginURL string,
	auth string,
	repoName string,
	orderBy string,
	last string) (*acrapi.TagAttributeList, error) {
	hostname := LoginURLWithPrefix(loginURL)
	client := acrapi.NewWithBaseURI(hostname,
		repoName,
		"",
		"",
		"",
		"",
		auth,
		orderBy,
		"100",
		last,
		"")
	tags, err := client.AcrListTags(ctx)
	if err != nil {
		return nil, err
	}
	var listTagResult acrapi.TagAttributeList
	switch tags.StatusCode {
	case http.StatusOK:
		if err = mapstructure.Decode(tags.Value, &listTagResult); err != nil {
			return nil, err
		}
		return &listTagResult, nil

	case http.StatusUnauthorized, http.StatusNotFound:
		var apiError acrapi.Error
		if err = mapstructure.Decode(tags.Value, &apiError); err != nil {
			return nil, errors.Wrap(err, "unable to decode error")
		}
		if apiError.Errors != nil && len(*apiError.Errors) > 0 {
			return nil, fmt.Errorf("%s %s", *(*apiError.Errors)[0].Code, *(*apiError.Errors)[0].Message)
		}
		return nil, errors.New("unable to decode apiError")

	default:
		return nil, fmt.Errorf("unexpected response code: %v", tags.StatusCode)
	}
}

// AcrDeleteTag deletes the tag by reference.
func AcrDeleteTag(ctx context.Context,
	loginURL string,
	auth string,
	repoName string,
	reference string) error {
	hostname := LoginURLWithPrefix(loginURL)
	client := acrapi.NewWithBaseURI(hostname,
		repoName,
		reference,
		"",
		"",
		"",
		auth,
		"",
		"",
		"",
		"")
	tag, err := client.AcrDeleteTag(ctx)
	if err != nil {
		return err
	}
	switch tag.StatusCode {
	case http.StatusAccepted:
		return nil
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusMethodNotAllowed:
		var apiError acrapi.Error
		if err = mapstructure.Decode(tag, &apiError); err != nil {
			return errors.Wrap(err, "unable to decode error")
		}
		if apiError.Errors != nil && len(*apiError.Errors) > 0 {
			return fmt.Errorf("%s %s", *(*apiError.Errors)[0].Code, *(*apiError.Errors)[0].Message)
		}
		return errors.New("unable to decode apiError")

	default:
		return fmt.Errorf("unexpected response code: %v", tag.StatusCode)
	}
}

// AcrListManifests list all the manifest in a repository with their attributes.
func AcrListManifests(ctx context.Context,
	loginURL string,
	auth string,
	repoName string,
	orderBy string,
	last string) (*acrapi.ManifestAttributeList, error) {
	hostname := LoginURLWithPrefix(loginURL)
	client := acrapi.NewWithBaseURI(hostname,
		repoName,
		"",
		"",
		"",
		"",
		auth,
		orderBy,
		"100",
		last,
		"")
	manifests, err := client.AcrListManifests(ctx)
	if err != nil {
		return nil, err
	}
	switch manifests.StatusCode {
	case http.StatusOK:
		var acrListManifestsAttributesResult acrapi.ManifestAttributeList
		if err = mapstructure.Decode(manifests.Value, &acrListManifestsAttributesResult); err != nil {
			return nil, err
		}
		return &acrListManifestsAttributesResult, nil

	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusMethodNotAllowed:
		var apiError acrapi.Error
		if err = mapstructure.Decode(manifests.Value, &apiError); err != nil {
			return nil, errors.Wrap(err, "unable to decode error")
		}
		if apiError.Errors != nil && len(*apiError.Errors) > 0 {
			return nil, fmt.Errorf("%s %s", *(*apiError.Errors)[0].Code, *(*apiError.Errors)[0].Message)
		}
		return nil, errors.New("unable to decode apiError")

	default:
		return nil, fmt.Errorf("unexpected response code: %v", manifests.StatusCode)
	}
}

// DeleteManifest deletes a manifest using the digest as a reference.
func DeleteManifest(ctx context.Context,
	loginURL string,
	auth string,
	repoName string,
	reference string) error {
	hostname := LoginURLWithPrefix(loginURL)
	client := acrapi.NewWithBaseURI(hostname,
		repoName,
		reference,
		"",
		"",
		"",
		auth,
		"",
		"",
		"",
		"")
	deleteManifest, err := client.DeleteManifest(ctx)
	if err != nil {
		return err
	}
	switch deleteManifest.StatusCode {
	case http.StatusAccepted:
		return nil

	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusMethodNotAllowed:
		var apiError acrapi.Error
		if err = mapstructure.Decode(deleteManifest, &apiError); err != nil {
			return errors.Wrap(err, "unable to decode error")
		}
		if apiError.Errors != nil && len(*apiError.Errors) > 0 {
			return fmt.Errorf("%s %s", *(*apiError.Errors)[0].Code, *(*apiError.Errors)[0].Message)
		}
		return errors.New("unable to decode apiError")

	default:
		return fmt.Errorf("unexpected response code: %v", deleteManifest.StatusCode)
	}
}
