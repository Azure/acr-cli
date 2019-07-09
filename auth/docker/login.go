// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package docker

import (
	"context"

	ctypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/registry"
)

const (
	userAgent = "acr-cli"
)

// Login logs into a registry.
func (c *Client) Login(ctx context.Context, hostname, username, secret string) error {
	hostname = resolveHostname(hostname)
	cred := types.AuthConfig{
		Username:      username,
		ServerAddress: hostname,
	}

	if username == "" {
		cred.IdentityToken = secret
	} else {
		cred.Password = secret
	}

	remote, err := registry.NewService(
		registry.ServiceOptions{
			V2Only: true,
		})
	if err != nil {
		return err
	}

	if _, token, err := remote.Auth(ctx, &cred, userAgent); err != nil {
		return err
	} else if token != "" {
		cred.Username = ""
		cred.Password = ""
		cred.IdentityToken = token
	}

	return c.getCredentialStore(hostname).Store(ctypes.AuthConfig(cred))
}
