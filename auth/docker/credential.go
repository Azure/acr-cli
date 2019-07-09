// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package docker

import ctypes "github.com/docker/cli/cli/config/types"

// GetCredential tries to return a valid credential.
func (c *Client) GetCredential(hostname string) (string, string, error) {
	hostname = resolveHostname(hostname)

	var (
		auth ctypes.AuthConfig
		err  error
	)

	for _, cfg := range c.cfgs {
		auth, err = cfg.GetAuthConfig(hostname)
		if err != nil {
			continue
		}
		if auth.IdentityToken != "" {
			return "", auth.IdentityToken, nil
		}
		if auth.Username == "" && auth.Password == "" {
			continue
		}
		return auth.Username, auth.Password, nil
	}

	return "", "", err
}
