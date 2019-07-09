// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package docker

import (
	"github.com/Azure/acr-cli/auth"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/credentials"
	"github.com/pkg/errors"
)

// Client provides authentication operations for docker registries.
type Client struct {
	cfgs []*configfile.ConfigFile
}

// NewClient creates a new client with credentials specified from the provided
// configuration paths.
func NewClient(configPaths ...string) (auth.Client, error) {
	var cfgs []*configfile.ConfigFile
	for _, path := range configPaths {
		cfg, err := loadConfigFile(path)
		if err != nil {
			return nil, errors.Wrap(err, path)
		}
		cfgs = append(cfgs, cfg)
	}

	if len(cfgs) == 0 {
		cfg, err := config.Load(config.Dir())
		if err != nil {
			return nil, err
		}
		if !cfg.ContainsAuth() {
			cfg.CredentialsStore = credentials.DetectDefaultStore(cfg.CredentialsStore)
		}
		cfgs = []*configfile.ConfigFile{cfg}
	}

	return &Client{
		cfgs: cfgs,
	}, nil
}

func (c *Client) getCredentialStore(hostname string) credentials.Store {
	return c.cfgs[0].GetCredentialsStore(hostname)
}
