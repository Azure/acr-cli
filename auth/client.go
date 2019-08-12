// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package auth

import (
	"context"
)

// Client is the interface defined for a docker authentication client
type Client interface {
	// Login logs into a container registry.
	Login(ctx context.Context, hostname, username, secret string) error

	// Logout logs out of a container registry.
	Logout(ctx context.Context, hostname string) error

	// GetCredential returns a credential for the hostname.
	GetCredential(hostname string) (string, string, error)
}
