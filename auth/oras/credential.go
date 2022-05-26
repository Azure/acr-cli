package oras

import "oras.land/oras-go/v2/registry/remote/auth"

// Credential returns an auth.Credential that Store can use.
func Credential(username, password string) auth.Credential {
	if username == "" {
		return auth.Credential{
			RefreshToken: password,
		}
	}
	return auth.Credential{
		Username: username,
		Password: password,
	}
}
