package oras

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/credentials"
	"github.com/docker/cli/cli/config/types"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// Store provides credential CRUD operations.
type Store struct {
	configs []*configfile.ConfigFile
}

// NewStore generates a store based on the passed in config file path.
func NewStore(configPaths ...string) (*Store, error) {
	if len(configPaths) == 0 {
		// No config path passed, load default docker config file.
		cfg, err := config.Load(config.Dir())
		if err != nil {
			return nil, err
		}
		if !cfg.ContainsAuth() {
			cfg.CredentialsStore = credentials.DetectDefaultStore(cfg.CredentialsStore)
		}

		return &Store{
			configs: []*configfile.ConfigFile{cfg},
		}, nil
	}

	var configs []*configfile.ConfigFile
	for _, path := range configPaths {
		cfg, err := loadConfigFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		configs = append(configs, cfg)
	}

	return &Store{
		configs: configs,
	}, nil
}

// loadConfigFile reads the credential-related configurationfrom the given path.
func loadConfigFile(path string) (*configfile.ConfigFile, error) {
	var cfg *configfile.ConfigFile
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			cfg = configfile.New(path)
		} else {
			return nil, err
		}
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		cfg = configfile.New(path)
		if err := cfg.LoadFromReader(file); err != nil {
			return nil, err
		}
	}

	if !cfg.ContainsAuth() {
		cfg.CredentialsStore = credentials.DetectDefaultStore(cfg.CredentialsStore)
	}
	return cfg, nil
}

// Store stores a credential for a given registry.
func (s *Store) Store(registry string, cred auth.Credential) error {
	authConf := types.AuthConfig{
		Username:      cred.Username,
		Password:      cred.Password,
		ServerAddress: registry,
		IdentityToken: cred.RefreshToken,
		RegistryToken: cred.AccessToken,
	}

	return s.configs[0].GetCredentialsStore(registry).Store(authConf)
}

// Erase erases a credential for a given registry.
func (s *Store) Erase(registry string) error {
	return s.configs[0].GetCredentialsStore(registry).Erase(registry)
}

// Credential iterates all the config files, returns the first non-empty
// credential in a best-effort way.
// `EmptyCredential` is a valid return value and should not be considered as
// an error.
// If nil, the credential is always resolved to `EmptyCredential`.
func (s *Store) Credential(_ context.Context, registry string) (auth.Credential, error) {
	for _, c := range s.configs {
		authConf, err := c.GetCredentialsStore(registry).Get(registry)
		if err != nil {
			return auth.EmptyCredential, err
		}
		cred := auth.Credential{
			Username:     authConf.Username,
			Password:     authConf.Password,
			AccessToken:  authConf.RegistryToken,
			RefreshToken: authConf.IdentityToken,
		}
		if cred != auth.EmptyCredential {
			return cred, nil
		}
	}
	return auth.EmptyCredential, nil
}
