// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package docker

import (
	"os"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/credentials"
)

func loadConfigFile(path string) (*configfile.ConfigFile, error) {
	cfg := configfile.New(path)
	if _, err := os.Stat(path); err == nil {
		file, innerErr := os.Open(path)
		if innerErr != nil {
			return nil, innerErr
		}
		defer file.Close()
		if cfgErr := cfg.LoadFromReader(file); cfgErr != nil {
			return nil, cfgErr
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if !cfg.ContainsAuth() {
		cfg.CredentialsStore = credentials.DetectDefaultStore(cfg.CredentialsStore)
	}
	return cfg, nil
}
