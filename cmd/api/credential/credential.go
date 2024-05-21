package credential

import "oras.land/oras-go/v2/registry/remote/auth"

// Credential converts user input username and password to a credential.
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
