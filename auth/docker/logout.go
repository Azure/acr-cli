// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package docker

import (
	"context"
	"errors"

	"github.com/docker/cli/cli/config/configfile"
)

var (
	// ErrNotLoggedIn defines the error that appears when a user is not logged into a registry.
	ErrNotLoggedIn = errors.New("not logged in")
)

// Logout logs out from a registry.
func (c *Client) Logout(ctx context.Context, hostname string) error {
	hostname = resolveHostname(hostname)

	var cfgs []*configfile.ConfigFile
	for _, cfg := range c.cfgs {
		if _, ok := cfg.AuthConfigs[hostname]; ok {
			cfgs = append(cfgs, cfg)
		}
	}

	if len(cfgs) == 0 {
		return ErrNotLoggedIn
	}

	return c.getCredentialStore(hostname).Erase(hostname)
}
