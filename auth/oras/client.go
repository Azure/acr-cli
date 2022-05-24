package oras

import (
	"context"

	"github.com/Azure/acr-cli/version"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// client option struct
type ClientOptions struct {
	Credential      auth.Credential
	CredentialStore *Store
	Debug           bool
}

func NewClient(opts ClientOptions) remote.Client {
	client := &auth.Client{
		Cache:    auth.NewCache(),
		ClientID: "acr-cli",
	}
	client.SetUserAgent("acr-cli/" + version.GetVersion())
	if opts.Debug {
		client.Client.Transport = NewTransport(client.Client.Transport)
	}
	if opts.Credential != auth.EmptyCredential {
		client.Credential = func(ctx context.Context, s string) (auth.Credential, error) {
			return opts.Credential, nil
		}
	} else if opts.CredentialStore != nil {
		client.Credential = opts.CredentialStore.Credential
	}
	return client
}
