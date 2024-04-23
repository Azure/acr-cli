package oras

import (
	"context"
	"net/http"

	"github.com/Azure/acr-cli/version"
	"github.com/google/uuid"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// ClientOptions type includes a Credential that stores the credentials
// of the client. CredentialStore will be used if a Credential is not
// provided. ClientOptions also includes a Debug flag.
type ClientOptions struct {
	Credential      auth.Credential
	CredentialStore *Store
	Debug           bool
}

// NewClient generates a client based on the passed in options.
func NewClient(opts ClientOptions) remote.Client {
	client := &auth.Client{
		Cache:    auth.NewCache(),
		ClientID: "acr-cli",
		Header:   http.Header{},
	}
	client.Header.Set("x-ms-correlation-request-id", uuid.New().String())
	client.SetUserAgent("acr-cli/" + version.FullVersion())
	if opts.Debug {
		client.Client.Transport = NewDebugTransport(client.Client.Transport)
	}
	if opts.Credential != auth.EmptyCredential {
		client.Credential = func(_ context.Context, _ string) (auth.Credential, error) {
			return opts.Credential, nil
		}
	} else if opts.CredentialStore != nil {
		client.Credential = opts.CredentialStore.Credential
	}
	return client
}
